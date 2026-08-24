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
	// ErrPreparedFinalized reports an attempt to publish staging bytes that were
	// already published or aborted. A PreparedImport is a one-shot capability.
	ErrPreparedFinalized = errors.New("prepared blob is already finalized")
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
// False also covers a fully verified post-rename recovery whose creating actor
// cannot be proven. Callers must not use Created as a cross-process uniqueness
// decision.
type ImportResult struct {
	ID      BlobID
	Size    int64
	Created bool
}

type preparedState uint8

const (
	preparedOpen preparedState = iota
	preparedPublished
	preparedAborted
)

// PreparedImport owns one bounded, synced staging file. Its bytes are not
// visible through Open until Publish succeeds. Callers must either Publish or
// Abort it. The unexported implementation prevents copying or rebinding the
// capability; its state transition is concurrency-safe and deliberately
// one-shot.
type PreparedImport interface {
	ID() BlobID
	Size() int64
	Publish(context.Context) (ImportResult, error)
	Abort() error
}

type preparedImport struct {
	mu          sync.Mutex
	store       *Store
	file        *os.File
	original    os.FileInfo
	stagingPath string
	stagingName string
	digest      string
	id          BlobID
	size        int64
	state       preparedState
	active      bool
	sourceMoved bool
}

// ID returns the immutable content address computed by Prepare.
func (prepared *preparedImport) ID() BlobID {
	if prepared == nil {
		return ""
	}
	return prepared.id
}

