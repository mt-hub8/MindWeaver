package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/ideashook"
	"github.com/mt-hub8/MindWeaver/v2/internal/ideasollama"
	"github.com/mt-hub8/MindWeaver/v2/internal/ollama"
	"github.com/mt-hub8/MindWeaver/v2/internal/sessiondistill"
	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
)

var ideasExecutablePath = os.Executable
var ideasWorkingDirectory = os.Getwd
var ideasCurrentSessionRequest = ideashook.RequestForCurrentSession
var ideasHookCodeOf = ideashook.CodeOf
var ideasRankCandidates = ideasollama.Rank

type ideasModelFlags struct {
	model    *string
	endpoint *string
	timeout  *time.Duration
}

type ideasModelOptions struct {
	enabled bool
	ollama  ollama.Options
}

func runIdeas(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return apperror.New(apperror.KindInvalid, "ideas.command_required", "ideas requires current, extract, sessions, or hooks")
	}
	switch args[0] {
	case "current":
		return runIdeasCurrent(ctx, args[1:], stdout)
	case "extract":
		return runIdeasExtract(ctx, args[1:], stdin, stdout)
	case "sessions":
		return runIdeasSessions(ctx, args[1:], stdout)
	case "hooks":
		return runIdeasHooks(ctx, args[1:], stdout)
	default:
		return apperror.New(apperror.KindInvalid, "ideas.command_unknown", "unknown ideas command")
	}
}

