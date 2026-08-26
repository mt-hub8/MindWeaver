//go:build windows

package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

func recoverResidueForTest(t *testing.T, ctx context.Context, parentPath string, expected Residue) error {
	t.Helper()
	activeRoot := filepath.Join(t.TempDir(), "active-vault")
	if err := os.Mkdir(activeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	active, err := openRetainedDirectory(activeRoot)
	if err != nil {
		t.Fatal(err)
	}
	coordinator := &Coordinator{activeVault: active}
	defer coordinator.Close()
	return coordinator.RecoverResidue(ctx, parentPath, expected)
}

func TestResidueReceiptWriteIsCompleteAndWireFormatCanonical(t *testing.T) {
	encoded := []byte("complete receipt")
	calls := 0
	err := writeFullResidueReceipt(nil, encoded, func(_ *os.File, remaining []byte) (int, error) {
		calls++
		if calls == 1 {
			return len(remaining) - 1, nil
		}
		return len(remaining), nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("complete short write retry = %v, calls=%d", err, calls)
	}
	err = writeFullResidueReceipt(nil, encoded, func(_ *os.File, remaining []byte) (int, error) {
		if len(remaining) == len(encoded) {
			return len(remaining) - 1, nil
		}
		return 0, nil
	})
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("zero-progress short write = %v, want io.ErrShortWrite", err)
	}

	staging := &stagingDirectory{
		name: stagingPrefix + "22222222222222222222222222222222", operationID: "22222222222222222222222222222222",
		kind: "backup", destinationName: "destination", parentIdentity: "parent", identityToken: "unused",
	}
	receipt := receiptForStaging(staging)
	receipt.Revision, err = residueReceiptRevision(receipt)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	canonical = append(canonical, '\n')
	if _, err := decodeResidueReceipt(t.Context(), residueReceiptName(receipt.ID), canonical); err != nil {
		t.Fatalf("canonical receipt rejected: %v", err)
	}
	nonCanonical := append(append([]byte(nil), canonical[:len(canonical)-1]...), ' ', '\n')
	if _, err := decodeResidueReceipt(t.Context(), residueReceiptName(receipt.ID), nonCanonical); err == nil {
		t.Fatal("non-canonical receipt accepted")
	}
}

const (
	residueHelperModeEnv   = "MW_BACKUP_RESIDUE_HELPER_MODE"
	residueHelperParentEnv = "MW_BACKUP_RESIDUE_HELPER_PARENT"
	residueHelperKindEnv   = "MW_BACKUP_RESIDUE_HELPER_KIND"
	residueHelperIDEnv     = "MW_BACKUP_RESIDUE_HELPER_ID"
)

func TestResidueForcedExitHelper(t *testing.T) {
	mode := os.Getenv(residueHelperModeEnv)
	if mode == "" {
		return
	}
	parentPath := os.Getenv(residueHelperParentEnv)
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		os.Exit(80)
	}
	prefix := stagingPrefix
	if os.Getenv(residueHelperKindEnv) == "restore" {
		prefix = restorePrefix
	}
	switch mode {
	case "temp":
		_, _ = createStagingDirectoryWithHooks(parent, prefix, "destination", stagingCreationHooks{
			afterReceiptTempCreate: func(file *os.File) error {
				_, _ = file.Write([]byte("{\"partial\":"))
				os.Exit(90)
				return nil
			},
		})
	case "receipt":
		_, _ = createStagingDirectoryWithHooks(parent, prefix, "destination", stagingCreationHooks{
			afterReceiptDurable: func(*stagingDirectory) error { os.Exit(91); return nil },
		})
	case "directory":
		_, _ = createStagingDirectoryWithHooks(parent, prefix, "destination", stagingCreationHooks{
			afterDirectoryDurable: func(*stagingDirectory) error { os.Exit(92); return nil },
		})
	case "binding":
		_, _ = createStagingDirectoryWithHooks(parent, prefix, "destination", stagingCreationHooks{
			afterBindingDurable: func(*stagingDirectory) error { os.Exit(95); return nil },
		})
	case "copy":
		staging, createErr := createStagingDirectory(parent, prefix, "destination")
		if createErr == nil {
			_ = staging.directory.root.MkdirAll("partial", 0o700)
			_ = staging.directory.root.WriteFile("partial/payload", []byte("copy interrupted"), 0o600)
			os.Exit(93)
		}
	case "rename":
		staging, createErr := createStagingDirectory(parent, prefix, "destination")
		if createErr == nil {
			_ = staging.directory.root.WriteFile("owned", []byte("renamed destination"), 0o600)
			expected := staging.directory.identity
			_ = staging.directory.Close()
			_ = closeStagingCreationWitness(staging)
			_, _ = publishDirectory(
				staging.directory.path,
				filepath.Join(parentPath, "destination"),
				parent,
				"destination",
				publicationAttempt{
					sourceName: staging.name,
					expected:   expected,
					ctx:        context.Background(),
					verify:     acceptPublishedContent,
					hooks: publicationHooks{
						syncParent: func(*retainedDirectory) error { os.Exit(94); return nil },
					},
				},
			)
		}
	case "confirmation-cleanup-gap":
		id := os.Getenv(residueHelperIDEnv)
		name := residueReceiptName(id)
		receipt, info, readErr := readResidueReceipt(context.Background(), parent, name)
		if readErr != nil || receipt.ID != id || parent.acquireSyncHandle() != nil {
			os.Exit(96)
		}
		lease, lockErr := openAndLockResidueReceipt(parent, name, info)
		if lockErr != nil {
			os.Exit(96)
		}
		locked, lockedInfo, lockedErr := readLockedResidueReceipt(context.Background(), lease, name)
		if lockedErr != nil || locked != receipt || !os.SameFile(info, lockedInfo) {
			_ = unlockResidueFile(lease)
			_ = lease.Close()
			os.Exit(96)
		}
		if deleteResidueMutationFile(parent, name, info, lease) != nil || syncRetainedDirectory(parent) != nil {
			os.Exit(96)
		}
		os.Exit(97)
	}
	_ = parent.Close()
	os.Exit(81)
}

