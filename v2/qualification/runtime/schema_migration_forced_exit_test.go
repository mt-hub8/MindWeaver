package runtimequalification_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/app"
	"github.com/mt-hub8/MindWeaver/v2/internal/localhttp"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
	"github.com/mt-hub8/MindWeaver/v2/platform/config"
	"github.com/ncruces/go-sqlite3"
	sqliteDriver "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
)

const (
	run002ChildSandboxEnvironment = "MWQ_RUN002_CHILD_SANDBOX"
	run002ChildNonceEnvironment   = "MWQ_RUN002_CHILD_NONCE"
	run002ChildModeEnvironment    = "MWQ_RUN002_CHILD_MODE"
	run002ChildVersionEnvironment = "MWQ_RUN002_CHILD_FROM_VERSION"
	run002CapabilityFile          = ".run002-capability"
	run002CheckpointVersion       = 1
	run002BeforePhase             = "BEFORE_FIRST_PENDING_TRANSACTION"
	run002AfterPhase              = "AFTER_ALL_MIGRATIONS_BEFORE_LISTENER"
	run002ChildCheckpointTimeout  = 20 * time.Second
	run002BlockedObservation      = 200 * time.Millisecond
)

type run002Migration struct {
	version  int
	name     string
	checksum string
	sql      string
}

type run002Checkpoint struct {
	Version         int    `json:"version"`
	Nonce           string `json:"nonce"`
	Phase           string `json:"phase"`
	FromVersion     int    `json:"from_version"`
	ObservedVersion int    `json:"observed_version"`
}

func TestRUN002SchemaMigrationForcedExitMatrix(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("RUN-002 forced-exit qualification requires Windows amd64")
	}
	migrations := loadRUN002Migrations(t)
	if len(migrations) != 7 {
		t.Fatalf("RUN002_SUPPORTED_MIGRATION_SET_INVALID: got %d", len(migrations))
	}
	for fromVersion := 1; fromVersion < len(migrations); fromVersion++ {
		fromVersion := fromVersion
		for _, phase := range []string{run002BeforePhase, run002AfterPhase} {
			phase := phase
			t.Run(fmt.Sprintf("from_%03d/%s", fromVersion, phase), func(t *testing.T) {
				qualifyRUN002ForcedExit(t, migrations, fromVersion, phase)
			})
		}
	}
}