func runIdeasExtract(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("ideas extract", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	input := flags.String("input", "-", "session input file or - for stdin")
	inputFormat := flags.String("input-format", "session-json", "session-json, chat-jsonl, or note")
	sessionID := flags.String("session", "", "captured session ID from ideas sessions")
	output := flags.String("output", "", "new report directory")
	modelFlags := addIdeasModelFlags(flags)
	if err := flags.Parse(args); err != nil {
		return apperror.Wrap(err, apperror.KindInvalid, "ideas.flags_invalid", "ideas.extract", "invalid ideas extract flags")
	}
	if flags.NArg() != 0 || *output == "" {
		return apperror.New(apperror.KindInvalid, "ideas.arguments_invalid", "ideas extract requires -output and accepts no positional arguments")
	}
	modelOptions, modelErr := parseIdeasModelOptions(flags, modelFlags)
	if modelErr != nil {
		return modelErr
	}
	var request sessiondistill.Request
	var err error
	visited := make(map[string]bool)
	flags.Visit(func(current *flag.Flag) { visited[current.Name] = true })
	if visited["session"] {
		if *sessionID == "" {
			return apperror.New(apperror.KindInvalid, "ideas.arguments_invalid", "ideas extract requires a non-empty captured session ID")
		}
		if visited["input"] || visited["input-format"] {
			return apperror.New(apperror.KindInvalid, "ideas.arguments_invalid", "ideas extract accepts either -session or file input")
		}
		request, err = ideashook.RequestForSession(ctx, *sessionID)
		if err != nil {
			return mapIdeasHookError(err)
		}
	} else {
		reader, closeInput, openErr := openIdeasInput(*input, stdin)
		if openErr != nil {
			return openErr
		}
		if closeInput != nil {
			defer closeInput()
		}
		switch *inputFormat {
		case "session-json":
			request, err = sessiondistill.DecodeRequest(reader)
		case "chat-jsonl":
			request, err = sessiondistill.DecodeChatJSONL(reader)
		case "note":
			request, err = sessiondistill.RequestFromNote(reader)
		default:
			return apperror.New(apperror.KindInvalid, "ideas.input_format_invalid", "ideas input format is invalid")
		}
	}
	if err != nil {
		return mapIdeasError(err)
	}
	return publishIdeasReport(ctx, request, *output, stdout, modelOptions)
}

func runIdeasCurrent(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return apperror.New(apperror.KindInvalid, "ideas.current_command_invalid", "ideas current requires extract")
	}
	switch args[0] {
	case "extract":
	default:
		return apperror.New(apperror.KindInvalid, "ideas.current_command_invalid", "ideas current requires extract")
	}
	flags := flag.NewFlagSet("ideas current extract", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	output := flags.String("output", "", "new report directory")
	modelFlags := addIdeasModelFlags(flags)
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *output == "" {
		return apperror.New(apperror.KindInvalid, "ideas.current_arguments_invalid", "ideas current extract requires -output and accepts no positional arguments")
	}
	modelOptions, modelErr := parseIdeasModelOptions(flags, modelFlags)
	if modelErr != nil {
		return modelErr
	}
	workingDirectory, err := ideasWorkingDirectory()
	if err != nil {
		return apperror.Wrap(err, apperror.KindUnavailable, "ideas.current_environment_unavailable", "ideas.current", "current Codex working directory is unavailable")
	}
	request, err := ideasCurrentSessionRequest(ctx, workingDirectory)
	if err != nil {
		return mapIdeasHookError(err)
	}
	return publishIdeasReport(ctx, request, *output, stdout, modelOptions)
}

func addIdeasModelFlags(flags *flag.FlagSet) ideasModelFlags {
	return ideasModelFlags{
		model:    flags.String("ollama-model", "", "optional local Ollama model for ranking existing user items"),
		endpoint: flags.String("ollama-endpoint", ideasollama.DefaultBaseURL, "literal loopback Ollama endpoint"),
		timeout:  flags.Duration("ollama-timeout", ideasollama.DefaultTimeout, "Ollama ranking timeout"),
	}
}

func parseIdeasModelOptions(flags *flag.FlagSet, values ideasModelFlags) (ideasModelOptions, error) {
	visited := make(map[string]bool)
	flags.Visit(func(current *flag.Flag) { visited[current.Name] = true })
	if !visited["ollama-model"] {
		if visited["ollama-endpoint"] || visited["ollama-timeout"] {
			return ideasModelOptions{}, apperror.New(apperror.KindInvalid, "ideas.ollama_model_required", "Ollama endpoint and timeout require -ollama-model")
		}
		return ideasModelOptions{}, nil
	}
	if values.model == nil || values.endpoint == nil || values.timeout == nil || *values.model == "" || *values.endpoint == "" ||
		*values.timeout < time.Millisecond || *values.timeout > ideasollama.MaxTimeout {
		return ideasModelOptions{}, apperror.New(apperror.KindInvalid, "ideas.ollama_configuration_invalid", "Ollama ranking configuration is invalid")
	}
	return ideasModelOptions{enabled: true, ollama: ollama.Options{BaseURL: *values.endpoint, Model: *values.model, Timeout: *values.timeout}}, nil
}

func publishIdeasReport(ctx context.Context, request sessiondistill.Request, output string, stdout io.Writer, modelOptions ideasModelOptions) error {
	bundle, err := sessiondistill.NewV1().Distill(ctx, request)
	if err != nil {
		return mapIdeasError(err)
	}
	if modelOptions.enabled {
		result := bundle.Result()
		candidates := make([]ideasollama.Candidate, len(result.UserItems))
		for index, item := range result.UserItems {
			candidates[index] = ideasollama.Candidate{ItemID: item.ID, Kind: item.Kind, Statement: item.Statement}
		}
		ranking, rankErr := ideasRankCandidates(ctx, modelOptions.ollama, candidates)
		if rankErr != nil {
			return mapIdeasModelError(rankErr)
		}
		bundle, err = sessiondistill.AttachModelAssistance(bundle, sessiondistill.ModelAssistanceInput{
			ProviderConfigDigest: ranking.ProviderConfigDigest,
			LimitationCode:       ranking.LimitationCode,
			RankedItemIDs:        ranking.RankedItemIDs,
		})
		if err != nil {
			return mapIdeasError(err)
		}
	}
	if err := sessiondistill.PublishBundle(ctx, output, bundle); err != nil {
		return mapIdeasError(err)
	}
	result := bundle.Result()
	_, err = fmt.Fprintf(stdout, "created ideas report (%d user items, %d assistant context items)\n", len(result.UserItems), len(result.AssistantContext))
	return outputError(err)
}

func mapIdeasModelError(err error) error {
	if errors.Is(err, ideasollama.ErrInvalidArgument) {
		return apperror.Wrap(err, apperror.KindInvalid, "ideas.ollama_configuration_invalid", "ideas.extract", "Ollama ranking configuration is invalid")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return apperror.Wrap(err, apperror.KindCanceled, "ideas.canceled", "ideas.extract", "ideas extraction was canceled")
	}
	return apperror.Wrap(err, apperror.KindInternal, "ideas.ollama_internal", "ideas.extract", "local-model ranking failed")
}

func runIdeasSessions(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) != 0 {
		return apperror.New(apperror.KindInvalid, "ideas.sessions_arguments_invalid", "ideas sessions accepts no arguments")
	}
	sessions, err := ideashook.ListSessions(ctx)
	if err != nil {
		return mapIdeasHookError(err)
	}
	result := struct {
		SchemaVersion int                        `json:"schema_version"`
		Sessions      []ideashook.SessionSummary `json:"sessions"`
	}{SchemaVersion: 1, Sessions: sessions}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	return outputError(encoder.Encode(result))
}

