package backup

import (
	"context"
	"errors"
	"sync"
)

type startupResidueCapability struct {
	mu       sync.Mutex
	parent   *retainedDirectory
	validate func(*retainedDirectory) error
}

// StartupVerifyScratchRecovery is an identity-bound cleanup capability for the
// dedicated scratch parent used by VerifyStandalone. It has no active-Vault
// guard and therefore may be constructed only during the mutually exclusive
// startup/recovery phase before any Vault is opened. It accepts only verify
// residues; conflicts and other residue kinds are retained.
type StartupVerifyScratchRecovery struct {
	capability *startupResidueCapability
}

// NewStartupVerifyScratchRecovery retains and fixed-local validates an existing
// scratch parent without writing it.
func NewStartupVerifyScratchRecovery(scratchParent string) (*StartupVerifyScratchRecovery, error) {
	capability, err := newStartupResidueCapability(scratchParent, validateVerifyScratchParent)
	if err != nil {
		return nil, err
	}
	return &StartupVerifyScratchRecovery{capability: capability}, nil
}

// Cleanup performs one bounded pass. Truncated must be surfaced and followed
// by another explicit pass; it is never interpreted as completion.
func (recovery *StartupVerifyScratchRecovery) Cleanup(
	ctx context.Context,
	limit int,
) (ScratchCleanupSummary, error) {
	capability, err := startupCapability(recovery)
	if err != nil {
		return ScratchCleanupSummary{}, classifiedBackupError(err)
	}
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if capability.parent == nil {
		return ScratchCleanupSummary{}, classifiedBackupError(
			failOperation(FailureInvalid, errors.New("backup: startup verification recovery is closed")),
		)
	}
	result, err := cleanupVerifyScratchRoot(
		ctx,
		capability.parent,
		limit,
		func(*retainedDirectory) error { return nil },
		func(*destinationTarget) error { return nil },
	)
	if err != nil {
		return result, classifiedBackupError(classifyStartupResidueFailure(err))
	}
	return result, nil
}

// Close releases the retained scratch-parent identity and waits for an
// in-flight Cleanup.
func (recovery *StartupVerifyScratchRecovery) Close() error {
	if recovery == nil {
		return nil
	}
	return closeStartupResidueCapability(recovery.capability)
}

// StartupRestoreResidueRecovery is an identity-bound inspection and cleanup
// capability for clean-machine Restore residues. It has no active-Vault guard
// and therefore may be constructed only during the mutually exclusive
// startup/recovery phase before any Vault is opened. Recover refuses every
// non-restore residue and every publication-uncertain destination.
type StartupRestoreResidueRecovery struct {
	capability *startupResidueCapability
}

// NewStartupRestoreResidueRecovery retains and fixed-local validates an
// existing restore destination parent without writing it.
func NewStartupRestoreResidueRecovery(parent string) (*StartupRestoreResidueRecovery, error) {
	capability, err := newStartupResidueCapability(parent, validateRestoreDestinationParent)
	if err != nil {
		return nil, err
	}
	return &StartupRestoreResidueRecovery{capability: capability}, nil
}

// List performs one bounded scan through the retained parent. The page may
// include other residue kinds so they remain visible, but Recover accepts only
// Kind "restore".
func (recovery *StartupRestoreResidueRecovery) List(
	ctx context.Context,
	limit int,
) (ResiduePage, error) {
	capability, err := startupRestoreCapability(recovery)
	if err != nil {
		return ResiduePage{}, classifiedBackupError(err)
	}
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if capability.parent == nil {
		return ResiduePage{}, classifiedBackupError(
			failOperation(FailureInvalid, errors.New("backup: startup restore recovery is closed")),
		)
	}
	if ctx == nil || limit < 1 || limit > maxResiduePageSize {
		return ResiduePage{}, classifiedBackupError(
			failOperation(FailureInvalid, errors.New("backup: invalid startup residue listing input")),
		)
	}
	if err := capability.validate(capability.parent); err != nil {
		return ResiduePage{}, classifiedBackupError(classifyRestoreDestinationValidation(err))
	}
	page, err := listResiduesRoot(ctx, capability.parent, limit)
	if err != nil {
		return page, classifiedBackupError(classifyStartupResidueFailure(err))
	}
	return page, nil
}

