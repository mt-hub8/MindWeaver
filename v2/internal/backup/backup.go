// Package backup creates and restores complete, self-verifying local Vault
// backups. A backup is a directory with one SQLite snapshot, only the immutable
// blobs referenced by that snapshot, and a bounded versioned manifest.
package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
)

const (
	FormatVersion    = 1
	manifestFileName = "manifest.json"
	databasePath     = "data/" + store.DatabaseFileName
	maxManifestBytes = 16 << 20
	maxArtifactBytes = int64(1 << 40)
	copyBufferBytes  = 128 << 10
	stagingPrefix    = ".mindweaver-backup-"
	restorePrefix    = ".mindweaver-restore-"
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
	database *store.Store
	blobs    *blob.Store
	now      func() time.Time
}

// New constructs a concrete Vault backup coordinator.
func New(database *store.Store, blobs *blob.Store) (*Coordinator, error) {
	if database == nil {
		return nil, errors.New("backup: nil database")
	}
	if blobs == nil {
		return nil, errors.New("backup: nil blob store")
	}
	return &Coordinator{database: database, blobs: blobs, now: time.Now}, nil
}

// Create publishes a new backup directory. destination and its parent are
// never overwritten. Blob deletion is pinned from before the SQLite snapshot
// until every snapshot-referenced object has been copied and verified.
func (c *Coordinator) Create(ctx context.Context, destination string) (result Manifest, resultErr error) {
	if c == nil || c.database == nil || c.blobs == nil || c.now == nil {
		return Manifest{}, errors.New("backup: coordinator is not initialized")
	}
	if ctx == nil {
		return Manifest{}, errors.New("backup: nil context")
	}
	finalPath, parent, err := newDestination(destination)
	if err != nil {
		return Manifest{}, err
	}
	pin, err := c.blobs.PinObjectsContext(ctx)
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: pin immutable objects: %w", err)
	}
	defer pin.Release()

	staging, err := os.MkdirTemp(parent, stagingPrefix)
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: create staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			resultErr = errors.Join(resultErr, cleanupStagingTree(staging, parent, "backup"))
		}
	}()
	if err := os.Chmod(staging, 0o700); err != nil {
		return Manifest{}, fmt.Errorf("backup: secure staging directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(staging, "data"), 0o700); err != nil {
		return Manifest{}, fmt.Errorf("backup: create data directory: %w", err)
	}

	snapshotPath := filepath.Join(staging, filepath.FromSlash(databasePath))
	if err := c.database.BackupSnapshot(ctx, snapshotPath); err != nil {
		return Manifest{}, fmt.Errorf("backup: snapshot database: %w", err)
	}
	snapshot, err := store.Open(ctx, snapshotPath, store.Options{})
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: open database snapshot: %w", err)
	}
	version, versionErr := snapshot.SchemaVersion(ctx)
	references, referencesErr := snapshot.ReferencedBlobIDs(ctx)
	integrityErr := snapshot.IntegrityCheck(ctx)
	closeErr := snapshot.Close()
	if err := errors.Join(versionErr, referencesErr, integrityErr, closeErr); err != nil {
		return Manifest{}, fmt.Errorf("backup: verify database snapshot: %w", err)
	}
	if err := removeSQLiteSidecars(snapshotPath); err != nil {
		return Manifest{}, fmt.Errorf("backup: remove snapshot sidecars: %w", err)
	}
	databaseArtifact, err := inspectFile(snapshotPath, databasePath, "")
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: hash database snapshot: %w", err)
	}

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
		id, err := blob.ParseID(rawID)
		if err != nil {
			return Manifest{}, fmt.Errorf("backup: snapshot contains invalid blob reference: %w", err)
		}
		relative := blobArtifactPath(id)
		destinationPath := filepath.Join(staging, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o700); err != nil {
			return Manifest{}, fmt.Errorf("backup: create blob directory: %w", err)
		}
		source, err := c.blobs.Open(id)
		if err != nil {
			return Manifest{}, fmt.Errorf("backup: open referenced blob %s: %w", id, err)
		}
		artifact, copyErr := copyOpenedBlob(ctx, source, destinationPath, relative, id)
		closeErr := source.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return Manifest{}, fmt.Errorf("backup: copy referenced blob %s: %w", id, err)
		}
		manifest.Blobs = append(manifest.Blobs, artifact)
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, fmt.Errorf("backup: validate generated manifest: %w", err)
	}
	if err := writeManifest(filepath.Join(staging, manifestFileName), manifest); err != nil {
		return Manifest{}, err
	}
	if err := verifyBackupTree(ctx, staging, manifest); err != nil {
		return Manifest{}, fmt.Errorf("backup: verify staged backup: %w", err)
	}
	if err := syncTreeDirectories(staging); err != nil {
		return Manifest{}, fmt.Errorf("backup: sync staged backup directories: %w", err)
	}
	if err := publishDirectory(staging, finalPath, parent); err != nil {
		return Manifest{}, fmt.Errorf("backup: publish backup directory: %w", err)
	}
	published = true
	return manifest, nil
}

