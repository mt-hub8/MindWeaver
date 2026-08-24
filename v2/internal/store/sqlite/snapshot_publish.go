package sqlite

import (
	"errors"
	"fmt"
	"os"
)

// ErrSnapshotPublicationUncertain means the no-replace rename succeeded, but
// the parent directory could neither be durably synced nor durably rolled
// back. Callers must inspect the requested destination before retrying.
var ErrSnapshotPublicationUncertain = errors.New("sqlite: snapshot publication outcome uncertain")

func finishSnapshotPublication(parent, destination string) error {
	if err := syncSnapshotParent(parent); err == nil {
		return nil
	} else {
		removeErr := os.Remove(destination)
		rollbackSyncErr := syncSnapshotParent(parent)
		if removeErr == nil && rollbackSyncErr == nil {
			return fmt.Errorf("sqlite: snapshot publication durability failed and was rolled back: %w", err)
		}
		return errors.Join(
			ErrSnapshotPublicationUncertain,
			fmt.Errorf("sync published snapshot parent: %w", err),
			wrapIf(removeErr, "remove uncertain snapshot destination"),
			wrapIf(rollbackSyncErr, "sync snapshot publication rollback"),
		)
	}
}
