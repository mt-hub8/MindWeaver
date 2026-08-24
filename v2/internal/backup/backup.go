// Package backup creates and restores complete, self-verifying local Vault
// backups. A backup is a directory with one SQLite snapshot, only the immutable
// blobs referenced by that snapshot, and a bounded versioned manifest.
package backup

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
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
	"github.com/ncruces/go-sqlite3"
	sqliteDriver "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
)

const (
	FormatVersion         = 1
	manifestFileName      = "manifest.json"
	databasePath          = "data/" + store.DatabaseFileName
	vaultLockFileName     = ".mindweaver.lock"
	maxManifestBytes      = 16 << 20
	maxArtifactBytes      = int64(1 << 40)
	maxBackupBytes        = int64(8 << 40)
	maxBackupArtifacts    = 40_000
	maxManifestJSONDepth  = 64
	maxManifestJSONTokens = 2_000_000
	directoryReadBatch    = 128
	maxExactTreeEntries   = 120_000
	maxExactTreeDepth     = 128
	maxCleanupDepth       = 128
	copyBufferBytes       = 128 << 10
	stagingPrefix         = ".mindweaver-backup-"
	restorePrefix         = ".mindweaver-restore-"
)

var (
	ErrCleanupIdentityLost = errors.New("backup: cleanup target identity lost")
	ErrCleanupResidual     = errors.New("backup: cleanup retained staging residue")
	ErrFilesystemBoundary  = errors.New("backup: unsupported filesystem boundary")
	// ErrUnsupportedPlatform means the requested operation needs a filesystem
	// mutation guarantee that is qualified only on the Windows Tier-1 runtime.
	ErrUnsupportedPlatform = errors.New("backup: operation is unsupported on this platform")
	ErrActiveVaultOverlap  = errors.New("backup: destination overlaps the active Vault")
	ErrRestoreOverlap      = errors.New("backup: restore destination overlaps its backup source")
)

// Artifact describes one exact regular file inside a backup.
type Artifact struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	BlobID string `json:"blob_id,omitempty"`
}

// Manifest is the complete portable backup contract. Blobs are sorted by ID.
type Manifest struct {
	FormatVersion       int        `json:"format_version"`
	SchemaVersion       int        `json:"schema_version"`
	CreatedAtUnixMicros int64      `json:"created_at_unix_micros"`
	Database            Artifact   `json:"database"`
	Blobs               []Artifact `json:"blobs"`
}

// Coordinator owns the live resources needed to create a consistent backup.
type Coordinator struct {
	mu          sync.RWMutex
	database    *store.Store
	blobs       *blob.Store
	activeVault *retainedDirectory
	now         func() time.Time
}

// retainedDirectory turns a validated path into an object-capability boundary.
// root is always the read/identity capability. syncHandle is nil unless a
// mutating destination, staging, sync, or publication path explicitly upgrades
// the capability and verifies that both handles retain the same directory.
// path remains only for APIs that cannot accept an os.Root.
type retainedDirectory struct {
	path       string
	root       *os.Root
	syncHandle *os.File
	identity   directoryIdentity
}

type directoryIdentity struct{ info os.FileInfo }

type destinationTarget struct {
	finalPath string
	finalName string
	parent    *retainedDirectory
}

type stagingDirectory struct {
	name            string
	directory       *retainedDirectory
	creationWitness *os.File
	operationID     string
	revision        string
	kind            string
	destinationName string
	parentIdentity  string
	identityToken   string
	receiptName     string
	receiptIdentity os.FileInfo
	receiptLease    *os.File
	bindingName     string
	bindingIdentity os.FileInfo
}

type publicationResult struct{ stagingConsumed bool }

type publicationHooks struct {
	afterOverlapCheck                         func(*destinationTarget) error
	afterCreateQualificationBeforeCommitment  func(string) error
	beforeRestoreQualification                func(string) error
	afterRestoreQualificationBeforeCommitment func(string) error
	afterStagingCloseBeforeRename             func(string) error
	syncParent                                func(*retainedDirectory) error
	afterSyncFailure                          func() error
}

type publicationVerifier func(context.Context, *retainedDirectory) error

type publicationAttempt struct {
	sourceName string
	expected   directoryIdentity
	ctx        context.Context
	verify     publicationVerifier
	hooks      publicationHooks
}

type restoredVaultCommitment struct {
	schemaVersion int
	database      Artifact
	blobs         []Artifact
}

// New constructs a concrete Vault backup coordinator.
func New(database *store.Store, blobs *blob.Store, activeVaultRoot string) (*Coordinator, error) {
	if database == nil {
		return nil, errors.New("backup: nil database")
	}
	if blobs == nil {
		return nil, errors.New("backup: nil blob store")
	}
	active, err := openRetainedDirectory(activeVaultRoot)
	if err != nil {
		return nil, fmt.Errorf("backup: retain active Vault identity: %w", err)
	}
	return &Coordinator{
		database: database, blobs: blobs,
		activeVault: active, now: time.Now,
	}, nil
}

// Close releases the retained active-Vault identity capability. It waits for
// in-flight Create or Restore operations and must be called by the owner.
func (c *Coordinator) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.activeVault == nil {
		return nil
	}
	active := c.activeVault
	c.activeVault = nil
	return active.Close()
}

func (c *Coordinator) rejectActiveVaultOverlap(target *destinationTarget) error {
	if c == nil || c.activeVault == nil || c.activeVault.root == nil || c.activeVault.identity.info == nil {
		return errors.New("backup: active Vault identity is unavailable")
	}
	if target == nil || target.parent == nil || target.parent.root == nil || target.finalName == "" {
		return errors.New("backup: destination capability is unavailable")
	}
	if err := c.activeVault.verifyPath(); err != nil {
		return errors.Join(
			ErrActiveVaultOverlap,
			fmt.Errorf("backup: active Vault identity changed since coordinator creation: %w", err),
		)
	}
	if err := visitRetainedAncestorsFromWitness(target.parent, func(directory *retainedDirectory) error {
		if os.SameFile(directory.identity.info, c.activeVault.identity.info) {
			return ErrActiveVaultOverlap
		}
		return nil
	}); err != nil {
		return fmt.Errorf("backup: inspect destination ancestors: %w", err)
	}
	finalInfo, statErr := target.parent.root.Lstat(target.finalName)
	if errors.Is(statErr, os.ErrNotExist) {
		return nil
	}
	if statErr != nil {
		return fmt.Errorf("backup: inspect destination for Vault overlap: %w", statErr)
	}
	if finalInfo.Mode()&os.ModeSymlink != 0 || !finalInfo.IsDir() {
		return nil
	}
	finalDirectory, err := openExpectedDirectory(target.parent, target.finalName, directoryIdentity{info: finalInfo})
	if err != nil {
		return fmt.Errorf("backup: validate existing destination for Vault overlap: %w", err)
	}
	finalIdentity := finalDirectory.identity.info
	if err := finalDirectory.Close(); err != nil {
		return err
	}
	if os.SameFile(finalIdentity, c.activeVault.identity.info) {
		return ErrActiveVaultOverlap
	}
	if err := visitRetainedAncestors(c.activeVault.path, func(directory *retainedDirectory) error {
		if os.SameFile(directory.identity.info, finalIdentity) {
			return ErrActiveVaultOverlap
		}
		return nil
	}); err != nil {
		return fmt.Errorf("backup: inspect active Vault ancestors: %w", err)
	}
	return nil
}

