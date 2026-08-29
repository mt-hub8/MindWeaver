package sqlite

import (
	"errors"

	"github.com/ncruces/go-sqlite3"
)

// IsRetryableContention reports whether err is SQLite's numeric BUSY or
// LOCKED result. Extended results such as BUSY_SNAPSHOT, BUSY_TIMEOUT, and
// LOCKED_SHAREDCACHE retain their primary result code and are included.
//
// Callers must retry the complete operation with its existing idempotency key
// or revision precondition. This predicate does not authorize replaying an
// arbitrary transaction callback inside the storage adapter.
func IsRetryableContention(err error) bool {
	return errors.Is(err, sqlite3.BUSY) || errors.Is(err, sqlite3.LOCKED)
}
