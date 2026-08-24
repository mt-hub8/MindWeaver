package blob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	idPrefix             = "sha256:"
	digestHexLength      = sha256.Size * 2
	copyBufferSize       = 64 * 1024
	maxEmptyReads        = 100
	stagingFilePrefix    = "import-"
	stagingReadBatchSize = 128
)

var (
	// ErrInvalidID reports a BlobID that is not in canonical SHA-256 form.
	ErrInvalidID = errors.New("invalid blob ID")
	// ErrInvalidLimit reports a negative import limit.
	ErrInvalidLimit = errors.New("invalid blob size limit")
	// ErrTooLarge reports input that exceeds the caller-supplied byte limit.
	ErrTooLarge = errors.New("blob exceeds size limit")
	// ErrCorrupt reports an object whose bytes do not match its content address.
	ErrCorrupt = errors.New("blob content does not match its ID")
)

// BlobID is a canonical, path-independent content address. Its string form is
// "sha256:" followed by exactly 64 lowercase hexadecimal characters.
type BlobID string

// ParseID validates and returns a canonical BlobID. Absolute paths, traversal,
// uppercase digests, and alternative spellings are rejected.
func ParseID(raw string) (BlobID, error) {
	if len(raw) != len(idPrefix)+digestHexLength || !strings.HasPrefix(raw, idPrefix) {
		return "", fmt.Errorf("%w: expected %s followed by %d lowercase hexadecimal characters", ErrInvalidID, idPrefix, digestHexLength)
	}

	digest := raw[len(idPrefix):]
	for _, c := range digest {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return "", fmt.Errorf("%w: digest must be lowercase hexadecimal", ErrInvalidID)
		}
	}

	return BlobID(raw), nil
}

func (id BlobID) String() string { return string(id) }

// ImportResult describes bytes already published in the store. Created is true
// only when this Store instance published the destination during this call.
// Another process may race to publish identical bytes, so callers must not use
// Created as a cross-process uniqueness decision.
type ImportResult struct {
	ID      BlobID
	Size    int64
	Created bool
}

// Store is safe for concurrent use within a process.
type Store struct {
	objectsDir string
	stagingDir string
	syncDir    func(string) error
	rename     func(string, string) error
	runtime    *storeRuntime
}

// storeRuntime is shared by every Store opened on the same physical root in
// this process. Store permits multiple handles for dedupe tests and helpers, so
// publication/cleanup/deletion coordination cannot live on one pointer.
type storeRuntime struct {
	maintenance objectBarrier
	objectLocks [256]sync.Mutex
	activeMu    sync.RWMutex
	active      map[string]struct{}
}

var runtimeRegistry struct {
	sync.Mutex
	entries []runtimeEntry
}

type runtimeEntry struct {
	canonical string
	rootInfo  os.FileInfo
	runtime   *storeRuntime
}

// OpenStore opens or creates the blob directories rooted at root. The root must
// be an explicit non-empty path. If root does not exist, its immediate parent
// must already exist as a real directory. Internal directories must be real
// directories, not symbolic links.
func OpenStore(root string) (*Store, error) {
	return openStore(root, syncDirectory, renamePublished)
}

func openStore(root string, syncDir func(string) error, rename func(string, string) error) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("blob store root must not be empty")
	}
	if syncDir == nil || rename == nil {
		return nil, errors.New("blob store filesystem operations must not be nil")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve blob store root: %w", err)
	}
	absRoot = filepath.Clean(absRoot)

	if err := ensureRootDirectory(absRoot, syncDir); err != nil {
		return nil, err
	}

	objectsDir := filepath.Join(absRoot, "objects")
	algorithmDir := filepath.Join(objectsDir, "sha256")
	stagingDir := filepath.Join(absRoot, "staging")
	for _, item := range []struct {
		path   string
		parent string
	}{
		{objectsDir, absRoot},
		{algorithmDir, objectsDir},
		{stagingDir, absRoot},
	} {
		if err := ensureDirectory(item.path, item.parent, syncDir); err != nil {
			return nil, err
		}
	}
	shared, err := runtimeForRoot(absRoot)
	if err != nil {
		return nil, err
	}

	return &Store{
		objectsDir: algorithmDir,
		stagingDir: stagingDir,
		syncDir:    syncDir,
		rename:     rename,
		runtime:    shared,
	}, nil
}