func visitRetainedAncestors(start string, visit func(*retainedDirectory) error) error {
	if visit == nil {
		return errors.New("backup: nil directory ancestor visitor")
	}
	current, err := filepath.Abs(start)
	if err != nil {
		return err
	}
	current = filepath.Clean(current)
	for depth := 0; depth < maxExactTreeDepth; depth++ {
		directory, err := openRetainedDirectory(current)
		if err != nil {
			return err
		}
		visitErr := visit(directory)
		closeErr := directory.Close()
		if err := errors.Join(visitErr, closeErr); err != nil {
			return err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
	return errors.New("backup: directory ancestor depth exceeds limit")
}

func visitRetainedAncestorsFromWitness(witness *retainedDirectory, visit func(*retainedDirectory) error) error {
	if witness == nil || witness.root == nil || witness.identity.info == nil || visit == nil {
		return errors.New("backup: invalid retained ancestor witness")
	}
	if err := visit(witness); err != nil {
		return err
	}
	if err := witness.verifyPath(); err != nil {
		return err
	}
	if err := visitRetainedAncestors(filepath.Dir(witness.path), visit); err != nil {
		return err
	}
	return witness.verifyPath()
}

func rejectRestoreDestinationOverlap(source *retainedDirectory, target *destinationTarget) error {
	if source == nil || source.root == nil || source.identity.info == nil ||
		target == nil || target.parent == nil || target.parent.root == nil {
		return errors.New("backup: invalid restore overlap capabilities")
	}
	if err := visitRetainedAncestorsFromWitness(target.parent, func(directory *retainedDirectory) error {
		if os.SameFile(directory.identity.info, source.identity.info) {
			return ErrRestoreOverlap
		}
		return nil
	}); err != nil {
		return err
	}
	finalInfo, err := target.parent.root.Lstat(target.finalName)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if finalInfo.Mode()&os.ModeSymlink != 0 || !finalInfo.IsDir() {
		return nil
	}
	finalDirectory, err := openExpectedDirectory(target.parent, target.finalName, directoryIdentity{info: finalInfo})
	if err != nil {
		return err
	}
	finalIdentity := finalDirectory.identity.info
	if err := finalDirectory.Close(); err != nil {
		return err
	}
	if os.SameFile(finalIdentity, source.identity.info) {
		return ErrRestoreOverlap
	}
	return visitRetainedAncestors(source.path, func(directory *retainedDirectory) error {
		if os.SameFile(directory.identity.info, finalIdentity) {
			return ErrRestoreOverlap
		}
		return nil
	})
}

// Create publishes a new backup directory. destination and its parent are
// never overwritten. Blob deletion is pinned from before the SQLite snapshot
// until every snapshot-referenced object has been copied and verified.
func (c *Coordinator) Create(ctx context.Context, destination string) (result Manifest, resultErr error) {
	return c.create(ctx, destination, publicationHooks{})
}

func (c *Coordinator) create(
	ctx context.Context,
	destination string,
	hooks publicationHooks,
) (result Manifest, resultErr error) {
	if c == nil {
		return Manifest{}, errors.New("backup: coordinator is not initialized")
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.database == nil || c.blobs == nil || c.now == nil ||
		c.activeVault == nil || c.activeVault.root == nil || c.activeVault.identity.info == nil {
		return Manifest{}, errors.New("backup: coordinator is not initialized")
	}
	if ctx == nil {
		return Manifest{}, errors.New("backup: nil context")
	}
	if err := ensureStagingCleanupSupported(); err != nil {
		return Manifest{}, err
	}
	target, err := newDestination(destination)
	if err != nil {
		return Manifest{}, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, wrapBackupError(target.parent.Close(), "backup: close destination parent"))
	}()
	if err := c.rejectActiveVaultOverlap(target); err != nil {
		return Manifest{}, err
	}
	if hooks.afterOverlapCheck != nil {
		if err := hooks.afterOverlapCheck(target); err != nil {
			return Manifest{}, fmt.Errorf("backup: overlap check hook: %w", err)
		}
	}
	if err := ensureDestinationAbsent(target); err != nil {
		return Manifest{}, err
	}
	pin, err := c.blobs.PinObjectsContext(ctx)
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: pin immutable objects: %w", err)
	}
	defer pin.Release()

	staging, err := createStagingDirectory(target.parent, stagingPrefix, target.finalName)
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: create staging directory: %w", err)
	}
	publicationConsumed := false
	defer func() {
		if resultErr != nil && staging.receiptName != "" && !publicationConsumed {
			resultErr = errors.Join(resultErr, ErrCleanupResidual)
		}
	}()
	defer func() { resultErr = errors.Join(resultErr, closeResidueLease(staging)) }()
	defer func() { resultErr = errors.Join(resultErr, closeStagingCreationWitness(staging)) }()
	defer func() { resultErr = errors.Join(resultErr, staging.directory.Close()) }()
	if err := staging.directory.root.MkdirAll("data", 0o700); err != nil {
		return Manifest{}, fmt.Errorf("backup: create data directory: %w", err)
	}

	if err := staging.directory.verifyPath(); err != nil {
		return Manifest{}, fmt.Errorf("backup: staging directory changed before snapshot: %w", err)
	}
	snapshotPath := filepath.Join(staging.directory.path, filepath.FromSlash(databasePath))
	if err := c.database.BackupSnapshot(ctx, snapshotPath); err != nil {
		return Manifest{}, fmt.Errorf("backup: snapshot database: %w", err)
	}
	initializedSnapshot, err := store.Open(ctx, snapshotPath, store.Options{})
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: open database snapshot: %w", err)
	}
	if err := initializedSnapshot.Close(); err != nil {
		return Manifest{}, fmt.Errorf("backup: stabilize database snapshot: %w", err)
	}
	if err := removeSQLiteSidecarsRoot(staging.directory, databasePath); err != nil {
		return Manifest{}, fmt.Errorf("backup: remove snapshot sidecars: %w", err)
	}
	var version int
	var references []string
	databaseArtifact, err := freezeQualifiedDatabaseArtifact(
		ctx,
		staging.directory,
		func() error {
			snapshot, err := store.Open(ctx, snapshotPath, store.Options{})
			if err != nil {
				return fmt.Errorf("open stabilized database snapshot: %w", err)
			}
			var versionErr, referencesErr error
			version, versionErr = snapshot.SchemaVersion(ctx)
			references, referencesErr = snapshot.ReferencedBlobIDs(ctx, maxBackupArtifacts-1)
			integrityErr := snapshot.IntegrityCheck(ctx)
			canonicalErr := snapshot.CanonicalConsistencyCheck(ctx)
			closeErr := snapshot.Close()
			if err := errors.Join(versionErr, referencesErr, integrityErr, canonicalErr, closeErr); err != nil {
				return err
			}
			return removeSQLiteSidecarsRoot(staging.directory, databasePath)
		},
		hooks.afterCreateQualificationBeforeCommitment,
	)
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: verify and commit database snapshot: %w", err)
	}
	if databaseArtifact.Size > maxBackupBytes {
		return Manifest{}, fmt.Errorf("backup: aggregate bytes exceed limit %d before blob copy", maxBackupBytes)
	}
	aggregateBytes := databaseArtifact.Size

	manifest := Manifest{
		FormatVersion:       FormatVersion,
		SchemaVersion:       version,
		CreatedAtUnixMicros: c.now().UTC().UnixMicro(),
		Database:            databaseArtifact,
		Blobs:               make([]Artifact, 0, len(references)),
	}
	for _, rawID := range references {
		if err := ctx.Err(); err != nil {
			return Manifest{}, err
		}
		if len(manifest.Blobs) >= maxBackupArtifacts-1 {
			return Manifest{}, fmt.Errorf("backup: artifact count exceeds limit %d during copy", maxBackupArtifacts)
		}
		id, err := blob.ParseID(rawID)
		if err != nil {
			return Manifest{}, fmt.Errorf("backup: snapshot contains invalid blob reference: %w", err)
		}
		relative := blobArtifactPath(id)
		if err := staging.directory.root.MkdirAll(filepath.Dir(filepath.FromSlash(relative)), 0o700); err != nil {
			return Manifest{}, fmt.Errorf("backup: create blob directory: %w", err)
		}
		source, err := c.blobs.Open(id)
		if err != nil {
			return Manifest{}, fmt.Errorf("backup: open referenced blob %s: %w", id, err)
		}
		artifact, copyErr := copyOpenedBlob(
			ctx, source, staging.directory, relative, id, maxBackupBytes-aggregateBytes,
		)
		closeErr := source.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return Manifest{}, fmt.Errorf("backup: copy referenced blob %s: %w", id, err)
		}
		if artifact.Size > maxBackupBytes-aggregateBytes {
			return Manifest{}, fmt.Errorf("backup: aggregate bytes exceed limit %d during copy", maxBackupBytes)
		}
		aggregateBytes += artifact.Size
		manifest.Blobs = append(manifest.Blobs, artifact)
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, fmt.Errorf("backup: validate generated manifest: %w", err)
	}
	if err := writeManifestRoot(staging.directory, manifestFileName, manifest); err != nil {
		return Manifest{}, err
	}
	committedManifest := cloneManifest(manifest)
	if err := verifyBackupPublicationRoot(ctx, staging.directory, committedManifest); err != nil {
		return Manifest{}, fmt.Errorf("backup: verify staged backup: %w", err)
	}
	if err := syncBackupTreeDirectoriesRoot(ctx, staging.directory, committedManifest); err != nil {
		return Manifest{}, fmt.Errorf("backup: sync staged backup directories: %w", err)
	}
	if err := target.parent.verifyPath(); err != nil {
		return Manifest{}, fmt.Errorf("backup: destination parent changed before publication: %w", err)
	}
	publication, publishErr := publishPreparedDirectory(
		ctx,
		staging,
		target,
		func(ctx context.Context, published *retainedDirectory) error {
			return verifyBackupPublicationRoot(ctx, published, committedManifest)
		},
		hooks,
	)
	publicationConsumed = publication.stagingConsumed
	if publishErr != nil {
		return Manifest{}, fmt.Errorf("backup: publish backup directory: %w", publishErr)
	}
	return manifest, nil
}

// Restore verifies a backup without modifying it, builds a new Vault under a
// generated sibling directory, verifies the restored database and exact blob
// reachability, and only then atomically renames it to destinationVault. It
// never merges into or overwrites an existing Vault, and refuses every target
// that overlaps the Coordinator's active Vault identity.
func (c *Coordinator) Restore(ctx context.Context, backupDirectory, destinationVault string) (resultErr error) {
	return c.restore(ctx, backupDirectory, destinationVault, publicationHooks{})
}

