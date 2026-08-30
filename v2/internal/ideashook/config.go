package ideashook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

const (
	HookConfigMarker    = "MindWeaver Ideas capture v1"
	HookActivationState = "unknown"

	CodePublicationUncertain = "ideas.hook_publication_uncertain"

	maxHookConfigBytes  = 1 << 20
	maxHookConfigDepth  = 16
	maxHookConfigTokens = 32768
)

var (
	errHookConfigMutationUnsupported  = errors.New("hook configuration mutation unsupported")
	errHookConfigSnapshotChanged      = errors.New("hook configuration snapshot changed")
	errHookConfigPublicationUncertain = errors.New("hook configuration publication uncertain")
)

type HookScope string

const (
	HookScopeUser HookScope = "user"
	HookScopeRepo HookScope = "repo"
)

type HookConfigState string

const (
	HookConfigAbsent    HookConfigState = "absent"
	HookConfigPartial   HookConfigState = "partial"
	HookConfigDrifted   HookConfigState = "drifted"
	HookConfigConflict  HookConfigState = "conflict"
	HookConfigInvalid   HookConfigState = "invalid"
	HookConfigInstalled HookConfigState = "installed"
)

type HookConfigOptions struct {
	Scope            HookScope
	WorkingDirectory string
	ExecutablePath   string
}

type HookConfigResult struct {
	Scope         HookScope       `json:"scope"`
	State         HookConfigState `json:"state"`
	Activation    string          `json:"activation"`
	ManagedEvents int             `json:"managed_events"`
	Changed       bool            `json:"changed"`
}

var managedHookEvents = []string{
	"SessionStart",
	"UserPromptSubmit",
	"Stop",
	"SessionEnd",
}

var officialHookEvents = map[string]struct{}{
	"PreToolUse": {}, "PermissionRequest": {}, "PostToolUse": {},
	"PreCompact": {}, "PostCompact": {}, "SessionStart": {},
	"SessionEnd": {}, "UserPromptSubmit": {}, "SubagentStart": {},
	"SubagentStop": {}, "Stop": {}, "Interrupt": {},
}

type hookConfigDocument struct {
	description *string
	hooks       map[string][]json.RawMessage
}

type managedHookHandler struct {
	Type           string `json:"type"`
	Command        string `json:"command"`
	CommandWindows string `json:"commandWindows"`
	Timeout        uint64 `json:"timeout"`
	Async          bool   `json:"async"`
	StatusMessage  string `json:"statusMessage"`
}

type managedHookGroup struct {
	Hooks []managedHookHandler `json:"hooks"`
}

// InspectHookConfig reports the current Codex hooks configuration without
// creating directories or changing the configuration file.
func InspectHookConfig(ctx context.Context, options HookConfigOptions) (HookConfigResult, error) {
	path, command, err := resolveHookConfigTarget(ctx, options)
	if err != nil {
		return HookConfigResult{}, err
	}
	document, _, exists, parseErr, readErr := readHookConfig(path)
	if readErr != nil {
		return HookConfigResult{}, hookError(CodeStorage, readErr)
	}
	result := baseHookConfigResult(options.Scope)
	if !exists {
		result.State = HookConfigAbsent
		return result, nil
	}
	if parseErr != nil {
		result.State = HookConfigInvalid
		return result, nil
	}
	result.State, result.ManagedEvents = inspectHookDocument(document, command)
	return result, nil
}

// RenderHookConfig returns the complete merged hooks.json bytes without
// changing the filesystem. Invalid or conflicting existing configurations are
// never rewritten automatically.
func RenderHookConfig(ctx context.Context, options HookConfigOptions) ([]byte, HookConfigResult, error) {
	if err := ensureHookConfigMutationSupported(); err != nil {
		return nil, HookConfigResult{}, hookError(CodeUnsupported, err)
	}
	path, command, err := resolveHookConfigTarget(ctx, options)
	if err != nil {
		return nil, HookConfigResult{}, err
	}
	document, original, exists, parseErr, readErr := readHookConfig(path)
	if readErr != nil {
		return nil, HookConfigResult{}, hookError(CodeStorage, readErr)
	}
	if !exists {
		document = hookConfigDocument{hooks: make(map[string][]json.RawMessage)}
	}
	return renderHookConfig(options.Scope, document, original, exists, parseErr, command)
}