func runtimeForRoot(root string) (*storeRuntime, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect blob store identity: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("blob store identity is not a directory")
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve blob store identity: %w", err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return nil, fmt.Errorf("resolve absolute blob store identity: %w", err)
	}
	canonical = filepath.Clean(canonical)
	runtimeRegistry.Lock()
	defer runtimeRegistry.Unlock()
	for index := 0; index < len(runtimeRegistry.entries); {
		entry := runtimeRegistry.entries[index]
		// Compare physical identities only while the entry's original name still
		// resolves to that identity. This supports aliases, case-sensitive Windows
		// directories, and safe eviction after remove/recreate without relying on
		// lower-cased path strings.
		current, statErr := os.Stat(entry.canonical)
		if statErr != nil || !os.SameFile(current, entry.rootInfo) {
			runtimeRegistry.entries = append(runtimeRegistry.entries[:index], runtimeRegistry.entries[index+1:]...)
			continue
		}
		if os.SameFile(info, current) {
			return entry.runtime, nil
		}
		index++
	}
	shared := &storeRuntime{maintenance: newObjectBarrier(), active: make(map[string]struct{})}
	runtimeRegistry.entries = append(runtimeRegistry.entries, runtimeEntry{canonical: canonical, rootInfo: info, runtime: shared})
	return shared, nil
}

// Import copies at most maxBytes from src into the store. The hash is computed
// while streaming; input is never buffered in full. An exact maxBytes payload is
// accepted, including an empty payload when maxBytes is zero.
//
// Cancellation is checked before and after every source read, but it cannot
// interrupt a source whose Read method itself blocks forever. On an error before
// publication no object is published. An error returned after the atomic rename
// may leave an unreferenced object at its content address; retrying the same bytes
// is the recovery path.
func (s *Store) Import(ctx context.Context, src io.Reader, maxBytes int64) (ImportResult, error) {
	if s == nil {
		return ImportResult{}, errors.New("nil blob store")
	}
	if ctx == nil {
		return ImportResult{}, errors.New("nil context")
	}
	if src == nil {
		return ImportResult{}, errors.New("nil blob source")
	}
	if maxBytes < 0 {
		return ImportResult{}, ErrInvalidLimit
	}
	if err := ctx.Err(); err != nil {
		return ImportResult{}, err
	}

	temp, err := os.CreateTemp(s.stagingDir, stagingFilePrefix)
	if err != nil {
		return ImportResult{}, fmt.Errorf("create blob staging file: %w", err)
	}
	stagingPath := temp.Name()
	stagingName := filepath.Base(stagingPath)
	s.markActive(stagingName)
	defer s.unmarkActive(stagingName)

	closed := false
	abort := func(cause error) (ImportResult, error) {
		var cleanupErr error
		if !closed {
			cleanupErr = temp.Close()
			closed = true
		}
		if removeErr := os.Remove(stagingPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove blob staging file: %w", removeErr))
		}
		if syncErr := s.syncDir(s.stagingDir); syncErr != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("sync blob staging directory: %w", syncErr))
		}
		return ImportResult{}, errors.Join(cause, cleanupErr)
	}

	hasher := sha256.New()
	size, err := copyBounded(ctx, temp, hasher, src, maxBytes)
	if err != nil {
		return abort(err)
	}
	if err := ctx.Err(); err != nil {
		return abort(err)
	}
	if err := temp.Sync(); err != nil {
		return abort(fmt.Errorf("sync blob staging file: %w", err))
	}
	if err := temp.Close(); err != nil {
		closed = true
		return abort(fmt.Errorf("close blob staging file: %w", err))
	}
	closed = true
	if err := ctx.Err(); err != nil {
		return abort(err)
	}

	digest := hex.EncodeToString(hasher.Sum(nil))
	id := BlobID(idPrefix + digest)
	created, err := s.publish(stagingPath, digest, size)
	if err != nil {
		return abort(err)
	}

	// A deduplicated import still owns its staging file. A created import has
	// already renamed it, so Remove simply observes os.ErrNotExist.
	if _, cleanupErr := abort(nil); cleanupErr != nil {
		return ImportResult{}, cleanupErr
	}

	return ImportResult{ID: id, Size: size, Created: created}, nil
}

// Open opens an immutable object for reading. The caller must close the file.
// Open validates the address and file type but does not re-hash the complete file;
// callers that need an integrity audit should re-import or hash the returned data.
func (s *Store) Open(id BlobID) (*os.File, error) {
	if s == nil {
		return nil, errors.New("nil blob store")
	}
	path, _, err := s.objectPath(id)
	if err != nil {
		return nil, err
	}

	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect blob object: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("open blob object: %w: object is not a regular file", ErrCorrupt)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open blob object: %w", err)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect opened blob object: %w", err)
	}
	if !openedInfo.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("open blob object: %w: object changed type", ErrCorrupt)
	}
	return file, nil
}