func (c *Coordinator) restore(
	ctx context.Context,
	backupDirectory, destinationVault string,
	hooks publicationHooks,
) (resultErr error) {
	if c == nil {
		return errors.New("backup: coordinator is not initialized")
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.database == nil || c.blobs == nil ||
		c.activeVault == nil || c.activeVault.root == nil || c.activeVault.identity.info == nil {
		return errors.New("backup: coordinator is not initialized")
	}
	if ctx == nil {
		return errors.New("backup: nil context")
	}
	if err := ensureStagingCleanupSupported(); err != nil {
		return err
	}
	target, err := newDestination(destinationVault)
	if err != nil {
		return fmt.Errorf("backup: restore destination: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, wrapBackupError(target.parent.Close(), "backup: close restore destination parent"))
	}()
	if err := c.rejectActiveVaultOverlap(target); err != nil {
		return fmt.Errorf("backup: restore destination overlaps active Vault: %w", err)
	}
	if hooks.afterOverlapCheck != nil {
		if err := hooks.afterOverlapCheck(target); err != nil {
			return fmt.Errorf("backup: restore overlap check hook: %w", err)
		}
	}
	backupRoot, err := openRetainedDirectory(backupDirectory)
	if err != nil {
		return fmt.Errorf("backup: open backup: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, wrapBackupError(backupRoot.Close(), "backup: close backup root"))
	}()
	if err := rejectRestoreDestinationOverlap(backupRoot, target); err != nil {
		return fmt.Errorf("backup: restore destination overlap: %w", err)
	}
	if err := ensureDestinationAbsent(target); err != nil {
		return fmt.Errorf("backup: restore destination: %w", err)
	}
	manifest, err := readManifestRoot(ctx, backupRoot, manifestFileName)
	if err != nil {
		return err
	}
	if err := verifyBackupTreeRoot(ctx, backupRoot, manifest); err != nil {
		return fmt.Errorf("backup: backup verification failed: %w", err)
	}

	staging, err := createStagingDirectory(target.parent, restorePrefix, target.finalName)
	if err != nil {
		return fmt.Errorf("backup: create restore staging directory: %w", err)
	}
	publicationConsumed := false
	defer func() {
		if resultErr != nil && staging.receiptName != "" && !publicationConsumed {
			resultErr = errors.Join(resultErr, ErrCleanupResidual)
		}
	}()
	defer func() { resultErr = errors.Join(resultErr, closeResidueLease(staging)) }()
	defer func() { resultErr = errors.Join(resultErr, closeStagingCreationWitness(staging)) }()
	defer func() { resultErr = errors.Join(resultErr, staging.directory.Close()) }()
	if err := staging.directory.verifyPath(); err != nil {
		return fmt.Errorf("backup: restore staging directory changed before Vault open: %w", err)
	}
	ownedVault, err := vault.Open(staging.directory.path)
	if err != nil {
		return fmt.Errorf("backup: open restore Vault: %w", err)
	}
	vaultOpen := true
	defer func() {
		if vaultOpen {
			resultErr = errors.Join(resultErr, wrapBackupError(ownedVault.Close(), "backup: close failed restore Vault"))
		}
	}()
	paths := ownedVault.Paths()

	databaseDestination := filepath.Join(paths.Data, store.DatabaseFileName)
	if err := copyVerifiedArtifact(ctx, backupRoot, manifest.Database, databaseDestination); err != nil {
		return fmt.Errorf("backup: restore database: %w", err)
	}
	actualSchemaVersion, err := store.InspectSchemaVersion(ctx, databaseDestination)
	if err != nil {
		return fmt.Errorf("backup: inspect restored schema: %w", err)
	}
	if actualSchemaVersion != manifest.SchemaVersion {
		return fmt.Errorf("backup: manifest schema version %d differs from database version %d", manifest.SchemaVersion, actualSchemaVersion)
	}
	restoredBlobs, err := blob.OpenStore(paths.Blobs)
	if err != nil {
		return fmt.Errorf("backup: open restored blob store: %w", err)
	}
	for _, artifact := range manifest.Blobs {
		id, err := blob.ParseID(artifact.BlobID)
		if err != nil {
			return fmt.Errorf("backup: restore invalid blob ID: %w", err)
		}
		source, sourceInfo, err := openRootRegularFile(backupRoot, artifact.Path)
		if err != nil {
			return fmt.Errorf("backup: open blob artifact: %w", err)
		}
		imported, importErr := restoredBlobs.Import(ctx, source, artifact.Size)
		stableErr := verifyOpenedRootFile(backupRoot, artifact.Path, source, sourceInfo)
		closeErr := source.Close()
		if err := errors.Join(importErr, stableErr, closeErr); err != nil {
			return fmt.Errorf("backup: import restored blob %s: %w", id, err)
		}
		if imported.ID != id || imported.Size != artifact.Size {
			return fmt.Errorf("backup: restored blob %s identity mismatch", id)
		}
	}
	if err := verifyExactTreeRoot(ctx, backupRoot, manifest); err != nil {
		return fmt.Errorf("backup: source tree changed while restoring: %w", err)
	}

	restoredDatabase, err := store.Open(ctx, databaseDestination, store.Options{})
	if err != nil {
		return fmt.Errorf("backup: open restored database: %w", err)
	}
	publishedSchemaVersion, schemaErr := restoredDatabase.SchemaVersion(ctx)
	closeErr := restoredDatabase.Close()
	if err := errors.Join(schemaErr, closeErr); err != nil {
		return fmt.Errorf("backup: finalize restored database schema: %w", err)
	}
	if err := ownedVault.Close(); err != nil {
		return fmt.Errorf("backup: close restored Vault: %w", err)
	}
	vaultOpen = false
	committedBlobs := cloneArtifacts(manifest.Blobs)
	if err := removeSQLiteSidecarsRoot(staging.directory, databasePath); err != nil {
		return fmt.Errorf("backup: remove restored database sidecars before qualification: %w", err)
	}
	databaseArtifact, err := freezeQualifiedDatabaseArtifact(
		ctx,
		staging.directory,
		func() error {
			if hooks.beforeRestoreQualification != nil {
				if err := hooks.beforeRestoreQualification(databaseDestination); err != nil {
					return fmt.Errorf("restore qualification hook: %w", err)
				}
			}
			return qualifyRestoredVaultRoot(ctx, staging.directory, publishedSchemaVersion, committedBlobs)
		},
		hooks.afterRestoreQualificationBeforeCommitment,
	)
	if err != nil {
		return fmt.Errorf("backup: qualify and freeze restored database commitment: %w", err)
	}
	commitment := restoredVaultCommitment{
		schemaVersion: publishedSchemaVersion,
		database:      databaseArtifact,
		blobs:         committedBlobs,
	}
	if err := verifyRestoredVaultRoot(ctx, staging.directory, commitment); err != nil {
		return fmt.Errorf("backup: verify staged restored Vault: %w", err)
	}
	if err := syncRestoredTreeDirectoriesRoot(ctx, staging.directory, commitment.blobs); err != nil {
		return fmt.Errorf("backup: sync restored Vault directories: %w", err)
	}
	if err := target.parent.verifyPath(); err != nil {
		return fmt.Errorf("backup: restore destination parent changed before publication: %w", err)
	}
	publication, publishErr := publishPreparedDirectory(
		ctx,
		staging,
		target,
		func(ctx context.Context, published *retainedDirectory) error {
			return verifyRestoredVaultRoot(ctx, published, commitment)
		},
		hooks,
	)
	publicationConsumed = publication.stagingConsumed
	if publishErr != nil {
		return fmt.Errorf("backup: publish restored Vault: %w", publishErr)
	}
	return nil
}

func cleanupStagingTree(staging *stagingDirectory, parent *retainedDirectory, kind string) error {
	if err := cleanupOwnedStaging(staging, parent, kind); err != nil {
		return err
	}
	if err := removeResidueReceipt(parent, staging); err != nil {
		return errors.Join(ErrCleanupResidual, err)
	}
	return nil
}

func wrapBackupError(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

func readManifestRoot(ctx context.Context, root *retainedDirectory, relative string) (Manifest, error) {
	if ctx == nil {
		return Manifest{}, errors.New("backup: nil manifest context")
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	file, info, err := openRootRegularFile(root, relative)
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: open manifest: %w", err)
	}
	if info.Size() > maxManifestBytes {
		_ = file.Close()
		return Manifest{}, errors.New("backup: manifest exceeds size limit")
	}
	var encoded bytes.Buffer
	_, readErr := copyContext(ctx, &encoded, io.LimitReader(file, maxManifestBytes+1))
	data := encoded.Bytes()
	stableErr := verifyOpenedRootFile(root, relative, file, info)
	closeErr := file.Close()
	if err := errors.Join(readErr, stableErr, closeErr); err != nil {
		return Manifest{}, fmt.Errorf("backup: read manifest: %w", err)
	}
	if len(data) > maxManifestBytes {
		return Manifest{}, errors.New("backup: manifest exceeds size limit")
	}
	if err := rejectDuplicateJSONFields(ctx, data); err != nil {
		return Manifest{}, fmt.Errorf("backup: invalid manifest JSON: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	if err := validateManifestJSONSchema(ctx, data); err != nil {
		return Manifest{}, fmt.Errorf("backup: invalid manifest schema: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("backup: decode manifest: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Manifest{}, errors.New("backup: manifest has trailing JSON")
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, fmt.Errorf("backup: invalid manifest: %w", err)
	}
	return manifest, nil
}

func validateManifestJSONSchema(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	manifest, err := exactJSONObject(data, "manifest",
		[]string{"format_version", "schema_version", "created_at_unix_micros", "database", "blobs"}, nil)
	if err != nil {
		return err
	}
	if _, err := exactJSONObject(manifest["database"], "database artifact",
		[]string{"path", "size", "sha256"}, []string{"blob_id"}); err != nil {
		return err
	}

	var blobs []json.RawMessage
	if err := json.Unmarshal(manifest["blobs"], &blobs); err != nil {
		return errors.New("field \"blobs\" must be an array")
	}
	if blobs == nil {
		return errors.New("field \"blobs\" must be a non-null array")
	}
	if len(blobs) > maxBackupArtifacts-1 {
		return fmt.Errorf("backup artifact count exceeds limit %d", maxBackupArtifacts)
	}
	for index, raw := range blobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := exactJSONObject(raw, fmt.Sprintf("blob artifact %d", index),
			[]string{"path", "size", "sha256", "blob_id"}, nil); err != nil {
			return err
		}
	}
	return nil
}

func exactJSONObject(raw []byte, label string, required, optional []string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be an object", label)
	}
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for _, field := range required {
		allowed[field] = struct{}{}
	}
	for _, field := range optional {
		allowed[field] = struct{}{}
	}
	for field := range object {
		if _, ok := allowed[field]; !ok {
			return nil, fmt.Errorf("%s contains unknown field %q", label, field)
		}
	}
	for _, field := range required {
		value, ok := object[field]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("%s requires non-null field %q", label, field)
		}
	}
	for _, field := range optional {
		if value, ok := object[field]; ok && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("%s field %q must not be null", label, field)
		}
	}
	return object, nil
}

func writeManifestRoot(root *retainedDirectory, relative string, manifest Manifest) error {
	if !safeRelativePath(relative) {
		return errors.New("backup: unsafe manifest destination path")
	}
	if err := validateManifest(manifest); err != nil {
		return fmt.Errorf("backup: refuse invalid manifest write: %w", err)
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("backup: encode manifest: %w", err)
	}
	if len(encoded) >= maxManifestBytes {
		return errors.New("backup: encoded manifest exceeds size limit")
	}
	encoded = append(encoded, '\n')
	file, err := root.root.OpenFile(filepath.FromSlash(relative), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("backup: create manifest: %w", err)
	}
	_, writeErr := file.Write(encoded)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return fmt.Errorf("backup: persist manifest: %w", err)
	}
	return nil
}

func validateManifest(manifest Manifest) error {
	supported, err := store.SupportedSchemaVersion()
	if err != nil {
		return err
	}
	if manifest.FormatVersion != FormatVersion {
		return fmt.Errorf("unsupported format version %d", manifest.FormatVersion)
	}
	if manifest.SchemaVersion < 1 || manifest.SchemaVersion > supported {
		return fmt.Errorf("unsupported schema version %d (supported through %d)", manifest.SchemaVersion, supported)
	}
	if manifest.CreatedAtUnixMicros <= 0 {
		return errors.New("invalid creation time")
	}
	if manifest.Blobs == nil {
		return errors.New("blob artifacts must be a non-null array")
	}
	if len(manifest.Blobs) > maxBackupArtifacts-1 {
		return fmt.Errorf("backup artifact count exceeds limit %d", maxBackupArtifacts)
	}
	if manifest.Database.BlobID != "" {
		return errors.New("database artifact must not have a blob ID")
	}
	if err := validateArtifact(manifest.Database, databasePath, ""); err != nil {
		return fmt.Errorf("database artifact: %w", err)
	}
	aggregateBytes := manifest.Database.Size
	if aggregateBytes > maxBackupBytes {
		return fmt.Errorf("backup aggregate bytes exceed limit %d", maxBackupBytes)
	}
	previous := ""
	for index, artifact := range manifest.Blobs {
		id, err := blob.ParseID(artifact.BlobID)
		if err != nil {
			return fmt.Errorf("blob %d: %w", index, err)
		}
		if previous != "" && artifact.BlobID <= previous {
			return errors.New("blob artifacts must be strictly sorted and unique")
		}
		previous = artifact.BlobID
		if err := validateArtifact(artifact, blobArtifactPath(id), id.String()[7:]); err != nil {
			return fmt.Errorf("blob %d: %w", index, err)
		}
		if artifact.Size > maxBackupBytes-aggregateBytes {
			return fmt.Errorf("backup aggregate bytes exceed limit %d", maxBackupBytes)
		}
		aggregateBytes += artifact.Size
	}
	return nil
}

func validateArtifact(artifact Artifact, wantPath, wantDigest string) error {
	if artifact.Path != wantPath || !safeRelativePath(artifact.Path) {
		return errors.New("artifact path is not canonical")
	}
	if artifact.Size < 0 || artifact.Size > maxArtifactBytes {
		return errors.New("artifact size is outside the supported bound")
	}
	if !lowerSHA256(artifact.SHA256) {
		return errors.New("artifact digest is not lowercase SHA-256")
	}
	if wantDigest != "" && artifact.SHA256 != wantDigest {
		return errors.New("blob artifact digest differs from its content address")
	}
	return nil
}

func cloneManifest(manifest Manifest) Manifest {
	manifest.Blobs = cloneArtifacts(manifest.Blobs)
	return manifest
}

func cloneArtifacts(artifacts []Artifact) []Artifact {
	cloned := make([]Artifact, len(artifacts))
	copy(cloned, artifacts)
	return cloned
}

func manifestsEqual(left, right Manifest) bool {
	if left.FormatVersion != right.FormatVersion ||
		left.SchemaVersion != right.SchemaVersion ||
		left.CreatedAtUnixMicros != right.CreatedAtUnixMicros ||
		left.Database != right.Database ||
		len(left.Blobs) != len(right.Blobs) {
		return false
	}
	for index := range left.Blobs {
		if left.Blobs[index] != right.Blobs[index] {
			return false
		}
	}
	return true
}

func verifyBackupPublicationRoot(ctx context.Context, root *retainedDirectory, expected Manifest) error {
	if ctx == nil {
		return errors.New("backup: nil backup publication context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	persisted, err := readManifestRoot(ctx, root, manifestFileName)
	if err != nil {
		return fmt.Errorf("backup: read committed manifest: %w", err)
	}
	if !manifestsEqual(persisted, expected) {
		return errors.New("backup: published manifest differs from committed manifest")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return verifyBackupTreeRoot(ctx, root, expected)
}

func verifyRestoredVaultRoot(
	ctx context.Context,
	root *retainedDirectory,
	commitment restoredVaultCommitment,
) error {
	if ctx == nil {
		return errors.New("backup: nil restored Vault verification context")
	}
	if err := validateManifest(Manifest{
		FormatVersion:       FormatVersion,
		SchemaVersion:       commitment.schemaVersion,
		CreatedAtUnixMicros: 1,
		Database:            commitment.database,
		Blobs:               commitment.blobs,
	}); err != nil {
		return fmt.Errorf("backup: invalid restored Vault commitment: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := verifyExactRestoredVaultTreeRoot(ctx, root, commitment.blobs); err != nil {
		return err
	}
	// Publication verification is deliberately byte-only and retained-root
	// relative. SQLite qualification happened while the directory was still
	// private staging; equality with this frozen artifact transfers that result
	// without reopening the published path or permitting SQLite sidecar writes.
	if err := verifyRootArtifact(ctx, root, commitment.database); err != nil {
		return fmt.Errorf("backup: restored database: %w", err)
	}
	for _, artifact := range commitment.blobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := verifyRootArtifact(ctx, root, artifact); err != nil {
			return fmt.Errorf("backup: restored blob %s: %w", artifact.BlobID, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return verifyExactRestoredVaultTreeRoot(ctx, root, commitment.blobs)
}

func qualifyRestoredVaultRoot(
	ctx context.Context,
	root *retainedDirectory,
	schemaVersion int,
	blobs []Artifact,
) error {
	if ctx == nil {
		return errors.New("backup: nil restored Vault qualification context")
	}
	if schemaVersion < 1 || blobs == nil {
		return errors.New("backup: invalid restored Vault qualification input")
	}
	if err := verifyExactRestoredVaultTreeRoot(ctx, root, blobs); err != nil {
		return err
	}
	if err := qualifyRestoredDatabaseRoot(ctx, root, schemaVersion, blobs); err != nil {
		return err
	}
	if err := removeSQLiteSidecarsRoot(root, databasePath); err != nil {
		return fmt.Errorf("backup: remove database qualification sidecars: %w", err)
	}
	for _, artifact := range blobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := verifyRootArtifact(ctx, root, artifact); err != nil {
			return fmt.Errorf("backup: restored blob %s: %w", artifact.BlobID, err)
		}
	}
	return verifyExactRestoredVaultTreeRoot(ctx, root, blobs)
}

// qualifyRestoredDatabaseRoot is staging-only. It may use SQLite's path-based
// API and FTS special integrity command because the result is followed by a
// close, sidecar removal, and a retained-root byte commitment before publish.
func qualifyRestoredDatabaseRoot(
	ctx context.Context,
	root *retainedDirectory,
	schemaVersion int,
	blobs []Artifact,
) (resultErr error) {
	if err := root.verifyPath(); err != nil {
		return fmt.Errorf("backup: restored Vault root path changed: %w", err)
	}
	identityFile, identityInfo, err := openRootRegularFile(root, databasePath)
	if err != nil {
		return fmt.Errorf("backup: open restored database identity: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, identityFile.Close())
	}()
	databaseFile := filepath.Join(root.path, filepath.FromSlash(databasePath))
	version, err := store.InspectSchemaVersion(ctx, databaseFile)
	if err != nil {
		return err
	}
	if version != schemaVersion {
		return fmt.Errorf(
			"backup: restored database schema %d differs from committed schema %d",
			version,
			schemaVersion,
		)
	}
	dsn, err := verificationSQLiteURI(databaseFile)
	if err != nil {
		return err
	}
	database, err := sqliteDriver.Open(dsn, func(connection *sqlite3.Conn) error {
		if err := connection.BusyTimeout(5 * time.Second); err != nil {
			return err
		}
		if err := connection.Exec("PRAGMA trusted_schema=OFF"); err != nil {
			return err
		}
		if err := fts5.Register(connection); err != nil {
			return err
		}
		return connection.Exec("PRAGMA query_only=ON")
	})
	if err != nil {
		return fmt.Errorf("backup: open restored database read-only: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return fmt.Errorf("backup: connect restored database read-only: %w", err)
	}
	integrityErr := verifySQLiteIntegrityWithoutPersistentWrites(ctx, database)
	canonicalErr := store.CheckCanonicalConsistency(ctx, database)
	referencesErr := verifyRestoredBlobReferences(ctx, database, blobs)
	closeErr := database.Close()
	finalInfo, finalInfoErr := identityFile.Stat()
	stableErr := verifyOpenedRootFile(root, databasePath, identityFile, identityInfo)
	rootErr := root.verifyPath()
	var mutationErr error
	if finalInfoErr == nil && !finalInfo.ModTime().Equal(identityInfo.ModTime()) {
		mutationErr = errors.New("backup: restored database verification changed the database file")
	}
	if err := errors.Join(
		integrityErr,
		canonicalErr,
		referencesErr,
		closeErr,
		finalInfoErr,
		stableErr,
		rootErr,
		mutationErr,
	); err != nil {
		return fmt.Errorf("backup: verify restored database: %w", err)
	}
	return nil
}

func verifySQLiteIntegrityWithoutPersistentWrites(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("backup: run restored database integrity check: %w", err)
	}
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			_ = rows.Close()
			return err
		}
		if result != "ok" {
			_ = rows.Close()
			return fmt.Errorf("backup: restored database integrity check failed: %s", result)
		}
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return err
	}
	foreign, err := database.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("backup: run restored database foreign-key check: %w", err)
	}
	if foreign.Next() {
		_ = foreign.Close()
		return errors.New("backup: restored database foreign-key check failed")
	}
	err = errors.Join(foreign.Err(), foreign.Close())
	if err != nil {
		return err
	}
	if _, err := database.ExecContext(ctx, "PRAGMA query_only=OFF"); err != nil {
		return fmt.Errorf("backup: enable isolated FTS integrity command: %w", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO chunks_fts(chunks_fts, rank) VALUES('integrity-check', 1)
	`); err != nil {
		return fmt.Errorf("backup: restored database FTS integrity check failed: %w", err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil {
		return fmt.Errorf("backup: restore read-only database verification mode: %w", err)
	}
	return nil
}

func verifyRestoredBlobReferences(ctx context.Context, database *sql.DB, expected []Artifact) error {
	rows, err := database.QueryContext(ctx, `
		SELECT DISTINCT source_blob_id
		FROM document_revisions
		WHERE source_blob_id IS NOT NULL
		ORDER BY source_blob_id
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		var reference string
		if err := rows.Scan(&reference); err != nil {
			return err
		}
		if index >= len(expected) || reference != expected[index].BlobID {
			return errors.New("backup: restored database blob reachability differs from commitment")
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if index != len(expected) {
		return errors.New("backup: restored database blob reachability differs from commitment")
	}
	return nil
}

func verificationSQLiteURI(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	segments := strings.Split(filepath.ToSlash(abs), "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return (&url.URL{
		Scheme:   "file",
		Opaque:   strings.Join(segments, "/"),
		RawQuery: "mode=rw",
	}).String(), nil
}

func verifyExactRestoredVaultTreeRoot(ctx context.Context, root *retainedDirectory, blobs []Artifact) error {
	if ctx == nil {
		return errors.New("backup: nil restored Vault exact-tree context")
	}
	remainingFiles, remainingDirectories, err := expectedRestoredVaultTree(ctx, blobs)
	if err != nil {
		return err
	}
	return verifyExpectedTreeRoot(ctx, root, remainingFiles, remainingDirectories, "restored Vault")
}

func expectedRestoredVaultTree(ctx context.Context, blobs []Artifact) (map[string]bool, map[string]struct{}, error) {
	if ctx == nil {
		return nil, nil, errors.New("backup: nil restored tree commitment context")
	}
	if len(blobs) > maxBackupArtifacts-1 {
		return nil, nil, fmt.Errorf("backup artifact count exceeds limit %d", maxBackupArtifacts)
	}
	remainingFiles := make(map[string]bool, len(blobs)+2)
	remainingFiles[vaultLockFileName] = true
	remainingFiles[databasePath] = false
	remainingDirectories := map[string]struct{}{
		".":                    {},
		"data":                 {},
		"blobs":                {},
		"blobs/objects":        {},
		"blobs/objects/sha256": {},
		"blobs/staging":        {},
	}
	for _, artifact := range blobs {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		remainingFiles[artifact.Path] = false
		for directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(artifact.Path))); directory != "."; directory = filepath.ToSlash(filepath.Dir(filepath.FromSlash(directory))) {
			remainingDirectories[directory] = struct{}{}
		}
	}
	return remainingFiles, remainingDirectories, nil
}

type exactTreeDirectory struct {
	relative  string
	file      *os.File
	identity  os.FileInfo
	depth     int
	pending   []os.DirEntry
	exhausted bool
}

// verifyExpectedTreeRoot walks only through the retained os.Root. It never
// asks fs.WalkDir to materialize and sort a whole hostile directory: every
// directory is consumed in fixed batches, and the expected shape bounds both
// the open-directory stack and the total number of visited entries.
func verifyExpectedTreeRoot(
	ctx context.Context,
	root *retainedDirectory,
	remainingFiles map[string]bool,
	remainingDirectories map[string]struct{},
	label string,
) error {
	return walkExpectedTreeRoot(ctx, root, remainingFiles, remainingDirectories, label, nil)
}

type expectedDirectoryVisitor func(context.Context, *retainedDirectory, string, os.FileInfo) error

func walkExpectedTreeRoot(
	ctx context.Context,
	root *retainedDirectory,
	remainingFiles map[string]bool,
	remainingDirectories map[string]struct{},
	label string,
	visitDirectoryPostOrder expectedDirectoryVisitor,
) (resultErr error) {
	if ctx == nil {
		return errors.New("backup: nil exact-tree context")
	}
	if root == nil || root.root == nil {
		return errors.New("backup: nil exact-tree root")
	}
	expectedEntries := len(remainingFiles) + len(remainingDirectories)
	if expectedEntries < 1 || expectedEntries > maxExactTreeEntries {
		return fmt.Errorf("backup: %s entry count is outside the supported bound", label)
	}
	if _, ok := remainingDirectories["."]; !ok {
		return fmt.Errorf("backup: %s commitment omits its root", label)
	}
	rootInfo, err := root.root.Stat(".")
	if err != nil {
		return err
	}
	if err := validateDirectoryBoundary(rootInfo, rootInfo); err != nil {
		return err
	}
	rootDirectory, err := openExactTreeDirectory(root, rootInfo, ".")
	if err != nil {
		return fmt.Errorf("backup: open retained %s root: %w", label, err)
	}
	delete(remainingDirectories, ".")
	visited := 1
	stack := []*exactTreeDirectory{rootDirectory}
	defer func() {
		for _, directory := range stack {
			if directory.file != nil {
				resultErr = errors.Join(resultErr, directory.file.Close())
			}
		}
	}()

	for len(stack) != 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := stack[len(stack)-1]
		entry, hasEntry, readErr := nextExactTreeEntry(current)
		if readErr != nil {
			return fmt.Errorf("backup: read %s directory %s: %w", label, current.relative, readErr)
		}
		if !hasEntry {
			if visitDirectoryPostOrder != nil {
				if err := visitDirectoryPostOrder(ctx, root, current.relative, current.identity); err != nil {
					return fmt.Errorf("backup: visit %s directory %s: %w", label, current.relative, err)
				}
			}
			stack = stack[:len(stack)-1]
			if err := closeExactTreeDirectory(root, rootInfo, current); err != nil {
				return fmt.Errorf("backup: %s directory changed while reading %s: %w", label, current.relative, err)
			}
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative := filepath.ToSlash(filepath.Join(filepath.FromSlash(current.relative), entry.Name()))
		if current.relative == "." {
			relative = filepath.ToSlash(entry.Name())
		}
		info, err := root.root.Lstat(filepath.FromSlash(relative))
		if err != nil {
			return fmt.Errorf("backup: inspect %s entry %s: %w", label, relative, err)
		}
		if entry.Type()&os.ModeSymlink != 0 || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("backup: untrusted link in %s: %s", label, relative)
		}
		if info.IsDir() {
			if _, ok := remainingDirectories[relative]; !ok {
				if label == "backup tree" {
					return fmt.Errorf("backup: unlisted directory in backup tree: %s", relative)
				}
				return fmt.Errorf("backup: unexpected directory in %s: %s", label, relative)
			}
			if current.depth >= maxExactTreeDepth {
				return fmt.Errorf("backup: %s exceeds directory depth limit %d", label, maxExactTreeDepth)
			}
			visited++
			if visited > expectedEntries || visited > maxExactTreeEntries {
				return fmt.Errorf("backup: %s traversal exceeded its committed entry bound", label)
			}
			delete(remainingDirectories, relative)
			child, err := openExactTreeDirectory(root, rootInfo, relative)
			if err != nil {
				return fmt.Errorf("backup: open %s directory %s: %w", label, relative, err)
			}
			stack = append(stack, child)
			continue
		}
		requireEmpty, ok := remainingFiles[relative]
		if !ok {
			if label == "backup tree" {
				return fmt.Errorf("backup: unlisted artifact in backup tree: %s", relative)
			}
			return fmt.Errorf("backup: unexpected file in %s: %s", label, relative)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("backup: non-regular file in %s: %s", label, relative)
		}
		visited++
		if visited > expectedEntries || visited > maxExactTreeEntries {
			return fmt.Errorf("backup: %s traversal exceeded its committed entry bound", label)
		}
		delete(remainingFiles, relative)
		if requireEmpty {
			file, openedInfo, err := openRootRegularFile(root, relative)
			if err != nil {
				return err
			}
			stableErr := verifyOpenedRootFile(root, relative, file, openedInfo)
			closeErr := file.Close()
			if err := errors.Join(stableErr, closeErr); err != nil {
				return err
			}
			if openedInfo.Size() != 0 {
				return fmt.Errorf("backup: required empty file in %s is not empty: %s", label, relative)
			}
		}
	}
	if len(remainingDirectories) != 0 || len(remainingFiles) != 0 || visited != expectedEntries {
		return fmt.Errorf("backup: %s is missing a required artifact or directory", label)
	}
	return nil
}

func openExactTreeDirectory(root *retainedDirectory, rootInfo os.FileInfo, relative string) (*exactTreeDirectory, error) {
	path := filepath.FromSlash(relative)
	before, err := root.root.Lstat(path)
	if err != nil {
		return nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
		return nil, errors.New("backup: expected retained directory")
	}
	if err := validateDirectoryBoundary(rootInfo, before); err != nil {
		return nil, err
	}
	file, err := root.root.Open(path)
	if err != nil {
		return nil, err
	}
	opened, openedErr := file.Stat()
	after, afterErr := root.root.Lstat(path)
	if err := errors.Join(openedErr, afterErr); err != nil ||
		opened.Mode()&os.ModeSymlink != 0 || !opened.IsDir() ||
		after.Mode()&os.ModeSymlink != 0 || !after.IsDir() ||
		!os.SameFile(before, opened) || !os.SameFile(opened, after) {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("backup: retained directory identity changed while opening")
	}
	if err := errors.Join(validateDirectoryBoundary(rootInfo, opened), validateDirectoryBoundary(rootInfo, after)); err != nil {
		_ = file.Close()
		return nil, err
	}
	depth := 0
	if relative != "." {
		depth = strings.Count(filepath.ToSlash(relative), "/") + 1
	}
	return &exactTreeDirectory{relative: relative, file: file, identity: opened, depth: depth}, nil
}

func nextExactTreeEntry(directory *exactTreeDirectory) (os.DirEntry, bool, error) {
	if directory == nil || directory.file == nil {
		return nil, false, errors.New("backup: invalid exact-tree cursor")
	}
	for len(directory.pending) == 0 && !directory.exhausted {
		entries, err := directory.file.ReadDir(directoryReadBatch)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, false, err
		}
		directory.pending = entries
		directory.exhausted = errors.Is(err, io.EOF)
		if len(entries) == 0 && !directory.exhausted {
			return nil, false, errors.New("backup: zero-progress exact-tree directory read")
		}
	}
	if len(directory.pending) == 0 {
		return nil, false, nil
	}
	entry := directory.pending[0]
	directory.pending[0] = nil
	directory.pending = directory.pending[1:]
	return entry, true, nil
}

func closeExactTreeDirectory(root *retainedDirectory, rootInfo os.FileInfo, directory *exactTreeDirectory) error {
	after, pathErr := root.root.Lstat(filepath.FromSlash(directory.relative))
	var identityErr error
	if pathErr == nil && (after.Mode()&os.ModeSymlink != 0 || !after.IsDir() ||
		!os.SameFile(after, directory.identity)) {
		identityErr = errors.New("backup: retained directory identity changed after reading")
	}
	boundaryErr := error(nil)
	if pathErr == nil {
		boundaryErr = validateDirectoryBoundary(rootInfo, after)
	}
	closeErr := directory.file.Close()
	directory.file = nil
	return errors.Join(pathErr, identityErr, boundaryErr, closeErr)
}

func verifyBackupTreeRoot(ctx context.Context, root *retainedDirectory, manifest Manifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := verifyRootArtifact(ctx, root, manifest.Database); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	for _, artifact := range manifest.Blobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := verifyRootArtifact(ctx, root, artifact); err != nil {
			return fmt.Errorf("blob %s: %w", artifact.BlobID, err)
		}
	}
	return verifyExactTreeRoot(ctx, root, manifest)
}

func verifyExactTreeRoot(ctx context.Context, root *retainedDirectory, manifest Manifest) error {
	if ctx == nil {
		return errors.New("backup: nil backup exact-tree context")
	}
	remainingFiles, remainingDirectories, err := expectedBackupTree(ctx, manifest)
	if err != nil {
		return err
	}
	return verifyExpectedTreeRoot(ctx, root, remainingFiles, remainingDirectories, "backup tree")
}

func expectedBackupTree(ctx context.Context, manifest Manifest) (map[string]bool, map[string]struct{}, error) {
	if ctx == nil {
		return nil, nil, errors.New("backup: nil backup tree commitment context")
	}
	if len(manifest.Blobs) > maxBackupArtifacts-1 {
		return nil, nil, fmt.Errorf("backup artifact count exceeds limit %d", maxBackupArtifacts)
	}
	remainingFiles := make(map[string]bool, len(manifest.Blobs)+2)
	remainingFiles[manifestFileName] = false
	remainingFiles[manifest.Database.Path] = false
	remainingDirectories := map[string]struct{}{".": {}}
	addParents := func(relative string) {
		for directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relative))); directory != "."; directory = filepath.ToSlash(filepath.Dir(filepath.FromSlash(directory))) {
			remainingDirectories[directory] = struct{}{}
		}
	}
	addParents(manifestFileName)
	addParents(manifest.Database.Path)
	for _, artifact := range manifest.Blobs {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		remainingFiles[artifact.Path] = false
		addParents(artifact.Path)
	}
	return remainingFiles, remainingDirectories, nil
}

