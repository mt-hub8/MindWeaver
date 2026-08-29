//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/internal/backup"
	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
	"golang.org/x/sys/windows"
)

func TestRecoveryCLIRealBackupVerifyAndCleanMachineRestore(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	cache := filepath.Join(root, "local-app-data-canary")
	if err := os.Mkdir(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	usePrivateRecoveryScratchForTest(t, cache)
	t.Setenv("LOCALAPPDATA", cache)

	activeRoot := filepath.Join(root, "active-vault-canary")
	backupPath := filepath.Join(root, "plaintext-backup-canary")
	createEmptyBackup(t, activeRoot, backupPath)

	var verifyOutput bytes.Buffer
	if err := run(t.Context(), []string{"recovery", "verify", "-backup", backupPath}, &verifyOutput); err != nil {
		t.Fatalf("recovery verify: %v", err)
	}
	assertSafeRecoveryRecords(t, verifyOutput.Bytes(), "verify", backupPath, activeRoot, cache)

	restoreParent := filepath.Join(root, "restore-parent-canary")
	if err := os.Mkdir(restoreParent, 0o700); err != nil {
		t.Fatal(err)
	}
	restoredRoot := filepath.Join(restoreParent, "restored-vault-canary")
	var restoreOutput bytes.Buffer
	if err := run(t.Context(), []string{
		"recovery", "restore", "-backup", backupPath, "-vault", restoredRoot,
	}, &restoreOutput); err != nil {
		t.Fatalf("recovery restore: %v", err)
	}
	assertSafeRecoveryRecords(t, restoreOutput.Bytes(), "restore", backupPath, activeRoot, restoredRoot, cache)

	restoredVault, err := vault.Open(restoredRoot)
	if err != nil {
		t.Fatalf("open restored Vault: %v", err)
	}
	restoredDatabase, err := store.Open(
		t.Context(), filepath.Join(restoredVault.Paths().Data, store.DatabaseFileName), store.Options{},
	)
	if err != nil {
		_ = restoredVault.Close()
		t.Fatalf("open restored database: %v", err)
	}
	if err := errors.Join(restoredDatabase.Close(), restoredVault.Close()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "mindweaver.v1.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery mode created normal configuration: %v", err)
	}
}

func TestRecoveryCLIFailureIsPathAndContentFree(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(root, "cache-path-canary")
	if err := os.Mkdir(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	usePrivateRecoveryScratchForTest(t, cache)
	t.Setenv("LOCALAPPDATA", cache)
	source := filepath.Join(root, "source-path-canary")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	secret := "PRIVATE-SOURCE-CONTENT-CANARY"
	if err := os.WriteFile(filepath.Join(source, "manifest.json"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	err := run(t.Context(), []string{"recovery", "verify", "-backup", source}, &output)
	if err == nil || apperror.PublicMessage(err) == "" {
		t.Fatalf("corrupt verify error = %v", err)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"type":"outcome"`)) ||
		!bytes.Contains(output.Bytes(), []byte(`"failure":"corrupt"`)) {
		t.Fatalf("corrupt verify omitted stable outcome: %q", output.Bytes())
	}
	for _, exposed := range []string{root, source, cache, secret} {
		for _, surface := range []string{output.String(), err.Error(), apperror.PublicMessage(err)} {
			if strings.Contains(surface, exposed) {
				t.Fatalf("recovery surface leaked %q: %q", exposed, surface)
			}
		}
	}
}

func TestRecoveryLeafMatchingRequiresExactBytes(t *testing.T) {
	if sameRecoveryLeaf("Restored-Vault", "restored-vault") {
		t.Fatal("case-variant restore target was treated as the exact residue target")
	}
	if !sameRecoveryLeaf("restored-vault", "restored-vault") {
		t.Fatal("identical restore target was rejected")
	}
}

func usePrivateRecoveryScratchForTest(t *testing.T, scratch string) {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		"O:" + user.User.Sid.String() + "D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")",
	)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(
		scratch,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|
			windows.PROTECTED_DACL_SECURITY_INFORMATION,
		owner, nil, dacl, nil,
	); err != nil {
		t.Fatal(err)
	}
	original := prepareRecoveryVerifyScratch
	prepareRecoveryVerifyScratch = func(string) (string, error) { return scratch, nil }
	t.Cleanup(func() { prepareRecoveryVerifyScratch = original })
}

func createEmptyBackup(t *testing.T, activeRoot, destination string) {
	t.Helper()
	activeVault, err := vault.Open(activeRoot)
	if err != nil {
		t.Fatal(err)
	}
	paths := activeVault.Paths()
	blobs, err := blob.OpenStore(paths.Blobs)
	if err != nil {
		_ = activeVault.Close()
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), filepath.Join(paths.Data, store.DatabaseFileName), store.Options{})
	if err != nil {
		_ = activeVault.Close()
		t.Fatal(err)
	}
	coordinator, err := backup.New(database, blobs, paths.Root)
	if err != nil {
		_ = database.Close()
		_ = activeVault.Close()
		t.Fatal(err)
	}
	_, createErr := coordinator.Create(t.Context(), destination)
	if err := errors.Join(createErr, coordinator.Close(), database.Close(), activeVault.Close()); err != nil {
		t.Fatal(err)
	}
}

func assertSafeRecoveryRecords(t *testing.T, encoded []byte, operation string, forbidden ...string) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	seenOutcome := false
	for {
		var record recoveryRecord
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode recovery record: %v; output=%q", err, encoded)
		}
		if record.Type != "progress" && record.Type != "outcome" {
			t.Fatalf("recovery record type = %q", record.Type)
		}
		if record.Operation != operation {
			t.Fatalf("recovery record operation = %q, want %q", record.Operation, operation)
		}
		if record.Type == "outcome" {
			seenOutcome = true
			if record.Outcome == nil || !record.Outcome.Succeeded || record.Outcome.Failure != "" || record.Outcome.CleanupRequired {
				t.Fatalf("recovery outcome = %+v", record.Outcome)
			}
		}
	}
	if !seenOutcome {
		t.Fatalf("recovery output omitted outcome: %q", encoded)
	}
	for _, value := range forbidden {
		if value != "" && bytes.Contains(encoded, []byte(value)) {
			t.Fatalf("recovery output leaked %q: %q", value, encoded)
		}
	}
}
