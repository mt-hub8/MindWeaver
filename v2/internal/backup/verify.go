package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
)

const (
	verifyScratchPrefix      = ".mindweaver-verify-"
	verifyScratchDestination = "verify-scratch"
)

// Phase is a stable, path-free description of the current backup operation
// stage. Phases are ordered, but callers must not derive a percentage from
// them or from byte counts.
type Phase string

const (
	PhaseManifest              Phase = "manifest"
	PhaseExactTree             Phase = "exact_tree"
	PhaseArtifactHashes        Phase = "artifact_hashes"
	PhaseDatabaseCopy          Phase = "database_copy"
	PhaseDatabaseQualification Phase = "database_qualification"
	PhaseSourceRecheck         Phase = "source_recheck"
	PhaseCleanup               Phase = "cleanup"
	PhaseComplete              Phase = "complete"
)

// FailureClass is the stable external error taxonomy for backup operations.
// It intentionally carries no path, database content, or provider detail.
type FailureClass string

const (
	FailureCanceled             FailureClass = "canceled"
	FailureInvalid              FailureClass = "invalid"
	FailureCorrupt              FailureClass = "corrupt"
	FailureUnsupported          FailureClass = "unsupported"
	FailureCleanupRequired      FailureClass = "cleanup_required"
	FailurePublicationUncertain FailureClass = "publication_uncertain"
	FailureInternal             FailureClass = "internal"
)

// Summary contains only bounded aggregate facts proven by Verify or Restore.
type Summary struct {
	SchemaVersion     int   `json:"schema_version"`
	ArtifactCount     int   `json:"artifact_count"`
	BlobCount         int   `json:"blob_count"`
	VerifiedBytes     int64 `json:"verified_bytes"`
	DatabaseBytes     int64 `json:"database_bytes"`
	ReferencedBlobIDs int   `json:"referenced_blob_ids"`
}

// Progress is a monotonic snapshot. Total counts and bytes are commitments
// from the validated manifest, not a completion percentage.
type Progress struct {
	Phase             Phase `json:"phase"`
	ArtifactCount     int   `json:"artifact_count"`
	ArtifactsVerified int   `json:"artifacts_verified"`
	BlobCount         int   `json:"blob_count"`
	BlobsVerified     int   `json:"blobs_verified"`
	TotalBytes        int64 `json:"total_bytes"`
	VerifiedBytes     int64 `json:"verified_bytes"`
}

// Outcome is always returned, including on failure. Failure is empty only when
// the selected operation and every required cleanup or publication proof
// succeed.
type Outcome struct {
	Succeeded       bool         `json:"succeeded"`
	Failure         FailureClass `json:"failure,omitempty"`
	CleanupRequired bool         `json:"cleanup_required"`
	Summary         Summary      `json:"summary"`
}

// VerifyOptions controls the one temporary copy needed for SQLite semantic
// qualification. ScratchParent must be an existing, application-owned,
// fixed-local directory dedicated to verification scratch that the caller has
// already capability-validated. Verify independently revalidates that exact
// retained handle as fixed-local before writing. It must not equal the backup
// source or be inside it. Progress is synchronous: Verify contains a panic,
// but deliberately does not spawn or time out the callback, so blocking it
// blocks Verify and violates this contract.
type VerifyOptions struct {
	ScratchParent string
	Progress      func(Progress)
}

type verifyHooks struct {
	afterScratchSecured        func(*stagingDirectory) error
	afterDatabaseCopy          func(*stagingDirectory) error
	afterDatabaseQualification func(*retainedDirectory, *stagingDirectory) error
	beforeBlobHash             func(int) error
	beforeScratchCleanup       func(*stagingDirectory) error
}

// ScratchCleanupSummary reports one bounded pass over crash residues. A true
// Truncated value must be surfaced to the operator and followed by another
// explicit pass; it is never treated as complete cleanup.
type ScratchCleanupSummary struct {
	Examined  int  `json:"examined"`
	Removed   int  `json:"removed"`
	Truncated bool `json:"truncated"`
}

type classifiedFailure struct {
	class FailureClass
	cause error
}

func (failure *classifiedFailure) Error() string {
	if failure == nil {
		return "backup operation failed"
	}
	return "backup operation failed: " + string(failure.class)
}