func TestForcedExitResiduesAreListedAndExplicitlyRecovered(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("mutating backup residue recovery is qualified only on Windows")
	}
	tests := []struct {
		mode             string
		exitCode         int
		state            ResidueState
		destinationLives bool
		recoveryError    error
	}{
		{mode: "receipt", exitCode: 91, state: ResidueStateReceiptOnly},
		{mode: "directory", exitCode: 92, state: ResidueStateConflict, recoveryError: ErrResidueConflict},
		{mode: "binding", exitCode: 95, state: ResidueStateStaging},
		{mode: "copy", exitCode: 93, state: ResidueStateStaging},
		{mode: "rename", exitCode: 94, state: ResidueStatePublicationUncertain, destinationLives: true, recoveryError: ErrPublicationUncertain},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			parentPath := t.TempDir()
			unowned := stagingPrefix + "00000000000000000000000000000000"
			if err := os.Mkdir(filepath.Join(parentPath, unowned), 0o700); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(os.Args[0], "-test.run=^TestResidueForcedExitHelper$")
			command.Env = append(os.Environ(),
				residueHelperModeEnv+"="+test.mode,
				residueHelperParentEnv+"="+parentPath,
			)
			err := command.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != test.exitCode {
				t.Fatalf("helper exit = %v, want %d", err, test.exitCode)
			}
			page, err := ListResidues(t.Context(), parentPath, 10)
			if err != nil {
				t.Fatal(err)
			}
			if page.Truncated || len(page.Items) != 1 || page.Items[0].State != test.state {
				t.Fatalf("residue page = %+v, want one %s", page, test.state)
			}
			listed := page.Items[0]
			wrongIdentity := listed
			wrongIdentity.Identity += "-stale"
			if err := recoverResidueForTest(t, t.Context(), parentPath, wrongIdentity); !errors.Is(err, ErrResidueConflict) {
				t.Fatalf("wrong identity recovery = %v, want ErrResidueConflict", err)
			}
			wrongRevision := listed
			if wrongRevision.Revision[0] == '0' {
				wrongRevision.Revision = "1" + wrongRevision.Revision[1:]
			} else {
				wrongRevision.Revision = "0" + wrongRevision.Revision[1:]
			}
			if err := recoverResidueForTest(t, t.Context(), parentPath, wrongRevision); !errors.Is(err, ErrResidueConflict) {
				t.Fatalf("wrong revision recovery = %v, want ErrResidueConflict", err)
			}
			if test.recoveryError != nil {
				if err := recoverResidueForTest(t, t.Context(), parentPath, listed); !errors.Is(err, test.recoveryError) {
					t.Fatalf("unsafe residue recovery = %v, want %v", err, test.recoveryError)
				}
				after, err := ListResidues(t.Context(), parentPath, 10)
				if err != nil || len(after.Items) != 1 || after.Items[0] != listed {
					t.Fatalf("failed recovery erased or changed evidence: %+v, %v", after, err)
				}
				if test.destinationLives {
					data, err := os.ReadFile(filepath.Join(parentPath, "destination", "owned"))
					if err != nil || string(data) != "renamed destination" {
						t.Fatalf("publication-uncertain destination changed: %q, %v", data, err)
					}
				}
				return
			}
			if err := recoverResidueForTest(t, t.Context(), parentPath, listed); err != nil {
				t.Fatalf("recover residue: %v", err)
			}
			page, err = ListResidues(t.Context(), parentPath, 10)
			if err != nil || len(page.Items) != 0 {
				t.Fatalf("residue remains after recovery: %+v, %v", page, err)
			}
			if _, err := os.Stat(filepath.Join(parentPath, unowned)); err != nil {
				t.Fatalf("unreceipted prefix directory changed: %v", err)
			}
			destinationData, destinationErr := os.ReadFile(filepath.Join(parentPath, "destination", "owned"))
			if test.destinationLives {
				if destinationErr != nil || string(destinationData) != "renamed destination" {
					t.Fatalf("publication-uncertain destination changed: %q, %v", destinationData, destinationErr)
				}
			} else if !errors.Is(destinationErr, os.ErrNotExist) {
				t.Fatalf("unexpected destination after %s recovery: %v", test.mode, destinationErr)
			}
		})
	}
}