func qualifyRUN002ForcedExit(t *testing.T, migrations []run002Migration, fromVersion int, phase string) {
	t.Helper()
	ctx := t.Context()
	root := t.TempDir()
	sandbox := filepath.Join(root, "child-sandbox")
	nonce := prepareRUN002Sandbox(t, sandbox)
	vaultRoot := filepath.Join(sandbox, "vault")
	configPath := filepath.Join(sandbox, "mindweaver.v1.json")
	databasePath := prepareRUN002HistoricalVault(t, ctx, migrations, vaultRoot, configPath, fromVersion)

	var blocker *sql.Tx
	var blockerDatabase *sql.DB
	if phase == run002BeforePhase {
		blockerDatabase = openRUN002Database(t, databasePath)
		var err error
		blocker, err = blockerDatabase.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			t.Fatal("RUN002_BLOCKER_BEGIN_FAILED")
		}
		assertRUN002LedgerQuery(t, blocker, migrations[:fromVersion])
		assertRUN002CollectionCountQuery(t, blocker, 0)
	}

	checkpoint, processDone := startRUN002Child(t, sandbox, nonce, phase, fromVersion)
	if checkpoint.Version != run002CheckpointVersion || checkpoint.Nonce != nonce ||
		checkpoint.Phase != phase || checkpoint.FromVersion != fromVersion {
		t.Fatal("RUN002_CHILD_CHECKPOINT_INVALID")
	}
	wantObserved := len(migrations)
	if phase == run002BeforePhase {
		wantObserved = fromVersion
	}
	if checkpoint.ObservedVersion != wantObserved {
		t.Fatalf("RUN002_CHILD_OBSERVED_VERSION_INVALID: got %d want %d", checkpoint.ObservedVersion, wantObserved)
	}

	if phase == run002BeforePhase {
		select {
		case err := <-processDone.done:
			t.Fatalf("RUN002_CHILD_DID_NOT_REMAIN_BLOCKED: %v", err)
		case <-time.After(run002BlockedObservation):
		}
		assertRUN002LedgerQuery(t, blocker, migrations[:fromVersion])
		assertRUN002CollectionCountQuery(t, blocker, 0)
	}
	killRUN002Child(t, processDone)

	if blocker != nil {
		if err := blocker.Rollback(); err != nil {
			t.Fatal("RUN002_BLOCKER_ROLLBACK_FAILED")
		}
		if err := blockerDatabase.Close(); err != nil {
			t.Fatal("RUN002_BLOCKER_CLOSE_FAILED")
		}
	} else {
		observer := openRUN002Database(t, databasePath)
		assertRUN002LedgerQuery(t, observer, migrations)
		assertRUN002CollectionCountQuery(t, observer, 0)
		if err := observer.Close(); err != nil {
			t.Fatal("RUN002_POST_KILL_OBSERVER_CLOSE_FAILED")
		}
	}

	routeObserved := false
	application, err := app.Start(ctx, app.Options{
		ConfigPath:        configPath,
		FirstRunVaultRoot: vaultRoot,
		WorkerInterval:    10 * time.Second,
		PDFHelperPath:     filepath.Join(sandbox, "missing-pdf-helper.exe"),
		ExtraRoutes: []app.RouteRegistrar{func(*localhttp.Router) error {
			version, inspectErr := store.InspectSchemaVersion(ctx, databasePath)
			if inspectErr != nil || version != len(migrations) {
				return errors.New("RUN002_ROUTE_SCHEMA_NOT_CURRENT")
			}
			routeObserved = true
			return nil
		}},
	})
	if err != nil {
		t.Fatal("RUN002_FIRST_OWNER_START_FAILED")
	}
	shutdownComplete := false
	t.Cleanup(func() {
		if !shutdownComplete {
			shutdownRUN002App(t, application)
		}
	})
	if !routeObserved || application.Origin() == "" {
		t.Fatal("RUN002_STARTUP_ORDER_NOT_OBSERVED")
	}
	collectionID := createRUN002Collection(t, application, fromVersion, phase)
	shutdownRUN002App(t, application)
	shutdownComplete = true

	reopened, err := store.Open(ctx, databasePath, store.Options{BusyTimeout: time.Second, Connections: 1})
	if err != nil {
		t.Fatal("RUN002_RECOVERED_STORE_OPEN_FAILED")
	}
	version, versionErr := reopened.SchemaVersion(ctx)
	integrityErr := reopened.IntegrityCheck(ctx)
	collection, collectionErr := reopened.GetCollection(ctx, collectionID)
	closeErr := reopened.Close()
	if versionErr != nil || version != len(migrations) || integrityErr != nil ||
		collectionErr != nil || collection.ID != collectionID || closeErr != nil {
		t.Fatal("RUN002_RECOVERED_STORE_QUALIFICATION_FAILED")
	}
	verifyRUN002Database(t, databasePath, migrations, collectionID)

	replay, err := store.Open(ctx, databasePath, store.Options{BusyTimeout: time.Second, Connections: 1})
	if err != nil {
		t.Fatal("RUN002_SECOND_OWNER_OPEN_FAILED")
	}
	replayVersion, replayVersionErr := replay.SchemaVersion(ctx)
	replayCollection, replayCollectionErr := replay.GetCollection(ctx, collectionID)
	replayIntegrityErr := replay.IntegrityCheck(ctx)
	replayCloseErr := replay.Close()
	if replayVersionErr != nil || replayVersion != len(migrations) || replayCollectionErr != nil ||
		replayCollection.ID != collectionID || replayIntegrityErr != nil || replayCloseErr != nil {
		t.Fatal("RUN002_SECOND_OWNER_REOPEN_FAILED")
	}
}