func runIdeasHooks(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return apperror.New(apperror.KindInvalid, "ideas.hooks_command_required", "ideas hooks requires print, install, or status")
	}
	command := args[0]
	if command != "print" && command != "install" && command != "status" {
		return apperror.New(apperror.KindInvalid, "ideas.hooks_command_unknown", "unknown ideas hooks command")
	}
	flags := flag.NewFlagSet("ideas hooks "+command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	scopeValue := flags.String("scope", "", "Codex hook configuration scope: user or repo")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return apperror.New(apperror.KindInvalid, "ideas.hooks_arguments_invalid", "ideas hooks arguments are invalid")
	}
	var scope ideashook.HookScope
	switch *scopeValue {
	case string(ideashook.HookScopeUser):
		scope = ideashook.HookScopeUser
	case string(ideashook.HookScopeRepo):
		scope = ideashook.HookScopeRepo
	default:
		return apperror.New(apperror.KindInvalid, "ideas.hooks_scope_required", "ideas hooks requires --scope user or --scope repo")
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return apperror.Wrap(err, apperror.KindUnavailable, "ideas.hooks_environment_unavailable", "ideas.hooks", "Codex hook environment is unavailable")
	}
	executablePath, err := ideasExecutablePath()
	if err != nil {
		return apperror.Wrap(err, apperror.KindUnavailable, "ideas.hooks_executable_unavailable", "ideas.hooks", "MindWeaver executable identity is unavailable")
	}
	options := ideashook.HookConfigOptions{
		Scope: scope, WorkingDirectory: workingDirectory, ExecutablePath: executablePath,
	}
	switch command {
	case "print":
		configuration, _, err := ideashook.RenderHookConfig(ctx, options)
		if err != nil {
			return mapIdeasHookError(err)
		}
		_, err = stdout.Write(configuration)
		return outputError(err)
	case "install":
		result, err := ideashook.InstallHookConfig(ctx, options)
		if err != nil {
			return mapIdeasHookError(err)
		}
		return writeIdeasHookConfigResult(stdout, command, result)
	case "status":
		result, err := ideashook.InspectHookConfig(ctx, options)
		if err != nil {
			return mapIdeasHookError(err)
		}
		return writeIdeasHookConfigResult(stdout, command, result)
	}
	return apperror.New(apperror.KindInternal, "ideas.hooks_internal", "ideas hooks failed")
}

func writeIdeasHookConfigResult(writer io.Writer, command string, result ideashook.HookConfigResult) error {
	output := struct {
		SchemaVersion int                       `json:"schema_version"`
		Command       string                    `json:"command"`
		Scope         ideashook.HookScope       `json:"scope"`
		State         ideashook.HookConfigState `json:"state"`
		Activation    string                    `json:"activation"`
		ManagedEvents int                       `json:"managed_events"`
		Changed       bool                      `json:"changed"`
		NextStep      string                    `json:"next_step"`
		Warning       string                    `json:"warning,omitempty"`
	}{
		SchemaVersion: 1,
		Command:       command,
		Scope:         result.Scope,
		State:         result.State,
		Activation:    result.Activation,
		ManagedEvents: result.ManagedEvents,
		Changed:       result.Changed,
		NextStep:      "review_and_trust_with_slash_hooks_then_start_a_new_codex_task_to_verify_activation",
	}
	if result.Scope == ideashook.HookScopeRepo {
		output.Warning = "repository_hook_configuration_contains_a_machine_local_executable_path_do_not_commit"
	}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return outputError(encoder.Encode(output))
}