func verifyRootArtifact(ctx context.Context, root *retainedDirectory, artifact Artifact) error {
	actual, err := inspectRootFile(ctx, root, artifact.Path, artifact.BlobID)
	if err != nil {
		return err
	}
	if actual.Size != artifact.Size || actual.SHA256 != artifact.SHA256 {
		return errors.New("size or SHA-256 mismatch")
	}
	return nil
}

func inspectRootFile(ctx context.Context, root *retainedDirectory, relative, blobID string) (Artifact, error) {
	if ctx == nil {
		return Artifact{}, errors.New("backup: nil artifact inspection context")
	}
	file, info, err := openRootRegularFile(root, relative)
	if err != nil {
		return Artifact{}, err
	}
	hasher := sha256.New()
	readLimit := info.Size() + 1
	written, copyErr := copyContext(ctx, hasher, io.LimitReader(file, readLimit))
	stableErr := verifyOpenedRootFile(root, relative, file, info)
	closeErr := file.Close()
	if err := errors.Join(copyErr, stableErr, closeErr); err != nil {
		return Artifact{}, err
	}
	if written != info.Size() {
		return Artifact{}, errors.New("backup: artifact size changed while hashing")
	}
	return Artifact{
		Path: relative, Size: written, SHA256: hex.EncodeToString(hasher.Sum(nil)), BlobID: blobID,
	}, nil
}

