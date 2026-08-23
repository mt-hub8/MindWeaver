package sqliteprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ncruces/go-sqlite3"
)

type HelperProcess struct {
	Executable string
	PrefixArgs []string
	ExtraEnv   []string
}

type ContentionResult struct {
	ObservedBusy         bool      `json:"observed_busy"`
	BusyTimeoutMS        int64     `json:"busy_timeout_ms"`
	ExpectedElapsedMinMS int64     `json:"expected_elapsed_min_ms"`
	ExpectedElapsedMaxMS int64     `json:"expected_elapsed_max_ms"`
	ActualElapsedMS      int64     `json:"actual_elapsed_ms"`
	Error                ErrorInfo `json:"error"`
}

type SidecarState struct {
	RelativePath string `json:"relative_path"`
	Exists       bool   `json:"exists"`
	SizeBytes    int64  `json:"size_bytes"`
}

func runLockHolder(ctx context.Context, dbPath, readyPath string, hold time.Duration) error {
	if err := requireTemporaryPath(dbPath); err != nil {
		return err
	}
	if err := requireTemporaryPath(readyPath); err != nil {
		return err
	}
	db, err := openConfigured(dbPath, defaultBusyTimeout)
	if err != nil {
		return err
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO lock_events(value) VALUES ('uncommitted')"); err != nil {
		conn.ExecContext(context.Background(), "ROLLBACK")
		return err
	}
	ready, _ := json.Marshal(map[string]any{"pid": os.Getpid(), "transaction": "BEGIN IMMEDIATE"})
	if err := os.WriteFile(readyPath, ready, 0o600); err != nil {
		conn.ExecContext(context.Background(), "ROLLBACK")
		return err
	}

	timer := time.NewTimer(hold)
	defer timer.Stop()
	select {
	case <-timer.C:
		_, err = conn.ExecContext(context.Background(), "ROLLBACK")
		return err
	case <-ctx.Done():
		conn.ExecContext(context.Background(), "ROLLBACK")
		return ctx.Err()
	}
}

func runLockContender(ctx context.Context, dbPath string, busy time.Duration) (ContentionResult, error) {
	if err := requireTemporaryPath(dbPath); err != nil {
		return ContentionResult{}, err
	}
	db, err := openConfigured(dbPath, busy)
	if err != nil {
		return ContentionResult{}, err
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return ContentionResult{}, err
	}
	defer conn.Close()
	started := time.Now()
	_, lockErr := conn.ExecContext(ctx, "BEGIN IMMEDIATE")
	elapsed := time.Since(started)
	if lockErr == nil {
		conn.ExecContext(context.Background(), "ROLLBACK")
		return ContentionResult{}, errors.New("contender unexpectedly acquired the writer lock")
	}
	info, ok := ClassifySQLiteError(lockErr)
	if !ok || info.PrimaryCode != int(sqlite3.BUSY) || info.Category != "contention" || !info.Retryable {
		return ContentionResult{}, fmt.Errorf("unexpected contention error: %#v (%v)", info, lockErr)
	}
	timeoutMS := busy.Milliseconds()
	minElapsed := max(int64(1), timeoutMS/2)
	maxElapsed := max(int64(1000), timeoutMS*5)
	if elapsed.Milliseconds() < minElapsed || elapsed.Milliseconds() > maxElapsed {
		return ContentionResult{}, fmt.Errorf("busy timeout elapsed %dms, expected within [%dms,%dms]", elapsed.Milliseconds(), minElapsed, maxElapsed)
	}
	return ContentionResult{
		ObservedBusy:         true,
		BusyTimeoutMS:        timeoutMS,
		ExpectedElapsedMinMS: minElapsed,
		ExpectedElapsedMaxMS: maxElapsed,
		ActualElapsedMS:      elapsed.Milliseconds(),
		Error:                info,
	}, nil
}

