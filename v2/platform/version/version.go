// Package version exposes build metadata without requiring generated files.
package version

import "fmt"

// These variables may be replaced with -ldflags at release time.
var (
	Version = "0.1.0-dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// String returns compact, human-readable build metadata.
func String() string {
	return fmt.Sprintf("mindweaver %s (commit=%s, built=%s)", Version, Commit, Date)
}