// freezeQualifiedDatabaseArtifact binds product-level qualification to exact
// SQLite bytes. A retained identity witness stays open across the pre-hash,
// qualification, optional fault hook, and post-hash; only the pre-qualified
// artifact may become a manifest or restored-Vault commitment.
func freezeQualifiedDatabaseArtifact(
	ctx context.Context,
	root *retainedDirectory,
	qualify func() error,
	afterQualification func(string) error,
) (result Artifact, resultErr error) {
	if ctx == nil || root == nil || root.root == nil || qualify == nil {
		return Artifact{}, errors.New("backup: invalid qualified database commitment input")
	}
	witness, witnessInfo, err := openRootRegularFile(root, databasePath)
	if err != nil {
		return Artifact{}, fmt.Errorf("backup: retain database qualification identity: %w", err)
	}
	defer func() {
		resultErr = errors.Join(
			resultErr,
			wrapBackupError(
				verifyOpenedRootFile(root, databasePath, witness, witnessInfo),
				"backup: database qualification identity changed",
			),
			witness.Close(),
		)
	}()
	before, err := inspectRootFile(ctx, root, databasePath, "")
	if err != nil {
		return Artifact{}, fmt.Errorf("backup: hash database before qualification: %w", err)
	}
	if err := qualify(); err != nil {
		return Artifact{}, fmt.Errorf("backup: qualify committed database bytes: %w", err)
	}
	if afterQualification != nil {
		path := filepath.Join(root.path, filepath.FromSlash(databasePath))
		if err := afterQualification(path); err != nil {
			return Artifact{}, fmt.Errorf("backup: after database qualification hook: %w", err)
		}
	}
	after, err := inspectRootFile(ctx, root, databasePath, "")
	if err != nil {
		return Artifact{}, fmt.Errorf("backup: hash database after qualification: %w", err)
	}
	if before != after {
		return Artifact{}, errors.New("backup: database bytes changed across semantic qualification")
	}
	return before, nil
}