func runRestoreResidueForcedExit(t *testing.T, parentPath, mode string, exitCode int) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestResidueForcedExitHelper$")
	command.Env = append(os.Environ(),
		residueHelperModeEnv+"="+mode,
		residueHelperParentEnv+"="+parentPath,
		residueHelperKindEnv+"=restore",
	)
	err := command.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != exitCode {
		t.Fatalf("restore %s helper exit = %v, want %d", mode, err, exitCode)
	}
}

func TestStartupRestoreResidueRecoveryRecoversOnlyExactForcedExitResidues(t *testing.T) {
	t.Run("bounded staging recovery", func(t *testing.T) {
		parentPath := t.TempDir()
		runRestoreResidueForcedExit(t, parentPath, "binding", 95)
		runRestoreResidueForcedExit(t, parentPath, "copy", 93)

		recovery, err := NewStartupRestoreResidueRecovery(t.TempDir(), parentPath)
		if err != nil {
			t.Fatal(err)
		}
		defer recovery.Close()

		page, err := recovery.List(t.Context(), 1)
		if err != nil || !page.Truncated || len(page.Items) != 1 ||
			page.Items[0].Kind != "restore" || page.Items[0].State != ResidueStateStaging {
			t.Fatalf("first startup restore residue page = %+v, %v", page, err)
		}
		listed := page.Items[0]
		stale := listed
		stale.Identity += "-stale"
		outcome, err := recovery.Recover(t.Context(), stale)
		if err == nil || outcome.Succeeded || outcome.Failure != FailureInvalid ||
			!errors.Is(err, ErrResidueConflict) {
			t.Fatalf("stale startup restore recovery = %+v, %v", outcome, err)
		}
		assertPathFreeBackupError(t, err, parentPath, listed.StagingName)

		outcome, err = recovery.Recover(t.Context(), listed)
		if err != nil || !outcome.Succeeded || outcome.Failure != "" || outcome.CleanupRequired {
			t.Fatalf("exact startup restore recovery = %+v, %v", outcome, err)
		}
		page, err = recovery.List(t.Context(), 1)
		if err != nil || page.Truncated || len(page.Items) != 1 ||
			page.Items[0].Kind != "restore" || page.Items[0].State != ResidueStateStaging {
			t.Fatalf("second startup restore residue page = %+v, %v", page, err)
		}
		outcome, err = recovery.Recover(t.Context(), page.Items[0])
		if err != nil || !outcome.Succeeded {
			t.Fatalf("second startup restore recovery = %+v, %v", outcome, err)
		}
		page, err = recovery.List(t.Context(), 1)
		if err != nil || page.Truncated || len(page.Items) != 0 {
			t.Fatalf("startup restore residues remain = %+v, %v", page, err)
		}
	})

	t.Run("receipt only", func(t *testing.T) {
		parentPath := t.TempDir()
		runRestoreResidueForcedExit(t, parentPath, "receipt", 91)
		recovery, err := NewStartupRestoreResidueRecovery(t.TempDir(), parentPath)
		if err != nil {
			t.Fatal(err)
		}
		defer recovery.Close()
		page, err := recovery.List(t.Context(), 1)
		if err != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStateReceiptOnly {
			t.Fatalf("receipt-only restore residue = %+v, %v", page, err)
		}
		outcome, err := recovery.Recover(t.Context(), page.Items[0])
		if err != nil || !outcome.Succeeded {
			t.Fatalf("receipt-only startup recovery = %+v, %v", outcome, err)
		}
	})

	t.Run("prebinding conflict", func(t *testing.T) {
		parentPath := t.TempDir()
		runRestoreResidueForcedExit(t, parentPath, "directory", 92)
		recovery, err := NewStartupRestoreResidueRecovery(t.TempDir(), parentPath)
		if err != nil {
			t.Fatal(err)
		}
		defer recovery.Close()
		page, err := recovery.List(t.Context(), 1)
		if err != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStateConflict {
			t.Fatalf("prebinding startup residue = %+v, %v", page, err)
		}
		listed := page.Items[0]
		outcome, err := recovery.Recover(t.Context(), listed)
		if err == nil || outcome.Succeeded || outcome.Failure != FailureInvalid ||
			!errors.Is(err, ErrResidueConflict) {
			t.Fatalf("prebinding startup recovery = %+v, %v", outcome, err)
		}
		assertPathFreeBackupError(t, err, parentPath, listed.StagingName)
		after, listErr := recovery.List(t.Context(), 1)
		if listErr != nil || len(after.Items) != 1 || after.Items[0] != listed {
			t.Fatalf("conflict recovery changed evidence = %+v, %v", after, listErr)
		}
	})

	t.Run("publication uncertain", func(t *testing.T) {
		parentPath := t.TempDir()
		runRestoreResidueForcedExit(t, parentPath, "rename", 94)
		recovery, err := NewStartupRestoreResidueRecovery(t.TempDir(), parentPath)
		if err != nil {
			t.Fatal(err)
		}
		defer recovery.Close()
		page, err := recovery.List(t.Context(), 1)
		if err != nil || len(page.Items) != 1 ||
			page.Items[0].State != ResidueStatePublicationUncertain {
			t.Fatalf("publication-uncertain startup residue = %+v, %v", page, err)
		}
		listed := page.Items[0]
		outcome, err := recovery.Recover(t.Context(), listed)
		if err == nil || outcome.Succeeded || outcome.Failure != FailurePublicationUncertain ||
			!errors.Is(err, ErrPublicationUncertain) {
			t.Fatalf("publication-uncertain startup recovery = %+v, %v", outcome, err)
		}
		assertPathFreeBackupError(t, err, parentPath, listed.DestinationName)
		after, listErr := recovery.List(t.Context(), 1)
		if listErr != nil || len(after.Items) != 1 || after.Items[0] != listed {
			t.Fatalf("uncertain recovery changed evidence = %+v, %v", after, listErr)
		}
		data, readErr := os.ReadFile(filepath.Join(parentPath, "destination", "owned"))
		if readErr != nil || string(data) != "renamed destination" {
			t.Fatalf("uncertain destination changed = %q, %v", data, readErr)
		}
	})
}