// Restore verifies a backup without modifying it, builds a new Vault under a
// generated sibling directory, verifies the restored database and exact blob
// reachability, and only then atomically renames it to destinationVault. It
// never merges into or overwrites an existing Vault.
func Restore(ctx context.Context, backupDirectory, destinationVault string) (resultErr error) {
	if ctx == nil {
		return errors.New("backup: nil context")
	}
	backupRoot, err := existingDirectory(backupDirectory)
	if err != nil {
		return fmt.Errorf("backup: open backup: %w", err)
	}
	finalPath, parent, err := newDestination(destinationVault)
	if err != nil {
		return fmt.Errorf("backup: restore destination: %w", err)
	}
	if pathsOverlap(backupRoot, finalPath) {
		return errors.New("backup: backup and restore destination must not overlap")
	}
	manifest, err := readManifest(filepath.Join(backupRoot, manifestFileName))
	if err != nil {
		return err
	}
	if err := verifyBackupTree(ctx, backupRoot, manifest); err != nil {
		return fmt.Errorf("backup: backup verification failed: %w", err)
	}

	staging, err := os.MkdirTemp(parent, restorePrefix)
	if err != nil {
		return fmt.Errorf("backup: create restore staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			resultErr = errors.Join(resultErr, cleanupStagingTree(staging, parent, "restore"))
		}
	}()
	if err := os.Chmod(staging, 0o700); err != nil {
		return fmt.Errorf("backup: secure restore staging directory: %w", err)
	}
	ownedVault, err := vault.Open(staging)
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
		sourcePath, err := resolveArtifactPath(backupRoot, artifact.Path)
		if err != nil {
			return err
		}
		source, err := os.Open(sourcePath)
		if err != nil {
			return fmt.Errorf("backup: open blob artifact: %w", err)
		}
		imported, importErr := restoredBlobs.Import(ctx, source, artifact.Size)
		closeErr := source.Close()
		if err := errors.Join(importErr, closeErr); err != nil {
			return fmt.Errorf("backup: import restored blob %s: %w", id, err)
		}
		if imported.ID != id || imported.Size != artifact.Size {
			return fmt.Errorf("backup: restored blob %s identity mismatch", id)
		}
	}

	restoredDatabase, err := store.Open(ctx, databaseDestination, store.Options{})
	if err != nil {
		return fmt.Errorf("backup: open restored database: %w", err)
	}
	integrityErr := restoredDatabase.IntegrityCheck(ctx)
	references, referencesErr := restoredDatabase.ReferencedBlobIDs(ctx)
	closeErr := restoredDatabase.Close()
	if err := errors.Join(integrityErr, referencesErr, closeErr); err != nil {
		return fmt.Errorf("backup: verify restored database: %w", err)
	}
	wantReferences := make([]string, len(manifest.Blobs))
	for index := range manifest.Blobs {
		wantReferences[index] = manifest.Blobs[index].BlobID
	}
	if !equalStrings(references, wantReferences) {
		return fmt.Errorf("backup: restored blob references differ from manifest")
	}
	if err := ownedVault.Close(); err != nil {
		return fmt.Errorf("backup: close restored Vault: %w", err)
	}
	vaultOpen = false
	if err := syncTreeDirectories(staging); err != nil {
		return fmt.Errorf("backup: sync restored Vault directories: %w", err)
	}
	if err := publishDirectory(staging, finalPath, parent); err != nil {
		return fmt.Errorf("backup: publish restored Vault: %w", err)
	}
	published = true
	return nil
}