// Size returns the exact number of bytes durably written by Prepare.
func (prepared *preparedImport) Size() int64 {
	if prepared == nil {
		return 0
	}
	return prepared.size
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

// Prepare copies at most maxBytes into a private staging file. The hash is
// computed while streaming; input is never buffered in full. The file and its
// directory entry are synced before this method returns, but the content address
// is not yet visible through Open.
//
// Cancellation is checked before and after every source read, but it cannot
// interrupt a source whose Read method itself blocks forever. Every failure is
// followed by a best-effort Abort and reports any cleanup failure as well.
func (s *Store) Prepare(ctx context.Context, src io.Reader, maxBytes int64) (PreparedImport, error) {
	if s == nil {
		return nil, errors.New("nil blob store")
	}
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if src == nil {
		return nil, errors.New("nil blob source")
	}
	if maxBytes < 0 {
		return nil, ErrInvalidLimit
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	temp, err := createPreparedTemp(s.stagingDir, stagingFilePrefix)
	if err != nil {
		return nil, fmt.Errorf("create blob staging file: %w", err)
	}
	prepared := &preparedImport{
		store:       s,
		file:        temp,
		stagingPath: temp.Name(),
		stagingName: filepath.Base(temp.Name()),
		state:       preparedOpen,
	}
	initial, err := temp.Stat()
	if err != nil {
		cleanupErr := s.discardUninspectedPrepared(temp)
		return nil, errors.Join(fmt.Errorf("inspect new blob staging identity: %w", err), cleanupErr)
	}
	prepared.original = initial
	s.markActive(prepared.stagingName)
	prepared.active = true
	abort := func(cause error) (PreparedImport, error) {
		return nil, errors.Join(cause, prepared.Abort())
	}
	if !initial.Mode().IsRegular() {
		return abort(fmt.Errorf("%w: new blob staging file is not regular", ErrCorrupt))
	}
	if err := verifyFilePlatformInvariant(temp); err != nil {
		return abort(fmt.Errorf("inspect new blob staging invariant: %w", err))
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
	if err := ctx.Err(); err != nil {
		return abort(err)
	}
	original, err := temp.Stat()
	if err != nil {
		return abort(fmt.Errorf("inspect prepared blob identity: %w", err))
	}
	if !original.Mode().IsRegular() || original.Size() != size {
		return abort(fmt.Errorf("%w: prepared blob has unexpected type or size", ErrCorrupt))
	}
	if err := verifyFilePlatformInvariant(temp); err != nil {
		return abort(fmt.Errorf("inspect prepared blob invariant: %w", err))
	}
	prepared.original = original
	if err := s.syncDir(s.stagingDir); err != nil {
		return abort(fmt.Errorf("sync blob staging directory: %w", err))
	}

	prepared.digest = hex.EncodeToString(hasher.Sum(nil))
	prepared.id = BlobID(idPrefix + prepared.digest)
	prepared.size = size
	return prepared, nil
}

func (s *Store) discardUninspectedPrepared(file *os.File) error {
	if file == nil {
		return nil
	}
	_, removeErr := removeUninspectedPreparedIdentity(file, file.Name())
	if removeErr != nil {
		removeErr = fmt.Errorf("remove uninspected blob staging identity: %w", removeErr)
	}
	closeErr := file.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close uninspected blob staging identity: %w", closeErr)
	}
	syncErr := s.syncDir(s.stagingDir)
	if syncErr != nil {
		syncErr = fmt.Errorf("sync blob staging directory after failed inspection: %w", syncErr)
	}
	return errors.Join(removeErr, closeErr, syncErr)
}

// Publish makes the prepared bytes visible at their content address using the
// Store's no-replace publication and deduplication path. Exactly one call may
// attempt publication. A returned error can follow the atomic rename, so a
// database-backed caller must queue the address for reference-aware GC first.
func (prepared *preparedImport) Publish(ctx context.Context) (ImportResult, error) {
	if prepared == nil || prepared.store == nil {
		return ImportResult{}, errors.New("nil prepared blob")
	}
	if ctx == nil {
		return ImportResult{}, errors.New("nil context")
	}
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if prepared.state != preparedOpen {
		return ImportResult{}, ErrPreparedFinalized
	}
	if err := ctx.Err(); err != nil {
		return ImportResult{}, err
	}

	created, publishErr := prepared.store.publish(prepared)
	prepared.state = preparedPublished
	cleanupErr := prepared.cleanupStagingLocked()
	prepared.finishActiveLocked()
	if err := errors.Join(publishErr, cleanupErr); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{ID: prepared.id, Size: prepared.size, Created: created}, nil
}

// Abort permanently revokes publication and removes only this capability's
// staging file. It is safe and idempotent, including after Publish; it never
// removes a published content-addressed object.
func (prepared *preparedImport) Abort() error {
	if prepared == nil {
		return nil
	}
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if prepared.store == nil {
		return nil
	}
	if prepared.state == preparedOpen {
		prepared.state = preparedAborted
	}
	err := prepared.cleanupStagingLocked()
	prepared.finishActiveLocked()
	return err
}

func (prepared *preparedImport) cleanupStagingLocked() error {
	if prepared.file == nil {
		return nil
	}
	var cleanupErr error
	if !prepared.sourceMoved {
		if _, removeErr := removePreparedIdentity(prepared.file, prepared.stagingPath, prepared.original); removeErr != nil {
			cleanupErr = fmt.Errorf("remove blob staging identity: %w", removeErr)
		}
	}
	cleanupErr = errors.Join(cleanupErr, prepared.file.Close())
	prepared.file = nil
	if syncErr := prepared.store.syncDir(prepared.store.stagingDir); syncErr != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("sync blob staging directory: %w", syncErr))
	}
	prepared.stagingPath = ""
	return cleanupErr
}

func (prepared *preparedImport) finishActiveLocked() {
	if !prepared.active {
		return
	}
	prepared.store.unmarkActive(prepared.stagingName)
	prepared.active = false
}