func TestPartialReceiptTempNeverBecomesAuthorityOrBlocksListing(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("mutating backup residue creation is qualified only on Windows")
	}
	parentPath := t.TempDir()
	run := func(mode string, exitCode int) {
		t.Helper()
		command := exec.Command(os.Args[0], "-test.run=^TestResidueForcedExitHelper$")
		command.Env = append(os.Environ(),
			residueHelperModeEnv+"="+mode,
			residueHelperParentEnv+"="+parentPath,
		)
		err := command.Run()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != exitCode {
			t.Fatalf("%s helper exit = %v, want %d", mode, err, exitCode)
		}
	}
	run("temp", 90)
	run("receipt", 91)
	page, err := ListResidues(t.Context(), parentPath, 10)
	if err != nil || page.Truncated || len(page.Items) != 1 || page.Items[0].State != ResidueStateReceiptOnly {
		t.Fatalf("partial temp blocked authoritative residue listing: %+v, %v", page, err)
	}
	if err := recoverResidueForTest(t, t.Context(), parentPath, page.Items[0]); err != nil {
		t.Fatalf("recover valid receipt beside partial temp: %v", err)
	}
	entries, err := os.ReadDir(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	foundPartial := false
	for _, entry := range entries {
		if len(entry.Name()) >= len(residueTempPrefix) && entry.Name()[:len(residueTempPrefix)] == residueTempPrefix {
			foundPartial = true
		}
	}
	if !foundPartial {
		t.Fatal("forced exit did not leave the expected non-authoritative temp evidence")
	}
}

