package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/app"
	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
	"github.com/mt-hub8/MindWeaver/v2/platform/config"
	"github.com/mt-hub8/MindWeaver/v2/platform/version"
)

const serveShutdownLimit = 10 * time.Second

type serveShutdowner interface {
	Shutdown(context.Context) error
}

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
		return runServe(ctx, nil, stdout)
	}
	switch args[0] {
	case "help", "-h", "--help":
		return printUsage(stdout)
	case "version":
		_, err := fmt.Fprintln(stdout, version.String())
		return outputError(err)
	case "config":
		return runConfig(ctx, args[1:], stdout)
	case "recovery":
		return runRecovery(ctx, args[1:], stdout)
	case "serve":
		return runServe(ctx, args[1:], stdout)
	default:
		return apperror.New(apperror.KindInvalid, "cli.command_unknown", "unknown command; run mindweaver help")
	}
}

func runServe(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	file := flags.String("config", config.DefaultFileName, "configuration file")
	firstVault := flags.String("vault", "./vault", "Vault root used only on first run")
	noBrowser := flags.Bool("no-browser", false, "print the local URL without opening a browser")
	if err := flags.Parse(args); err != nil {
		return apperror.Wrap(err, apperror.KindInvalid, "cli.flags_invalid", "cli.serve", "invalid serve flags")
	}
	if flags.NArg() != 0 {
		return apperror.New(apperror.KindInvalid, "cli.arguments_unexpected", "serve does not accept positional arguments")
	}

	application, err := app.Start(ctx, app.Options{ConfigPath: *file, FirstRunVaultRoot: *firstVault})
	if err != nil {
		if errors.Is(err, vault.ErrLocked) {
			return apperror.Wrap(err, apperror.KindConflict, "runtime.vault_locked", "cli.serve", "this Vault is already open in another MindWeaver process")
		}
		return err
	}
	shutdown := func() error {
		return shutdownServeApplication(application, serveShutdownLimit)
	}
	launchURL := application.LaunchURL()
	if _, err := fmt.Fprintf(stdout, "MindWeaver 已就绪。请打开一次性本地链接：\n%s\n", launchURL); err != nil {
		return errors.Join(outputError(err), shutdown())
	}
	if !*noBrowser {
		if err := openBrowser(launchURL); err != nil {
			// The printed one-use URL remains a complete recovery path. Browser
			// integration failure must be visible but must not take the owned Vault
			// and already-ready workbench back down.
			if _, writeErr := fmt.Fprintln(stdout, "未能自动打开浏览器；请手动打开上面的本地链接。"); writeErr != nil {
				return errors.Join(outputError(writeErr), shutdown())
			}
		}
	}

	select {
	case <-ctx.Done():
		return shutdown()
	case serveErr, open := <-application.Done():
		shutdownErr := shutdown()
		if !open || serveErr == nil {
			return shutdownErr
		}
		return errors.Join(fmt.Errorf("local HTTP server stopped: %w", serveErr), shutdownErr)
	}
}

func shutdownServeApplication(application serveShutdowner, limit time.Duration) error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	// App owns a shorter fixed drain deadline. Call exactly once: an incomplete
	// drain must return non-zero and let process exit reclaim still-open lower
	// resources rather than waiting without a bound.
	err := application.Shutdown(shutdownContext)
	if errors.Is(err, app.ErrShutdownIncomplete) {
		return apperror.Wrap(
			err, apperror.KindDeadline, "runtime.shutdown_incomplete", "cli.shutdown",
			"shutdown did not complete before its safety deadline",
		)
	}
	return err
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
	if errors.Is(err, vault.ErrLocked) {
		return 3
	}
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
  mindweaver
  mindweaver serve [-config mindweaver.v1.json] [-vault ./vault] [-no-browser]
  mindweaver version
  mindweaver config init  [-file mindweaver.v1.json] [-vault ./vault]
  mindweaver config check [-file mindweaver.v1.json]
  mindweaver recovery verify  -backup <backup-directory>
  mindweaver recovery restore -backup <backup-directory> -vault <new-vault-directory>

Recovery is a mutually exclusive startup mode. Backups are plaintext, and
restore never overwrites or merges an existing Vault.`)
	return outputError(err)
}