// Import prepares and immediately publishes a blob through the same one-shot
// implementation used by transactional callers. An exact maxBytes payload is
// accepted, including an empty payload when maxBytes is zero. An error returned
// after the atomic rename may still leave an unreferenced object; callers that
// need crash-closed reference creation must queue a GC candidate before Publish.
func (s *Store) Import(ctx context.Context, src io.Reader, maxBytes int64) (ImportResult, error) {
	prepared, err := s.Prepare(ctx, src, maxBytes)
	if err != nil {
		return ImportResult{}, err
	}
	result, err := prepared.Publish(ctx)
	if err != nil {
		return ImportResult{}, errors.Join(err, prepared.Abort())
	}
	return result, nil
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

func (s *Store) publish(prepared *preparedImport) (bool, error) {
	lock := s.lockForDigest(prepared.digest)
	lock.Lock()
	defer lock.Unlock()
	if err := verifyPreparedSource(prepared); err != nil {
		return false, err
	}

	prefixDir := filepath.Join(s.objectsDir, prepared.digest[:2])
	if err := ensureDirectory(prefixDir, s.objectsDir, s.syncDir); err != nil {
		return false, err
	}
	destination := filepath.Join(prefixDir, prepared.digest[2:])

	if _, err := os.Lstat(destination); err == nil {
		if err := s.syncDir(prefixDir); err != nil {
			return false, fmt.Errorf("sync existing blob object directory: %w", err)
		}
		if _, err := verifyObject(destination, prepared.digest, prepared.size); err != nil {
			return false, err
		}
		if err := verifyPreparedSourceIdentity(prepared); err != nil {
			return false, fmt.Errorf("reverify deduplicated blob staging source: %w", err)
		}
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect blob destination: %w", err)
	}

	if err := s.rename(prepared.stagingPath, destination); err != nil {
		// A second process may have published identical bytes between Lstat and
		// rename. Accept that race only after syncing and verifying the complete
		// destination. If the destination is our retained identity, the source
		// name must also have disappeared (or name a different identity) and the
		// file must still have exactly one link. Otherwise a hard link could be
		// mistaken for a completed rename.
		destinationState, stateErr := inspectPathIdentity(destination, prepared.original)
		if stateErr != nil {
			return false, errors.Join(fmt.Errorf("publish blob: %w", err), stateErr)
		}
		if destinationState == pathIdentityAbsent {
			return false, fmt.Errorf("publish blob: %w", err)
		}

		var movedErr error
		if destinationState == pathIdentitySame {
			movedErr = provePreparedIdentityMoved(prepared, destination)
			if movedErr == nil {
				prepared.sourceMoved = true
			}
		}
		syncErr := s.syncDir(prefixDir)
		if syncErr != nil {
			syncErr = fmt.Errorf("sync concurrently published blob directory: %w", syncErr)
		}
		if movedErr != nil || syncErr != nil {
			return false, errors.Join(fmt.Errorf("publish blob: %w", err), movedErr, syncErr)
		}
		if prepared.sourceMoved {
			if verifyErr := verifyPublishedPrepared(prepared, destination); verifyErr != nil {
				return false, errors.Join(fmt.Errorf("publish blob: %w", err), verifyErr)
			}
			if closeErr := prepared.closeMovedSource(); closeErr != nil {
				return false, errors.Join(fmt.Errorf("publish blob: %w", err), closeErr)
			}
		} else {
			if _, verifyErr := verifyObject(destination, prepared.digest, prepared.size); verifyErr != nil {
				return false, errors.Join(fmt.Errorf("publish blob: %w", err), verifyErr)
			}
			if sourceErr := verifyPreparedSourceIdentity(prepared); sourceErr != nil {
				return false, errors.Join(fmt.Errorf("publish blob: %w", err), sourceErr)
			}
		}
		// The bytes are durably usable, but a rename that returned an error
		// cannot prove which actor created the destination.
		return false, nil
	}
	// A successful no-replace rename is authoritative even if a subsequent
	// durability or verification step fails. Keep the destination for the
	// already-queued reference-aware GC candidate instead of deleting it as
	// staging during cleanup.
	prepared.sourceMoved = true

	if err := s.syncDir(prefixDir); err != nil {
		return false, fmt.Errorf("sync blob object directory: %w", err)
	}
	if err := verifyPublishedPrepared(prepared, destination); err != nil {
		return false, fmt.Errorf("verify published blob: %w", err)
	}
	if err := prepared.closeMovedSource(); err != nil {
		return false, err
	}
	return true, nil
}

func verifyPreparedSource(prepared *preparedImport) error {
	if err := verifyPreparedSourceIdentity(prepared); err != nil {
		return err
	}
	if err := verifyOpenFile(prepared.file, prepared.digest, prepared.size); err != nil {
		return fmt.Errorf("verify prepared blob handle: %w", err)
	}
	if err := verifyPreparedSourceIdentity(prepared); err != nil {
		return fmt.Errorf("reinspect prepared blob after verification: %w", err)
	}
	return nil
}

func verifyPreparedSourceIdentity(prepared *preparedImport) error {
	if prepared == nil || prepared.file == nil || prepared.original == nil {
		return fmt.Errorf("%w: prepared blob identity is unavailable", ErrCorrupt)
	}
	current, err := prepared.file.Stat()
	if err != nil {
		return fmt.Errorf("inspect prepared blob handle: %w", err)
	}
	if !current.Mode().IsRegular() || current.Size() != prepared.size || !os.SameFile(current, prepared.original) {
		return fmt.Errorf("%w: prepared blob identity or size changed", ErrCorrupt)
	}
	if err := verifyFilePlatformInvariant(prepared.file); err != nil {
		return err
	}
	state, err := inspectPathIdentity(prepared.stagingPath, current)
	if err != nil {
		return fmt.Errorf("inspect prepared blob staging name: %w", err)
	}
	if state != pathIdentitySame {
		return fmt.Errorf("%w: prepared blob staging name was replaced", ErrCorrupt)
	}
	return nil
}

type pathIdentityState uint8

const (
	pathIdentityAbsent pathIdentityState = iota
	pathIdentitySame
	pathIdentityDifferent
)

func inspectPathIdentity(path string, expected os.FileInfo) (pathIdentityState, error) {
	if expected == nil {
		return pathIdentityDifferent, fmt.Errorf("%w: expected blob identity is unavailable", ErrCorrupt)
	}
	entry, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return pathIdentityAbsent, nil
	}
	if err != nil {
		return pathIdentityDifferent, fmt.Errorf("inspect blob path identity: %w", err)
	}
	if entry.Mode().IsRegular() && os.SameFile(entry, expected) {
		return pathIdentitySame, nil
	}
	return pathIdentityDifferent, nil
}