// Format keeps every common fmt rendering on the same path-free public
// representation. The retained cause remains available only to errors.Is.
func (failure *classifiedFailure) Format(state fmt.State, verb rune) {
	message := failure.Error()
	if verb == 'q' {
		_, _ = fmt.Fprintf(state, "%q", message)
		return
	}
	_, _ = io.WriteString(state, message)
}

// Is preserves cancellation and stable sentinel checks without exposing the
// internal cause (which may contain a local path) through Unwrap or formatting.
func (failure *classifiedFailure) Is(target error) bool {
	return failure != nil && errors.Is(failure.cause, target)
}

// FailureClassOf returns the stable, path-free class carried by a public
// backup error. It returns an empty class for nil.
func FailureClassOf(err error) FailureClass {
	if err == nil {
		return ""
	}
	var classified *classifiedFailure
	if errors.As(err, &classified) {
		return classified.class
	}
	return classifyBackupFailure(err)
}

type operationFailure struct {
	class FailureClass
	err   error
}

func (failure *operationFailure) Error() string { return failure.err.Error() }
func (failure *operationFailure) Unwrap() error { return failure.err }

func failOperation(class FailureClass, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		class = FailureCanceled
	}
	return &operationFailure{class: class, err: err}
}

func failVerify(class FailureClass, err error) error { return failOperation(class, err) }

