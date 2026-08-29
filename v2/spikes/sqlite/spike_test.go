package sqliteprobe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/ncruces/go-sqlite3"
)

func TestCandidateVersionIsPinned(t *testing.T) {
	if got := dependencyVersion(CandidateModule); got != CandidateVersion {
		t.Fatalf("built %s version %q, want %q", CandidateModule, got, CandidateVersion)
	}
	info, ok := debug.ReadBuildInfo()
	if !ok || info.GoVersion == "" {
		t.Fatal("missing Go build information")
	}
}

func TestConnectionPragmas(t *testing.T) {
	details, err := probeConnectionPragmas(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if details["physical_connections_checked"] != 3 {
		t.Fatalf("unexpected evidence: %#v", details)
	}
}

func TestFTS5RealProbe(t *testing.T) {
	if _, err := probeFTS5(t.Context(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func TestWALOneWriterAndReaders(t *testing.T) {
	if _, err := probeWALWriterReaders(t.Context(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func TestContextCancellationLeavesConnectionHealthy(t *testing.T) {
	details, err := probeCancellationHealth(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if details["health_query"] != 42 {
		t.Fatalf("unexpected evidence: %#v", details)
	}
}

func TestSQLiteFullRollsBackAtomically(t *testing.T) {
	details, err := probeFullAtomicRollback(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	info, ok := details["error"].(ErrorInfo)
	if !ok || info.PrimaryCode != 13 {
		t.Fatalf("unexpected SQLITE_FULL evidence: %#v", details)
	}
}

func TestOnlineBackupRestoreIntegrity(t *testing.T) {
	if _, err := probeOnlineBackupRestore(t.Context(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func TestNumericErrorClassification(t *testing.T) {
	if _, err := probeNumericErrorCodes(t.Context(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		err       error
		code      int
		category  string
		retryable bool
	}{
		{sqlite3.BUSY, 5, "contention", true},
		{sqlite3.LOCKED, 6, "contention", true},
		{sqlite3.INTERRUPT, 9, "interrupted", false},
		{sqlite3.FULL, 13, "capacity", false},
		{sqlite3.CONSTRAINT_UNIQUE, 19, "constraint", false},
	}
	for _, test := range tests {
		info, ok := ClassifySQLiteError(fmt.Errorf("wrapped: %w", test.err))
		if !ok || info.PrimaryCode != test.code || info.Category != test.category || info.Retryable != test.retryable {
			t.Errorf("ClassifySQLiteError(%v) = %#v, %v", test.err, info, ok)
		}
	}
	if _, ok := ClassifySQLiteError(context.Canceled); ok {
		t.Fatal("context error must not be misclassified as SQLite")
	}
}

func TestTwoProcessBusyKillAndRecovery(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helper := HelperProcess{
		Executable: executable,
		PrefixArgs: []string{"-test.run=^TestSQLiteSubprocessHelper$", "--"},
		ExtraEnv:   []string{"MINDWEAVER_SQLITE_SPIKE_HELPER=1"},
	}
	details, err := probeTwoProcessLock(t.Context(), t.TempDir(), helper)
	if err != nil {
		t.Fatal(err)
	}
	if details["holder_force_killed"] != true || details["writer_lock_released_on_exit"] != true {
		t.Fatalf("unexpected process evidence: %#v", details)
	}
}

func TestSQLiteSubprocessHelper(t *testing.T) {
	if os.Getenv("MINDWEAVER_SQLITE_SPIKE_HELPER") != "1" {
		return
	}
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		fmt.Fprintln(os.Stderr, "missing subprocess helper arguments")
		os.Exit(2)
	}
	if err := RunHelperCommand(context.Background(), os.Args[separator+1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func TestReportJSONContract(t *testing.T) {
	report := Report{
		SchemaVersion: 1,
		Passed:        true,
		Candidate: CandidateInfo{
			Module:        CandidateModule,
			PinnedVersion: CandidateVersion,
		},
		SQLite: SQLiteInfo{Version: "test", SourceID: "test-source-id"},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"schema_version"`, `"duration_ms"`, `"source_id"`, `"rss"`, `"probes"`} {
		if !containsJSONField(data, field) {
			t.Errorf("JSON is missing %s: %s", field, data)
		}
	}
}

func TestHelperRejectsNonTemporaryDatabase(t *testing.T) {
	nonTemp := filepath.Join(filepath.VolumeName(os.TempDir())+string(filepath.Separator), "definitely-not-temp", "user.db")
	if err := requireTemporaryPath(nonTemp); err == nil {
		t.Fatalf("expected guard to reject %q", nonTemp)
	}
	if _, ok := ClassifySQLiteError(errors.New("not sqlite")); ok {
		t.Fatal("plain errors must not be classified as SQLite")
	}
}

func containsJSONField(data []byte, field string) bool {
	for i := 0; i+len(field) <= len(data); i++ {
		if string(data[i:i+len(field)]) == field {
			return true
		}
	}
	return false
}