func copyOpenedBlob(
	ctx context.Context,
	source *os.File,
	destination *retainedDirectory,
	relative string,
	id blob.BlobID,
	remainingBackupBytes int64,
) (Artifact, error) {
	if !safeRelativePath(relative) {
		return Artifact{}, errors.New("backup: unsafe blob destination path")
	}
	if err := validateRootParents(destination, relative); err != nil {
		return Artifact{}, err
	}
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxArtifactBytes ||
		remainingBackupBytes < 0 || info.Size() > remainingBackupBytes {
		return Artifact{}, errors.New("source blob is not a bounded regular file")
	}
	file, err := destination.root.OpenFile(filepath.FromSlash(relative), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Artifact{}, err
	}
	hasher := sha256.New()
	reader := io.LimitReader(source, info.Size()+1)
	written, copyErr := copyContext(ctx, io.MultiWriter(file, hasher), reader)
	syncErr := file.Sync()
	closeErr := file.Close()
	digest := hex.EncodeToString(hasher.Sum(nil))
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return Artifact{}, err
	}
	if written != info.Size() || digest != id.String()[7:] {
		return Artifact{}, blob.ErrCorrupt
	}
	return Artifact{Path: relative, Size: written, SHA256: digest, BlobID: id.String()}, nil
}

func copyVerifiedArtifact(ctx context.Context, root *retainedDirectory, artifact Artifact, destination string) error {
	source, sourceInfo, err := openRootRegularFile(root, artifact.Path)
	if err != nil {
		return err
	}
	destinationFile, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = source.Close()
		return err
	}
	hasher := sha256.New()
	written, copyErr := copyContext(ctx, io.MultiWriter(destinationFile, hasher), io.LimitReader(source, artifact.Size+1))
	stableSourceErr := verifyOpenedRootFile(root, artifact.Path, source, sourceInfo)
	syncErr := destinationFile.Sync()
	closeDestinationErr := destinationFile.Close()
	closeSourceErr := source.Close()
	if err := errors.Join(copyErr, stableSourceErr, syncErr, closeDestinationErr, closeSourceErr); err != nil {
		return err
	}
	if written != artifact.Size || hex.EncodeToString(hasher.Sum(nil)) != artifact.SHA256 {
		return errors.New("artifact changed while restoring")
	}
	return nil
}

func copyContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, copyBufferBytes)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, readErr := source.Read(buffer)
		if n > 0 {
			written, writeErr := destination.Write(buffer[:n])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return total, nil
			}
			return total, readErr
		}
	}
}

func rejectDuplicateJSONFields(ctx context.Context, data []byte) error {
	return rejectDuplicateJSONFieldsBounded(ctx, data, maxManifestJSONDepth, maxManifestJSONTokens)
}

