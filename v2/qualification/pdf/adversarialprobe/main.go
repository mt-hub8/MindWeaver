// adversarialprobe is a qualification-only helper used to verify timeout
// cleanup. It is never included in a MindWeaver artifact.
package main

import (
	"flag"
	"os"
	"strconv"
	"time"
)

const (
	readyEnvironment         = "MWQ_PDF_PROBE_READY"
	sentinelEnvironment      = "MWQ_PDF_PROBE_SENTINEL"
	sentinelDelayEnvironment = "MWQ_PDF_PROBE_SENTINEL_DELAY_MS"
)

func main() {
	input := flag.String("input", "", "qualification source locator")
	flag.Parse()
	if *input == "" {
		os.Exit(64)
	}
	if ready := os.Getenv(readyEnvironment); ready != "" {
		_ = os.WriteFile(ready, []byte("ready"), 0o600)
	}
	marker := os.Getenv(sentinelEnvironment)
	delay, err := strconv.Atoi(os.Getenv(sentinelDelayEnvironment))
	if marker != "" && err == nil && delay > 0 {
		go func() {
			time.Sleep(time.Duration(delay) * time.Millisecond)
			_ = os.WriteFile(marker, []byte("alive"), 0o600)
		}()
	}
	for {
		time.Sleep(time.Hour)
	}
}
