package backup

import (
	"errors"
	"fmt"
	"os"
)

// ErrPublicationUncertain means the atomic no-replace rename succeeded, but
// its directory entry could neither be durably synced nor durably rolled back.
// The exact destination in the wrapped error must be inspected before retry.
var ErrPublicationUncertain = errors.New("backup: publication outcome uncertain")

func finishDirectoryPublication(parent, destination string) error {
	if err := syncDirectoryPath(parent); err == nil {
		return nil
	} else {
		removeErr := os.RemoveAll(destination)
		rollbackSyncErr := syncDirectoryPath(parent)
		if removeErr == nil && rollbackSyncErr == nil {
			return fmt.Errorf("backup: publication durability failed and was rolled back: %w", err)
		}
		return errors.Join(
			ErrPublicationUncertain,
			fmt.Errorf("sync published destination %s: %w", destination, err),
			wrapBackupError(removeErr, "remove uncertain published destination"),
			wrapBackupError(rollbackSyncErr, "sync publication rollback"),
		)
	}
}
