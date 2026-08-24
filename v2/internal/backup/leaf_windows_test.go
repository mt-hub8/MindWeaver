//go:build windows

package backup

import (
	"path/filepath"
	"testing"
)

func TestWindowsLeafValidationRejectsAliasesDevicesAndADS(t *testing.T) {
	invalid := []string{
		"foo:ads", "trailing.", "trailing ", "control\x1f", "CON", "con.txt", "NUL.json",
		"PRN", "AUX.data", "COM1", "com9.log", "LPT1", "lpt9.txt", "CLOCK$", "CONIN$",
		"COM¹", "com².txt", "LPT³.log",
		`quote"name`, "less<name", "greater>name", "pipe|name", "question?name", "star*name",
	}
	parent := t.TempDir()
	for _, name := range invalid {
		t.Run(name, func(t *testing.T) {
			if validResidueLeaf(name) {
				t.Fatalf("invalid Windows leaf accepted: %q", name)
			}
			if target, err := newDestination(filepath.Join(parent, name)); err == nil {
				_ = target.parent.Close()
				t.Fatalf("invalid Windows destination accepted: %q", name)
			}
			receipt := residueReceipt{
				Version: residueReceiptVersion, ID: "33333333333333333333333333333333",
				ParentIdentity: "parent", Kind: "backup",
				StagingName: stagingPrefix + "33333333333333333333333333333333", DestinationName: name,
			}
			receipt.Revision, _ = residueReceiptRevision(receipt)
			if err := validateResidueReceipt(receipt); err == nil {
				t.Fatalf("invalid receipt destination accepted: %q", name)
			}
		})
	}

	for _, name := range []string{"concrete", "com10", "auxiliary", "report.txt", ".hidden"} {
		if !validResidueLeaf(name) {
			t.Fatalf("valid Windows leaf rejected: %q", name)
		}
	}
}