// CleanupStaging removes regular staging files created by this package whose
// modification time is at or before cutoff. Active imports in this Store instance
// are skipped. CleanupStaging must run while the caller holds the Vault's exclusive
// process lock, normally during startup before imports are accepted. The directory
// is read in bounded batches; entries belonging to other producers are untouched.
func (s *Store) CleanupStaging(ctx context.Context, cutoff time.Time) (int, error) {
	if s == nil {
		return 0, errors.New("nil blob store")
	}
	if ctx == nil {
		return 0, errors.New("nil context")
	}
	directory, err := os.Open(s.stagingDir)
	if err != nil {
		return 0, fmt.Errorf("open blob staging directory: %w", err)
	}

	removed := 0
	finish := func(cause error) (int, error) {
		closeErr := directory.Close()
		if closeErr != nil {
			closeErr = fmt.Errorf("close blob staging directory: %w", closeErr)
		}
		return removed, syncAfterCleanup(s.stagingDir, removed, errors.Join(cause, closeErr), s.syncDir)
	}

	for {
		if err := ctx.Err(); err != nil {
			return finish(err)
		}
		entries, readErr := directory.ReadDir(stagingReadBatchSize)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return finish(err)
			}
			name := entry.Name()
			if !strings.HasPrefix(name, stagingFilePrefix) || s.isActive(name) {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return finish(fmt.Errorf("inspect blob staging entry: %w", err))
			}
			if !info.Mode().IsRegular() || info.ModTime().After(cutoff) {
				continue
			}
			if err := os.Remove(filepath.Join(s.stagingDir, name)); err != nil {
				return finish(fmt.Errorf("remove stale blob staging file: %w", err))
			}
			removed++
		}
		if errors.Is(readErr, io.EOF) {
			return finish(nil)
		}
		if readErr != nil {
			return finish(fmt.Errorf("read blob staging directory: %w", readErr))
		}
	}
}

func (s *Store) publish(stagingPath, digest string, size int64) (bool, error) {
	lock := s.lockForDigest(digest)
	lock.Lock()
	defer lock.Unlock()

	prefixDir := filepath.Join(s.objectsDir, digest[:2])
	if err := ensureDirectory(prefixDir, s.objectsDir, s.syncDir); err != nil {
		return false, err
	}
	destination := filepath.Join(prefixDir, digest[2:])

	if _, err := os.Lstat(destination); err == nil {
		if err := verifyObject(destination, digest, size); err != nil {
			return false, err
		}
		if err := s.syncDir(prefixDir); err != nil {
			return false, fmt.Errorf("sync existing blob object directory: %w", err)
		}
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect blob destination: %w", err)
	}

	if err := s.rename(stagingPath, destination); err != nil {
		// A second process may have published identical bytes between Lstat and
		// rename. Accept that race only after verifying the complete object.
		if _, statErr := os.Lstat(destination); statErr == nil {
			if verifyErr := verifyObject(destination, digest, size); verifyErr != nil {
				return false, errors.Join(fmt.Errorf("publish blob: %w", err), verifyErr)
			}
			if syncErr := s.syncDir(prefixDir); syncErr != nil {
				return false, errors.Join(
					fmt.Errorf("publish blob: %w", err),
					fmt.Errorf("sync concurrently published blob directory: %w", syncErr),
				)
			}
			return false, nil
		}
		return false, fmt.Errorf("publish blob: %w", err)
	}

	if err := s.syncDir(prefixDir); err != nil {
		return false, fmt.Errorf("sync blob object directory: %w", err)
	}
	// Failure to persist removal of the staging name is not a loss of the
	// destination object. CleanupStaging can remove a resurrected stale name.
	_ = s.syncDir(s.stagingDir)
	return true, nil
}

func (s *Store) objectPath(id BlobID) (path string, digest string, err error) {
	valid, err := ParseID(id.String())
	if err != nil {
		return "", "", err
	}
	digest = valid.String()[len(idPrefix):]
	return filepath.Join(s.objectsDir, digest[:2], digest[2:]), digest, nil
}

func (s *Store) lockForDigest(digest string) *sync.Mutex {
	first, _ := hex.DecodeString(digest[:2])
	return &s.runtime.objectLocks[int(first[0])]
}