func TestRUN002SchemaMigrationForcedExitChild(t *testing.T) {
	sandbox := os.Getenv(run002ChildSandboxEnvironment)
	nonce := os.Getenv(run002ChildNonceEnvironment)
	phase := os.Getenv(run002ChildModeEnvironment)
	versionText := os.Getenv(run002ChildVersionEnvironment)
	if sandbox == "" && nonce == "" && phase == "" && versionText == "" {
		t.Skip("authorized RUN-002 forced-exit child only")
	}
	fromVersion, err := strconv.Atoi(versionText)
	if err != nil || fromVersion < 1 || fromVersion > 6 ||
		(phase != run002BeforePhase && phase != run002AfterPhase) ||
		!claimRUN002Sandbox(sandbox, nonce) {
		t.Fatal("RUN002_CHILD_AUTHORIZATION_INVALID")
	}
	databasePath := filepath.Join(sandbox, "vault", "data", store.DatabaseFileName)
	configPath := filepath.Join(sandbox, "mindweaver.v1.json")
	if phase == run002BeforePhase {
		observed, inspectErr := store.InspectSchemaVersion(t.Context(), databasePath)
		if inspectErr != nil || observed != fromVersion {
			t.Fatal("RUN002_CHILD_BASELINE_INVALID")
		}
		writeRUN002Checkpoint(t, run002Checkpoint{
			Version: run002CheckpointVersion, Nonce: nonce, Phase: phase,
			FromVersion: fromVersion, ObservedVersion: observed,
		})
		application, startErr := app.Start(t.Context(), app.Options{
			ConfigPath: configPath, WorkerInterval: 10 * time.Second,
			PDFHelperPath: filepath.Join(sandbox, "missing-pdf-helper.exe"),
		})
		if application != nil {
			shutdownRUN002App(t, application)
		}
		if startErr == nil {
			t.Fatal("RUN002_CHILD_BLOCKED_START_RETURNED")
		}
		t.Fatal("RUN002_CHILD_BLOCKED_START_FAILED")
	}

	application, startErr := app.Start(t.Context(), app.Options{
		ConfigPath: configPath, WorkerInterval: 10 * time.Second,
		PDFHelperPath: filepath.Join(sandbox, "missing-pdf-helper.exe"),
		ExtraRoutes: []app.RouteRegistrar{func(*localhttp.Router) error {
			observed, inspectErr := store.InspectSchemaVersion(t.Context(), databasePath)
			if inspectErr != nil || observed != 7 {
				return errors.New("RUN002_CHILD_MIGRATION_NOT_CURRENT")
			}
			writeRUN002Checkpoint(t, run002Checkpoint{
				Version: run002CheckpointVersion, Nonce: nonce, Phase: phase,
				FromVersion: fromVersion, ObservedVersion: observed,
			})
			select {}
		}},
	})
	if application != nil {
		shutdownRUN002App(t, application)
	}
	if startErr != nil {
		t.Fatal("RUN002_CHILD_AFTER_START_FAILED")
	}
	t.Fatal("RUN002_CHILD_AFTER_START_RETURNED")
}

