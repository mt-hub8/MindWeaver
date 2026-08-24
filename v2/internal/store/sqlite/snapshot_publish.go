package sqlite

import (
	"errors"
	"fmt"
)

// ErrSnapshotPublicationUncertain means the no-replace rename succeeded, but
// the parent directory could not be durably synced. The published name is
// deliberately retained; path-only rollback could delete a replacement.
var ErrSnapshotPublicationUncertain = errors.New("sqlite: snapshot publication outcome uncertain")

func finishSnapshotPublication(parent, destination string) error {
	return finishSnapshotPublicationWithSync(parent, destination, syncSnapshotParent)

}

func finishSnapshotPublicationWithSync(parent, destination string, syncParent func(string) error) error {
	if syncParent == nil {
		return errors.New("sqlite: nil snapshot parent sync")
	}
	if err := syncParent(parent); err != nil {
		return errors.Join(
			ErrSnapshotPublicationUncertain,
			fmt.Errorf("sync published snapshot %s parent: %w", destination, err),
		)
	}
	return nil
}
