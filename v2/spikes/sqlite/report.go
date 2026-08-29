package sqliteprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

const (
	CandidateModule  = "github.com/ncruces/go-sqlite3"
	CandidateVersion = "v0.35.3"
)

type Options struct {
	TempRoot string
	KeepTemp bool
	Helper   HelperProcess
}

type Report struct {
	SchemaVersion  int           `json:"schema_version"`
	EvidenceStatus string        `json:"evidence_status"`
	Passed         bool          `json:"passed"`
	StartedAt      time.Time     `json:"started_at"`
	DurationMS     int64         `json:"duration_ms"`
	Candidate      CandidateInfo `json:"candidate"`
	Runtime        RuntimeInfo   `json:"runtime"`
	SQLite         SQLiteInfo    `json:"sqlite"`
	RSS            RSSInfo       `json:"rss"`
	Probes         []ProbeResult `json:"probes"`
	Notes          []string      `json:"notes"`
}

type CandidateInfo struct {
	Module          string `json:"module"`
	PinnedVersion   string `json:"pinned_version"`
	BuiltVersion    string `json:"built_version"`
	CGOFree         bool   `json:"cgo_free"`
	ComparisonScope string `json:"comparison_scope"`
}

type RuntimeInfo struct {
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	GoVersion  string `json:"go_version"`
	CGOEnabled string `json:"cgo_enabled"`
}

type SQLiteInfo struct {
	Version  string `json:"version"`
	SourceID string `json:"source_id"`
}

type RSSInfo struct {
	Supported      bool   `json:"supported"`
	Measurement    string `json:"measurement"`
	StartBytes     uint64 `json:"start_bytes"`
	EndBytes       uint64 `json:"end_bytes"`
	PeakBytes      uint64 `json:"peak_bytes"`
	MeasurementErr string `json:"measurement_error,omitempty"`
}

type ProbeResult struct {
	Name       string         `json:"name"`
	Passed     bool           `json:"passed"`
	DurationMS int64          `json:"duration_ms"`
	Details    map[string]any `json:"details,omitempty"`
	Error      string         `json:"error,omitempty"`
}

type probeFunc func(context.Context, string) (map[string]any, error)