// InstallHookConfig atomically creates a missing hooks.json. An existing file
// is never rewritten automatically: an exact semantic reinstall is a no-op,
// while any required merge or upgrade returns CodeIdentityConflict so the
// caller can review RenderHookConfig and apply it manually. Activation remains
// unknown until Codex reloads and trusts the hook.
func InstallHookConfig(ctx context.Context, options HookConfigOptions) (HookConfigResult, error) {
	if err := ensureHookConfigMutationSupported(); err != nil {
		return HookConfigResult{}, hookError(CodeUnsupported, err)
	}
	path, command, err := resolveHookConfigTarget(ctx, options)
	if err != nil {
		return HookConfigResult{}, err
	}
	lock, err := acquireHookConfigMutationLock(ctx, path)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return HookConfigResult{}, hookError(CodeCanceled, err)
		}
		return HookConfigResult{}, hookError(CodeStorage, err)
	}
	locked := true
	defer func() {
		if locked {
			_ = lock.Close()
		}
	}()
	document, original, exists, parseErr, readErr := readHookConfig(path)
	if readErr != nil {
		return HookConfigResult{}, hookError(CodeStorage, readErr)
	}
	if !exists {
		document = hookConfigDocument{hooks: make(map[string][]json.RawMessage)}
	}
	preview, result, err := renderHookConfig(options.Scope, document, original, exists, parseErr, command)
	if err != nil {
		return HookConfigResult{}, err
	}
	if !result.Changed {
		locked = false
		if err := lock.Close(); err != nil {
			return HookConfigResult{}, hookError(CodeStorage, err)
		}
		return result, nil
	}
	if exists {
		return HookConfigResult{}, hookError(CodeIdentityConflict, fmt.Errorf("existing hook configuration requires manual merge"))
	}
	if err := contextError(ctx); err != nil {
		return HookConfigResult{}, hookError(CodeCanceled, err)
	}
	if err := writeHookConfigAtomic(ctx, path, preview); err != nil {
		return HookConfigResult{}, classifyHookConfigWriteError(err)
	}
	locked = false
	if err := lock.Close(); err != nil {
		return HookConfigResult{}, hookError(CodeStorage, err)
	}
	return result, nil
}

func classifyHookConfigWriteError(err error) error {
	switch {
	case errors.Is(err, errHookConfigSnapshotChanged):
		return hookError(CodeIdentityConflict, err)
	case errors.Is(err, errHookConfigPublicationUncertain):
		return hookError(CodePublicationUncertain, err)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return hookError(CodeCanceled, err)
	default:
		return hookError(CodeStorage, err)
	}
}

func baseHookConfigResult(scope HookScope) HookConfigResult {
	return HookConfigResult{Scope: scope, Activation: HookActivationState}
}

func renderHookConfig(scope HookScope, document hookConfigDocument, original []byte, exists bool, parseErr error, command string) ([]byte, HookConfigResult, error) {
	result := baseHookConfigResult(scope)
	if parseErr != nil {
		result.State = HookConfigInvalid
		return nil, result, hookError(CodeInvalidInput, parseErr)
	}
	state, _ := inspectHookDocument(document, command)
	if state == HookConfigConflict {
		result.State = state
		return nil, result, hookError(CodeIdentityConflict, fmt.Errorf("hook configuration conflict"))
	}
	if state == HookConfigInstalled && exists {
		result.State = HookConfigInstalled
		result.ManagedEvents = len(managedHookEvents)
		result.Changed = false
		return bytes.Clone(original), result, nil
	}
	managed, err := json.Marshal(managedHookGroup{Hooks: []managedHookHandler{{
		Type: "command", Command: command, CommandWindows: command,
		Timeout: 3, Async: false, StatusMessage: HookConfigMarker,
	}}})
	if err != nil {
		return nil, HookConfigResult{}, hookError(CodeStorage, err)
	}
	for _, event := range managedHookEvents {
		groups := document.hooks[event]
		if index, found := ownedManagedGroupIndex(groups); found {
			groups = append([]json.RawMessage(nil), groups...)
			groups[index] = json.RawMessage(bytes.Clone(managed))
			document.hooks[event] = groups
		} else {
			document.hooks[event] = append(groups, json.RawMessage(bytes.Clone(managed)))
		}
	}
	encoded, err := encodeHookDocument(document)
	if err != nil {
		return nil, HookConfigResult{}, hookError(CodeStorage, err)
	}
	result.State = HookConfigInstalled
	result.ManagedEvents = len(managedHookEvents)
	result.Changed = !exists || !bytes.Equal(original, encoded)
	return encoded, result, nil
}