// Verify proves that source is an exact, self-consistent backup while keeping
// source read-only for the entire call. Only the database is copied, with a
// strict manifest size bound, into an identity-bound private scratch tree. The
// Coordinator retains the active Vault identity and rejects scratch equal to,
// inside, or containing that Vault.
func (c *Coordinator) Verify(ctx context.Context, source string, options VerifyOptions) (Outcome, error) {
	if c == nil {
		return failedOutcome(FailureInvalid, errors.New("backup: coordinator is not initialized"))
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.activeVault == nil || c.activeVault.root == nil || c.activeVault.identity.info == nil {
		return failedOutcome(FailureInvalid, errors.New("backup: coordinator is not initialized"))
	}
	summary, err := verifyBackup(ctx, source, options, verifyHooks{}, func(scratch *retainedDirectory) error {
		return rejectRetainedDirectoryOverlap(c.activeVault, scratch, ErrActiveVaultOverlap)
	})
	return outcomeFromResult(summary, err)
}

// VerifyStandalone verifies a backup without requiring an active Vault,
// database, or blob store. The source remains read-only. ScratchParent is
// retained and independently proven fixed-local before any temporary write;
// source/scratch overlap is rejected by the shared verification kernel.
func VerifyStandalone(ctx context.Context, source string, options VerifyOptions) (Outcome, error) {
	summary, err := verifyBackup(ctx, source, options, verifyHooks{}, validateOwnerOnlyVerifyScratch)
	return outcomeFromResult(summary, err)
}

func outcomeFromResult(summary Summary, err error) (Outcome, error) {
	outcome := Outcome{Summary: summary}
	if err == nil {
		outcome.Succeeded = true
		return outcome, nil
	}
	class := classifyBackupFailure(err)
	outcome.Failure = class
	outcome.CleanupRequired = errors.Is(err, ErrCleanupResidual)
	return outcome, &classifiedFailure{class: class, cause: err}
}

func failedOutcome(class FailureClass, cause error) (Outcome, error) {
	return Outcome{Failure: class}, &classifiedFailure{class: class, cause: cause}
}

// CleanupVerifyScratch performs one explicit bounded startup-cleanup pass in
// the dedicated ScratchParent used by Verify. It accepts only verify-kind
// authoritative receipts and uses their immutable filesystem identity binding;
// conflicts are retained for operator attention. Truncated is never hidden.
func (c *Coordinator) CleanupVerifyScratch(
	ctx context.Context,
	scratchParent string,
	limit int,
) (ScratchCleanupSummary, error) {
	if c == nil {
		return ScratchCleanupSummary{}, &classifiedFailure{
			class: FailureInvalid,
			cause: errors.New("backup: coordinator is not initialized"),
		}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.activeVault == nil || c.activeVault.root == nil || c.activeVault.identity.info == nil {
		return ScratchCleanupSummary{}, &classifiedFailure{
			class: FailureInvalid,
			cause: errors.New("backup: coordinator is not initialized"),
		}
	}
	result, err := cleanupVerifyScratch(
		ctx,
		scratchParent,
		limit,
		func(scratch *retainedDirectory) error {
			return rejectRetainedDirectoryOverlap(c.activeVault, scratch, ErrActiveVaultOverlap)
		},
		c.rejectActiveVaultOverlap,
	)
	if err == nil {
		return result, nil
	}
	class := classifyBackupFailure(err)
	return result, &classifiedFailure{class: class, cause: err}
}

func cleanupVerifyScratch(
	ctx context.Context,
	scratchParent string,
	limit int,
	scratchGuard func(*retainedDirectory) error,
	residueGuard residueRecoveryGuard,
) (result ScratchCleanupSummary, resultErr error) {
	if ctx == nil || strings.TrimSpace(scratchParent) == "" || limit < 1 || limit > maxResiduePageSize {
		return result, failVerify(FailureInvalid, errors.New("backup: invalid verification scratch cleanup input"))
	}
	if err := ensureResidueRecoverySupported(); err != nil {
		return result, err
	}
	capability, err := openRetainedDirectory(scratchParent)
	if err != nil {
		return result, failVerify(FailureInvalid, err)
	}
	defer func() { resultErr = errors.Join(resultErr, capability.Close()) }()
	return cleanupVerifyScratchRoot(ctx, capability, limit, scratchGuard, residueGuard)
}

func cleanupVerifyScratchRoot(
	ctx context.Context,
	capability *retainedDirectory,
	limit int,
	scratchGuard func(*retainedDirectory) error,
	residueGuard residueRecoveryGuard,
) (result ScratchCleanupSummary, resultErr error) {
	if ctx == nil || capability == nil || capability.root == nil ||
		limit < 1 || limit > maxResiduePageSize {
		return result, failVerify(FailureInvalid, errors.New("backup: invalid retained verification scratch cleanup input"))
	}
	if err := ensureResidueRecoverySupported(); err != nil {
		return result, err
	}
	if err := validateVerifyScratchParent(capability); err != nil {
		return result, err
	}
	if scratchGuard == nil || residueGuard == nil {
		return result, failVerify(FailureInvalid, errors.New("backup: verification scratch guards are unavailable"))
	}
	if err := scratchGuard(capability); err != nil {
		return result, failVerify(FailureInvalid, err)
	}
	page, err := listResiduesRoot(ctx, capability, limit)
	if err != nil {
		return result, err
	}
	result.Truncated = page.Truncated
	for _, item := range page.Items {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		result.Examined++
		if item.Kind != "verify" || item.DestinationName != verifyScratchDestination {
			return result, failVerify(
				FailureInvalid,
				errors.New("backup: verification scratch parent contains another residue target"),
			)
		}
		if item.State == ResidueStateConflict {
			return result, errors.Join(ErrCleanupResidual, ErrResidueConflict)
		}
		if item.State == ResidueStatePublicationUncertain {
			return result, ErrPublicationUncertain
		}
		if item.State != ResidueStateStaging && item.State != ResidueStateReceiptOnly {
			return result, errors.Join(
				ErrCleanupResidual,
				errors.New("backup: unsupported verification residue state"),
			)
		}
		if err := capability.verifyPath(); err != nil {
			return result, errors.Join(ErrCleanupResidual, err)
		}
		if err := recoverResidueRoot(ctx, capability, item, residueGuard, false); err != nil {
			return result, errors.Join(ErrCleanupResidual, err)
		}
		if err := capability.verifyPath(); err != nil {
			return result, errors.Join(ErrCleanupResidual, err)
		}
		result.Removed++
	}
	if err := capability.verifyPath(); err != nil {
		return result, errors.Join(ErrCleanupResidual, err)
	}
	return result, nil
}

func verifyBackup(
	ctx context.Context,
	source string,
	options VerifyOptions,
	hooks verifyHooks,
	scratchGuard func(*retainedDirectory) error,
) (summary Summary, resultErr error) {
	if ctx == nil {
		return summary, failVerify(FailureInvalid, errors.New("backup: nil verification context"))
	}
	if strings.TrimSpace(source) == "" || strings.TrimSpace(options.ScratchParent) == "" {
		return summary, failVerify(FailureInvalid, errors.New("backup: verification paths must not be empty"))
	}
	if err := ensureStagingCleanupSupported(); err != nil {
		return summary, err
	}

	progress := Progress{Phase: PhaseManifest}
	if err := emitVerifyProgress(options.Progress, progress); err != nil {
		return summary, err
	}
	sourceRoot, err := openRetainedDirectory(source)
	if err != nil {
		return summary, failVerify(FailureInvalid, fmt.Errorf("backup: open verification source: %w", err))
	}
	defer func() {
		resultErr = errors.Join(resultErr, sourceRoot.Close())
	}()
	manifestWitness, manifestInfo, err := openRootRegularFile(sourceRoot, manifestFileName)
	if err != nil {
		return summary, failVerify(FailureCorrupt, err)
	}
	defer func() {
		stableErr := verifyOpenedRootFile(sourceRoot, manifestFileName, manifestWitness, manifestInfo)
		if stableErr != nil {
			stableErr = failVerify(FailureCorrupt, stableErr)
		}
		resultErr = errors.Join(resultErr, stableErr, manifestWitness.Close())
	}()
	manifest, manifestWire, err := readManifestWireRoot(ctx, sourceRoot, manifestFileName)
	if err != nil {
		return summary, failVerify(FailureCorrupt, err)
	}
	if int64(len(manifestWire)) != manifestInfo.Size() {
		return summary, failVerify(FailureCorrupt, errors.New("backup: manifest changed while establishing source commitment"))
	}
	if err := verifyOpenedRootFile(sourceRoot, manifestFileName, manifestWitness, manifestInfo); err != nil {
		return summary, failVerify(FailureCorrupt, err)
	}
	totalBytes, err := manifestAggregateBytes(manifest)
	if err != nil {
		return summary, failVerify(FailureCorrupt, err)
	}
	summary = Summary{
		SchemaVersion:     manifest.SchemaVersion,
		ArtifactCount:     len(manifest.Blobs) + 1,
		BlobCount:         len(manifest.Blobs),
		DatabaseBytes:     manifest.Database.Size,
		ReferencedBlobIDs: len(manifest.Blobs),
	}
	progress.ArtifactCount = summary.ArtifactCount
	progress.BlobCount = summary.BlobCount
	progress.TotalBytes = totalBytes

	progress.Phase = PhaseExactTree
	if err := emitVerifyProgress(options.Progress, progress); err != nil {
		return summary, err
	}
	if err := verifyExactTreeRoot(ctx, sourceRoot, manifest); err != nil {
		return summary, failVerify(FailureCorrupt, err)
	}

	databaseWitness, databaseInfo, err := openRootRegularFile(sourceRoot, manifest.Database.Path)
	if err != nil {
		return summary, failVerify(FailureCorrupt, err)
	}
	defer func() {
		stableErr := verifyOpenedRootFile(sourceRoot, manifest.Database.Path, databaseWitness, databaseInfo)
		if stableErr != nil {
			stableErr = failVerify(FailureCorrupt, stableErr)
		}
		resultErr = errors.Join(resultErr, stableErr, databaseWitness.Close())
	}()
	blobInitialIdentities, err := captureRootArtifactIdentities(ctx, sourceRoot, manifest.Blobs)
	if err != nil {
		return summary, failVerify(FailureCorrupt, err)
	}

	progress.Phase = PhaseArtifactHashes
	if err := emitVerifyProgress(options.Progress, progress); err != nil {
		return summary, err
	}
	beforeDatabase, beforeDatabaseIdentity, err := inspectRootFileIdentity(
		ctx,
		sourceRoot,
		manifest.Database.Path,
		"",
	)
	if err != nil || beforeDatabase != manifest.Database {
		if err == nil {
			err = errors.New("backup: database size or SHA-256 differs from manifest")
		}
		return summary, failVerify(FailureCorrupt, err)
	}
	progress.ArtifactsVerified++
	progress.VerifiedBytes += beforeDatabase.Size
	if err := emitVerifyProgress(options.Progress, progress); err != nil {
		return summary, err
	}

	scratchParent, err := openRetainedDirectory(options.ScratchParent)
	if err != nil {
		return summary, failVerify(FailureInvalid, fmt.Errorf("backup: open verification scratch parent: %w", err))
	}
	defer func() {
		resultErr = errors.Join(resultErr, scratchParent.Close())
	}()
	if err := validateVerifyScratchParent(scratchParent); err != nil {
		return summary, err
	}
	if scratchGuard == nil {
		return summary, failVerify(FailureInvalid, errors.New("backup: verification scratch guard is unavailable"))
	}
	if err := scratchGuard(scratchParent); err != nil {
		return summary, failVerify(FailureInvalid, err)
	}
	if err := rejectVerifyScratchOverlap(sourceRoot, scratchParent); err != nil {
		return summary, failVerify(FailureInvalid, err)
	}
	staging, err := createStagingDirectory(scratchParent, verifyScratchPrefix, verifyScratchDestination)
	if err != nil {
		return summary, err
	}
	cleanupPending := true
	defer func() {
		if cleanupPending {
			resultErr = classifyPrincipalOperationError(resultErr)
			resultErr = errors.Join(resultErr, cleanupVerifyStaging(staging, scratchParent))
		}
	}()
	if err := secureVerifyScratchDirectory(staging.creationWitness); err != nil {
		return summary, err
	}
	if hooks.afterScratchSecured != nil {
		if err := hooks.afterScratchSecured(staging); err != nil {
			return summary, err
		}
	}
	if err := staging.directory.root.MkdirAll("data", 0o700); err != nil {
		return summary, fmt.Errorf("backup: create verification scratch data directory: %w", err)
	}

	progress.Phase = PhaseDatabaseCopy
	if err := emitVerifyProgress(options.Progress, progress); err != nil {
		return summary, err
	}
	if _, err := databaseWitness.Seek(0, io.SeekStart); err != nil {
		return summary, err
	}
	databaseCopy, err := staging.directory.root.OpenFile(
		filepath.FromSlash(databasePath),
		os.O_RDWR|os.O_CREATE|os.O_EXCL,
		0o600,
	)
	if err != nil {
		return summary, err
	}
	if err := secureVerifyScratchFile(databaseCopy); err != nil {
		_ = databaseCopy.Close()
		return summary, err
	}
	written, copyErr := copyContext(ctx, databaseCopy, io.LimitReader(databaseWitness, manifest.Database.Size+1))
	syncErr := databaseCopy.Sync()
	closeErr := databaseCopy.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return summary, err
	}
	if written != manifest.Database.Size {
		return summary, failVerify(FailureCorrupt, errors.New("backup: database size changed during scratch copy"))
	}
	if err := verifyOpenedRootFile(sourceRoot, manifest.Database.Path, databaseWitness, databaseInfo); err != nil {
		return summary, failVerify(FailureCorrupt, err)
	}
	if hooks.afterDatabaseCopy != nil {
		if err := hooks.afterDatabaseCopy(staging); err != nil {
			return summary, err
		}
	}

	progress.Phase = PhaseDatabaseQualification
	if err := emitVerifyProgress(options.Progress, progress); err != nil {
		return summary, err
	}
	qualified, err := freezeQualifiedDatabaseArtifact(
		ctx,
		staging.directory,
		func() error {
			return qualifyRestoredDatabaseRoot(ctx, staging.directory, manifest.SchemaVersion, manifest.Blobs)
		},
		nil,
	)
	if err != nil {
		return summary, failVerify(FailureCorrupt, err)
	}
	if qualified.Path != manifest.Database.Path || qualified.Size != manifest.Database.Size ||
		qualified.SHA256 != manifest.Database.SHA256 || qualified.BlobID != manifest.Database.BlobID {
		return summary, failVerify(FailureCorrupt, errors.New("backup: qualified database differs from manifest"))
	}
	if err := removeSQLiteSidecarsRoot(staging.directory, databasePath); err != nil {
		return summary, err
	}
	if hooks.afterDatabaseQualification != nil {
		if err := hooks.afterDatabaseQualification(sourceRoot, staging); err != nil {
			return summary, err
		}
	}

	progress.Phase = PhaseSourceRecheck
	if err := emitVerifyProgress(options.Progress, progress); err != nil {
		return summary, err
	}
	recheckedManifest, recheckedWire, err := readManifestWireRoot(ctx, sourceRoot, manifestFileName)
	if err != nil || !bytes.Equal(recheckedWire, manifestWire) {
		if err == nil {
			err = errors.New("backup: manifest bytes changed across verification")
		}
		return summary, failVerify(FailureCorrupt, err)
	}
	if err := validateManifestEquality(manifest, recheckedManifest); err != nil {
		return summary, failVerify(FailureCorrupt, err)
	}
	if err := verifyOpenedRootFile(sourceRoot, manifestFileName, manifestWitness, manifestInfo); err != nil {
		return summary, failVerify(FailureCorrupt, err)
	}
	if err := verifyExactTreeRoot(ctx, sourceRoot, manifest); err != nil {
		return summary, failVerify(FailureCorrupt, err)
	}
	afterDatabase, afterDatabaseIdentity, err := inspectRootFileIdentity(
		ctx,
		sourceRoot,
		manifest.Database.Path,
		"",
	)
	if err != nil || afterDatabase != beforeDatabase {
		if err == nil {
			err = errors.New("backup: source database changed across verification")
		}
		return summary, failVerify(FailureCorrupt, err)
	}
	if !os.SameFile(databaseInfo, beforeDatabaseIdentity) ||
		!os.SameFile(beforeDatabaseIdentity, afterDatabaseIdentity) ||
		!beforeDatabaseIdentity.ModTime().Equal(afterDatabaseIdentity.ModTime()) {
		return summary, failVerify(FailureCorrupt, errors.New("backup: source database identity changed across verification"))
	}
	if err := verifyOpenedRootFile(sourceRoot, manifest.Database.Path, databaseWitness, databaseInfo); err != nil {
		return summary, failVerify(FailureCorrupt, err)
	}
	blobHashedIdentities := make([]os.FileInfo, len(manifest.Blobs))
	for index, artifact := range manifest.Blobs {
		if hooks.beforeBlobHash != nil {
			if err := hooks.beforeBlobHash(index); err != nil {
				return summary, err
			}
		}
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		actual, identity, err := inspectRootFileIdentity(ctx, sourceRoot, artifact.Path, artifact.BlobID)
		if err != nil || actual != artifact {
			if err == nil {
				err = errors.New("backup: blob size or SHA-256 differs from manifest")
			}
			return summary, failVerify(FailureCorrupt, err)
		}
		if !os.SameFile(blobInitialIdentities[index], identity) ||
			!blobInitialIdentities[index].ModTime().Equal(identity.ModTime()) {
			return summary, failVerify(FailureCorrupt, errors.New("backup: blob identity changed across verification"))
		}
		blobHashedIdentities[index] = identity
		progress.ArtifactsVerified++
		progress.BlobsVerified++
		progress.VerifiedBytes += artifact.Size
		if err := emitVerifyProgress(options.Progress, progress); err != nil {
			return summary, err
		}
	}

	progress.Phase = PhaseCleanup
	if err := emitVerifyProgress(options.Progress, progress); err != nil {
		return summary, err
	}
	if hooks.beforeScratchCleanup != nil {
		if err := hooks.beforeScratchCleanup(staging); err != nil {
			return summary, err
		}
	}
	cleanupErr := cleanupVerifyStaging(staging, scratchParent)
	cleanupPending = false
	if cleanupErr != nil {
		return summary, cleanupErr
	}
	if err := verifyFinalSourceCommitment(
		ctx,
		sourceRoot,
		manifest,
		manifestWire,
		manifestWitness,
		manifestInfo,
		databaseWitness,
		databaseInfo,
		afterDatabaseIdentity,
		blobInitialIdentities,
		blobHashedIdentities,
	); err != nil {
		return summary, failVerify(FailureCorrupt, err)
	}
	summary.VerifiedBytes = progress.VerifiedBytes
	progress.Phase = PhaseComplete
	if err := emitVerifyProgress(options.Progress, progress); err != nil {
		return summary, err
	}
	return summary, nil
}

func cleanupVerifyStaging(staging *stagingDirectory, parent *retainedDirectory) error {
	if parent == nil {
		return errors.Join(ErrCleanupResidual, errors.New("backup: verification scratch parent is unavailable"))
	}
	if err := parent.verifyPath(); err != nil {
		return errors.Join(
			ErrCleanupResidual,
			err,
			closeStagingCreationWitness(staging),
			closeResidueLease(staging),
			staging.directory.Close(),
		)
	}
	cleanupErr := cleanupStagingTree(staging, parent, "verification scratch")
	afterErr := parent.verifyPath()
	return errors.Join(
		cleanupErr,
		afterErr,
		closeStagingCreationWitness(staging),
		closeResidueLease(staging),
		staging.directory.Close(),
	)
}

func captureRootArtifactIdentities(
	ctx context.Context,
	root *retainedDirectory,
	artifacts []Artifact,
) ([]os.FileInfo, error) {
	identities := make([]os.FileInfo, len(artifacts))
	for index, artifact := range artifacts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		file, info, err := openRootRegularFile(root, artifact.Path)
		if err != nil {
			return nil, err
		}
		stableErr := verifyOpenedRootFile(root, artifact.Path, file, info)
		closeErr := file.Close()
		if err := errors.Join(stableErr, closeErr); err != nil {
			return nil, err
		}
		if info.Size() != artifact.Size {
			return nil, errors.New("backup: artifact size differs from manifest before verification")
		}
		identities[index] = info
	}
	return identities, nil
}