func TestCrashThenStagingReplacementIsConflictAndNeverDeleted(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("mutating backup residue recovery is qualified only on Windows")
	}
	parentPath := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestResidueForcedExitHelper$")
	command.Env = append(os.Environ(),
		residueHelperModeEnv+"=copy",
		residueHelperParentEnv+"="+parentPath,
	)
	err := command.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 93 {
		t.Fatalf("copy helper exit = %v, want 93", err)
	}
	page, err := ListResidues(t.Context(), parentPath, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStateStaging {
		t.Fatalf("initial residue = %+v, %v", page, err)
	}
	stagingPath := filepath.Join(parentPath, page.Items[0].StagingName)
	originalPath := filepath.Join(parentPath, "original-staging")
	if err := os.Rename(stagingPath, originalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(stagingPath, 0o700); err != nil {
		t.Fatal(err)
	}
	canaryPath := filepath.Join(stagingPath, "replacement-canary")
	if err := os.WriteFile(canaryPath, []byte("replacement must survive"), 0o600); err != nil {
		t.Fatal(err)
	}
	page, err = ListResidues(t.Context(), parentPath, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStateConflict {
		t.Fatalf("replacement residue = %+v, %v, want conflict", page, err)
	}
	if err := recoverResidueForTest(t, t.Context(), parentPath, page.Items[0]); !errors.Is(err, ErrResidueConflict) {
		t.Fatalf("replacement recovery = %v, want ErrResidueConflict", err)
	}
	data, err := os.ReadFile(canaryPath)
	if err != nil || string(data) != "replacement must survive" {
		t.Fatalf("replacement tree changed: %q, %v", data, err)
	}
}

func TestBoundTreeRenamedAwayRemainsConflictEvidence(t *testing.T) {
	parentPath := t.TempDir()
	command := exec.Command(os.Args[0], "-test.run=^TestResidueForcedExitHelper$")
	command.Env = append(os.Environ(),
		residueHelperModeEnv+"=copy",
		residueHelperParentEnv+"="+parentPath,
	)
	err := command.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 93 {
		t.Fatalf("copy helper exit = %v, want 93", err)
	}
	page, err := ListResidues(t.Context(), parentPath, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStateStaging {
		t.Fatalf("initial residue = %+v, %v", page, err)
	}
	listed := page.Items[0]
	movedPath := filepath.Join(parentPath, "operator-moved-staging")
	if err := os.Rename(filepath.Join(parentPath, listed.StagingName), movedPath); err != nil {
		t.Fatal(err)
	}

	page, err = ListResidues(t.Context(), parentPath, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStateConflict {
		t.Fatalf("renamed-away residue = %+v, %v, want conflict", page, err)
	}
	if err := recoverResidueForTest(t, t.Context(), parentPath, page.Items[0]); !errors.Is(err, ErrResidueConflict) {
		t.Fatalf("renamed-away recovery = %v, want ErrResidueConflict", err)
	}
	data, err := os.ReadFile(filepath.Join(movedPath, "partial", "payload"))
	if err != nil || string(data) != "copy interrupted" {
		t.Fatalf("renamed-away tree changed: %q, %v", data, err)
	}
	for _, name := range []string{residueReceiptName(listed.ID), residueBindingName(listed.ID)} {
		if info, err := os.Lstat(filepath.Join(parentPath, name)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("conflict evidence %s missing: %v", name, err)
		}
	}
}

func createRecoverableStagingForTest(t *testing.T, parentPath string) (Residue, string) {
	t.Helper()
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	staging, err := createStagingDirectory(parent, stagingPrefix, "destination")
	if err != nil {
		_ = parent.Close()
		t.Fatal(err)
	}
	canaryPath := filepath.Join(staging.directory.path, "active-canary")
	if err := staging.directory.root.WriteFile("active-canary", []byte("must survive active overlap"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(
		staging.directory.Close(),
		closeStagingCreationWitness(staging),
		closeResidueLease(staging),
		parent.Close(),
	); err != nil {
		t.Fatal(err)
	}
	page, err := ListResidues(t.Context(), parentPath, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStateStaging {
		t.Fatalf("recoverable staging = %+v, %v", page, err)
	}
	return page.Items[0], canaryPath
}

func assertActiveRecoveryRejected(t *testing.T, activePath, parentPath string, listed Residue, canaryPath string) {
	t.Helper()
	active, err := openRetainedDirectory(activePath)
	if err != nil {
		t.Fatal(err)
	}
	coordinator := &Coordinator{activeVault: active}
	defer coordinator.Close()
	if err := coordinator.RecoverResidue(t.Context(), parentPath, listed); !errors.Is(err, ErrActiveVaultOverlap) {
		t.Fatalf("active-overlap recovery = %v, want ErrActiveVaultOverlap", err)
	}
	data, err := os.ReadFile(canaryPath)
	if err != nil || string(data) != "must survive active overlap" {
		t.Fatalf("active canary changed: %q, %v", data, err)
	}
	page, err := ListResidues(t.Context(), parentPath, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0] != listed {
		t.Fatalf("active-overlap recovery changed evidence: %+v, %v", page, err)
	}
}

func TestRecoverResidueRejectsTreesOverlappingActiveVault(t *testing.T) {
	t.Run("staging is active Vault", func(t *testing.T) {
		parentPath := t.TempDir()
		listed, canaryPath := createRecoverableStagingForTest(t, parentPath)
		assertActiveRecoveryRejected(
			t,
			filepath.Join(parentPath, listed.StagingName),
			parentPath,
			listed,
			canaryPath,
		)
	})

	t.Run("staging is inside active Vault", func(t *testing.T) {
		activePath := t.TempDir()
		listed, canaryPath := createRecoverableStagingForTest(t, activePath)
		assertActiveRecoveryRejected(t, activePath, activePath, listed, canaryPath)
	})

	t.Run("staging contains active Vault", func(t *testing.T) {
		parentPath := t.TempDir()
		listed, canaryPath := createRecoverableStagingForTest(t, parentPath)
		activePath := filepath.Join(parentPath, listed.StagingName, "nested-active-vault")
		if err := os.Mkdir(activePath, 0o700); err != nil {
			t.Fatal(err)
		}
		assertActiveRecoveryRejected(t, activePath, parentPath, listed, canaryPath)
	})
}

func TestCorruptIdentityBindingIsConflictAndNeverDeletesTree(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("mutating backup residue recovery is qualified only on Windows")
	}
	parentPath := t.TempDir()
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	staging, err := createStagingDirectory(parent, stagingPrefix, "destination")
	if err != nil {
		_ = parent.Close()
		t.Fatal(err)
	}
	canaryPath := filepath.Join(staging.directory.path, "owned-canary")
	if err := os.WriteFile(canaryPath, []byte("must survive corrupt binding"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(
		staging.directory.Close(),
		closeStagingCreationWitness(staging),
		closeResidueLease(staging),
		parent.Close(),
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parentPath, staging.bindingName), []byte("{\"partial\":"), 0o600); err != nil {
		t.Fatal(err)
	}
	page, err := ListResidues(t.Context(), parentPath, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStateConflict {
		t.Fatalf("corrupt binding residue = %+v, %v, want conflict", page, err)
	}
	if err := recoverResidueForTest(t, t.Context(), parentPath, page.Items[0]); !errors.Is(err, ErrResidueConflict) {
		t.Fatalf("corrupt binding recovery = %v, want ErrResidueConflict", err)
	}
	data, err := os.ReadFile(canaryPath)
	if err != nil || string(data) != "must survive corrupt binding" {
		t.Fatalf("tree changed after corrupt binding: %q, %v", data, err)
	}
}

func TestRecoverResidueRejectsActiveReceiptLease(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staging, err := createStagingDirectory(parent, stagingPrefix, "destination")
	if err != nil {
		t.Fatal(err)
	}
	page, err := ListResidues(t.Context(), parentPath, 1)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("list active receipt = %+v, %v", page, err)
	}
	if err := recoverResidueForTest(t, t.Context(), parentPath, page.Items[0]); !errors.Is(err, ErrResidueActive) {
		t.Fatalf("active recovery = %v, want ErrResidueActive", err)
	}
	if err := cleanupStagingTree(staging, parent, "test"); err != nil {
		t.Fatal(err)
	}
	if page, err := ListResidues(t.Context(), parentPath, 1); err != nil || len(page.Items) != 0 {
		t.Fatalf("active cleanup residue = %+v, %v", page, err)
	}
}

func TestBindingPersistenceFailureLeavesConflictForExplicitRecoveryPolicy(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("mutating backup residue cleanup is qualified only on Windows")
	}
	parentPath := t.TempDir()
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	injected := errors.New("injected identity binding persistence failure")
	_, err = createStagingDirectoryWithHooks(parent, stagingPrefix, "destination", stagingCreationHooks{
		persistBinding: func(*retainedDirectory, *stagingDirectory) error { return injected },
	})
	if !errors.Is(err, injected) || !errors.Is(err, ErrCleanupResidual) {
		t.Fatalf("binding failure = %v, want injected error and ErrCleanupResidual", err)
	}
	page, err := ListResidues(t.Context(), parentPath, 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStateConflict {
		t.Fatalf("pre-binding failure residue = %+v, %v, want conflict", page, err)
	}
	if err := recoverResidueForTest(t, t.Context(), parentPath, page.Items[0]); !errors.Is(err, ErrResidueConflict) {
		t.Fatalf("pre-binding conflict recovery = %v, want ErrResidueConflict", err)
	}
	if _, err := os.Lstat(filepath.Join(parentPath, page.Items[0].StagingName)); err != nil {
		t.Fatalf("pre-binding staging was deleted: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(parentPath, residueBindingName(page.Items[0].ID))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-binding failure unexpectedly published identity sidecar: %v", err)
	}
}

func TestReceiptOnlyBindingCASRetainsMarkerWhenTreeAppearsDuringRecovery(t *testing.T) {
	parentPath := t.TempDir()
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if err := parent.acquireSyncHandle(); err != nil {
		t.Fatal(err)
	}
	staging, err := createStagingDirectory(parent, stagingPrefix, "destination")
	if err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(staging.directory.Close(), closeStagingCreationWitness(staging), closeResidueLease(staging)); err != nil {
		t.Fatal(err)
	}
	if err := parent.root.Remove(staging.name); err != nil {
		t.Fatal(err)
	}
	receiptLease, err := openAndLockResidueReceipt(parent, staging.receiptName, staging.receiptIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if err := deleteResidueMutationFile(parent, staging.receiptName, staging.receiptIdentity, receiptLease); err != nil {
		t.Fatal(err)
	}
	if err := syncRetainedDirectory(parent); err != nil {
		t.Fatal(err)
	}
	page, err := ListResidues(t.Context(), parentPath, 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStateReceiptOnly {
		t.Fatalf("binding-only residue = %+v, %v", page, err)
	}
	expected := page.Items[0]
	err = recoverResidue(t.Context(), parentPath, expected, func(*destinationTarget) error {
		return parent.root.Mkdir(expected.StagingName, 0o700)
	}, nil)
	if !errors.Is(err, ErrResidueConflict) {
		t.Fatalf("tree-appeared recovery error = %v, want ErrResidueConflict", err)
	}
	for _, name := range []string{residueBindingName(expected.ID), expected.StagingName} {
		if _, statErr := parent.root.Lstat(name); statErr != nil {
			t.Fatalf("CAS conflict removed evidence %q: %v", name, statErr)
		}
	}
}

func TestOrdinaryCreationHookFailuresEitherCleanOrReportDurableResidue(t *testing.T) {
	tests := []struct {
		name         string
		hooks        stagingCreationHooks
		wantResidual bool
		wantState    ResidueState
	}{
		{
			name: "after receipt",
			hooks: stagingCreationHooks{
				afterReceiptDurable: func(*stagingDirectory) error { return errors.New("after receipt") },
			},
		},
		{
			name: "after directory before identity binding",
			hooks: stagingCreationHooks{
				afterDirectoryDurable: func(*stagingDirectory) error { return errors.New("after directory") },
			},
			wantResidual: true,
			wantState:    ResidueStateConflict,
		},
		{
			name: "after identity binding",
			hooks: stagingCreationHooks{
				afterBindingDurable: func(*stagingDirectory) error { return errors.New("after binding") },
			},
			wantResidual: true,
			wantState:    ResidueStateStaging,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parentPath := t.TempDir()
			parent, err := openRetainedDirectory(parentPath)
			if err != nil {
				t.Fatal(err)
			}
			defer parent.Close()
			_, err = createStagingDirectoryWithHooks(parent, stagingPrefix, "destination", test.hooks)
			if err == nil {
				t.Fatal("injected creation failure returned nil")
			}
			if errors.Is(err, ErrCleanupResidual) != test.wantResidual {
				t.Fatalf("cleanup classification = %v, want residual=%v", err, test.wantResidual)
			}
			page, listErr := ListResidues(t.Context(), parentPath, 10)
			if listErr != nil {
				t.Fatal(listErr)
			}
			if !test.wantResidual {
				if len(page.Items) != 0 {
					t.Fatalf("ordinary hook failure left residue: %+v", page)
				}
				return
			}
			if len(page.Items) != 1 || page.Items[0].State != test.wantState {
				t.Fatalf("durable hook residue = %+v, want one %s", page, test.wantState)
			}
		})
	}
}

func TestCreationWitnessBlocksRenameAtEveryPreBindingWindow(t *testing.T) {
	injected := errors.New("stop at creation window")
	tests := []struct {
		name    string
		install func(*stagingCreationHooks, func(*stagingDirectory) error)
	}{
		{name: "atomic create", install: func(hooks *stagingCreationHooks, hook func(*stagingDirectory) error) {
			hooks.afterAtomicCreate = hook
		}},
		{name: "root adopted", install: func(hooks *stagingCreationHooks, hook func(*stagingDirectory) error) {
			hooks.afterRootAdopted = hook
		}},
		{name: "child synced", install: func(hooks *stagingCreationHooks, hook func(*stagingDirectory) error) {
			hooks.afterChildSync = hook
		}},
		{name: "parent synced before binding", install: func(hooks *stagingCreationHooks, hook func(*stagingDirectory) error) {
			hooks.afterDirectoryDurable = hook
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parentPath := t.TempDir()
			parent, err := openRetainedDirectory(parentPath)
			if err != nil {
				t.Fatal(err)
			}
			defer parent.Close()
			var operationID, stagingName string
			var expected os.FileInfo
			var renameErr error
			hooks := stagingCreationHooks{}
			test.install(&hooks, func(staging *stagingDirectory) error {
				operationID = staging.operationID
				stagingName = staging.name
				expected, err = staging.creationWitness.Stat()
				if err != nil {
					return err
				}
				renameErr = os.Rename(
					filepath.Join(parentPath, staging.name),
					filepath.Join(parentPath, staging.name+"-moved"),
				)
				return injected
			})
			_, err = createStagingDirectoryWithHooks(parent, stagingPrefix, "destination", hooks)
			if !errors.Is(err, injected) || !errors.Is(err, ErrCleanupResidual) {
				t.Fatalf("creation-window error = %v", err)
			}
			if renameErr == nil || !errors.Is(renameErr, windows.ERROR_SHARING_VIOLATION) {
				t.Fatalf("creation witness did not block rename: %v", renameErr)
			}
			current, err := os.Lstat(filepath.Join(parentPath, stagingName))
			if err != nil || !os.SameFile(expected, current) {
				t.Fatalf("created staging identity changed: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(parentPath, residueBindingName(operationID))); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("pre-binding window published an identity sidecar: %v", err)
			}
			page, err := ListResidues(t.Context(), parentPath, 1)
			if err != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStateConflict {
				t.Fatalf("pre-binding residue = %+v, %v, want conflict", page, err)
			}
		})
	}
}

func TestListResiduesIsBoundedAndIgnoresPrefixOnlyDirectories(t *testing.T) {
	parentPath := t.TempDir()
	for index := 0; index < directoryReadBatch+7; index++ {
		name := fmt.Sprintf("ordinary-%04d", index)
		if err := os.WriteFile(filepath.Join(parentPath, name), []byte("ordinary"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	unowned := stagingPrefix + "11111111111111111111111111111111"
	if err := os.Mkdir(filepath.Join(parentPath, unowned), 0o700); err != nil {
		t.Fatal(err)
	}
	page, err := ListResidues(t.Context(), parentPath, maxResiduePageSize)
	if err != nil || page.Truncated || len(page.Items) != 0 {
		t.Fatalf("prefix-only listing = %+v, %v", page, err)
	}
	if _, err := os.Stat(filepath.Join(parentPath, unowned)); err != nil {
		t.Fatalf("prefix-only directory changed: %v", err)
	}
}