func resolveHookConfigTarget(ctx context.Context, options HookConfigOptions) (string, string, error) {
	if err := contextError(ctx); err != nil {
		return "", "", hookError(CodeCanceled, err)
	}
	command, err := managedHookCommand(options.ExecutablePath)
	if err != nil {
		return "", "", hookError(CodeInvalidInput, err)
	}
	var root string
	switch options.Scope {
	case HookScopeUser:
		root, err = resolveCodexHome()
	case HookScopeRepo:
		root, err = resolveMainCheckoutRoot(ctx, options.WorkingDirectory)
		if err == nil {
			root = filepath.Join(root, ".codex")
		}
	default:
		err = fmt.Errorf("hook scope")
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", "", hookError(CodeCanceled, err)
		}
		return "", "", hookError(CodeInvalidInput, err)
	}
	return filepath.Join(root, "hooks.json"), command, nil
}

func resolveCodexHome() (string, error) {
	root := os.Getenv("CODEX_HOME")
	explicit := root != ""
	if !explicit {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", fmt.Errorf("Codex home")
		}
		root = filepath.Join(home, ".codex")
	}
	root, err := canonicalAbsolutePath(root)
	if err != nil {
		return "", err
	}
	if explicit {
		info, err := os.Lstat(root)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("Codex home")
		}
		resolved, err := filepath.EvalSymlinks(root)
		if err != nil || !samePath(resolved, root) {
			return "", fmt.Errorf("Codex home")
		}
	}
	return root, nil
}

func resolveMainCheckoutRoot(ctx context.Context, workingDirectory string) (string, error) {
	workingDirectory, err := canonicalAbsolutePath(workingDirectory)
	if err != nil {
		return "", err
	}
	common, err := gitOutput(ctx, workingDirectory, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	common, err = canonicalAbsolutePath(common)
	if err != nil || !strings.EqualFold(filepath.Base(common), ".git") {
		return "", fmt.Errorf("Git common directory")
	}
	commonInfo, err := os.Lstat(common)
	if err != nil || !commonInfo.IsDir() || commonInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("Git common directory")
	}
	mainRoot := filepath.Dir(common)
	top, err := gitOutput(ctx, mainRoot, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil {
		return "", err
	}
	top, err = canonicalAbsolutePath(top)
	if err != nil || !samePath(top, mainRoot) {
		return "", fmt.Errorf("Git main checkout")
	}
	verifiedCommon, err := gitOutput(ctx, mainRoot, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	verifiedCommon, err = canonicalAbsolutePath(verifiedCommon)
	if err != nil || !samePath(common, verifiedCommon) {
		return "", fmt.Errorf("Git common directory")
	}
	bare, err := gitOutput(ctx, mainRoot, "rev-parse", "--is-bare-repository")
	if err != nil || bare != "false" {
		return "", fmt.Errorf("Git checkout")
	}
	return mainRoot, nil
}

func gitOutput(ctx context.Context, directory string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", directory}, arguments...)...)
	command.Env = sanitizedGitEnvironment()
	command.Stderr = io.Discard
	var output bytes.Buffer
	command.Stdout = &output
	if err := command.Run(); err != nil {
		return "", err
	}
	if output.Len() == 0 || output.Len() > 32<<10 {
		return "", fmt.Errorf("Git output")
	}
	value := strings.TrimSpace(output.String())
	if value == "" || strings.ContainsAny(value, "\r\n\x00") {
		return "", fmt.Errorf("Git output")
	}
	return value, nil
}

func sanitizedGitEnvironment() []string {
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		key := entry
		if index := strings.IndexByte(entry, '='); index >= 0 {
			key = entry[:index]
		}
		if strings.HasPrefix(strings.ToUpper(key), "GIT_") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "GIT_TERMINAL_PROMPT=0")
}

func managedHookCommand(executablePath string) (string, error) {
	path, err := canonicalAbsolutePath(executablePath)
	if err != nil || path != executablePath {
		return "", fmt.Errorf("hook executable")
	}
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) ||
		strings.ContainsAny(path, "\x00\r\n\"&|<>^%!()") {
		return "", fmt.Errorf("hook executable")
	}
	if runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(path), ".exe") {
		return "", fmt.Errorf("hook executable")
	}
	wantName := "mindweaver"
	if runtime.GOOS == "windows" {
		wantName += ".exe"
	}
	if !samePath(filepath.Base(path), wantName) {
		return "", fmt.Errorf("hook executable")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("hook executable")
	}
	return `"` + path + `" ideas hook codex`, nil
}