func (s *Store) markActive(name string) {
	s.runtime.activeMu.Lock()
	s.runtime.active[name] = struct{}{}
	s.runtime.activeMu.Unlock()
}

func (s *Store) unmarkActive(name string) {
	s.runtime.activeMu.Lock()
	delete(s.runtime.active, name)
	s.runtime.activeMu.Unlock()
}

func (s *Store) isActive(name string) bool {
	s.runtime.activeMu.RLock()
	_, ok := s.runtime.active[name]
	s.runtime.activeMu.RUnlock()
	return ok
}

func copyBounded(ctx context.Context, dst *os.File, digest hash.Hash, src io.Reader, limit int64) (int64, error) {
	buffer := make([]byte, copyBufferSize)
	var total int64
	emptyReads := 0

	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}

		readSize := len(buffer)
		remaining := limit - total
		if remaining < int64(readSize) {
			readSize = int(remaining) + 1
		}
		n, readErr := src.Read(buffer[:readSize])
		if n < 0 || n > readSize {
			return total, fmt.Errorf("copy blob source: invalid Read count %d", n)
		}
		if err := ctx.Err(); err != nil {
			return total, err
		}
		if int64(n) > remaining {
			return total, ErrTooLarge
		}
		if n > 0 {
			emptyReads = 0
			if err := writeFull(dst, buffer[:n]); err != nil {
				return total, fmt.Errorf("write blob staging file: %w", err)
			}
			_, _ = digest.Write(buffer[:n])
			total += int64(n)
		} else if readErr == nil {
			emptyReads++
			if emptyReads >= maxEmptyReads {
				return total, io.ErrNoProgress
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return total, nil
			}
			return total, fmt.Errorf("read blob source: %w", readErr)
		}
	}
}

func writeFull(dst *os.File, data []byte) error {
	for len(data) > 0 {
		n, err := dst.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func verifyObject(path, digest string, expectedSize int64) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect existing blob: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != expectedSize {
		return fmt.Errorf("%w: existing object has unexpected type or size", ErrCorrupt)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open existing blob for verification: %w", err)
	}
	hasher := sha256.New()
	_, copyErr := io.CopyBuffer(hasher, file, make([]byte, copyBufferSize))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		if copyErr != nil {
			copyErr = fmt.Errorf("verify existing blob: %w", copyErr)
		}
		return errors.Join(copyErr, closeErr)
	}
	if hex.EncodeToString(hasher.Sum(nil)) != digest {
		return fmt.Errorf("%w: existing object hash differs", ErrCorrupt)
	}
	return nil
}

func ensureRootDirectory(path string, syncDir func(string) error) error {
	parent := filepath.Dir(path)
	_, statErr := os.Lstat(path)
	if errors.Is(statErr, os.ErrNotExist) {
		if err := requireRealDirectory(parent); err != nil {
			return fmt.Errorf("blob store root parent must already exist: %w", err)
		}
		if err := os.Mkdir(path, 0o700); err != nil {
			return fmt.Errorf("create blob store root: %w", err)
		}
	} else if statErr != nil {
		return fmt.Errorf("inspect blob store root: %w", statErr)
	}
	if err := requireRealDirectory(path); err != nil {
		return err
	}
	// Always repeat the parent sync. A previous attempt may have created root
	// and then failed its flush; observing root on retry is not durability proof.
	if err := syncDir(parent); err != nil {
		return fmt.Errorf("sync blob store parent directory: %w", err)
	}
	return nil
}

func ensureDirectory(path, parent string, syncDir func(string) error) error {
	err := os.Mkdir(path, 0o700)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create blob directory: %w", err)
	}
	if err := requireRealDirectory(path); err != nil {
		return err
	}
	// Sync even when the child already exists so retry closes a prior
	// mkdir-success/parent-sync-failure outcome.
	if err := syncDir(parent); err != nil {
		return fmt.Errorf("sync parent for blob directory: %w", err)
	}
	return nil
}

func requireRealDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect blob directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("blob path is not a real directory: %s", filepath.Base(path))
	}
	return nil
}

func syncAfterCleanup(directory string, removed int, cause error, syncDir func(string) error) error {
	if removed == 0 {
		return cause
	}
	if err := syncDir(directory); err != nil {
		return errors.Join(cause, fmt.Errorf("sync blob staging directory after cleanup: %w", err))
	}
	return cause
}