func validateManifestEquality(expected, actual Manifest) error {
	if expected.FormatVersion != actual.FormatVersion || expected.SchemaVersion != actual.SchemaVersion ||
		expected.CreatedAtUnixMicros != actual.CreatedAtUnixMicros || expected.Database != actual.Database ||
		len(expected.Blobs) != len(actual.Blobs) {
		return errors.New("backup: manifest commitment changed across verification")
	}
	for index := range expected.Blobs {
		if expected.Blobs[index] != actual.Blobs[index] {
			return errors.New("backup: manifest commitment changed across verification")
		}
	}
	return nil
}

func verifyFinalSourceCommitment(
	ctx context.Context,
	root *retainedDirectory,
	manifest Manifest,
	manifestWire []byte,
	manifestWitness *os.File,
	manifestInfo os.FileInfo,
	databaseWitness *os.File,
	databaseInfo os.FileInfo,
	databaseHashedInfo os.FileInfo,
	blobInitialInfos, blobHashedInfos []os.FileInfo,
) error {
	if ctx == nil || root == nil || root.root == nil || manifestWitness == nil || databaseWitness == nil ||
		manifestInfo == nil || databaseInfo == nil || databaseHashedInfo == nil ||
		len(blobInitialInfos) != len(manifest.Blobs) || len(blobHashedInfos) != len(manifest.Blobs) {
		return errors.New("backup: invalid final source commitment input")
	}
	finalManifest, finalWire, err := readManifestWireRoot(ctx, root, manifestFileName)
	if err != nil {
		return err
	}
	if !bytes.Equal(finalWire, manifestWire) {
		return errors.New("backup: manifest bytes changed before verification completed")
	}
	if err := validateManifestEquality(manifest, finalManifest); err != nil {
		return err
	}
	if err := verifyOpenFileSnapshot(root, manifestFileName, manifestWitness, manifestInfo); err != nil {
		return err
	}
	if !os.SameFile(databaseInfo, databaseHashedInfo) ||
		!databaseInfo.ModTime().Equal(databaseHashedInfo.ModTime()) {
		return errors.New("backup: database changed before final source verification")
	}
	if err := verifyOpenFileSnapshot(root, databasePath, databaseWitness, databaseHashedInfo); err != nil {
		return err
	}
	for index, artifact := range manifest.Blobs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := verifyRootArtifactSnapshot(
			root,
			artifact,
			blobInitialInfos[index],
			blobHashedInfos[index],
		); err != nil {
			return err
		}
	}
	if err := verifyExactTreeRoot(ctx, root, manifest); err != nil {
		return err
	}
	if err := verifyOpenFileSnapshot(root, manifestFileName, manifestWitness, manifestInfo); err != nil {
		return err
	}
	if err := verifyOpenFileSnapshot(root, databasePath, databaseWitness, databaseHashedInfo); err != nil {
		return err
	}
	return root.verifyPath()
}

