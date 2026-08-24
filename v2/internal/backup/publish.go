package backup

import (
	"errors"
	"fmt"
)

// ErrPublicationUncertain means the atomic no-replace rename succeeded, but
// the published content commitment, final identity, or directory durability
// could not be proven. The destination is deliberately retained for inspection.
var ErrPublicationUncertain = errors.New("backup: publication outcome uncertain")

func finishDirectoryPublication(
	parent *retainedDirectory,
	destinationName, destinationPath string,
	attempt publicationAttempt,
) error {
	// Unix publication consumes the retained parent descriptor directly.
	// Platforms whose atomic no-replace primitive only accepts paths retain
	// that proven primitive, then immediately verify through the retained
	// parent. Every check must resolve to the staging identity recorded before
	// its handle was closed. A replaced pathname is never a cleanup target.
	// On path-only platforms, an adversarial rename-away-and-back entirely
	// inside the platform rename call remains a boundary until Go exposes an
	// equivalent handle-relative no-replace primitive.
	syncParent := syncRetainedDirectory
	if attempt.hooks.syncParent != nil {
		syncParent = attempt.hooks.syncParent
	}
	if err := syncParent(parent); err != nil {
		var hookErr error
		if attempt.hooks.afterSyncFailure != nil {
			hookErr = attempt.hooks.afterSyncFailure()
		}
		identityErr := verifyPublishedDirectory(parent, destinationName, attempt.expected)
		// There is no identity-bound recursive deletion primitive in os.Root.
		// Even when the identity still matches, leave the complete destination
		// in place for inspection instead of risking traversal of a swapped leaf.
		return errors.Join(
			ErrPublicationUncertain,
			fmt.Errorf("sync published destination %s; retained for inspection: %w", destinationPath, err),
			wrapBackupError(hookErr, "publication failure hook"),
			wrapBackupError(identityErr, "published destination changed after sync failure"),
		)
	}
	// Durability must precede the final content proof. A platform sync helper
	// (or an injected equivalent) can observe and mutate the newly published
	// leaf; verifying first would leave that mutation outside the commitment.
	if err := verifyPublishedContent(parent, destinationName, attempt); err != nil {
		return errors.Join(
			ErrPublicationUncertain,
			fmt.Errorf("published destination content is uncertain at %s: %w", destinationPath, err),
		)
	}
	if err := verifyPublishedDirectory(parent, destinationName, attempt.expected); err != nil {
		return errors.Join(
			ErrPublicationUncertain,
			fmt.Errorf("published destination changed after parent sync at %s: %w", destinationPath, err),
		)
	}
	return nil
}

func verifyPublishedDirectory(parent *retainedDirectory, destinationName string, expected directoryIdentity) error {
	destination, err := openExpectedDirectory(parent, destinationName, expected)
	if err != nil {
		return err
	}
	return destination.Close()
}

func verifyPublishedContent(
	parent *retainedDirectory,
	destinationName string,
	attempt publicationAttempt,
) error {
	if attempt.ctx == nil || attempt.verify == nil {
		return errors.New("backup: publication content commitment is missing")
	}
	if err := attempt.ctx.Err(); err != nil {
		return err
	}
	destination, err := openExpectedDirectory(parent, destinationName, attempt.expected)
	if err != nil {
		return err
	}
	verifyErr := attempt.verify(attempt.ctx, destination)
	contextErr := attempt.ctx.Err()
	closeErr := destination.Close()
	return errors.Join(verifyErr, contextErr, closeErr)
}