func openIdeasInput(path string, stdin io.Reader) (io.Reader, func(), error) {
	if path == "-" {
		if stdin == nil {
			return nil, nil, apperror.New(apperror.KindInvalid, "ideas.input_invalid", "ideas input is invalid")
		}
		return stdin, nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, apperror.Wrap(err, apperror.KindNotFound, "ideas.input_unavailable", "ideas.extract", "ideas input is unavailable")
	}
	return file, func() { _ = file.Close() }, nil
}

func mapIdeasError(err error) error {
	if err == nil {
		return nil
	}
	switch sessiondistill.CodeOf(err) {
	case sessiondistill.CodeInvalidArgument, sessiondistill.CodeUnsupportedSchema, sessiondistill.CodeInputLimitExceeded:
		return apperror.Wrap(err, apperror.KindInvalid, sessiondistill.CodeOf(err), "ideas.extract", "ideas input is invalid")
	case sessiondistill.CodeDestinationExists:
		return apperror.Wrap(err, apperror.KindConflict, sessiondistill.CodeOf(err), "ideas.extract", "ideas report destination already exists")
	case sessiondistill.CodeCanceled:
		return apperror.Wrap(err, apperror.KindCanceled, sessiondistill.CodeOf(err), "ideas.extract", "ideas extraction was canceled")
	case sessiondistill.CodePublicationUncertain:
		return apperror.Wrap(err, apperror.KindUnavailable, sessiondistill.CodeOf(err), "ideas.extract", "ideas report publication outcome is uncertain; inspect the destination before retrying")
	case sessiondistill.CodeUnsupportedPlatform:
		return apperror.Wrap(err, apperror.KindUnavailable, sessiondistill.CodeOf(err), "ideas.extract", "ideas report publication is unsupported on this platform")
	case sessiondistill.CodeOutputLimitExceeded, sessiondistill.CodeOutputFailed:
		return apperror.Wrap(err, apperror.KindUnavailable, sessiondistill.CodeOf(err), "ideas.extract", "ideas report could not be created")
	default:
		if errors.Is(err, context.Canceled) {
			return apperror.Wrap(err, apperror.KindCanceled, "ideas.canceled", "ideas.extract", "ideas extraction was canceled")
		}
		return apperror.Wrap(err, apperror.KindInternal, "ideas.internal", "ideas.extract", "ideas extraction failed")
	}
}

func mapIdeasHookError(err error) error {
	if err == nil {
		return nil
	}
	code := ideasHookCodeOf(err)
	switch code {
	case ideashook.CodeInvalidInput, ideashook.CodeLimitExceeded:
		return apperror.Wrap(err, apperror.KindInvalid, code, "ideas.hook", "captured ideas input is invalid")
	case ideashook.CodeNotFound, ideashook.CodeNoVisibleEvents:
		return apperror.Wrap(err, apperror.KindNotFound, code, "ideas.hook", "no captured Codex session is available")
	case ideashook.CodeIdentityConflict:
		return apperror.Wrap(err, apperror.KindConflict, code, "ideas.hook", "Codex ideas state has an identity conflict")
	case ideashook.CodeCanceled:
		return apperror.Wrap(err, apperror.KindCanceled, code, "ideas.hook", "captured ideas operation was canceled")
	case ideashook.CodePublicationUncertain:
		return apperror.Wrap(err, apperror.KindUnavailable, code, "ideas.hook", "Codex hook configuration publication is uncertain; inspect hook status before retrying")
	default:
		return apperror.Wrap(err, apperror.KindUnavailable, code, "ideas.hook", "captured Codex session is unavailable")
	}
}