func rejectDuplicateJSONFieldsBounded(ctx context.Context, data []byte, maxDepth, maxTokens int) error {
	if ctx == nil {
		return errors.New("backup: nil JSON validation context")
	}
	if maxDepth < 1 || maxTokens < 1 {
		return errors.New("backup: invalid JSON validation bounds")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	tokenCount := 0
	nextToken := func() (json.Token, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if tokenCount >= maxTokens {
			return nil, fmt.Errorf("backup: manifest JSON token count exceeds limit %d", maxTokens)
		}
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		tokenCount++
		return token, nil
	}
	var walk func(int) error
	walk = func(depth int) error {
		if depth > maxDepth {
			return fmt.Errorf("backup: manifest JSON depth exceeds limit %d", maxDepth)
		}
		token, err := nextToken()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := nextToken()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("object key is not text")
				}
				if _, duplicate := seen[key]; duplicate {
					return fmt.Errorf("duplicate field %q", key)
				}
				seen[key] = struct{}{}
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			closing, err := nextToken()
			if err != nil || closing != json.Delim('}') {
				return errors.New("invalid object ending")
			}
		case '[':
			for decoder.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			closing, err := nextToken()
			if err != nil || closing != json.Delim(']') {
				return errors.New("invalid array ending")
			}
		default:
			return errors.New("unexpected JSON delimiter")
		}
		return nil
	}
	if err := walk(1); err != nil {
		return err
	}
	if _, err := nextToken(); err == nil {
		return errors.New("trailing JSON value")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func newDestination(raw string) (*destinationTarget, error) {
	if strings.TrimSpace(raw) == "" || strings.ContainsRune(raw, '\x00') || !validPlatformDestinationInput(raw) {
		return nil, errors.New("backup: destination is empty or invalid")
	}
	finalPath, err := filepath.Abs(raw)
	if err != nil {
		return nil, fmt.Errorf("backup: resolve destination: %w", err)
	}
	parent, err := openRetainedDirectory(filepath.Dir(finalPath))
	if err != nil {
		return nil, fmt.Errorf("backup: destination parent: %w", err)
	}
	if err := parent.acquireSyncHandle(); err != nil {
		_ = parent.Close()
		return nil, fmt.Errorf("backup: destination parent sync capability: %w", err)
	}
	finalName := filepath.Base(finalPath)
	if !validDestinationLeaf(finalName) {
		_ = parent.Close()
		return nil, errors.New("backup: destination leaf is invalid")
	}
	return &destinationTarget{finalPath: finalPath, finalName: finalName, parent: parent}, nil
}

func ensureDestinationAbsent(target *destinationTarget) error {
	if target == nil || target.parent == nil || target.parent.root == nil || !validDestinationLeaf(target.finalName) {
		return errors.New("backup: invalid destination capability")
	}
	if _, err := target.parent.root.Lstat(target.finalName); err == nil {
		return errors.New("backup: destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("backup: inspect destination: %w", err)
	}
	return target.parent.verifyPath()
}

func existingDirectory(raw string) (string, error) {
	return vault.ValidateExistingDirectory(raw)
}

func openRetainedDirectory(raw string) (*retainedDirectory, error) {
	candidate, err := filepath.Abs(raw)
	if err != nil {
		return nil, err
	}
	// The pre-validation handle prevents a rename on platforms whose directory
	// handles deny delete sharing, and supplies an identity witness elsewhere.
	// No data is read through it until the path passes Vault validation.
	witness, err := os.OpenRoot(filepath.Clean(candidate))
	if err != nil {
		return nil, err
	}
	witnessInfo, err := witness.Stat(".")
	if err != nil {
		_ = witness.Close()
		return nil, err
	}
	path, err := existingDirectory(raw)
	if err != nil {
		_ = witness.Close()
		return nil, err
	}
	validated, err := os.Lstat(path)
	if err != nil {
		_ = witness.Close()
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		_ = witness.Close()
		return nil, err
	}
	fail := func(cause error) (*retainedDirectory, error) {
		return nil, errors.Join(cause, root.Close(), witness.Close())
	}
	retained, err := root.Stat(".")
	if err != nil {
		return fail(err)
	}
	current, err := os.Lstat(path)
	if err != nil {
		return fail(err)
	}
	if validated.Mode()&os.ModeSymlink != 0 || current.Mode()&os.ModeSymlink != 0 ||
		!validated.IsDir() || !current.IsDir() || !retained.IsDir() ||
		!witnessInfo.IsDir() || !os.SameFile(witnessInfo, retained) ||
		!os.SameFile(validated, retained) || !os.SameFile(current, retained) {
		return fail(errors.New("backup: validated directory identity changed while retaining root"))
	}
	if err := witness.Close(); err != nil {
		return nil, errors.Join(err, root.Close())
	}
	return &retainedDirectory{
		path: path, root: root,
		identity: directoryIdentity{info: retained},
	}, nil
}

func (directory *retainedDirectory) acquireSyncHandle() error {
	if directory == nil || directory.root == nil || directory.identity.info == nil {
		return errors.New("backup: invalid retained directory sync upgrade")
	}
	if directory.syncHandle != nil {
		return nil
	}
	before, err := directory.root.Stat(".")
	if err != nil {
		return err
	}
	beforePath, err := os.Lstat(directory.path)
	if err != nil {
		return err
	}
	if beforePath.Mode()&os.ModeSymlink != 0 || !beforePath.IsDir() ||
		!os.SameFile(before, directory.identity.info) || !os.SameFile(beforePath, before) {
		return errors.New("backup: retained directory changed before sync upgrade")
	}
	if err := validateDirectoryBoundary(before, beforePath); err != nil {
		return err
	}
	handle, err := openDirectorySyncHandle(directory.path)
	if err != nil {
		return err
	}
	fail := func(cause error) error { return errors.Join(cause, handle.Close()) }
	handleInfo, err := handle.Stat()
	if err != nil {
		return fail(err)
	}
	after, afterErr := directory.root.Stat(".")
	afterPath, pathErr := os.Lstat(directory.path)
	if err := errors.Join(afterErr, pathErr); err != nil {
		return fail(err)
	}
	if afterPath.Mode()&os.ModeSymlink != 0 || !handleInfo.IsDir() || !after.IsDir() || !afterPath.IsDir() ||
		!os.SameFile(before, handleInfo) || !os.SameFile(after, handleInfo) ||
		!os.SameFile(afterPath, handleInfo) || !os.SameFile(directory.identity.info, handleInfo) {
		return fail(errors.New("backup: directory identity changed during sync upgrade"))
	}
	if err := errors.Join(
		validateDirectoryBoundary(after, handleInfo),
		validateDirectoryBoundary(after, afterPath),
	); err != nil {
		return fail(err)
	}
	directory.syncHandle = handle
	return nil
}

func (directory *retainedDirectory) Close() error {
	if directory == nil || directory.root == nil {
		return nil
	}
	rootErr := directory.root.Close()
	var syncErr error
	if directory.syncHandle != nil {
		syncErr = directory.syncHandle.Close()
	}
	err := errors.Join(rootErr, syncErr)
	directory.root = nil
	directory.syncHandle = nil
	return err
}

func (directory *retainedDirectory) verifyPath() error {
	if directory == nil || directory.root == nil {
		return errors.New("backup: retained directory is closed")
	}
	retained, err := directory.root.Stat(".")
	if err != nil {
		return err
	}
	current, err := os.Lstat(directory.path)
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !retained.IsDir() || !os.SameFile(current, retained) {
		return errors.New("backup: retained directory no longer matches its path")
	}
	return nil
}

type stagingCreationHooks struct {
	afterReceiptTempCreate func(*os.File) error
	afterReceiptDurable    func(*stagingDirectory) error
	afterAtomicCreate      func(*stagingDirectory) error
	afterRootAdopted       func(*stagingDirectory) error
	afterChildSync         func(*stagingDirectory) error
	afterDirectoryDurable  func(*stagingDirectory) error
	afterBindingDurable    func(*stagingDirectory) error
	persistBinding         func(*retainedDirectory, *stagingDirectory) error
}

func createStagingDirectory(parent *retainedDirectory, prefix, destinationName string) (*stagingDirectory, error) {
	return createStagingDirectoryWithHooks(parent, prefix, destinationName, stagingCreationHooks{})
}

func createStagingDirectoryWithHooks(
	parent *retainedDirectory,
	prefix, destinationName string,
	hooks stagingCreationHooks,
) (*stagingDirectory, error) {
	if parent == nil || parent.root == nil || prefix == "" || !validResidueLeaf(destinationName) {
		return nil, errors.New("backup: invalid staging parent")
	}
	kind, err := residueKindForPrefix(prefix)
	if err != nil {
		return nil, err
	}
	if err := parent.acquireSyncHandle(); err != nil {
		return nil, fmt.Errorf("backup: acquire staging parent sync capability: %w", err)
	}
	for range 100 {
		var entropy [16]byte
		if _, err := rand.Read(entropy[:]); err != nil {
			return nil, err
		}
		operationID := hex.EncodeToString(entropy[:])
		name := prefix + operationID
		if name == destinationName {
			continue
		}
		staging := &stagingDirectory{
			name:            name,
			operationID:     operationID,
			kind:            kind,
			destinationName: destinationName,
		}
		if err := createResidueReceiptWithHooks(parent, staging, nil, hooks.afterReceiptTempCreate); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return nil, errors.Join(err, closeResidueLease(staging))
		}
		if hooks.afterReceiptDurable != nil {
			if err := hooks.afterReceiptDurable(staging); err != nil {
				return nil, errors.Join(err, removeResidueReceipt(parent, staging), closeResidueLease(staging))
			}
		}
		creationWitness, created, err := createRetainedStagingLeaf(parent, name)
		if err != nil {
			if errors.Is(err, ErrCleanupResidual) {
				return nil, errors.Join(err, closeResidueLease(staging))
			}
			removeErr := removeResidueReceipt(parent, staging)
			if errors.Is(err, os.ErrExist) && removeErr == nil {
				continue
			}
			return nil, errors.Join(err, removeErr, closeResidueLease(staging))
		}
		staging.creationWitness = creationWitness
		if hooks.afterAtomicCreate != nil {
			if err := hooks.afterAtomicCreate(staging); err != nil {
				return nil, retainFailedStaging(staging, err)
			}
		}
		entry, err := parent.root.Lstat(name)
		if err != nil || entry.Mode()&os.ModeSymlink != 0 || !entry.IsDir() || !os.SameFile(created, entry) {
			if err == nil {
				err = errors.New("backup: created staging leaf differs from its retained creation handle")
			}
			return nil, retainFailedStaging(staging, err)
		}
		identity := directoryIdentity{info: created}
		root, err := parent.root.OpenRoot(name)
		if err != nil {
			return nil, retainFailedStaging(staging, err)
		}
		entryAfter, entryErr := parent.root.Lstat(name)
		retained, retainedErr := root.Stat(".")
		if err := errors.Join(entryErr, retainedErr); err != nil || entryAfter.Mode()&os.ModeSymlink != 0 ||
			!entryAfter.IsDir() || !retained.IsDir() || !os.SameFile(created, retained) || !os.SameFile(entryAfter, retained) {
			_ = root.Close()
			if err != nil {
				return nil, retainFailedStaging(staging, err)
			}
			return nil, retainFailedStaging(staging, errors.New("backup: staging directory identity mismatch"))
		}
		path := filepath.Join(parent.path, name)
		if err := parent.verifyPath(); err != nil {
			_ = root.Close()
			return nil, retainFailedStaging(staging, err)
		}
		directory := &retainedDirectory{path: path, root: root, identity: identity}
		staging.directory = directory
		if hooks.afterRootAdopted != nil {
			if err := hooks.afterRootAdopted(staging); err != nil {
				return nil, retainFailedStaging(staging, err)
			}
		}
		if err := directory.acquireSyncHandle(); err != nil {
			return nil, retainFailedStaging(staging, err)
		}
		witnessToken, err := persistentStagingWitnessIdentityToken(staging.creationWitness)
		if err != nil {
			return nil, retainFailedStaging(staging, err)
		}
		identityToken, err := persistentDirectoryIdentityToken(directory)
		if err != nil || witnessToken != identityToken {
			if err == nil {
				err = errors.New("backup: staging witness and retained root identity differ")
			}
			return nil, retainFailedStaging(staging, err)
		}
		staging.identityToken = witnessToken
		if err := syncRetainedDirectory(directory); err != nil {
			return nil, retainFailedStaging(staging, fmt.Errorf("backup: sync newly created staging directory: %w", err))
		}
		if hooks.afterChildSync != nil {
			if err := hooks.afterChildSync(staging); err != nil {
				return nil, retainFailedStaging(staging, err)
			}
		}
		if err := syncRetainedDirectory(parent); err != nil {
			return nil, retainFailedStaging(staging, fmt.Errorf("backup: sync staging parent after creation: %w", err))
		}
		if hooks.afterDirectoryDurable != nil {
			if err := hooks.afterDirectoryDurable(staging); err != nil {
				return nil, retainFailedStaging(staging, err)
			}
		}
		persistBinding := createResidueBinding
		if hooks.persistBinding != nil {
			persistBinding = hooks.persistBinding
		}
		if err := persistBinding(parent, staging); err != nil {
			return nil, retainFailedStaging(staging, err)
		}
		if hooks.afterBindingDurable != nil {
			if err := hooks.afterBindingDurable(staging); err != nil {
				return nil, retainFailedStaging(staging, err)
			}
		}
		return staging, nil
	}
	return nil, errors.New("backup: could not allocate a unique staging directory")
}

func closeStagingCreationWitness(staging *stagingDirectory) error {
	if staging == nil || staging.creationWitness == nil {
		return nil
	}
	witness := staging.creationWitness
	staging.creationWitness = nil
	return witness.Close()
}

func retainFailedStaging(staging *stagingDirectory, failures ...error) error {
	var directoryErr error
	if staging != nil && staging.directory != nil {
		directoryErr = staging.directory.Close()
	}
	return errors.Join(
		ErrCleanupResidual,
		errors.Join(failures...),
		directoryErr,
		closeStagingCreationWitness(staging),
		closeResidueLease(staging),
	)
}

func publishPreparedDirectory(
	ctx context.Context,
	staging *stagingDirectory,
	target *destinationTarget,
	verify publicationVerifier,
	hooks publicationHooks,
) (publicationResult, error) {
	if staging == nil || staging.directory == nil || target == nil || target.parent == nil {
		return publicationResult{}, errors.New("backup: invalid prepared publication")
	}
	if ctx == nil || verify == nil {
		return publicationResult{}, errors.New("backup: publication requires context and content verifier")
	}
	if err := ctx.Err(); err != nil {
		return publicationResult{}, err
	}
	expected := staging.directory.identity
	if err := target.parent.acquireSyncHandle(); err != nil {
		return publicationResult{}, fmt.Errorf("backup: acquire publication parent sync capability: %w", err)
	}
	if err := staging.directory.verifyPath(); err != nil {
		return publicationResult{}, fmt.Errorf("backup: staging directory changed before publication: %w", err)
	}
	if staging.destinationName != target.finalName {
		return publicationResult{}, errors.New("backup: staging receipt destination differs from publication target")
	}
	if err := staging.directory.Close(); err != nil {
		return publicationResult{}, fmt.Errorf("backup: close staged directory before publication: %w", err)
	}
	if err := closeStagingCreationWitness(staging); err != nil {
		return publicationResult{}, fmt.Errorf("backup: close staging creation witness before publication: %w", err)
	}
	if hooks.afterStagingCloseBeforeRename != nil {
		if err := hooks.afterStagingCloseBeforeRename(staging.directory.path); err != nil {
			return publicationResult{}, fmt.Errorf("backup: publication hook: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return publicationResult{}, err
	}
	if _, err := verifyLeafIdentity(target.parent, staging.name, expected); err != nil {
		return publicationResult{}, errors.Join(
			ErrPublicationUncertain,
			fmt.Errorf("backup: staging identity changed before atomic publication: %w", err),
		)
	}
	publication, err := publishDirectory(
		staging.directory.path,
		target.finalPath,
		target.parent,
		target.finalName,
		publicationAttempt{
			sourceName: staging.name,
			expected:   expected,
			ctx:        ctx,
			verify:     verify,
			hooks:      hooks,
		},
	)
	if err != nil {
		return publication, err
	}
	if !publication.stagingConsumed {
		return publication, errors.New("backup: successful publication did not consume staging")
	}
	if err := removeResidueReceipt(target.parent, staging); err != nil {
		return publication, errors.Join(ErrCleanupResidual, fmt.Errorf("backup: remove published receipt: %w", err))
	}
	return publication, nil
}

func verifyLeafIdentity(parent *retainedDirectory, name string, expected directoryIdentity) (os.FileInfo, error) {
	if parent == nil || parent.root == nil || expected.info == nil {
		return nil, errors.New("backup: invalid retained leaf identity")
	}
	parentInfo, err := parent.root.Stat(".")
	if err != nil {
		return nil, err
	}
	current, err := parent.root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !os.SameFile(current, expected.info) {
		return nil, errors.New("backup: directory leaf identity mismatch")
	}
	if err := validateDirectoryBoundary(parentInfo, current); err != nil {
		return nil, err
	}
	return current, nil
}

func openExpectedDirectory(parent *retainedDirectory, name string, expected directoryIdentity) (*retainedDirectory, error) {
	if _, err := verifyLeafIdentity(parent, name, expected); err != nil {
		return nil, err
	}
	parentInfo, err := parent.root.Stat(".")
	if err != nil {
		return nil, err
	}
	root, err := parent.root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	retained, retainedErr := root.Stat(".")
	after, afterErr := parent.root.Lstat(name)
	if err := errors.Join(retainedErr, afterErr); err != nil || retained.Mode()&os.ModeSymlink != 0 ||
		!retained.IsDir() || after.Mode()&os.ModeSymlink != 0 || !after.IsDir() ||
		!os.SameFile(retained, expected.info) || !os.SameFile(after, expected.info) {
		_ = root.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("backup: directory leaf changed while retaining it")
	}
	if err := errors.Join(
		validateDirectoryBoundary(parentInfo, retained),
		validateDirectoryBoundary(parentInfo, after),
	); err != nil {
		_ = root.Close()
		return nil, err
	}
	return &retainedDirectory{
		path: filepath.Join(parent.path, name), root: root,
		identity: expected,
	}, nil
}

func retainUncertainLeaf(parent *retainedDirectory, name string, expected directoryIdentity) error {
	if _, err := verifyLeafIdentity(parent, name, expected); err != nil {
		return errors.Join(ErrCleanupResidual, ErrCleanupIdentityLost, err)
	}
	// os.Root has no identity-bound unlink/rmdir operation. Even an empty leaf
	// can be swapped after the check above, so retain it for explicit operator
	// cleanup rather than deleting an attacker-selected name.
	return ErrCleanupResidual
}

func openRootRegularFile(root *retainedDirectory, relative string) (*os.File, os.FileInfo, error) {
	if root == nil || root.root == nil || !safeRelativePath(relative) {
		return nil, nil, errors.New("backup: unsafe root-relative artifact path")
	}
	if err := validateRootParents(root, relative); err != nil {
		return nil, nil, err
	}
	name := filepath.FromSlash(relative)
	before, err := root.root.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > maxArtifactBytes {
		return nil, nil, errors.New("backup: artifact is not a bounded direct regular file")
	}
	file, err := root.root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		_ = file.Close()
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errors.New("backup: artifact identity changed while opening")
	}
	after, err := root.root.Lstat(name)
	if err != nil || after.Mode()&os.ModeSymlink != 0 || !after.Mode().IsRegular() || !os.SameFile(after, opened) {
		_ = file.Close()
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errors.New("backup: artifact path changed while opening")
	}
	if err := validateRootParents(root, relative); err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return file, opened, nil
}

func verifyOpenedRootFile(root *retainedDirectory, relative string, file *os.File, opened os.FileInfo) error {
	if err := validateRootParents(root, relative); err != nil {
		return err
	}
	currentFile, err := file.Stat()
	if err != nil {
		return err
	}
	currentPath, err := root.root.Lstat(filepath.FromSlash(relative))
	if err != nil {
		return err
	}
	if currentPath.Mode()&os.ModeSymlink != 0 || !currentPath.Mode().IsRegular() ||
		!currentFile.Mode().IsRegular() || currentFile.Size() != opened.Size() ||
		!os.SameFile(opened, currentFile) || !os.SameFile(currentPath, currentFile) {
		return errors.New("backup: artifact changed while open")
	}
	return nil
}

func validateRootParents(root *retainedDirectory, relative string) error {
	rootInfo, err := root.root.Stat(".")
	if err != nil {
		return err
	}
	if err := validateDirectoryBoundary(rootInfo, rootInfo); err != nil {
		return err
	}
	components := strings.Split(relative, "/")
	current := ""
	for _, component := range components[:len(components)-1] {
		if current == "" {
			current = component
		} else {
			current += "/" + component
		}
		info, err := root.root.Lstat(filepath.FromSlash(current))
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("backup: artifact parent is not a direct directory: %s", current)
		}
		if err := validateDirectoryBoundary(rootInfo, info); err != nil {
			return fmt.Errorf("backup: artifact parent crosses a filesystem boundary %s: %w", current, err)
		}
	}
	return nil
}

func syncRetainedDirectory(directory *retainedDirectory) error {
	if directory == nil || directory.root == nil || directory.syncHandle == nil {
		return errors.New("backup: retained directory is closed")
	}
	return directory.syncHandle.Sync()
}

func safeRelativePath(value string) bool {
	return value != "" && !strings.Contains(value, "\\") && !strings.ContainsRune(value, '\x00') &&
		!filepath.IsAbs(filepath.FromSlash(value)) && filepath.ToSlash(filepath.Clean(filepath.FromSlash(value))) == value &&
		value != "." && value != ".." && !strings.HasPrefix(value, "../")
}

func blobArtifactPath(id blob.BlobID) string {
	digest := id.String()[7:]
	return "blobs/objects/sha256/" + digest[:2] + "/" + digest[2:]
}

func removeSQLiteSidecarsRoot(root *retainedDirectory, relative string) error {
	var failures []error
	for _, sidecar := range []string{relative + "-wal", relative + "-shm"} {
		if err := root.root.Remove(filepath.FromSlash(sidecar)); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, fmt.Errorf("remove %s: %w", filepath.Base(sidecar), err))
		}
	}
	return errors.Join(failures...)
}

func lowerSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func syncBackupTreeDirectoriesRoot(ctx context.Context, root *retainedDirectory, manifest Manifest) error {
	files, directories, err := expectedBackupTree(ctx, manifest)
	if err != nil {
		return err
	}
	return syncExpectedTreeDirectoriesRoot(ctx, root, files, directories, "backup tree")
}

func syncRestoredTreeDirectoriesRoot(ctx context.Context, root *retainedDirectory, blobs []Artifact) error {
	files, directories, err := expectedRestoredVaultTree(ctx, blobs)
	if err != nil {
		return err
	}
	return syncExpectedTreeDirectoriesRoot(ctx, root, files, directories, "restored Vault")
}

func syncExpectedTreeDirectoriesRoot(
	ctx context.Context,
	root *retainedDirectory,
	files map[string]bool,
	directories map[string]struct{},
	label string,
) error {
	if ctx == nil {
		return errors.New("backup: nil tree sync context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := root.acquireSyncHandle(); err != nil {
		return fmt.Errorf("backup: acquire tree root sync capability: %w", err)
	}
	return walkExpectedTreeRoot(ctx, root, files, directories, label, syncExpectedDirectory)
}

func syncExpectedDirectory(
	ctx context.Context,
	root *retainedDirectory,
	relative string,
	expected os.FileInfo,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rootInfo, err := root.root.Stat(".")
	if err != nil {
		return err
	}
	before, err := root.root.Lstat(filepath.FromSlash(relative))
	if err != nil {
		return err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.IsDir() || !os.SameFile(before, expected) {
		return fmt.Errorf("backup: staged directory identity changed before sync: %s", relative)
	}
	if err := validateDirectoryBoundary(rootInfo, before); err != nil {
		return fmt.Errorf("backup: staged directory crosses a filesystem boundary %s: %w", relative, err)
	}
	if relative == "." {
		if err := syncRetainedDirectory(root); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		after, err := root.root.Stat(".")
		if err != nil || !after.IsDir() || !os.SameFile(after, expected) {
			if err != nil {
				return err
			}
			return errors.New("backup: staged root identity changed after sync")
		}
		return validateDirectoryBoundary(rootInfo, after)
	}

	absolute := filepath.Join(root.path, filepath.FromSlash(relative))
	current, err := os.Lstat(absolute)
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !os.SameFile(expected, current) {
		return fmt.Errorf("backup: staged directory path identity changed: %s", relative)
	}
	handle, err := openDirectorySyncHandle(absolute)
	if err != nil {
		return err
	}
	handleInfo, statErr := handle.Stat()
	after, afterErr := root.root.Lstat(filepath.FromSlash(relative))
	if joined := errors.Join(statErr, afterErr); joined != nil || !handleInfo.IsDir() ||
		after.Mode()&os.ModeSymlink != 0 || !after.IsDir() ||
		!os.SameFile(expected, handleInfo) || !os.SameFile(after, handleInfo) {
		if joined == nil {
			joined = errors.New("backup: staged directory sync handle identity mismatch")
		}
		return errors.Join(joined, handle.Close())
	}
	syncErr := handle.Sync()
	closeErr := handle.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	final, err := root.root.Lstat(filepath.FromSlash(relative))
	if err != nil || final.Mode()&os.ModeSymlink != 0 || !final.IsDir() || !os.SameFile(final, expected) {
		if err != nil {
			return err
		}
		return errors.New("backup: staged directory identity changed after sync")
	}
	return validateDirectoryBoundary(rootInfo, final)
}