func verifyOpenFileSnapshot(
	root *retainedDirectory,
	relative string,
	file *os.File,
	expected os.FileInfo,
) error {
	if file == nil || expected == nil {
		return errors.New("backup: missing retained artifact snapshot")
	}
	current, err := file.Stat()
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || current.Size() != expected.Size() ||
		!current.ModTime().Equal(expected.ModTime()) || !os.SameFile(current, expected) {
		return errors.New("backup: retained artifact changed after hashing")
	}
	return verifyOpenedRootFile(root, relative, file, expected)
}

func verifyRootArtifactSnapshot(
	root *retainedDirectory,
	artifact Artifact,
	initial, hashed os.FileInfo,
) error {
	if initial == nil || hashed == nil || !os.SameFile(initial, hashed) ||
		initial.Size() != artifact.Size || hashed.Size() != artifact.Size ||
		!initial.ModTime().Equal(hashed.ModTime()) {
		return errors.New("backup: artifact changed across verification")
	}
	file, current, err := openRootRegularFile(root, artifact.Path)
	if err != nil {
		return err
	}
	stableErr := verifyOpenedRootFile(root, artifact.Path, file, current)
	closeErr := file.Close()
	if err := errors.Join(stableErr, closeErr); err != nil {
		return err
	}
	if !os.SameFile(initial, current) || !os.SameFile(hashed, current) ||
		current.Size() != artifact.Size || !current.ModTime().Equal(hashed.ModTime()) {
		return errors.New("backup: artifact changed after hashing")
	}
	return nil
}

