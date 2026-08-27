package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3"
)

const (
	qualificationStressInitialBalance = int64(1_000_000)
	qualificationStressWriters        = 4
	qualificationStressRounds         = 12
	qualificationStressBusyTimeout    = 80 * time.Millisecond
)

var errQualificationStressRollback = errors.New("qualification: injected rollback")

type qualificationStressOperation struct {
	id           string
	delta        int64
	shouldCommit bool
}

type qualificationStressState struct {
	balanceOne int64
	operations map[string]int64
}

type qualificationStressSnapshot struct {
	connection  *sql.Conn
	transaction *sql.Tx
}

type qualificationStressQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// TestQualificationBoundedSeededWALStress is a deterministic, bounded
// developer stress slice. It does not replace the release-machine random-kill
// campaign or the 24-48 hour WAL/checkpoint/backup soak required by ADR 0010.
func TestQualificationBoundedSeededWALStress(t *testing.T) {
	scenarios := []struct {
		seed    int64
		readers int
	}{
		{seed: 0x0db002001, readers: 1},
		{seed: 0x0db0020a5, readers: 4},
		{seed: 0x0db002f17, readers: 8},
	}
	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(fmt.Sprintf("seed_%09x_readers_%d", scenario.seed, scenario.readers), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()

			path := filepath.Join(t.TempDir(), "bounded-wal-stress.sqlite3")
			var store *Store
			store, err := Open(ctx, path, Options{
				BusyTimeout: defaultBusyTimeout,
				Connections: scenario.readers + qualificationStressWriters + 2,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if store != nil {
					_ = store.Close()
				}
			})
			if err := qualificationStressCreateState(ctx, store); err != nil {
				t.Fatal(err)
			}

			state := qualificationStressState{
				balanceOne: qualificationStressInitialBalance,
				operations: make(map[string]int64),
			}
			plan := qualificationStressPlan(scenario.seed)
			busyOperation, busyElapsed, err := qualificationStressBusyRetry(ctx, store, scenario.seed)
			if err != nil {
				t.Fatalf("seed=%#x busy probe: %v", scenario.seed, err)
			}
			state.apply(busyOperation)
			cancelElapsed, err := qualificationStressCancelAndReuse(ctx, store, scenario.seed, state)
			if err != nil {
				t.Fatalf("seed=%#x cancellation probe: %v", scenario.seed, err)
			}
			if err := qualificationStressCheckState(ctx, store.db, state); err != nil {
				t.Fatalf("seed=%#x probes changed committed state: %v", scenario.seed, err)
			}

			snapshotState := state.clone()
			snapshots, err := qualificationStressOpenSnapshots(ctx, store, scenario.readers, snapshotState)
			if err != nil {
				t.Fatalf("seed=%#x retained readers: %v", scenario.seed, err)
			}
			t.Cleanup(func() { qualificationStressCleanupSnapshots(snapshots) })

			for round := 0; round < qualificationStressRounds; round++ {
				results := make(chan error, qualificationStressWriters+len(snapshots))
				start := make(chan struct{})
				var wait sync.WaitGroup
				for writer := 0; writer < qualificationStressWriters; writer++ {
					operation := plan[round*qualificationStressWriters+writer]
					wait.Add(1)
					go func() {
						defer wait.Done()
						<-start
						if err := qualificationStressExecute(ctx, store, operation); err != nil {
							results <- fmt.Errorf("writer operation=%s: %w", operation.id, err)
						} else {
							results <- nil
						}
					}()
				}
				for index, snapshot := range snapshots {
					index, snapshot := index, snapshot
					wait.Add(1)
					go func() {
						defer wait.Done()
						<-start
						if err := qualificationStressCheckState(ctx, snapshot.transaction, snapshotState); err != nil {
							results <- fmt.Errorf("reader=%d: %w", index, err)
						} else {
							results <- nil
						}
					}()
				}
				close(start)
				wait.Wait()
				close(results)
				for result := range results {
					if result != nil {
						t.Fatalf("seed=%#x round=%d: %v", scenario.seed, round, result)
					}
				}
				for writer := 0; writer < qualificationStressWriters; writer++ {
					operation := plan[round*qualificationStressWriters+writer]
					if operation.shouldCommit {
						state.apply(operation)
					}
				}
				if err := qualificationStressCheckState(ctx, store.db, state); err != nil {
					t.Fatalf("seed=%#x round=%d committed state: %v", scenario.seed, round, err)
				}
			}

			busy, logFrames, checkpointed, err := qualificationStressCheckpoint(ctx, store.db, "PASSIVE")
			if err != nil || busy != 0 || logFrames <= checkpointed {
				t.Fatalf("seed=%#x retained-reader checkpoint = busy:%d log:%d checkpointed:%d err:%v", scenario.seed, busy, logFrames, checkpointed, err)
			}
			if err := qualificationStressReleaseSnapshots(snapshots); err != nil {
				t.Fatalf("seed=%#x release readers: %v", scenario.seed, err)
			}
			busy, logFrames, checkpointed, err = qualificationStressCheckpoint(ctx, store.db, "TRUNCATE")
			if err != nil || busy != 0 || logFrames != 0 || checkpointed != 0 {
				t.Fatalf("seed=%#x truncate checkpoint = busy:%d log:%d checkpointed:%d err:%v", scenario.seed, busy, logFrames, checkpointed, err)
			}
			if err := store.IntegrityCheck(ctx); err != nil {
				t.Fatalf("seed=%#x integrity before close: %v", scenario.seed, err)
			}
			if err := store.CanonicalConsistencyCheck(ctx); err != nil {
				t.Fatalf("seed=%#x canonical consistency before close: %v", scenario.seed, err)
			}
			if err := store.Close(); err != nil {
				t.Fatalf("seed=%#x close: %v", scenario.seed, err)
			}
			store = nil
			store, err = Open(ctx, path, Options{
				BusyTimeout: defaultBusyTimeout,
				Connections: scenario.readers + qualificationStressWriters + 2,
			})
			if err != nil {
				t.Fatalf("seed=%#x reopen: %v", scenario.seed, err)
			}
			if err := qualificationStressCheckState(ctx, store.db, state); err != nil {
				t.Fatalf("seed=%#x reopened state: %v", scenario.seed, err)
			}
			if err := store.IntegrityCheck(ctx); err != nil {
				t.Fatalf("seed=%#x integrity after reopen: %v", scenario.seed, err)
			}
			if err := store.CanonicalConsistencyCheck(ctx); err != nil {
				t.Fatalf("seed=%#x canonical consistency after reopen: %v", scenario.seed, err)
			}

			plannedCommits := 0
			for _, operation := range plan {
				if operation.shouldCommit {
					plannedCommits++
				}
			}
			t.Logf("seed=%#x readers=%d writers=%d rounds=%d committed=%d rolled_back=%d busy_wait=%s exact_busy_retry=committed cancel_wait=%s checkpoint=retained_then_truncated integrity=ok reopen=ok",
				scenario.seed, scenario.readers, qualificationStressWriters, qualificationStressRounds,
				len(state.operations), len(plan)-plannedCommits, busyElapsed, cancelElapsed)
		})
	}
}

