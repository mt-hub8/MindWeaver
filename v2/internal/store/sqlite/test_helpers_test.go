package sqlite

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

var testTime = time.Date(2026, 8, 24, 10, 0, 0, 123456000, time.UTC)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(now time.Time) *fakeClock {
	return &fakeClock{now: now}
}

func (clock *fakeClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *fakeClock) Set(now time.Time) {
	clock.mu.Lock()
	clock.now = now
	clock.mu.Unlock()
}

func (clock *fakeClock) Advance(duration time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(duration)
	clock.mu.Unlock()
}

func openTestStore(t *testing.T, path string, clock *fakeClock) *Store {
	t.Helper()
	store, err := Open(t.Context(), path, Options{BusyTimeout: time.Second, Connections: 4})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	store.now = clock.Now
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newTestStore(t *testing.T, clock *fakeClock) *Store {
	t.Helper()
	return openTestStore(t, filepath.Join(t.TempDir(), "mindweaver.db"), clock)
}

func expectedMigrationCount(t testing.TB) int {
	t.Helper()
	items, err := readMigrations()
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	return len(items)
}

func enqueueJob(t *testing.T, ctx context.Context, store *Store, id string, attempts int, runAfter time.Time) {
	t.Helper()
	if err := store.Enqueue(ctx, EnqueueParams{
		ID: id, Kind: "test", PayloadJSON: `{"source":"test"}`,
		MaxAttempts: attempts, RunAfter: runAfter,
	}); err != nil {
		t.Fatalf("enqueue %s: %v", id, err)
	}
}

func assertRowCount(t *testing.T, store *Store, table string, want int) {
	t.Helper()
	var got int
	if err := store.db.QueryRow("SELECT count(*) FROM " + table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}