func provePreparedIdentityMoved(prepared *preparedImport, destination string) error {
	if prepared == nil || prepared.file == nil || prepared.original == nil {
		return fmt.Errorf("%w: moved blob identity is unavailable", ErrCorrupt)
	}
	current, err := prepared.file.Stat()
	if err != nil {
		return fmt.Errorf("inspect possibly moved blob handle: %w", err)
	}
	if !current.Mode().IsRegular() || current.Size() != prepared.size || !os.SameFile(prepared.original, current) {
		return fmt.Errorf("%w: possibly moved blob identity or size differs", ErrCorrupt)
	}
	if err := verifyFilePlatformInvariant(prepared.file); err != nil {
		return err
	}
	destinationState, err := inspectPathIdentity(destination, current)
	if err != nil {
		return err
	}
	if destinationState != pathIdentitySame {
		return fmt.Errorf("%w: destination does not name the retained blob identity", ErrCorrupt)
	}
	stagingState, err := inspectPathIdentity(prepared.stagingPath, current)
	if err != nil {
		return err
	}
	if stagingState == pathIdentitySame {
		return fmt.Errorf("%w: staging and destination are hard links to one blob identity", ErrCorrupt)
	}
	return nil
}

func verifyPublishedPrepared(prepared *preparedImport, destination string) error {
	current, err := prepared.file.Stat()
	if err != nil {
		return fmt.Errorf("inspect published blob handle: %w", err)
	}
	if !current.Mode().IsRegular() || current.Size() != prepared.size || !os.SameFile(prepared.original, current) {
		return fmt.Errorf("%w: published destination identity differs", ErrCorrupt)
	}
	if err := verifyFilePlatformInvariant(prepared.file); err != nil {
		return err
	}
	if err := provePublishedNames(prepared, destination, current); err != nil {
		return err
	}
	if err := verifyOpenFile(prepared.file, prepared.digest, prepared.size); err != nil {
		return err
	}
	current, err = prepared.file.Stat()
	if err != nil {
		return fmt.Errorf("reinspect published blob handle: %w", err)
	}
	if !current.Mode().IsRegular() || current.Size() != prepared.size || !os.SameFile(prepared.original, current) {
		return fmt.Errorf("%w: published destination changed during verification", ErrCorrupt)
	}
	if err := verifyFilePlatformInvariant(prepared.file); err != nil {
		return err
	}
	if err := provePublishedNames(prepared, destination, current); err != nil {
		return err
	}
	return nil
}