func probeTwoProcessLock(ctx context.Context, dir string, helper HelperProcess) (map[string]any, error) {
	if helper.Executable == "" {
		return nil, errors.New("two-process probe requires a helper executable")
	}
	dbPath := filepath.Join(dir, "two-process.db")
	readyPath := filepath.Join(dir, "holder.ready")
	db, err := openConfigured(dbPath, defaultBusyTimeout)
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE lock_events (id INTEGER PRIMARY KEY, value TEXT NOT NULL)"); err != nil {
		db.Close()
		return nil, err
	}
	if err := db.Close(); err != nil {
		return nil, err
	}

	holder := helperCommand(ctx, helper,
		"lock-holder", "--db", dbPath, "--ready", readyPath, "--hold", "30s")
	var holderStdout, holderStderr bytes.Buffer
	holder.Stdout = &holderStdout
	holder.Stderr = &holderStderr
	if err := holder.Start(); err != nil {
		return nil, fmt.Errorf("start lock holder: %w", err)
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- holder.Wait() }()
	waited := false
	defer func() {
		if !waited {
			_ = holder.Process.Kill()
			select {
			case <-waitCh:
			case <-time.After(2 * time.Second):
			}
		}
	}()

	readyTimer := time.NewTimer(5 * time.Second)
	defer readyTimer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	ready := false
	for !ready {
		select {
		case err := <-waitCh:
			waited = true
			return nil, fmt.Errorf("lock holder exited before ready: %v; stdout=%q stderr=%q", err, holderStdout.String(), holderStderr.String())
		case <-ticker.C:
			_, err := os.Stat(readyPath)
			ready = err == nil
		case <-readyTimer.C:
			return nil, fmt.Errorf("lock holder did not become ready; stderr=%q", holderStderr.String())
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	contenderCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	contender := helperCommand(contenderCtx, helper,
		"lock-contender", "--db", dbPath, "--busy", "200ms")
	output, err := contender.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("lock contender: %w; output=%q", err, output)
	}
	var contention ContentionResult
	if err := json.Unmarshal(bytes.TrimSpace(output), &contention); err != nil {
		return nil, fmt.Errorf("decode contender JSON %q: %w", output, err)
	}
	if !contention.ObservedBusy || contention.Error.PrimaryCode != 5 {
		return nil, fmt.Errorf("unexpected contender evidence: %+v", contention)
	}

	if err := holder.Process.Kill(); err != nil {
		return nil, fmt.Errorf("force-kill holder: %w", err)
	}
	select {
	case <-waitCh:
		waited = true
	case <-time.After(5 * time.Second):
		return nil, errors.New("force-killed lock holder did not exit")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	postKillSidecars := sqliteSidecarStates(dbPath)

	recovered, err := openConfigured(dbPath, defaultBusyTimeout)
	if err != nil {
		return nil, fmt.Errorf("reopen after holder kill: %w", err)
	}
	conn, err := recovered.Conn(ctx)
	if err != nil {
		recovered.Close()
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		conn.Close()
		recovered.Close()
		return nil, fmt.Errorf("writer lock not released after process death: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "ROLLBACK"); err != nil {
		conn.Close()
		recovered.Close()
		return nil, err
	}
	conn.Close()
	var integrityCheck string
	var leaked int
	if err := recovered.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrityCheck); err != nil {
		recovered.Close()
		return nil, err
	}
	if err := recovered.QueryRowContext(ctx, "SELECT count(*) FROM lock_events WHERE value='uncommitted'").Scan(&leaked); err != nil {
		recovered.Close()
		return nil, err
	}
	postReopenSidecars := sqliteSidecarStates(dbPath)
	if integrityCheck != "ok" || leaked != 0 {
		recovered.Close()
		return nil, fmt.Errorf("post-kill recovery failed: integrity_check=%q leaked=%d", integrityCheck, leaked)
	}
	if err := recovered.Close(); err != nil {
		return nil, err
	}
	postCloseSidecars := sqliteSidecarStates(dbPath)
	return map[string]any{
		"processes":                    2,
		"contender":                    contention,
		"holder_force_killed":          true,
		"writer_lock_released_on_exit": true,
		"uncommitted_rows_after_kill":  leaked,
		"post_kill_sidecars":           postKillSidecars,
		"post_reopen_sidecars":         postReopenSidecars,
		"post_close_sidecars":          postCloseSidecars,
		"reopen_integrity_check":       integrityCheck,
	}, nil
}

func sqliteSidecarStates(dbPath string) []SidecarState {
	states := make([]SidecarState, 0, 2)
	for _, suffix := range []string{"-wal", "-shm"} {
		path := dbPath + suffix
		state := SidecarState{RelativePath: filepath.Base(path)}
		if info, err := os.Stat(path); err == nil {
			state.Exists = true
			state.SizeBytes = info.Size()
		}
		states = append(states, state)
	}
	return states
}

func helperCommand(ctx context.Context, helper HelperProcess, args ...string) *exec.Cmd {
	allArgs := append(append([]string{}, helper.PrefixArgs...), args...)
	cmd := exec.CommandContext(ctx, helper.Executable, allArgs...)
	cmd.Env = append(os.Environ(), helper.ExtraEnv...)
	return cmd
}

// RunHelperCommand implements the deliberately narrow subprocess entrypoints
// used by the two-process lock probe. Both reject paths outside the OS temp
// directory, so they cannot accidentally operate on a user's database.
func RunHelperCommand(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("missing helper command")
	}
	switch args[0] {
	case "lock-holder":
		flags := flag.NewFlagSet("lock-holder", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		dbPath := flags.String("db", "", "temporary SQLite database")
		readyPath := flags.String("ready", "", "temporary readiness file")
		hold := flags.Duration("hold", 30*time.Second, "maximum lock duration")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *dbPath == "" || *readyPath == "" {
			return errors.New("lock-holder requires --db and --ready")
		}
		return runLockHolder(ctx, *dbPath, *readyPath, *hold)
	case "lock-contender":
		flags := flag.NewFlagSet("lock-contender", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		dbPath := flags.String("db", "", "temporary SQLite database")
		busy := flags.Duration("busy", 200*time.Millisecond, "busy timeout")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *dbPath == "" {
			return errors.New("lock-contender requires --db")
		}
		result, err := runLockContender(ctx, *dbPath, *busy)
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(result)
	default:
		return fmt.Errorf("unknown helper command %q", args[0])
	}
}

func requireTemporaryPath(path string) error {
	if path == "" {
		return errors.New("empty path")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	absTemp, err := filepath.Abs(os.TempDir())
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absTemp, absPath)
	if err != nil {
		return err
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("refusing non-temporary path %q (temp root is %q)", absPath, absTemp)
	}
	return nil
}