func loadRUN002Migrations(t *testing.T) []run002Migration {
	t.Helper()
	root := moduleRoot(t)
	paths, err := filepath.Glob(filepath.Join(root, "internal", "store", "sqlite", "migrations", "*.sql"))
	if err != nil {
		t.Fatal("RUN002_MIGRATION_GLOB_FAILED")
	}
	sort.Strings(paths)
	result := make([]run002Migration, 0, len(paths))
	for index, path := range paths {
		base := filepath.Base(path)
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		versionText, name, ok := strings.Cut(stem, "_")
		version, parseErr := strconv.Atoi(versionText)
		if !ok || parseErr != nil || len(versionText) != 3 || version != index+1 || name == "" {
			t.Fatal("RUN002_MIGRATION_SEQUENCE_INVALID")
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal("RUN002_MIGRATION_READ_FAILED")
		}
		digest := sha256.Sum256(contents)
		result = append(result, run002Migration{
			version: version, name: name, checksum: hex.EncodeToString(digest[:]), sql: string(contents),
		})
	}
	return result
}

func prepareRUN002HistoricalVault(
	t *testing.T,
	ctx context.Context,
	migrations []run002Migration,
	vaultRoot, configPath string,
	fromVersion int,
) string {
	t.Helper()
	openedVault, err := vault.Open(vaultRoot)
	if err != nil {
		t.Fatal("RUN002_FIXTURE_VAULT_CREATE_FAILED")
	}
	paths := openedVault.Paths()
	if err := openedVault.Close(); err != nil {
		t.Fatal("RUN002_FIXTURE_VAULT_CLOSE_FAILED")
	}
	databasePath := filepath.Join(paths.Data, store.DatabaseFileName)
	database := openRUN002Database(t, databasePath)
	applyRUN002FixtureMigrations(t, database, migrations[:fromVersion])
	if err := database.Close(); err != nil {
		t.Fatal("RUN002_FIXTURE_DATABASE_CLOSE_FAILED")
	}
	if err := config.WriteNew(ctx, configPath, config.Default(vaultRoot)); err != nil {
		t.Fatal("RUN002_FIXTURE_CONFIG_FAILED")
	}
	return databasePath
}

func applyRUN002FixtureMigrations(t *testing.T, database *sql.DB, migrations []run002Migration) {
	t.Helper()
	transaction, err := database.BeginTx(t.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatal("RUN002_FIXTURE_LEDGER_BEGIN_FAILED")
	}
	if _, err := transaction.ExecContext(t.Context(), `
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			checksum TEXT NOT NULL,
			applied_at TEXT NOT NULL
		) STRICT
	`); err != nil {
		_ = transaction.Rollback()
		t.Fatal("RUN002_FIXTURE_LEDGER_CREATE_FAILED")
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal("RUN002_FIXTURE_LEDGER_COMMIT_FAILED")
	}
	for _, migration := range migrations {
		transaction, err = database.BeginTx(t.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			t.Fatal("RUN002_FIXTURE_MIGRATION_BEGIN_FAILED")
		}
		if _, err := transaction.ExecContext(t.Context(), migration.sql); err != nil {
			_ = transaction.Rollback()
			t.Fatal("RUN002_FIXTURE_MIGRATION_APPLY_FAILED")
		}
		if _, err := transaction.ExecContext(t.Context(), `
			INSERT INTO schema_migrations(version, name, checksum, applied_at)
			VALUES (?, ?, ?, '2026-08-27T00:00:00.000Z')
		`, migration.version, migration.name, migration.checksum); err != nil {
			_ = transaction.Rollback()
			t.Fatal("RUN002_FIXTURE_MIGRATION_LEDGER_FAILED")
		}
		if err := transaction.Commit(); err != nil {
			t.Fatal("RUN002_FIXTURE_MIGRATION_COMMIT_FAILED")
		}
	}
	assertRUN002LedgerQuery(t, database, migrations)
}