func provePublishedNames(prepared *preparedImport, destination string, current os.FileInfo) error {
	destinationState, err := inspectPathIdentity(destination, current)
	if err != nil {
		return fmt.Errorf("inspect published destination identity: %w", err)
	}
	if destinationState != pathIdentitySame {
		return fmt.Errorf("%w: published destination does not name the retained identity", ErrCorrupt)
	}
	stagingState, err := inspectPathIdentity(prepared.stagingPath, current)
	if err != nil {
		return fmt.Errorf("inspect former staging identity: %w", err)
	}
	if stagingState == pathIdentitySame {
		return fmt.Errorf("%w: published blob still has its staging hard link", ErrCorrupt)
	}
	return nil
}

// closeMovedSource releases the DELETE-capable identity handle before the
// per-digest publication lock is released. This lets a serialized dedupe
// verifier open the destination without weakening Windows sharing semantics.
func (prepared *preparedImport) closeMovedSource() error {
	if prepared.file == nil || !prepared.sourceMoved {
		return nil
	}
	closeErr := prepared.file.Close()
	prepared.file = nil
	syncErr := prepared.store.syncDir(prepared.store.stagingDir)
	prepared.stagingPath = ""
	if closeErr != nil {
		closeErr = fmt.Errorf("close published blob identity: %w", closeErr)
	}
	if syncErr != nil {
		syncErr = fmt.Errorf("sync blob staging directory after publication: %w", syncErr)
	}
	return errors.Join(closeErr, syncErr)
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

func verifyObject(path, digest string, expectedSize int64) (os.FileInfo, error) {
	entry, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect existing blob: %w", err)
	}
	if !entry.Mode().IsRegular() || entry.Size() != expectedSize {
		return nil, fmt.Errorf("%w: existing object has unexpected type or size", ErrCorrupt)
	}

	file, err := openObjectForVerification(path)
	if err != nil {
		return nil, fmt.Errorf("open existing blob for verification: %w", err)
	}
	opened, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect opened blob for verification: %w", statErr)
	}
	if !opened.Mode().IsRegular() || opened.Size() != expectedSize || !os.SameFile(entry, opened) {
		_ = file.Close()
		return nil, fmt.Errorf("%w: existing object identity changed", ErrCorrupt)
	}
	if err := verifyFilePlatformInvariant(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	verifyErr := verifyOpenFile(file, digest, expectedSize)
	openedAfter, openedAfterErr := file.Stat()
	if openedAfterErr == nil {
		if !openedAfter.Mode().IsRegular() || openedAfter.Size() != expectedSize || !os.SameFile(opened, openedAfter) {
			openedAfterErr = fmt.Errorf("%w: existing object handle changed during verification", ErrCorrupt)
		} else if invariantErr := verifyFilePlatformInvariant(file); invariantErr != nil {
			openedAfterErr = invariantErr
		}
	} else {
		openedAfterErr = fmt.Errorf("reinspect opened blob handle: %w", openedAfterErr)
	}
	after, afterErr := os.Lstat(path)
	closeErr := file.Close()
	if verifyErr != nil || openedAfterErr != nil || afterErr != nil || closeErr != nil {
		if afterErr != nil {
			afterErr = fmt.Errorf("reinspect verified blob path: %w", afterErr)
		}
		return nil, errors.Join(verifyErr, openedAfterErr, afterErr, closeErr)
	}
	if !after.Mode().IsRegular() || !os.SameFile(opened, after) {
		return nil, fmt.Errorf("%w: verified object path was replaced", ErrCorrupt)
	}
	return openedAfter, nil
}

func verifyOpenFile(file *os.File, digest string, expectedSize int64) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek blob for verification: %w", err)
	}
	hasher := sha256.New()
	size, copyErr := io.CopyBuffer(hasher, file, make([]byte, copyBufferSize))
	if copyErr != nil {
		return fmt.Errorf("verify blob bytes: %w", copyErr)
	}
	if size != expectedSize || hex.EncodeToString(hasher.Sum(nil)) != digest {
		return fmt.Errorf("%w: blob size or hash differs", ErrCorrupt)
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
