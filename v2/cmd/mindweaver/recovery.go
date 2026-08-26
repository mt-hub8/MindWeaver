package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"path/filepath"
	"strings"

	"github.com/mt-hub8/MindWeaver/v2/internal/backup"
	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
)

const (
	recoveryResidueLimit     = 256
	recoveryCleanupPassLimit = 32
)

var prepareRecoveryVerifyScratch = backup.PrepareStartupVerifyScratch

type recoveryRecord struct {
	Type      string           `json:"type"`
	Operation string           `json:"operation"`
	Phase     string           `json:"phase,omitempty"`
	Pass      int              `json:"pass,omitempty"`
	Examined  int              `json:"examined,omitempty"`
	Removed   int              `json:"removed,omitempty"`
	Truncated bool             `json:"truncated,omitempty"`
	Progress  *backup.Progress `json:"progress,omitempty"`
	Outcome   *backup.Outcome  `json:"outcome,omitempty"`
}

type recoveryReporter struct {
	encoder *json.Encoder
	cancel  context.CancelFunc
	err     error
}

func newRecoveryReporter(writer io.Writer, cancel context.CancelFunc) (*recoveryReporter, error) {
	if writer == nil || cancel == nil {
		return nil, apperror.New(apperror.KindInvalid, "cli.recovery_output_invalid", "recovery output is unavailable")
	}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(true)
	return &recoveryReporter{encoder: encoder, cancel: cancel}, nil
}

func (reporter *recoveryReporter) emit(record recoveryRecord) {
	if reporter == nil || reporter.err != nil {
		return
	}
	if err := reporter.encoder.Encode(record); err != nil {
		reporter.err = err
		reporter.cancel()
	}
}

func (reporter *recoveryReporter) outputError() error {
	if reporter == nil || reporter.err == nil {
		return nil
	}
	return outputError(reporter.err)
}

func runRecovery(ctx context.Context, args []string, stdout io.Writer) error {
	if ctx == nil {
		return apperror.New(apperror.KindInvalid, "cli.context_required", "recovery requires a context")
	}
	if len(args) == 0 {
		return apperror.New(apperror.KindInvalid, "cli.recovery_command_required", "recovery requires verify or restore")
	}
	switch args[0] {
	case "verify":
		flags := flag.NewFlagSet("recovery verify", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		source := flags.String("backup", "", "backup directory")
		if err := flags.Parse(args[1:]); err != nil {
			return apperror.Wrap(err, apperror.KindInvalid, "cli.flags_invalid", "cli.recovery_verify", "invalid recovery verify flags")
		}
		if flags.NArg() != 0 || strings.TrimSpace(*source) == "" {
			return apperror.New(apperror.KindInvalid, "cli.recovery_verify_input_invalid", "recovery verify requires one backup directory")
		}
		return runRecoveryVerify(ctx, *source, stdout)
	case "restore":
		flags := flag.NewFlagSet("recovery restore", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		source := flags.String("backup", "", "backup directory")
		destination := flags.String("vault", "", "new Vault destination")
		if err := flags.Parse(args[1:]); err != nil {
			return apperror.Wrap(err, apperror.KindInvalid, "cli.flags_invalid", "cli.recovery_restore", "invalid recovery restore flags")
		}
		if flags.NArg() != 0 || strings.TrimSpace(*source) == "" || strings.TrimSpace(*destination) == "" {
			return apperror.New(apperror.KindInvalid, "cli.recovery_restore_input_invalid", "recovery restore requires backup and new Vault directories")
		}
		return runRecoveryRestore(ctx, *source, *destination, stdout)
	default:
		return apperror.New(apperror.KindInvalid, "cli.recovery_command_unknown", "unknown recovery command; use verify or restore")
	}
}

func runRecoveryVerify(parent context.Context, source string, stdout io.Writer) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	reporter, err := newRecoveryReporter(stdout, cancel)
	if err != nil {
		return err
	}
	scratch, err := prepareRecoveryVerifyScratch(source)
	if err != nil {
		return reportRecoveryFailure(reporter, "verify", err)
	}
	if err := cleanupVerifyResidues(ctx, source, scratch, reporter); err != nil {
		return reportRecoveryFailure(reporter, "verify", err)
	}
	outcome, operationErr := backup.VerifyStandalone(ctx, source, backup.VerifyOptions{
		ScratchParent: scratch,
		Progress: func(progress backup.Progress) {
			copy := progress
			reporter.emit(recoveryRecord{
				Type: "progress", Operation: "verify", Phase: string(progress.Phase), Progress: &copy,
			})
		},
	})
	reporter.emit(recoveryRecord{Type: "outcome", Operation: "verify", Outcome: &outcome})
	if err := reporter.outputError(); err != nil {
		return err
	}
	return wrapRecoveryError("verify", operationErr)
}

func runRecoveryRestore(parent context.Context, source, destination string, stdout io.Writer) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	reporter, err := newRecoveryReporter(stdout, cancel)
	if err != nil {
		return err
	}
	destination, parentPath, destinationName, err := canonicalRecoveryDestination(destination)
	if err != nil {
		return reportRecoveryFailure(reporter, "restore", err)
	}
	if err := cleanupRestoreResidues(ctx, source, parentPath, destinationName, reporter); err != nil {
		return reportRecoveryFailure(reporter, "restore", err)
	}

	recovery, err := backup.NewCleanMachineRecovery(destination)
	if err != nil {
		outcome := backup.Outcome{Failure: backup.FailureClassOf(err)}
		reporter.emit(recoveryRecord{Type: "outcome", Operation: "restore", Outcome: &outcome})
		return preferRecoveryOutputError(reporter, wrapRecoveryError("restore", err))
	}
	reporter.emit(recoveryRecord{Type: "progress", Operation: "restore", Phase: "restore"})
	outcome, operationErr := recovery.Restore(ctx, source)
	operationErr = errors.Join(operationErr, recovery.Close())
	if operationErr != nil && outcome.Succeeded {
		outcome.Succeeded = false
		outcome.Failure = backup.FailureClassOf(operationErr)
		outcome.CleanupRequired = errors.Is(operationErr, backup.ErrCleanupResidual)
	}
	reporter.emit(recoveryRecord{Type: "outcome", Operation: "restore", Outcome: &outcome})
	if err := reporter.outputError(); err != nil {
		return err
	}
	return wrapRecoveryError("restore", operationErr)
}