func openRUN002Database(t *testing.T, path string) *sql.DB {
	t.Helper()
	dsn, err := run002FileURI(path)
	if err != nil {
		t.Fatal("RUN002_DATABASE_URI_FAILED")
	}
	database, err := sqliteDriver.Open(dsn, func(connection *sqlite3.Conn) error {
		if err := connection.BusyTimeout(30 * time.Second); err != nil {
			return err
		}
		for _, statement := range []string{
			"PRAGMA foreign_keys=ON", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL",
			"PRAGMA trusted_schema=OFF", "PRAGMA recursive_triggers=ON",
		} {
			if err := connection.Exec(statement); err != nil {
				return err
			}
		}
		return fts5.Register(connection)
	})
	if err != nil {
		t.Fatal("RUN002_DATABASE_OPEN_FAILED")
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	if err := database.PingContext(t.Context()); err != nil {
		_ = database.Close()
		t.Fatal("RUN002_DATABASE_PING_FAILED")
	}
	return database
}

func run002FileURI(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	segments := strings.Split(filepath.ToSlash(absolute), "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return (&url.URL{Scheme: "file", Opaque: strings.Join(segments, "/")}).String(), nil
}

func prepareRUN002Sandbox(t *testing.T, sandbox string) string {
	t.Helper()
	if err := os.Mkdir(sandbox, 0o700); err != nil {
		t.Fatal("RUN002_SANDBOX_CREATE_FAILED")
	}
	entropy := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, entropy); err != nil {
		t.Fatal("RUN002_NONCE_CREATE_FAILED")
	}
	nonce := hex.EncodeToString(entropy)
	file, err := os.OpenFile(filepath.Join(sandbox, run002CapabilityFile), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal("RUN002_CAPABILITY_CREATE_FAILED")
	}
	_, writeErr := file.WriteString(nonce)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		t.Fatal("RUN002_CAPABILITY_WRITE_FAILED")
	}
	return nonce
}

func claimRUN002Sandbox(sandbox, nonce string) bool {
	if !filepath.IsAbs(sandbox) || filepath.Clean(sandbox) != sandbox || len(nonce) != 64 {
		return false
	}
	if decoded, err := hex.DecodeString(nonce); err != nil || len(decoded) != 32 {
		return false
	}
	path := filepath.Join(sandbox, run002CapabilityFile)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != int64(len(nonce)) {
		return false
	}
	contents, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(contents, []byte(nonce)) {
		return false
	}
	return os.Remove(path) == nil
}

type run002ChildProcess struct {
	command *exec.Cmd
	done    chan error
	stderr  *boundedBuffer
	mu      sync.Mutex
	killed  bool
}

func startRUN002Child(
	t *testing.T,
	sandbox, nonce, phase string,
	fromVersion int,
) (run002Checkpoint, *run002ChildProcess) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("RUN002_EXECUTABLE_UNAVAILABLE")
	}
	command := exec.Command(executable, "-test.run=^TestRUN002SchemaMigrationForcedExitChild$")
	command.Env = append(run002CleanEnvironment(os.Environ()),
		run002ChildSandboxEnvironment+"="+sandbox,
		run002ChildNonceEnvironment+"="+nonce,
		run002ChildModeEnvironment+"="+phase,
		run002ChildVersionEnvironment+"="+strconv.Itoa(fromVersion),
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal("RUN002_CHILD_STDOUT_FAILED")
	}
	diagnostics := &boundedBuffer{limit: 16 << 10}
	command.Stderr = diagnostics
	if err := command.Start(); err != nil {
		t.Fatal("RUN002_CHILD_START_FAILED")
	}
	child := &run002ChildProcess{command: command, done: make(chan error, 1), stderr: diagnostics}
	go func() { child.done <- command.Wait() }()
	t.Cleanup(func() {
		child.mu.Lock()
		defer child.mu.Unlock()
		if !child.killed && child.command.Process != nil {
			_ = child.command.Process.Kill()
			select {
			case <-child.done:
			case <-time.After(10 * time.Second):
			}
			child.killed = true
		}
	})
	checkpointResult := make(chan struct {
		checkpoint run002Checkpoint
		err        error
	}, 1)
	go func() {
		decoder := json.NewDecoder(io.LimitReader(stdout, 4097))
		decoder.DisallowUnknownFields()
		var checkpoint run002Checkpoint
		err := decoder.Decode(&checkpoint)
		checkpointResult <- struct {
			checkpoint run002Checkpoint
			err        error
		}{checkpoint: checkpoint, err: err}
	}()
	select {
	case result := <-checkpointResult:
		if result.err != nil {
			t.Fatalf("RUN002_CHILD_CHECKPOINT_READ_FAILED: %v (%d diagnostic bytes)", result.err, diagnostics.Len())
		}
		return result.checkpoint, child
	case err := <-child.done:
		t.Fatalf("RUN002_CHILD_EXITED_BEFORE_CHECKPOINT: %v (%d diagnostic bytes)", err, diagnostics.Len())
		return run002Checkpoint{}, nil
	case <-time.After(run002ChildCheckpointTimeout):
		t.Fatalf("RUN002_CHILD_CHECKPOINT_TIMEOUT (%d diagnostic bytes)", diagnostics.Len())
		return run002Checkpoint{}, nil
	}
}

