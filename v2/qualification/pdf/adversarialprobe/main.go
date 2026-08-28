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
	ready := os.Getenv(readyEnvironment)
	marker := os.Getenv(sentinelEnvironment)
	delay, err := strconv.Atoi(os.Getenv(sentinelDelayEnvironment))
	if ready == "" || marker == "" || err != nil || delay <= 0 {
		os.Exit(64)
	}
	armed := make(chan struct{})
	go func() {
		timer := time.NewTimer(time.Duration(delay) * time.Millisecond)
		close(armed)
		<-timer.C
		_ = os.WriteFile(marker, []byte("alive"), 0o600)
	}()
	<-armed
	// Publish readiness only after the survival sentinel is armed. The parent
	// can then wait past ready+delay before proving that Job cleanup killed the
	// helper rather than merely observing a scheduler-delayed goroutine.
	if err := os.WriteFile(ready, []byte("ready"), 0o600); err != nil {
		os.Exit(74)
	}
	for {
		time.Sleep(time.Hour)
	}
}