func RunAll(ctx context.Context, options Options) Report {
	started := time.Now().UTC()
	startRSS := readRSS()
	report := Report{
		SchemaVersion:  1,
		EvidenceStatus: "spike_evidence_only_not_a_selection_decision",
		StartedAt:      started,
		Candidate: CandidateInfo{
			Module:          CandidateModule,
			PinnedVersion:   CandidateVersion,
			BuiltVersion:    dependencyVersion(CandidateModule),
			CGOFree:         true,
			ComparisonScope: "single-candidate Windows feasibility spike",
		},
		Runtime: RuntimeInfo{
			GOOS:       runtime.GOOS,
			GOARCH:     runtime.GOARCH,
			GoVersion:  runtime.Version(),
			CGOEnabled: buildSetting("CGO_ENABLED"),
		},
		Notes: []string{
			"This output is reproducible feasibility evidence, not a production dependency decision.",
			"All databases and process-coordination files are created below the OS temporary directory.",
			"The optional modernc comparison is intentionally outside this non-blocking spike.",
		},
	}

	root := options.TempRoot
	removeRoot := false
	if root == "" {
		var err error
		root, err = os.MkdirTemp("", "mindweaver-sqlite-spike-")
		if err != nil {
			report.Probes = append(report.Probes, ProbeResult{Name: "temporary_workspace", Error: err.Error()})
			return finishReport(report, started, startRSS)
		}
		removeRoot = !options.KeepTemp
	} else {
		if err := requireTemporaryPath(filepath.Join(root, "probe.guard")); err != nil {
			report.Probes = append(report.Probes, ProbeResult{Name: "temporary_workspace", Error: err.Error()})
			return finishReport(report, started, startRSS)
		}
		if err := os.MkdirAll(root, 0o700); err != nil {
			report.Probes = append(report.Probes, ProbeResult{Name: "temporary_workspace", Error: err.Error()})
			return finishReport(report, started, startRSS)
		}
	}
	if removeRoot {
		defer os.RemoveAll(root)
	}

	metadataDir := filepath.Join(root, "engine_metadata")
	if err := os.MkdirAll(metadataDir, 0o700); err != nil {
		report.Probes = append(report.Probes, ProbeResult{Name: "engine_metadata", Error: err.Error()})
	} else {
		report.Probes = append(report.Probes, runProbe(ctx, "engine_metadata", metadataDir, func(ctx context.Context, dir string) (map[string]any, error) {
			db, err := openConfigured(filepath.Join(dir, "metadata.db"), defaultBusyTimeout)
			if err != nil {
				return nil, err
			}
			defer db.Close()
			if err := db.QueryRowContext(ctx, "SELECT sqlite_version(), sqlite_source_id()").Scan(&report.SQLite.Version, &report.SQLite.SourceID); err != nil {
				return nil, err
			}
			return map[string]any{"sqlite_version": report.SQLite.Version, "sqlite_source_id": report.SQLite.SourceID}, nil
		}))
	}

	probes := []struct {
		name string
		fn   probeFunc
	}{
		{"connection_pragmas", probeConnectionPragmas},
		{"fts5", probeFTS5},
		{"wal_writer_readers", probeWALWriterReaders},
		{"context_cancellation_health", probeCancellationHealth},
		{"sqlite_full_atomic_rollback", probeFullAtomicRollback},
		{"online_backup_restore", probeOnlineBackupRestore},
		{"numeric_error_codes", probeNumericErrorCodes},
	}
	for _, probe := range probes {
		dir := filepath.Join(root, probe.name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			report.Probes = append(report.Probes, ProbeResult{Name: probe.name, Error: err.Error()})
			continue
		}
		report.Probes = append(report.Probes, runProbe(ctx, probe.name, dir, probe.fn))
	}

	processDir := filepath.Join(root, "two_process_lock")
	if err := os.MkdirAll(processDir, 0o700); err != nil {
		report.Probes = append(report.Probes, ProbeResult{Name: "two_process_lock", Error: err.Error()})
	} else {
		report.Probes = append(report.Probes, runProbe(ctx, "two_process_lock", processDir, func(ctx context.Context, dir string) (map[string]any, error) {
			return probeTwoProcessLock(ctx, dir, options.Helper)
		}))
	}
	return finishReport(report, started, startRSS)
}

func runProbe(ctx context.Context, name, dir string, fn probeFunc) ProbeResult {
	started := time.Now()
	details, err := fn(ctx, dir)
	result := ProbeResult{
		Name:       name,
		Passed:     err == nil,
		DurationMS: time.Since(started).Milliseconds(),
		Details:    details,
	}
	if err != nil {
		result.Error = redactMachinePaths(err.Error())
	}
	return result
}

func redactMachinePaths(message string) string {
	temp := os.TempDir()
	for _, spelling := range []string{temp, filepath.ToSlash(temp)} {
		if spelling != "" {
			message = strings.ReplaceAll(message, spelling, "<os-temp>")
		}
	}
	return message
}

func finishReport(report Report, started time.Time, startRSS rssSample) Report {
	endRSS := readRSS()
	report.DurationMS = time.Since(started).Milliseconds()
	report.RSS = RSSInfo{
		Supported:      startRSS.supported && endRSS.supported,
		Measurement:    endRSS.measurement,
		StartBytes:     startRSS.current,
		EndBytes:       endRSS.current,
		PeakBytes:      endRSS.peak,
		MeasurementErr: endRSS.err,
	}
	report.Passed = len(report.Probes) > 0
	for _, probe := range report.Probes {
		if !probe.Passed {
			report.Passed = false
			break
		}
	}
	if report.Candidate.BuiltVersion != CandidateVersion {
		report.Passed = false
		report.Notes = append(report.Notes, fmt.Sprintf("built candidate version %q does not match pin %q", report.Candidate.BuiltVersion, CandidateVersion))
	}
	return report
}

func dependencyVersion(module string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dep := range info.Deps {
		if dep.Path == module {
			return dep.Version
		}
	}
	return "unknown"
}

func buildSetting(key string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, setting := range info.Settings {
		if setting.Key == key {
			return setting.Value
		}
	}
	return "unknown"
}

func WriteReport(path string, report Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
