package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	browserqualification "github.com/mt-hub8/MindWeaver/v2/tests/browser"
)

const (
	exitFailed  = 1
	exitInvalid = 2
	exitBlocked = 3
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("browser-qualification", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	revision := flags.String("source-revision", "", "full source revision")
	reportPath := flags.String("report", "", "new JSON report path")
	bundleRoot := flags.String("artifact-bundle", "", "approved offline artifact bundle")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || !lowerSHA1(*revision) || !safeReportPath(*reportPath) {
		_, _ = io.WriteString(stderr, "browser qualification: invalid arguments\n")
		return exitInvalid
	}

	approval, err := browserqualification.ParseEmbeddedApproval()
	if err != nil {
		_, _ = io.WriteString(stderr, "browser qualification: approval unavailable\n")
		return exitFailed
	}
	report := browserqualification.RunQualification(context.Background(), approval, browserqualification.RunOptions{
		SourceRevision: *revision,
		BundleRoot:     *bundleRoot,
	})
	data, err := browserqualification.MarshalReport(report)
	if err != nil || writeNewReport(*reportPath, data) != nil {
		_, _ = io.WriteString(stderr, "browser qualification: report unavailable\n")
		return exitFailed
	}
	_, _ = fmt.Fprintf(stdout, "UI-001/UI-002 %s %s\n", report.Status(), report.Code())
	switch report.Status() {
	case "PASS":
		return 0
	case "BLOCKED":
		return exitBlocked
	default:
		return exitFailed
	}
}

func writeNewReport(path string, data []byte) (result error) {
	parent := filepath.Dir(path)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil || !samePath(parent, resolvedParent) {
		return errors.New("unsafe report parent")
	}
	if _, err := os.Lstat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
		return errors.New("report target already exists")
	}
	temporary, err := os.CreateTemp(parent, ".browser-qualification-*.tmp")
	if err != nil {
		return errors.New("create report staging file")
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			result = errors.Join(result, temporary.Close())
		}
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			result = errors.Join(result, errors.New("remove report staging file"))
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return errors.New("protect report staging file")
	}
	if _, err := temporary.Write(data); err != nil {
		return errors.New("write report staging file")
	}
	if err := temporary.Sync(); err != nil {
		return errors.New("sync report staging file")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("close report staging file")
	}
	closed = true
	// Linking a fully written sibling is atomic and cannot overwrite an existing
	// report. The deferred remove then drops the staging name only.
	if err := os.Link(temporaryPath, path); err != nil {
		return errors.New("publish new report")
	}
	return nil
}

func safeReportPath(path string) bool {
	if path == "" || !filepath.IsAbs(path) || filepath.Ext(path) != ".json" {
		return false
	}
	leaf := filepath.Base(path)
	if leaf == "." || leaf == ".." || len(leaf) > 128 {
		return false
	}
	for _, character := range leaf {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func lowerSHA1(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, character := range value {
		if (character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') {
			continue
		}
		return false
	}
	return true
}

func samePath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