func manifestAggregateBytes(manifest Manifest) (int64, error) {
	if err := validateManifest(manifest); err != nil {
		return 0, err
	}
	total := manifest.Database.Size
	for _, artifact := range manifest.Blobs {
		if artifact.Size > maxBackupBytes-total {
			return 0, errors.New("backup: aggregate bytes exceed verification limit")
		}
		total += artifact.Size
	}
	return total, nil
}

func emitVerifyProgress(callback func(Progress), progress Progress) (err error) {
	if callback == nil {
		return nil
	}
	defer func() {
		if recover() != nil {
			err = failVerify(FailureInternal, errors.New("backup: progress callback failed"))
		}
	}()
	callback(progress)
	return nil
}

func classifyPrincipalOperationError(err error) error {
	if err == nil {
		return nil
	}
	var explicit *operationFailure
	if errors.As(err, &explicit) || errors.Is(err, ErrCleanupResidual) ||
		errors.Is(err, ErrPublicationUncertain) || errors.Is(err, ErrUnsupportedPlatform) {
		return err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return failVerify(FailureCanceled, err)
	}
	return failVerify(FailureInternal, err)
}

func rejectVerifyScratchOverlap(source, scratchParent *retainedDirectory) error {
	if source == nil || source.root == nil || source.identity.info == nil ||
		scratchParent == nil || scratchParent.root == nil || scratchParent.identity.info == nil {
		return errors.New("backup: invalid verification source or scratch capability")
	}
	return visitRetainedAncestorsFromWitness(scratchParent, func(directory *retainedDirectory) error {
		if os.SameFile(directory.identity.info, source.identity.info) {
			return errors.New("backup: verification scratch directory overlaps source")
		}
		return nil
	})
}