// Recover revalidates one exact value returned by List and removes only a
// matching restore staging tree or receipt. Publication-uncertain and conflict
// evidence is retained. The Outcome and error contain no path or user content.
func (recovery *StartupRestoreResidueRecovery) Recover(
	ctx context.Context,
	expected Residue,
) (Outcome, error) {
	capability, err := startupRestoreCapability(recovery)
	if err != nil {
		return failedOutcome(FailureInvalid, err)
	}
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if capability.parent == nil {
		return failedOutcome(FailureInvalid, errors.New("backup: startup restore recovery is closed"))
	}
	if ctx == nil || expected.Kind != "restore" {
		return failedOutcome(FailureInvalid, errors.New("backup: invalid startup restore residue input"))
	}
	if err := capability.validate(capability.parent); err != nil {
		return outcomeFromResult(Summary{}, classifyRestoreDestinationValidation(err))
	}
	err = recoverResidueRoot(ctx, capability.parent, expected, func(*destinationTarget) error { return nil })
	return outcomeFromResult(Summary{}, classifyStartupResidueFailure(err))
}

// Close releases the retained restore-parent identity and waits for in-flight
// List or Recover calls.
func (recovery *StartupRestoreResidueRecovery) Close() error {
	if recovery == nil {
		return nil
	}
	return closeStartupResidueCapability(recovery.capability)
}

func newStartupResidueCapability(
	parent string,
	validate func(*retainedDirectory) error,
) (*startupResidueCapability, error) {
	if validate == nil {
		return nil, classifiedBackupError(
			failOperation(FailureInvalid, errors.New("backup: startup recovery validator is unavailable")),
		)
	}
	if err := ensureResidueRecoverySupported(); err != nil {
		return nil, classifiedBackupError(err)
	}
	directory, err := openRetainedDirectory(parent)
	if err != nil {
		return nil, classifiedBackupError(failOperation(FailureInvalid, err))
	}
	fail := func(cause error) (*startupResidueCapability, error) {
		return nil, classifiedBackupError(errors.Join(cause, directory.Close()))
	}
	if err := validate(directory); err != nil {
		return fail(err)
	}
	return &startupResidueCapability{parent: directory, validate: validate}, nil
}

func closeStartupResidueCapability(capability *startupResidueCapability) error {
	if capability == nil {
		return nil
	}
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if capability.parent == nil {
		return nil
	}
	parent := capability.parent
	capability.parent = nil
	if err := parent.Close(); err != nil {
		return classifiedBackupError(failOperation(FailureInternal, err))
	}
	return nil
}

func startupCapability(recovery *StartupVerifyScratchRecovery) (*startupResidueCapability, error) {
	if recovery == nil || recovery.capability == nil {
		return nil, failOperation(FailureInvalid, errors.New("backup: startup verification recovery is unavailable"))
	}
	return recovery.capability, nil
}

func startupRestoreCapability(recovery *StartupRestoreResidueRecovery) (*startupResidueCapability, error) {
	if recovery == nil || recovery.capability == nil {
		return nil, failOperation(FailureInvalid, errors.New("backup: startup restore recovery is unavailable"))
	}
	return recovery.capability, nil
}

func classifyStartupResidueFailure(err error) error {
	if err == nil {
		return nil
	}
	var explicit *operationFailure
	if errors.As(err, &explicit) {
		return err
	}
	if errors.Is(err, ErrResidueConflict) || errors.Is(err, ErrResidueActive) {
		if errors.Is(err, ErrCleanupResidual) {
			return err
		}
		return failOperation(FailureInvalid, err)
	}
	return err
}