func qualificationStressCreateState(ctx context.Context, store *Store) error {
	return store.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			CREATE TABLE qualification_stress_accounts(
				id INTEGER PRIMARY KEY,
				balance INTEGER NOT NULL CHECK(balance >= 0)
			) STRICT;
			CREATE TABLE qualification_stress_operations(
				id TEXT PRIMARY KEY,
				delta INTEGER NOT NULL CHECK(delta <> 0)
			) STRICT;
			CREATE TABLE qualification_stress_meta(
				singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
				committed INTEGER NOT NULL CHECK(committed >= 0)
			) STRICT;
			INSERT INTO qualification_stress_accounts(id, balance)
			VALUES(1, 1000000), (2, 1000000);
			INSERT INTO qualification_stress_meta(singleton, committed) VALUES(1, 0);
		`)
		return err
	})
}

func qualificationStressPlan(seed int64) []qualificationStressOperation {
	random := rand.New(rand.NewSource(seed))
	operations := make([]qualificationStressOperation, qualificationStressRounds*qualificationStressWriters)
	commits, rollbacks := 0, 0
	for index := range operations {
		delta := int64(random.Intn(1000) + 1)
		if random.Intn(2) == 0 {
			delta = -delta
		}
		operations[index] = qualificationStressOperation{
			id:           fmt.Sprintf("%016x-%03d", uint64(seed), index),
			delta:        delta,
			shouldCommit: random.Intn(5) != 0,
		}
		if operations[index].shouldCommit {
			commits++
		} else {
			rollbacks++
		}
	}
	if commits == 0 {
		operations[0].shouldCommit = true
	}
	if rollbacks == 0 {
		operations[len(operations)-1].shouldCommit = false
	}
	return operations
}

func (state *qualificationStressState) apply(operation qualificationStressOperation) {
	state.balanceOne -= operation.delta
	state.operations[operation.id] = operation.delta
}

func (state qualificationStressState) clone() qualificationStressState {
	cloned := qualificationStressState{
		balanceOne: state.balanceOne,
		operations: make(map[string]int64, len(state.operations)),
	}
	for id, delta := range state.operations {
		cloned.operations[id] = delta
	}
	return cloned
}

func qualificationStressCheckState(ctx context.Context, queryer qualificationStressQueryer, want qualificationStressState) error {
	var balanceOne, balanceTwo int64
	var revision, operationCount int
	var fingerprint string
	err := queryer.QueryRowContext(ctx, `
		SELECT
			(SELECT balance FROM qualification_stress_accounts WHERE id = 1),
			(SELECT balance FROM qualification_stress_accounts WHERE id = 2),
			(SELECT committed FROM qualification_stress_meta WHERE singleton = 1),
			(SELECT count(*) FROM qualification_stress_operations),
			(SELECT coalesce(group_concat(item, char(31)), '') FROM (
				SELECT id || ':' || CAST(delta AS TEXT) AS item
				FROM qualification_stress_operations ORDER BY id
			))
	`).Scan(&balanceOne, &balanceTwo, &revision, &operationCount, &fingerprint)
	if err != nil {
		return err
	}
	if balanceOne != want.balanceOne || balanceTwo != 2*qualificationStressInitialBalance-want.balanceOne {
		return fmt.Errorf("balances = %d/%d, want %d/%d", balanceOne, balanceTwo,
			want.balanceOne, 2*qualificationStressInitialBalance-want.balanceOne)
	}
	if revision != len(want.operations) || operationCount != len(want.operations) {
		return fmt.Errorf("revision/count = %d/%d, want %d", revision, operationCount, len(want.operations))
	}
	items := make([]string, 0, len(want.operations))
	for id, delta := range want.operations {
		items = append(items, fmt.Sprintf("%s:%d", id, delta))
	}
	sort.Strings(items)
	if wantFingerprint := strings.Join(items, "\x1f"); fingerprint != wantFingerprint {
		return fmt.Errorf("operation fingerprint = %q, want %q", fingerprint, wantFingerprint)
	}
	return nil
}

func qualificationStressApply(ctx context.Context, tx *sql.Tx, operation qualificationStressOperation, yield bool) error {
	if _, err := tx.ExecContext(ctx, `UPDATE qualification_stress_accounts SET balance = balance - ? WHERE id = 1`, operation.delta); err != nil {
		return err
	}
	if yield {
		runtime.Gosched()
	}
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`UPDATE qualification_stress_accounts SET balance = balance + ? WHERE id = 2`, []any{operation.delta}},
		{`INSERT INTO qualification_stress_operations(id, delta) VALUES(?, ?)`, []any{operation.id, operation.delta}},
		{`UPDATE qualification_stress_meta SET committed = committed + 1 WHERE singleton = 1`, nil},
	} {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return err
		}
	}
	return nil
}

func qualificationStressExecute(ctx context.Context, store *Store, operation qualificationStressOperation) error {
	err := store.withTx(ctx, func(tx *sql.Tx) error {
		if err := qualificationStressApply(ctx, tx, operation, true); err != nil {
			return err
		}
		if !operation.shouldCommit {
			return errQualificationStressRollback
		}
		return nil
	})
	if operation.shouldCommit {
		return err
	}
	if !errors.Is(err, errQualificationStressRollback) {
		return fmt.Errorf("injected rollback result = %w", err)
	}
	return nil
}

func qualificationStressBusyRetry(ctx context.Context, store *Store, seed int64) (qualificationStressOperation, time.Duration, error) {
	holder, err := store.db.Conn(ctx)
	if err != nil {
		return qualificationStressOperation{}, 0, err
	}
	defer holder.Close()
	contender, err := store.db.Conn(ctx)
	if err != nil {
		return qualificationStressOperation{}, 0, err
	}
	defer contender.Close()

	shortBusy := fmt.Sprintf("PRAGMA busy_timeout=%d", qualificationStressBusyTimeout.Milliseconds())
	defaultBusy := fmt.Sprintf("PRAGMA busy_timeout=%d", defaultBusyTimeout.Milliseconds())
	if _, err := contender.ExecContext(ctx, shortBusy); err != nil {
		return qualificationStressOperation{}, 0, err
	}
	defer func() { _, _ = contender.ExecContext(context.Background(), defaultBusy) }()
	var configured int
	if err := contender.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&configured); err != nil || configured != int(qualificationStressBusyTimeout.Milliseconds()) {
		return qualificationStressOperation{}, 0, fmt.Errorf("short busy timeout = %d, err=%v", configured, err)
	}

	holderTx, err := holder.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return qualificationStressOperation{}, 0, err
	}
	holderOpen := true
	defer func() {
		if holderOpen {
			_ = holderTx.Rollback()
		}
	}()
	if err := qualificationStressApply(ctx, holderTx, qualificationStressOperation{
		id: fmt.Sprintf("busy-holder-%016x", uint64(seed)), delta: 1,
	}, false); err != nil {
		return qualificationStressOperation{}, 0, err
	}
	retry := qualificationStressOperation{
		id: fmt.Sprintf("busy-retry-%016x", uint64(seed)), delta: -1, shouldCommit: true,
	}
	waitContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	started := time.Now()
	busyErr := qualificationStressCommit(waitContext, contender, retry)
	elapsed := time.Since(started)
	if busyErr == nil {
		return qualificationStressOperation{}, elapsed, errors.New("contender acquired concurrent write transaction")
	}
	if !errors.Is(busyErr, sqlite3.BUSY) && !errors.Is(busyErr, sqlite3.LOCKED) {
		return qualificationStressOperation{}, elapsed, fmt.Errorf("contender = %w, want numeric BUSY/LOCKED", busyErr)
	}
	if elapsed+25*time.Millisecond < qualificationStressBusyTimeout {
		return qualificationStressOperation{}, elapsed, fmt.Errorf("busy wait %s shorter than %s", elapsed, qualificationStressBusyTimeout)
	}
	if err := holderTx.Rollback(); err != nil {
		return qualificationStressOperation{}, elapsed, err
	}
	holderOpen = false
	if err := qualificationStressCommit(ctx, contender, retry); err != nil {
		return qualificationStressOperation{}, elapsed, fmt.Errorf("exact retry: %w", err)
	}
	if _, err := contender.ExecContext(ctx, defaultBusy); err != nil {
		return qualificationStressOperation{}, elapsed, err
	}
	if err := contender.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&configured); err != nil || configured != int(defaultBusyTimeout.Milliseconds()) {
		return qualificationStressOperation{}, elapsed, fmt.Errorf("restored busy timeout = %d, err=%v", configured, err)
	}
	return retry, elapsed, nil
}

func qualificationStressCommit(ctx context.Context, connection *sql.Conn, operation qualificationStressOperation) error {
	tx, err := connection.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := qualificationStressApply(ctx, tx, operation, false); err != nil {
		return err
	}
	return tx.Commit()
}

func qualificationStressCancelAndReuse(ctx context.Context, store *Store, seed int64, want qualificationStressState) (time.Duration, error) {
	connection, err := store.db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer connection.Close()
	tx, err := connection.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return 0, err
	}
	if err := qualificationStressApply(ctx, tx, qualificationStressOperation{
		id: fmt.Sprintf("cancel-%016x", uint64(seed)), delta: 1,
	}, false); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	cancelContext, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	started := time.Now()
	var ignored int64
	queryErr := tx.QueryRowContext(cancelContext, `
		WITH RECURSIVE sequence(value) AS (
			SELECT 0 UNION ALL SELECT value + 1 FROM sequence LIMIT 10000000
		)
		SELECT min(value) FROM sequence
	`).Scan(&ignored)
	elapsed := time.Since(started)
	cancel()
	if queryErr == nil {
		_ = tx.Rollback()
		return elapsed, errors.New("cancellation query completed unexpectedly")
	}
	if !errors.Is(queryErr, context.DeadlineExceeded) && !errors.Is(queryErr, context.Canceled) && !errors.Is(queryErr, sqlite3.INTERRUPT) {
		_ = tx.Rollback()
		return elapsed, fmt.Errorf("cancel result = %w, want context cancellation/INTERRUPT", queryErr)
	}
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return elapsed, err
	}
	if err := qualificationStressCheckState(ctx, connection, want); err != nil {
		return elapsed, fmt.Errorf("same connection retained partial transition: %w", err)
	}
	var answer int
	if err := connection.QueryRowContext(ctx, "SELECT 40 + 2").Scan(&answer); err != nil || answer != 42 {
		return elapsed, fmt.Errorf("reuse query = %d, err=%v", answer, err)
	}
	return elapsed, nil
}

func qualificationStressOpenSnapshots(ctx context.Context, store *Store, count int, want qualificationStressState) ([]*qualificationStressSnapshot, error) {
	snapshots := make([]*qualificationStressSnapshot, 0, count)
	for index := 0; index < count; index++ {
		connection, err := store.db.Conn(ctx)
		if err != nil {
			qualificationStressCleanupSnapshots(snapshots)
			return nil, err
		}
		transaction, err := connection.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			_ = connection.Close()
			qualificationStressCleanupSnapshots(snapshots)
			return nil, err
		}
		snapshots = append(snapshots, &qualificationStressSnapshot{connection, transaction})
		if err := qualificationStressCheckState(ctx, transaction, want); err != nil {
			qualificationStressCleanupSnapshots(snapshots)
			return nil, fmt.Errorf("reader %d initial state: %w", index, err)
		}
	}
	return snapshots, nil
}

func qualificationStressReleaseSnapshots(snapshots []*qualificationStressSnapshot) error {
	var result error
	for index, snapshot := range snapshots {
		if snapshot.transaction != nil {
			result = errors.Join(result, qualificationStressWrap(snapshot.transaction.Commit(), "reader %d commit", index))
			snapshot.transaction = nil
		}
		if snapshot.connection != nil {
			result = errors.Join(result, qualificationStressWrap(snapshot.connection.Close(), "reader %d close", index))
			snapshot.connection = nil
		}
	}
	return result
}

func qualificationStressCleanupSnapshots(snapshots []*qualificationStressSnapshot) {
	for _, snapshot := range snapshots {
		if snapshot.transaction != nil {
			_ = snapshot.transaction.Rollback()
			snapshot.transaction = nil
		}
		if snapshot.connection != nil {
			_ = snapshot.connection.Close()
			snapshot.connection = nil
		}
	}
}

func qualificationStressCheckpoint(ctx context.Context, database *sql.DB, mode string) (int, int, int, error) {
	var busy, logFrames, checkpointed int
	err := database.QueryRowContext(ctx, "PRAGMA wal_checkpoint("+mode+")").Scan(&busy, &logFrames, &checkpointed)
	return busy, logFrames, checkpointed, err
}

func qualificationStressWrap(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf(format+": %w", append(args, err)...)
}
