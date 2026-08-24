package backup

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
)

// CleanMachineRecovery is a one-shot capability bound to one absent Vault
// destination. It owns the retained destination-parent identity used for every
// preflight, staging write, and atomic publication. It does not construct or
// require a control Vault, database, or blob store.
//
// Every accepted Restore attempt consumes the capability, including failures
// before writing. Invalid method inputs are rejected before an attempt starts.
// Callers may construct a new capability only after explicitly inspecting any
// reported residue. This prevents accidental replay after an uncertain call.
type CleanMachineRecovery struct {
	mu     sync.Mutex
	target *destinationTarget
	used   bool
}

// NewCleanMachineRecovery binds one nonexistent direct-child destination under
// an existing fixed-local parent. It performs no filesystem writes. The
// returned error is path-free; stable details are available through errors.Is.
// It is exclusively for a recovery process with no active Vault; a process
// that owns an active Vault must use Coordinator.Restore so that identity is
// retained and protected from equal/inside/contains destinations.
func NewCleanMachineRecovery(destinationVault string) (*CleanMachineRecovery, error) {
	if err := ensureStagingCleanupSupported(); err != nil {
		return nil, classifiedBackupError(err)
	}
	target, err := newDestination(destinationVault)
	if err != nil {
		return nil, classifiedBackupError(failOperation(FailureInvalid, err))
	}
	fail := func(cause error) (*CleanMachineRecovery, error) {
		return nil, classifiedBackupError(errors.Join(cause, target.parent.Close()))
	}
	if err := validateRestoreDestinationParent(target.parent); err != nil {
		return fail(classifyRestoreDestinationValidation(err))
	}
	if err := ensureDestinationAbsent(target); err != nil {
		return fail(failOperation(FailureInvalid, err))
	}
	return &CleanMachineRecovery{target: target}, nil
}

// Restore verifies source read-only and atomically publishes the recovered
// Vault at the destination bound by NewCleanMachineRecovery. It never merges,
// overwrites, changes target, or retries an uncertain publication.
func (recovery *CleanMachineRecovery) Restore(ctx context.Context, source string) (Outcome, error) {
	return recovery.restore(ctx, source, publicationHooks{})
}

func (recovery *CleanMachineRecovery) restore(
	ctx context.Context,
	source string,
	hooks publicationHooks,
) (Outcome, error) {
	if recovery == nil {
		return failedOutcome(FailureInvalid, errors.New("backup: recovery capability is unavailable"))
	}
	recovery.mu.Lock()
	defer recovery.mu.Unlock()
	if recovery.target == nil || recovery.target.parent == nil {
		return failedOutcome(FailureInvalid, errors.New("backup: recovery capability is closed"))
	}
	if recovery.used {
		return failedOutcome(FailureInvalid, errors.New("backup: recovery capability is already consumed"))
	}
	if ctx == nil || strings.TrimSpace(source) == "" {
		return failedOutcome(FailureInvalid, errors.New("backup: invalid recovery input"))
	}
	recovery.used = true
	var summary Summary
	err := restoreToTarget(ctx, source, recovery.target, &summary, hooks, nil)
	return outcomeFromResult(summary, err)
}

// Close releases the retained destination-parent capability. It waits for an
// in-flight Restore and is safe to call repeatedly.
func (recovery *CleanMachineRecovery) Close() error {
	if recovery == nil {
		return nil
	}
	recovery.mu.Lock()
	defer recovery.mu.Unlock()
	if recovery.target == nil {
		return nil
	}
	target := recovery.target
	recovery.target = nil
	if err := target.parent.Close(); err != nil {
		return classifiedBackupError(failOperation(FailureInternal, err))
	}
	return nil
}

func validateRestoreDestinationParent(directory *retainedDirectory) (resultErr error) {
	if directory == nil || directory.root == nil || directory.identity.info == nil {
		return failOperation(FailureInvalid, errors.New("backup: restore destination capability is unavailable"))
	}
	probe, err := directory.root.Open(".")
	if err != nil {
		return failOperation(FailureInvalid, err)
	}
	defer func() { resultErr = errors.Join(resultErr, probe.Close()) }()
	info, err := probe.Stat()
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(info, directory.identity.info) {
		if err == nil {
			err = errors.New("backup: restore destination identity differs from retained parent")
		}
		return failOperation(FailureInvalid, err)
	}
	if err := vault.ValidateLocalDirectory(probe); err != nil {
		if errors.Is(err, vault.ErrRemoteUnsupported) || errors.Is(err, vault.ErrCloudSyncUnsupported) ||
			errors.Is(err, vault.ErrUnsafeMedia) {
			return failOperation(FailureUnsupported, err)
		}
		return failOperation(FailureInvalid, err)
	}
	if err := directory.verifyPath(); err != nil {
		return failOperation(FailureInvalid, err)
	}
	return nil
}

func classifyRestoreDestinationValidation(err error) error {
	if err == nil {
		return nil
	}
	var explicit *operationFailure
	if errors.As(err, &explicit) {
		return err
	}
	if errors.Is(err, vault.ErrRemoteUnsupported) || errors.Is(err, vault.ErrCloudSyncUnsupported) ||
		errors.Is(err, vault.ErrUnsafeMedia) || errors.Is(err, ErrUnsupportedPlatform) {
		return failOperation(FailureUnsupported, err)
	}
	return failOperation(FailureInvalid, err)
}

func classifiedBackupError(err error) error {
	if err == nil {
		return nil
	}
	return &classifiedFailure{class: classifyBackupFailure(err), cause: err}
}
