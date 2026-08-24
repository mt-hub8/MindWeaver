//go:build !windows

package backup

import "testing"

func TestNonWindowsLeafValidationRetainsColonSemantics(t *testing.T) {
	if !validResidueLeaf("foo:ads") {
		t.Fatal("non-Windows destination leaf validation unexpectedly rejects colon")
	}
}