func cleanupStagingTree(path, parent, kind string) error {
	removeErr := os.RemoveAll(path)
	var residualErr error
	if _, err := os.Lstat(path); err == nil {
		residualErr = fmt.Errorf("%s staging data remains at %s", kind, path)
	} else if !errors.Is(err, os.ErrNotExist) {
		residualErr = fmt.Errorf("inspect %s staging residue at %s: %w", kind, path, err)
	}
	syncErr := syncDirectoryPath(parent)
	if removeErr != nil {
		removeErr = fmt.Errorf("remove %s staging data at %s: %w", kind, path, removeErr)
	}
	if syncErr != nil {
		syncErr = fmt.Errorf("sync %s staging parent after cleanup: %w", kind, syncErr)
	}
	return errors.Join(removeErr, residualErr, syncErr)
}

func wrapBackupError(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

func readManifest(path string) (Manifest, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: inspect manifest: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Manifest{}, errors.New("backup: manifest is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: open manifest: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return Manifest{}, fmt.Errorf("backup: read manifest: %w", err)
	}
	if len(data) > maxManifestBytes {
		return Manifest{}, errors.New("backup: manifest exceeds size limit")
	}
	if err := rejectDuplicateJSONFields(data); err != nil {
		return Manifest{}, fmt.Errorf("backup: invalid manifest JSON: %w", err)
	}
	if err := validateManifestJSONSchema(data); err != nil {
		return Manifest{}, fmt.Errorf("backup: invalid manifest schema: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("backup: decode manifest: %w", err)
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

func validateManifestJSONSchema(data []byte) error {
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
	for index, raw := range blobs {
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

func writeManifest(path string, manifest Manifest) error {
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("backup: encode manifest: %w", err)
	}
	encoded = append(encoded, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
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
	if manifest.Database.BlobID != "" {
		return errors.New("database artifact must not have a blob ID")
	}
	if err := validateArtifact(manifest.Database, databasePath, ""); err != nil {
		return fmt.Errorf("database artifact: %w", err)
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

func verifyBackupTree(ctx context.Context, root string, manifest Manifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := verifyArtifact(root, manifest.Database); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	for _, artifact := range manifest.Blobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := verifyArtifact(root, artifact); err != nil {
			return fmt.Errorf("blob %s: %w", artifact.BlobID, err)
		}
	}
	return verifyExactTree(root, manifest)
}

func verifyExactTree(root string, manifest Manifest) error {
	allowedFiles := map[string]struct{}{
		manifestFileName:       {},
		manifest.Database.Path: {},
	}
	allowedDirectories := map[string]struct{}{".": {}}
	addParents := func(relative string) {
		for directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relative))); directory != "."; directory = filepath.ToSlash(filepath.Dir(filepath.FromSlash(directory))) {
			allowedDirectories[directory] = struct{}{}
		}
	}
	addParents(manifestFileName)
	addParents(manifest.Database.Path)
	for _, artifact := range manifest.Blobs {
		allowedFiles[artifact.Path] = struct{}{}
		addParents(artifact.Path)
	}

	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("backup: untrusted link in backup tree: %s", relative)
		}
		if entry.IsDir() {
			if _, ok := allowedDirectories[relative]; !ok {
				return fmt.Errorf("backup: unlisted directory in backup tree: %s", relative)
			}
			if _, err := vault.ValidateExistingDirectory(path); err != nil {
				return err
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("backup: non-regular artifact in backup tree: %s", relative)
		}
		if _, ok := allowedFiles[relative]; !ok {
			return fmt.Errorf("backup: unlisted artifact in backup tree: %s", relative)
		}
		return nil
	})
}

func verifyArtifact(root string, artifact Artifact) error {
	path, err := resolveArtifactPath(root, artifact.Path)
	if err != nil {
		return err
	}
	actual, err := inspectFile(path, artifact.Path, artifact.BlobID)
	if err != nil {
		return err
	}
	if actual.Size != artifact.Size || actual.SHA256 != artifact.SHA256 {
		return errors.New("size or SHA-256 mismatch")
	}
	return nil
}

func inspectFile(path, relative, blobID string) (Artifact, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Artifact{}, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxArtifactBytes {
		return Artifact{}, errors.New("artifact is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return Artifact{}, err
	}
	hasher := sha256.New()
	_, copyErr := io.CopyBuffer(hasher, file, make([]byte, copyBufferBytes))
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return Artifact{}, err
	}
	return Artifact{
		Path: relative, Size: info.Size(), SHA256: hex.EncodeToString(hasher.Sum(nil)), BlobID: blobID,
	}, nil
}

func copyOpenedBlob(ctx context.Context, source *os.File, destination, relative string, id blob.BlobID) (Artifact, error) {
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxArtifactBytes {
		return Artifact{}, errors.New("source blob is not a bounded regular file")
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
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

func copyVerifiedArtifact(ctx context.Context, root string, artifact Artifact, destination string) error {
	sourcePath, err := resolveArtifactPath(root, artifact.Path)
	if err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
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
	syncErr := destinationFile.Sync()
	closeDestinationErr := destinationFile.Close()
	closeSourceErr := source.Close()
	if err := errors.Join(copyErr, syncErr, closeDestinationErr, closeSourceErr); err != nil {
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

func rejectDuplicateJSONFields(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
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
				keyToken, err := decoder.Token()
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
				if err := walk(); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim('}') {
				return errors.New("invalid object ending")
			}
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim(']') {
				return errors.New("invalid array ending")
			}
		default:
			return errors.New("unexpected JSON delimiter")
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON value")
	}
	return nil
}

func newDestination(raw string) (finalPath, parent string, err error) {
	if strings.TrimSpace(raw) == "" || strings.ContainsRune(raw, '\x00') {
		return "", "", errors.New("backup: destination is empty or invalid")
	}
	finalPath, err = filepath.Abs(raw)
	if err != nil {
		return "", "", fmt.Errorf("backup: resolve destination: %w", err)
	}
	parent = filepath.Dir(finalPath)
	if _, err := existingDirectory(parent); err != nil {
		return "", "", fmt.Errorf("backup: destination parent: %w", err)
	}
	if _, err := os.Lstat(finalPath); err == nil {
		return "", "", errors.New("backup: destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", fmt.Errorf("backup: inspect destination: %w", err)
	}
	return finalPath, parent, nil
}

func existingDirectory(raw string) (string, error) {
	return vault.ValidateExistingDirectory(raw)
}

func resolveArtifactPath(root, relative string) (string, error) {
	if !safeRelativePath(relative) {
		return "", errors.New("backup: artifact path traversal rejected")
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("backup: artifact escapes backup root")
	}
	current := root
	components := strings.Split(filepath.FromSlash(relative), string(filepath.Separator))
	for _, component := range components[:len(components)-1] {
		current = filepath.Join(current, component)
		if _, err := vault.ValidateExistingDirectory(current); err != nil {
			return "", fmt.Errorf("backup: artifact parent is unsafe: %w", err)
		}
	}
	return path, nil
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

func removeSQLiteSidecars(path string) error {
	var failures []error
	for _, sidecar := range []string{path + "-wal", path + "-shm"} {
		if err := os.Remove(sidecar); err != nil && !errors.Is(err, os.ErrNotExist) {
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

func pathsOverlap(left, right string) bool {
	for _, pair := range [][2]string{{left, right}, {right, left}} {
		relative, err := filepath.Rel(pair[0], pair[1])
		if err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))) {
			return true
		}
	}
	return false
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	sort.Strings(left)
	sort.Strings(right)
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func syncTreeDirectories(root string) error {
	directories := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("backup: symbolic link appeared in staged tree")
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(directories, func(left, right int) bool {
		return len(directories[left]) > len(directories[right])
	})
	for _, directory := range directories {
		if _, err := vault.ValidateExistingDirectory(directory); err != nil {
			return err
		}
		if err := syncDirectoryPath(directory); err != nil {
			return err
		}
	}
	return nil
}
