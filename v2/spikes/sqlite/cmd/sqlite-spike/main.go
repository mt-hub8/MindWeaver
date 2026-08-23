package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	probe "mindweaver.dev/spikes/sqlite"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: sqlite-spike run [--json PATH] | lock-holder ... | lock-contender ...")
	}
	if args[0] == "lock-holder" || args[0] == "lock-contender" {
		return probe.RunHelperCommand(context.Background(), args, os.Stdout)
	}
	if args[0] != "run" {
		return fmt.Errorf("unknown command %q", args[0])
	}

	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	jsonPath := flags.String("json", "", "also write the evidence JSON to this path")
	timeout := flags.Duration("timeout", 2*time.Minute, "overall spike timeout")
	keepTemp := flags.Bool("keep-temp", false, "retain the isolated temporary workspace for debugging")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	report := probe.RunAll(ctx, probe.Options{
		KeepTemp: *keepTemp,
		Helper: probe.HelperProcess{
			Executable: executable,
		},
	})
	if *jsonPath != "" {
		abs, err := filepath.Abs(*jsonPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o700); err != nil {
			return err
		}
		if err := probe.WriteReport(abs, report); err != nil {
			return err
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return err
	}
	if !report.Passed {
		return errors.New("one or more SQLite spike probes failed")
	}
	return nil
}
