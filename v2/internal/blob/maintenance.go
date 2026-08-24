package blob

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ObjectPin prevents permanent deletion while a caller is turning published
// bytes into a durable database reference, or while a backup is copying the
// set of objects named by its database snapshot. Pins are process local; the
// Vault's exclusive OS lock is what makes that sufficient for this product.
type ObjectPin struct {
	store *Store
	once  sync.Once
}

// PinObjectsContext acquires the shared side of the blob
// publication/deletion barrier. Cancellation cannot
// revoke a pin already returned to its caller, but it prevents a waiting
// deletion epoch from turning new uploads and backups into unbounded waits.
func (s *Store) PinObjectsContext(ctx context.Context) (*ObjectPin, error) {
	if s == nil {
		return nil, errors.New("nil blob store")
	}
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if err := s.runtime.maintenance.acquireRead(ctx); err != nil {
		return nil, err
	}
	return &ObjectPin{store: s}, nil
}

// Release releases a pin. It is safe to call more than once.
func (pin *ObjectPin) Release() {
	if pin == nil || pin.store == nil {
		return
	}
	pin.once.Do(func() { pin.store.runtime.maintenance.releaseRead() })
}

// DeletionGuard holds the exclusive side of the object barrier. A retention
// coordinator obtains the guard before it removes database references, checks
// whether each content address remains referenced, and deletes proven orphans.
// This closes the race with uploads and backups without storing a second
// reference count.
type DeletionGuard struct {
	store *Store
	mu    sync.Mutex
	done  bool
}

// BeginDeletionContext waits cancellably for all current pins and blocks new
// pins while queued. It never delegates the wait to a goroutine, so a canceled
// caller cannot later acquire and leak the exclusive barrier.
func (s *Store) BeginDeletionContext(ctx context.Context) (*DeletionGuard, error) {
	if s == nil {
		return nil, errors.New("nil blob store")
	}
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if err := s.runtime.maintenance.acquireWrite(ctx); err != nil {
		return nil, err
	}
	return &DeletionGuard{store: s}, nil
}

// Delete removes one content-addressed object while the caller holds the
// deletion barrier. A missing object is idempotent success. The containing
// directory is synced even on retry after an earlier remove/sync uncertainty.
func (guard *DeletionGuard) Delete(ctx context.Context, id BlobID) (bool, error) {
	if ctx == nil {
		return false, errors.New("nil context")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if guard == nil || guard.store == nil {
		return false, errors.New("blob deletion guard is not active")
	}

	guard.mu.Lock()
	defer guard.mu.Unlock()
	if guard.done {
		return false, errors.New("blob deletion guard is released")
	}

	s := guard.store
	path, digest, err := s.objectPath(id)
	if err != nil {
		return false, err
	}
	objectLock := s.lockForDigest(digest)
	objectLock.Lock()
	defer objectLock.Unlock()

	info, err := os.Lstat(path)
	switch {
	case err == nil:
		if !info.Mode().IsRegular() {
			return false, fmt.Errorf("delete blob object: %w: object is not a regular file", ErrCorrupt)
		}
		if err := os.Remove(path); err != nil {
			return false, fmt.Errorf("delete blob object: %w", err)
		}
		if err := s.syncDir(filepath.Dir(path)); err != nil {
			return false, fmt.Errorf("sync blob object directory after delete: %w", err)
		}
		return true, nil
	case !errors.Is(err, os.ErrNotExist):
		return false, fmt.Errorf("inspect blob object for delete: %w", err)
	}

	// Prefix directories are never pruned. If an externally modified store is
	// missing the prefix too, syncing the algorithm directory still closes the
	// only directory boundary this Store owns.
	prefix := filepath.Dir(path)
	if info, statErr := os.Lstat(prefix); statErr == nil && info.IsDir() {
		if err := s.syncDir(prefix); err != nil {
			return false, fmt.Errorf("sync missing blob object directory: %w", err)
		}
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return false, fmt.Errorf("inspect blob prefix directory: %w", statErr)
	} else if err := s.syncDir(s.objectsDir); err != nil {
		return false, fmt.Errorf("sync blob algorithm directory: %w", err)
	}
	return false, nil
}

// Release ends a deletion epoch. It is safe to call more than once.
func (guard *DeletionGuard) Release() {
	if guard == nil || guard.store == nil {
		return
	}
	guard.mu.Lock()
	if !guard.done {
		guard.done = true
		guard.store.runtime.maintenance.releaseWrite()
	}
	guard.mu.Unlock()
}

// objectBarrier is a context-aware, writer-preferring RW barrier. A channel is
// replaced on each state transition so waiters can select on context without
// helper goroutines or lock leaks.
type objectBarrier struct {
	mu             sync.Mutex
	changed        chan struct{}
	readers        int
	writer         bool
	waitingWriters int
}

func newObjectBarrier() objectBarrier {
	return objectBarrier{changed: make(chan struct{})}
}

func (b *objectBarrier) acquireRead(ctx context.Context) error {
	for {
		b.mu.Lock()
		if !b.writer && b.waitingWriters == 0 {
			b.readers++
			b.mu.Unlock()
			return nil
		}
		changed := b.changed
		b.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (b *objectBarrier) releaseRead() {
	b.mu.Lock()
	if b.readers <= 0 {
		b.mu.Unlock()
		panic("blob: release of unacquired object pin")
	}
	b.readers--
	if b.readers == 0 {
		b.signalLocked()
	}
	b.mu.Unlock()
}

func (b *objectBarrier) acquireWrite(ctx context.Context) error {
	b.mu.Lock()
	b.waitingWriters++
	for {
		if !b.writer && b.readers == 0 {
			b.waitingWriters--
			b.writer = true
			b.mu.Unlock()
			return nil
		}
		changed := b.changed
		b.mu.Unlock()
		select {
		case <-ctx.Done():
			b.mu.Lock()
			b.waitingWriters--
			b.signalLocked()
			b.mu.Unlock()
			return ctx.Err()
		case <-changed:
			b.mu.Lock()
		}
	}
}

func (b *objectBarrier) releaseWrite() {
	b.mu.Lock()
	if !b.writer {
		b.mu.Unlock()
		panic("blob: release of inactive deletion guard")
	}
	b.writer = false
	b.signalLocked()
	b.mu.Unlock()
}

func (b *objectBarrier) signalLocked() {
	close(b.changed)
	b.changed = make(chan struct{})
}
