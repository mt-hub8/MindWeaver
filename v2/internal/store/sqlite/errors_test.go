package sqlite

import (
	"errors"
	"fmt"
	"testing"

	"github.com/ncruces/go-sqlite3"
)

func TestIsRetryableContentionUsesNumericPrimaryAndExtendedCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil"},
		{name: "busy primary", err: sqlite3.BUSY, want: true},
		{name: "locked primary", err: sqlite3.LOCKED, want: true},
		{name: "busy recovery", err: sqlite3.BUSY_RECOVERY, want: true},
		{name: "busy snapshot", err: sqlite3.BUSY_SNAPSHOT, want: true},
		{name: "busy timeout", err: sqlite3.BUSY_TIMEOUT, want: true},
		{name: "locked shared cache", err: sqlite3.LOCKED_SHAREDCACHE, want: true},
		{name: "locked virtual table", err: sqlite3.LOCKED_VTAB, want: true},
		{name: "wrapped extended", err: fmt.Errorf("store boundary: %w", sqlite3.BUSY_TIMEOUT), want: true},
		{name: "full", err: sqlite3.FULL},
		{name: "interrupt", err: sqlite3.INTERRUPT},
		{name: "error text is not a numeric result", err: errors.New("database is busy or locked")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsRetryableContention(test.err); got != test.want {
				t.Fatalf("IsRetryableContention(%v) = %v, want %v", test.err, got, test.want)
			}
		})
	}
}