func killRUN002Child(t *testing.T, child *run002ChildProcess) {
	t.Helper()
	child.mu.Lock()
	defer child.mu.Unlock()
	if child.killed || child.command == nil || child.command.Process == nil {
		t.Fatal("RUN002_CHILD_NOT_RUNNING")
	}
	if err := child.command.Process.Kill(); err != nil {
		t.Fatal("RUN002_CHILD_KILL_FAILED")
	}
	select {
	case err := <-child.done:
		var exitErr *exec.ExitError
		if err == nil || !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 {
			t.Fatal("RUN002_CHILD_KILL_NOT_OBSERVED")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RUN002_CHILD_REAP_TIMEOUT")
	}
	child.killed = true
}

func writeRUN002Checkpoint(t *testing.T, checkpoint run002Checkpoint) {
	t.Helper()
	if err := json.NewEncoder(os.Stdout).Encode(checkpoint); err != nil {
		t.Fatal("RUN002_CHILD_CHECKPOINT_WRITE_FAILED")
	}
}

func run002CleanEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.HasPrefix(strings.ToUpper(key), "MWQ_RUN002_CHILD_") {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func assertRUN002LedgerQuery(t *testing.T, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, migrations []run002Migration) {
	t.Helper()
	rows, err := queryer.QueryContext(t.Context(), `
		SELECT version, name, checksum FROM schema_migrations ORDER BY version
	`)
	if err != nil {
		t.Fatal("RUN002_LEDGER_QUERY_FAILED")
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		if index >= len(migrations) {
			t.Fatal("RUN002_LEDGER_HAS_EXTRA_ROW")
		}
		var version int
		var name, checksum string
		if err := rows.Scan(&version, &name, &checksum); err != nil {
			t.Fatal("RUN002_LEDGER_SCAN_FAILED")
		}
		want := migrations[index]
		if version != want.version || name != want.name || checksum != want.checksum {
			t.Fatal("RUN002_LEDGER_ROW_INVALID")
		}
		index++
	}
	if err := rows.Err(); err != nil || index != len(migrations) {
		t.Fatal("RUN002_LEDGER_SET_INVALID")
	}
}

func assertRUN002CollectionCountQuery(t *testing.T, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, want int) {
	t.Helper()
	var count int
	if err := queryer.QueryRowContext(t.Context(), "SELECT count(*) FROM collections").Scan(&count); err != nil || count != want {
		t.Fatal("RUN002_ORDINARY_WRITE_SET_INVALID")
	}
}

func verifyRUN002Database(t *testing.T, path string, migrations []run002Migration, collectionID string) {
	t.Helper()
	database := openRUN002Database(t, path)
	defer database.Close()
	assertRUN002LedgerQuery(t, database, migrations)
	assertRUN002CollectionCountQuery(t, database, 1)
	var settingsCount, collectionCount int
	if err := database.QueryRowContext(t.Context(), `
		SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'settings'
	`).Scan(&settingsCount); err != nil || settingsCount != 0 {
		t.Fatal("RUN002_REMOVED_SETTINGS_TABLE_PRESENT")
	}
	if err := database.QueryRowContext(t.Context(), `
		SELECT count(*) FROM collections WHERE id = ?
	`, collectionID).Scan(&collectionCount); err != nil || collectionCount != 1 {
		t.Fatal("RUN002_ORDINARY_WRITE_NOT_DURABLE")
	}
}

func createRUN002Collection(t *testing.T, application *app.App, fromVersion int, phase string) string {
	t.Helper()
	launch, err := url.Parse(application.LaunchURL())
	if err != nil {
		t.Fatal("RUN002_LAUNCH_URL_INVALID")
	}
	parameters, err := url.ParseQuery(launch.Fragment)
	bootstrap := parameters.Get("bootstrap")
	if err != nil || bootstrap == "" {
		t.Fatal("RUN002_BOOTSTRAP_TOKEN_MISSING")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal("RUN002_COOKIE_JAR_FAILED")
	}
	client := &http.Client{
		Jar: jar, Timeout: 10 * time.Second,
		Transport:     &http.Transport{Proxy: nil, DisableCompression: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	t.Cleanup(client.CloseIdleConnections)
	exchange, err := http.NewRequest(http.MethodPost, application.Origin()+localhttp.BootstrapExchangePath, nil)
	if err != nil {
		t.Fatal("RUN002_BOOTSTRAP_REQUEST_FAILED")
	}
	exchange.Header.Set("Origin", application.Origin())
	exchange.Header.Set(localhttp.BootstrapHeader, bootstrap)
	exchangeResponse, err := client.Do(exchange)
	if err != nil {
		t.Fatal("RUN002_BOOTSTRAP_EXCHANGE_FAILED")
	}
	var session struct {
		CSRFToken string `json:"csrfToken"`
	}
	exchangeDecodeErr := json.NewDecoder(io.LimitReader(exchangeResponse.Body, 4097)).Decode(&session)
	exchangeCloseErr := exchangeResponse.Body.Close()
	if exchangeResponse.StatusCode != http.StatusOK || exchangeDecodeErr != nil || exchangeCloseErr != nil || session.CSRFToken == "" {
		t.Fatal("RUN002_BOOTSTRAP_RESPONSE_INVALID")
	}
	name := fmt.Sprintf("RUN-002 v%03d %s", fromVersion, phase)
	body, err := json.Marshal(struct {
		Name string `json:"name"`
	}{Name: name})
	if err != nil {
		t.Fatal("RUN002_COLLECTION_BODY_FAILED")
	}
	request, err := http.NewRequest(http.MethodPost, application.Origin()+"/api/v1/collections", bytes.NewReader(body))
	if err != nil {
		t.Fatal("RUN002_COLLECTION_REQUEST_FAILED")
	}
	request.Header.Set("Origin", application.Origin())
	request.Header.Set(localhttp.CSRFHeader, session.CSRFToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", fmt.Sprintf("run002-v%03d-%x", fromVersion, sha256.Sum256([]byte(phase))))
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("RUN002_COLLECTION_WRITE_FAILED")
	}
	var payload struct {
		Collection struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"collection"`
		Created bool `json:"created"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(response.Body, 4097)).Decode(&payload)
	closeErr := response.Body.Close()
	if response.StatusCode != http.StatusCreated || decodeErr != nil || closeErr != nil ||
		!payload.Created || payload.Collection.ID == "" || payload.Collection.Name != name {
		t.Fatal("RUN002_COLLECTION_RESPONSE_INVALID")
	}
	return payload.Collection.ID
}

func shutdownRUN002App(t *testing.T, application *app.App) {
	t.Helper()
	if application == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := application.Shutdown(ctx); err != nil {
		t.Fatal("RUN002_APP_SHUTDOWN_FAILED")
	}
}