func parseManagedHookCommand(command string) (string, bool) {
	const suffix = `" ideas hook codex`
	if !strings.HasPrefix(command, `"`) || !strings.HasSuffix(command, suffix) || len(command) <= len(suffix)+1 {
		return "", false
	}
	path := command[1 : len(command)-len(suffix)]
	canonical, err := canonicalAbsolutePath(path)
	if err != nil || canonical != path || strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, `\\?\`) ||
		strings.HasPrefix(path, `\\.\`) || strings.ContainsAny(path, "\x00\r\n\"&|<>^%!()") {
		return "", false
	}
	wantName := "mindweaver"
	if runtime.GOOS == "windows" {
		wantName += ".exe"
	}
	if !samePath(filepath.Base(path), wantName) {
		return "", false
	}
	return path, true
}

func canonicalAbsolutePath(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) || strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("absolute path")
	}
	clean := filepath.Clean(path)
	absolute, err := filepath.Abs(clean)
	if err != nil || !samePath(clean, absolute) {
		return "", fmt.Errorf("canonical path")
	}
	return clean, nil
}

func samePath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func readHookConfig(path string) (hookConfigDocument, []byte, bool, error, error) {
	before, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return hookConfigDocument{}, nil, false, nil, nil
	}
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return hookConfigDocument{}, nil, false, nil, errors.Join(err, fmt.Errorf("hook configuration object"))
	}
	if before.Size() <= 0 || before.Size() > maxHookConfigBytes {
		return hookConfigDocument{}, nil, true, fmt.Errorf("hook configuration size"), nil
	}
	file, err := os.Open(path)
	if err != nil {
		return hookConfigDocument{}, nil, false, nil, err
	}
	after, statErr := file.Stat()
	if statErr != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		_ = file.Close()
		return hookConfigDocument{}, nil, false, nil, errors.Join(statErr, fmt.Errorf("hook configuration object"))
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxHookConfigBytes+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return hookConfigDocument{}, nil, false, nil, err
	}
	if len(data) == 0 || len(data) > maxHookConfigBytes || !utf8.Valid(data) || bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		return hookConfigDocument{}, data, true, fmt.Errorf("hook configuration encoding"), nil
	}
	document, parseErr := decodeHookDocument(data)
	return document, data, true, parseErr, nil
}

func decodeHookDocument(data []byte) (hookConfigDocument, error) {
	if err := validateJSONUnicodeEscapes(data); err != nil {
		return hookConfigDocument{}, err
	}
	if err := scanHookConfigJSON(data); err != nil {
		return hookConfigDocument{}, err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return hookConfigDocument{}, err
	}
	if top == nil {
		return hookConfigDocument{}, fmt.Errorf("hook configuration object")
	}
	for key := range top {
		if key != "description" && key != "hooks" {
			return hookConfigDocument{}, fmt.Errorf("hook configuration field")
		}
	}
	document := hookConfigDocument{hooks: make(map[string][]json.RawMessage)}
	if raw, exists := top["description"]; exists && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		var description string
		if err := json.Unmarshal(raw, &description); err != nil {
			return hookConfigDocument{}, fmt.Errorf("hook description")
		}
		document.description = &description
	}
	if raw, exists := top["hooks"]; exists {
		var events map[string]json.RawMessage
		if err := json.Unmarshal(raw, &events); err != nil || events == nil {
			return hookConfigDocument{}, fmt.Errorf("hook events")
		}
		for event, rawGroups := range events {
			if _, valid := officialHookEvents[event]; !valid {
				return hookConfigDocument{}, fmt.Errorf("hook event")
			}
			var groups []json.RawMessage
			if err := json.Unmarshal(rawGroups, &groups); err != nil || groups == nil {
				return hookConfigDocument{}, fmt.Errorf("hook matcher groups")
			}
			for _, group := range groups {
				if err := validateHookGroup(group); err != nil {
					return hookConfigDocument{}, err
				}
			}
			document.hooks[event] = groups
		}
	}
	return document, nil
}

func validateHookGroup(raw json.RawMessage) error {
	var group map[string]json.RawMessage
	if err := json.Unmarshal(raw, &group); err != nil || group == nil {
		return fmt.Errorf("hook matcher group")
	}
	for key := range group {
		if key != "matcher" && key != "hooks" {
			return fmt.Errorf("hook matcher group field")
		}
	}
	if matcher, exists := group["matcher"]; exists && !bytes.Equal(bytes.TrimSpace(matcher), []byte("null")) {
		var value string
		if err := json.Unmarshal(matcher, &value); err != nil {
			return fmt.Errorf("hook matcher")
		}
	}
	if hooks, exists := group["hooks"]; exists {
		var handlers []json.RawMessage
		if err := json.Unmarshal(hooks, &handlers); err != nil || handlers == nil {
			return fmt.Errorf("hook handlers")
		}
		for _, handler := range handlers {
			if err := validateHookHandler(handler); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateHookHandler(raw json.RawMessage) error {
	var handler map[string]json.RawMessage
	if err := json.Unmarshal(raw, &handler); err != nil || handler == nil {
		return fmt.Errorf("hook handler")
	}
	var kind string
	if rawType, exists := handler["type"]; !exists || json.Unmarshal(rawType, &kind) != nil {
		return fmt.Errorf("hook handler type")
	}
	var allowed map[string]struct{}
	var required []string
	switch kind {
	case "command":
		allowed = stringSet("type", "command", "commandWindows", "command_windows", "timeout", "async", "statusMessage", "additionalContextLimit")
		required = []string{"command"}
		if _, camel := handler["commandWindows"]; camel {
			if _, alias := handler["command_windows"]; alias {
				return fmt.Errorf("hook command alias")
			}
		}
	case "mcp_tool":
		allowed = stringSet("type", "server", "tool", "input", "timeout", "statusMessage")
		required = []string{"server", "tool"}
	case "prompt", "agent":
		allowed = stringSet("type")
	default:
		return fmt.Errorf("hook handler type")
	}
	for key := range handler {
		if _, valid := allowed[key]; !valid {
			return fmt.Errorf("hook handler field")
		}
	}
	for _, key := range required {
		if _, exists := handler[key]; !exists {
			return fmt.Errorf("hook handler required field")
		}
	}
	for _, key := range []string{"command", "commandWindows", "command_windows", "statusMessage", "server", "tool"} {
		if rawValue, exists := handler[key]; exists && !jsonStringOrNull(rawValue, key == "statusMessage" || key == "commandWindows" || key == "command_windows") {
			return fmt.Errorf("hook handler string")
		}
	}
	for _, key := range []string{"timeout", "additionalContextLimit"} {
		if rawValue, exists := handler[key]; exists && !jsonUnsignedOrNull(rawValue) {
			return fmt.Errorf("hook handler integer")
		}
	}
	if rawAsync, exists := handler["async"]; exists && !jsonBool(rawAsync) {
		return fmt.Errorf("hook handler async")
	}
	if rawInput, exists := handler["input"]; exists && !validMCPInput(rawInput) {
		return fmt.Errorf("hook MCP input")
	}
	return nil
}

func inspectHookDocument(document hookConfigDocument, command string) (HookConfigState, int) {
	expected := make(map[string]struct{}, len(managedHookEvents))
	for _, event := range managedHookEvents {
		expected[event] = struct{}{}
	}
	exactEvents := make(map[string]bool, len(managedHookEvents))
	markerEvents := make(map[string]int, len(managedHookEvents))
	markers := 0
	drifted := false
	conflict := false
	for event, groups := range document.hooks {
		for _, rawGroup := range groups {
			group, handlers := parseHookGroup(rawGroup)
			ownedInGroup := 0
			for _, handler := range handlers {
				marker := rawString(handler["statusMessage"]) == HookConfigMarker
				owned := handlerOwnsManagedCallback(handler)
				if handlerHasManagedCallbackShape(handler) && !marker {
					conflict = true
				}
				if !marker {
					continue
				}
				if !owned {
					conflict = true
					continue
				}
				markers++
				ownedInGroup++
				markerEvents[event]++
				if _, managed := expected[event]; !managed {
					conflict = true
					continue
				}
				if managedHandlerExact(handler, command) && len(handlers) == 1 {
					if _, hasMatcher := group["matcher"]; hasMatcher {
						drifted = true
					} else {
						exactEvents[event] = true
					}
				} else {
					drifted = true
				}
			}
			if ownedInGroup > 0 && len(handlers) != 1 {
				conflict = true
			}
		}
	}
	for event, count := range markerEvents {
		if count > 1 {
			conflict = true
		}
		if count == 1 && exactEvents[event] && drifted {
			// The global drift flag remains authoritative for another event.
		}
	}
	if conflict {
		return HookConfigConflict, len(exactEvents)
	}
	if markers == 0 {
		return HookConfigAbsent, 0
	}
	if drifted {
		return HookConfigDrifted, len(exactEvents)
	}
	if len(exactEvents) == len(managedHookEvents) {
		return HookConfigInstalled, len(exactEvents)
	}
	return HookConfigPartial, len(exactEvents)
}

func managedHandlerExact(handler map[string]json.RawMessage, command string) bool {
	if len(handler) != 6 || !handlerOwnsManagedCallback(handler) || !sameManagedCommand(rawString(handler["command"]), command) ||
		!sameManagedCommand(handlerWindowsCommand(handler), command) {
		return false
	}
	var timeout uint64
	var async bool
	return json.Unmarshal(handler["timeout"], &timeout) == nil && timeout == 3 &&
		json.Unmarshal(handler["async"], &async) == nil && !async
}

func handlerWindowsCommand(handler map[string]json.RawMessage) string {
	if raw, exists := handler["commandWindows"]; exists {
		return rawString(raw)
	}
	return rawString(handler["command_windows"])
}

func handlerHasManagedCallbackShape(handler map[string]json.RawMessage) bool {
	_, commandOK := parseManagedHookCommand(rawString(handler["command"]))
	_, windowsOK := parseManagedHookCommand(handlerWindowsCommand(handler))
	return commandOK || windowsOK
}

func handlerOwnsManagedCallback(handler map[string]json.RawMessage) bool {
	if rawString(handler["type"]) != "command" || rawString(handler["statusMessage"]) != HookConfigMarker {
		return false
	}
	commandPath, commandOK := parseManagedHookCommand(rawString(handler["command"]))
	windowsPath, windowsOK := parseManagedHookCommand(handlerWindowsCommand(handler))
	return commandOK && windowsOK && samePath(commandPath, windowsPath)
}

func sameManagedCommand(candidate, expected string) bool {
	candidate = strings.TrimSpace(candidate)
	if runtime.GOOS == "windows" {
		return candidate != "" && strings.EqualFold(candidate, expected)
	}
	return candidate != "" && candidate == expected
}

func ownedManagedGroupIndex(groups []json.RawMessage) (int, bool) {
	for index, raw := range groups {
		_, handlers := parseHookGroup(raw)
		for _, handler := range handlers {
			if handlerOwnsManagedCallback(handler) {
				return index, true
			}
		}
	}
	return 0, false
}

func parseHookGroup(raw json.RawMessage) (map[string]json.RawMessage, []map[string]json.RawMessage) {
	var group map[string]json.RawMessage
	_ = json.Unmarshal(raw, &group)
	var rawHandlers []json.RawMessage
	_ = json.Unmarshal(group["hooks"], &rawHandlers)
	handlers := make([]map[string]json.RawMessage, 0, len(rawHandlers))
	for _, rawHandler := range rawHandlers {
		var handler map[string]json.RawMessage
		_ = json.Unmarshal(rawHandler, &handler)
		handlers = append(handlers, handler)
	}
	return group, handlers
}

func rawString(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return value
}

func encodeHookDocument(document hookConfigDocument) ([]byte, error) {
	top := make(map[string]any, 2)
	if document.description != nil {
		top["description"] = *document.description
	}
	top["hooks"] = document.hooks
	data, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if len(data) > maxHookConfigBytes {
		return nil, fmt.Errorf("hook configuration size")
	}
	return data, nil
}

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func jsonStringOrNull(raw json.RawMessage, nullable bool) bool {
	if nullable && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return true
	}
	var value string
	return json.Unmarshal(raw, &value) == nil
}

func jsonUnsignedOrNull(raw json.RawMessage) bool {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return true
	}
	var value uint64
	return json.Unmarshal(raw, &value) == nil
}

func jsonBool(raw json.RawMessage) bool {
	var value bool
	return json.Unmarshal(raw, &value) == nil
}

func validMCPInput(raw json.RawMessage) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var input map[string]any
	if decoder.Decode(&input) != nil || input == nil {
		return false
	}
	for _, value := range input {
		if !validTOMLJSONValue(value) {
			return false
		}
	}
	return true
}

func validTOMLJSONValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string, bool:
		return true
	case json.Number:
		if strings.ContainsAny(string(typed), ".eE") {
			_, err := typed.Float64()
			return err == nil
		}
		_, err := typed.Int64()
		return err == nil
	case map[string]any:
		for _, child := range typed {
			if !validTOMLJSONValue(child) {
				return false
			}
		}
		return true
	case []any:
		for _, child := range typed {
			if !validTOMLJSONValue(child) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

type hookConfigJSONBudget struct{ tokens int }

func (budget *hookConfigJSONBudget) next(decoder *json.Decoder) (json.Token, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	budget.tokens++
	if budget.tokens > maxHookConfigTokens {
		return nil, fmt.Errorf("hook configuration token limit")
	}
	return token, nil
}

func scanHookConfigJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	budget := &hookConfigJSONBudget{}
	if err := scanHookConfigValue(decoder, budget, 0); err != nil {
		return err
	}
	if _, err := budget.next(decoder); !errors.Is(err, io.EOF) {
		return fmt.Errorf("hook configuration trailing data")
	}
	return nil
}

func scanHookConfigValue(decoder *json.Decoder, budget *hookConfigJSONBudget, depth int) error {
	token, err := budget.next(decoder)
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	if depth >= maxHookConfigDepth {
		return fmt.Errorf("hook configuration depth")
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := budget.next(decoder)
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("hook configuration key")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("hook configuration duplicate key")
			}
			seen[key] = struct{}{}
			if err := scanHookConfigValue(decoder, budget, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := scanHookConfigValue(decoder, budget, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("hook configuration delimiter")
	}
	end, err := budget.next(decoder)
	if err != nil {
		return err
	}
	if delimiter == '{' && end != json.Delim('}') || delimiter == '[' && end != json.Delim(']') {
		return fmt.Errorf("hook configuration delimiter")
	}
	return nil
}
