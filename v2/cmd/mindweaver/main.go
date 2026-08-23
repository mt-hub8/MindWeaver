package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
	"github.com/mt-hub8/MindWeaver/v2/platform/config"
	"github.com/mt-hub8/MindWeaver/v2/platform/version"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, apperror.PublicMessage(err))
		os.Exit(exitCode(err))
	}
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return printUsage(stdout)
	}
	switch args[0] {
	case "help", "-h", "--help":
		return printUsage(stdout)
	case "version":
		_, err := fmt.Fprintln(stdout, version.String())
		return outputError(err)
	case "config":
		return runConfig(ctx, args[1:], stdout)
	default:
		return apperror.New(apperror.KindInvalid, "cli.command_unknown", "unknown command; run mindweaver help")
	}
}

func runConfig(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return apperror.New(apperror.KindInvalid, "cli.config_command_required", "config requires init or check")
	}
	switch args[0] {
	case "init":
		flags := flag.NewFlagSet("config init", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		file := flags.String("file", config.DefaultFileName, "configuration file")
		vault := flags.String("vault", "./vault", "vault root")
		if err := flags.Parse(args[1:]); err != nil {
			return apperror.Wrap(err, apperror.KindInvalid, "cli.flags_invalid", "cli.config_init", "invalid config init flags")
		}
		if flags.NArg() != 0 {
			return apperror.New(apperror.KindInvalid, "cli.arguments_unexpected", "config init does not accept positional arguments")
		}
		if err := config.WriteNew(ctx, *file, config.Default(*vault)); err != nil {
			return err
		}
		_, err := fmt.Fprintf(stdout, "created %s (schema v%d)\n", *file, config.CurrentSchemaVersion)
		return outputError(err)
	case "check":
		flags := flag.NewFlagSet("config check", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		file := flags.String("file", config.DefaultFileName, "configuration file")
		if err := flags.Parse(args[1:]); err != nil {
			return apperror.Wrap(err, apperror.KindInvalid, "cli.flags_invalid", "cli.config_check", "invalid config check flags")
		}
		if flags.NArg() != 0 {
			return apperror.New(apperror.KindInvalid, "cli.arguments_unexpected", "config check does not accept positional arguments")
		}
		cfg, err := config.Load(ctx, *file)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "valid schema v%d; vault=%s\n", cfg.SchemaVersion, cfg.Vault.Root)
		return outputError(err)
	default:
		return apperror.New(apperror.KindInvalid, "cli.config_command_unknown", "unknown config command; use init or check")
	}
}

func outputError(err error) error {
	return apperror.Wrap(err, apperror.KindUnavailable, "cli.output_failed", "cli.write", "command output could not be written")
}

func exitCode(err error) int {
	switch apperror.KindOf(err) {
	case apperror.KindInvalid:
		return 2
	case apperror.KindConflict:
		return 3
	case apperror.KindNotFound:
		return 4
	case apperror.KindUnavailable:
		return 5
	case apperror.KindDeadline:
		return 124
	case apperror.KindCanceled:
		return 130
	default:
		return 1
	}
}

func printUsage(writer io.Writer) error {
	_, err := fmt.Fprintln(writer, `MindWeaver local workbench (Go rewrite)

Usage:
  mindweaver version
  mindweaver config init  [-file mindweaver.v1.json] [-vault ./vault]
  mindweaver config check [-file mindweaver.v1.json]`)
	return outputError(err)
}
