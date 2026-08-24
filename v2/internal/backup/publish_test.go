package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishDirectoryNeverReplacesRacingDestination(t *testing.T) {
	parent := t.TempDir()
	staging := filepath.Join(parent, "staging")
	destination := filepath.Join(parent, "destination")
	if err := os.Mkdir(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "new"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	// This models another launcher creating the destination after the initial
	// newDestination check but before publication.
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "owner"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := publishDirectory(staging, destination, parent); err == nil {
		t.Fatal("publication unexpectedly replaced a racing destination")
	}
	data, err := os.ReadFile(filepath.Join(destination, "owner"))
	if err != nil || string(data) != "existing" {
		t.Fatalf("racing destination changed: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(staging, "new")); err != nil {
		t.Fatalf("failed publication consumed staging tree: %v", err)
	}
}