func cleanupVerifyResidues(
	ctx context.Context,
	source string,
	scratch string,
	reporter *recoveryReporter,
) (resultErr error) {
	recovery, err := backup.NewStartupVerifyScratchRecovery(source, scratch)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, recovery.Close()) }()
	for pass := 1; pass <= recoveryCleanupPassLimit; pass++ {
		summary, err := recovery.Cleanup(ctx, recoveryResidueLimit)
		reporter.emit(recoveryRecord{
			Type: "progress", Operation: "verify", Phase: "startup_cleanup", Pass: pass,
			Examined: summary.Examined, Removed: summary.Removed, Truncated: summary.Truncated,
		})
		if err != nil {
			return err
		}
		if !summary.Truncated {
			return nil
		}
		if summary.Removed == 0 {
			return apperror.New(apperror.KindConflict, "recovery.verify_residue_attention_required", "verification residue needs attention")
		}
	}
	return apperror.New(apperror.KindConflict, "recovery.verify_cleanup_limit", "verification residue cleanup did not converge")
}

func cleanupRestoreResidues(
	ctx context.Context,
	source string,
	parentPath string,
	destinationName string,
	reporter *recoveryReporter,
) (resultErr error) {
	recovery, err := backup.NewStartupRestoreResidueRecovery(source, parentPath)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, recovery.Close()) }()
	for pass := 1; pass <= recoveryCleanupPassLimit; pass++ {
		page, err := recovery.List(ctx, recoveryResidueLimit)
		if err != nil {
			return err
		}
		if page.Truncated {
			return apperror.New(apperror.KindConflict, "recovery.restore_residue_limit", "restore residue needs attention")
		}
		// Validate the whole bounded observation before deleting anything. A
		// conflicting, unrelated, active, or publication-uncertain receipt must
		// block this startup attempt without partially consuming evidence.
		for _, residue := range page.Items {
			if residue.Kind != "restore" || !sameRecoveryLeaf(residue.DestinationName, destinationName) {
				return apperror.New(apperror.KindConflict, "recovery.restore_residue_attention_required", "restore residue needs attention")
			}
			if residue.State != backup.ResidueStateStaging && residue.State != backup.ResidueStateReceiptOnly {
				return apperror.New(apperror.KindConflict, "recovery.restore_outcome_uncertain", "restore outcome needs attention")
			}
		}
		removed := 0
		for _, residue := range page.Items {
			outcome, recoverErr := recovery.Recover(ctx, residue)
			if recoverErr != nil || !outcome.Succeeded {
				if recoverErr != nil {
					return recoverErr
				}
				return apperror.New(apperror.KindConflict, "recovery.restore_cleanup_failed", "restore residue cleanup failed")
			}
			removed++
		}
		reporter.emit(recoveryRecord{
			Type: "progress", Operation: "restore", Phase: "startup_cleanup", Pass: pass,
			Examined: len(page.Items), Removed: removed, Truncated: page.Truncated,
		})
		if len(page.Items) == 0 {
			return nil
		}
		if removed == 0 {
			return apperror.New(apperror.KindConflict, "recovery.restore_residue_attention_required", "restore residue needs attention")
		}
	}
	return apperror.New(apperror.KindConflict, "recovery.restore_cleanup_limit", "restore residue cleanup did not converge")
}