func rejectRetainedDirectoryOverlap(left, right *retainedDirectory, sentinel error) error {
	if left == nil || left.root == nil || left.identity.info == nil ||
		right == nil || right.root == nil || right.identity.info == nil || sentinel == nil {
		return errors.New("backup: invalid retained overlap capabilities")
	}
	if err := visitRetainedAncestorsFromWitness(right, func(directory *retainedDirectory) error {
		if os.SameFile(directory.identity.info, left.identity.info) {
			return sentinel
		}
		return nil
	}); err != nil {
		return err
	}
	return visitRetainedAncestorsFromWitness(left, func(directory *retainedDirectory) error {
		if os.SameFile(directory.identity.info, right.identity.info) {
			return sentinel
		}
		return nil
	})
}

func validateVerifyScratchParent(directory *retainedDirectory) (resultErr error) {
	if directory == nil || directory.root == nil || directory.identity.info == nil {
		return failVerify(FailureInvalid, errors.New("backup: verification scratch capability is unavailable"))
	}
	probe, err := directory.root.Open(".")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, probe.Close()) }()
	info, err := probe.Stat()
	if err != nil || !info.IsDir() || !os.SameFile(info, directory.identity.info) {
		if err == nil {
			err = errors.New("backup: verification scratch identity differs from retained root")
		}
		return failVerify(FailureInvalid, err)
	}
	if err := vault.ValidateLocalDirectory(probe); err != nil {
		if errors.Is(err, vault.ErrRemoteUnsupported) || errors.Is(err, vault.ErrCloudSyncUnsupported) ||
			errors.Is(err, vault.ErrUnsafeMedia) {
			return failVerify(FailureUnsupported, err)
		}
		return failVerify(FailureInvalid, err)
	}
	if err := directory.verifyPath(); err != nil {
		return failVerify(FailureInvalid, err)
	}
	return nil
}

func classifyBackupFailure(err error) FailureClass {
	if err == nil {
		return ""
	}
	var explicit *operationFailure
	if errors.As(err, &explicit) {
		return explicit.class
	}
	switch {
	case errors.Is(err, ErrPublicationUncertain):
		return FailurePublicationUncertain
	case errors.Is(err, ErrUnsupportedPlatform):
		return FailureUnsupported
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return FailureCanceled
	case errors.Is(err, ErrCleanupResidual):
		return FailureCleanupRequired
	}
	return FailureInternal
}