func canonicalRecoveryDestination(raw string) (destination, parent, name string, err error) {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) != raw {
		return "", "", "", apperror.New(apperror.KindInvalid, "recovery.destination_invalid", "new Vault destination is invalid")
	}
	destination, err = filepath.Abs(raw)
	if err != nil {
		return "", "", "", apperror.Wrap(err, apperror.KindInvalid, "recovery.destination_invalid", "recovery.destination", "new Vault destination is invalid")
	}
	destination = filepath.Clean(destination)
	parent, name = filepath.Dir(destination), filepath.Base(destination)
	if name == "" || name == "." || name == ".." || parent == destination {
		return "", "", "", apperror.New(apperror.KindInvalid, "recovery.destination_invalid", "new Vault destination is invalid")
	}
	return destination, parent, name, nil
}

func sameRecoveryLeaf(left, right string) bool {
	return left == right
}

func preferRecoveryOutputError(reporter *recoveryReporter, operationErr error) error {
	if outputErr := reporter.outputError(); outputErr != nil {
		return outputErr
	}
	return operationErr
}

func reportRecoveryFailure(reporter *recoveryReporter, operation string, operationErr error) error {
	class := recoveryFailureClass(operationErr)
	outcome := backup.Outcome{
		Failure:         class,
		CleanupRequired: class == backup.FailureCleanupRequired,
	}
	reporter.emit(recoveryRecord{Type: "outcome", Operation: operation, Outcome: &outcome})
	return preferRecoveryOutputError(reporter, wrapRecoveryError(operation, operationErr))
}

func recoveryFailureClass(err error) backup.FailureClass {
	switch apperror.CodeOf(err) {
	case "recovery.restore_outcome_uncertain":
		return backup.FailurePublicationUncertain
	case "recovery.restore_residue_attention_required", "recovery.restore_residue_limit",
		"recovery.restore_cleanup_limit", "recovery.restore_cleanup_failed",
		"recovery.verify_residue_attention_required", "recovery.verify_cleanup_limit":
		return backup.FailureCleanupRequired
	}
	return backup.FailureClassOf(err)
}

func wrapRecoveryError(operation string, err error) error {
	if err == nil || apperror.CodeOf(err) != "" {
		return err
	}
	class := backup.FailureClassOf(err)
	kind := apperror.KindInternal
	public := "recovery operation failed"
	if errors.Is(err, context.DeadlineExceeded) {
		kind, public = apperror.KindDeadline, "recovery operation timed out"
	} else {
		switch class {
		case backup.FailureCanceled:
			kind, public = apperror.KindCanceled, "recovery operation canceled"
		case backup.FailureInvalid, backup.FailureCorrupt:
			kind, public = apperror.KindInvalid, "backup or recovery input is invalid"
		case backup.FailureUnsupported:
			kind, public = apperror.KindUnavailable, "recovery is unavailable on this system"
		case backup.FailureCleanupRequired, backup.FailurePublicationUncertain:
			kind, public = apperror.KindConflict, "recovery residue needs attention"
		}
	}
	code := "recovery." + operation + "." + string(class)
	if class == "" {
		code = "recovery." + operation + ".internal"
	}
	return apperror.Wrap(err, kind, code, "cli.recovery_"+operation, public)
}
