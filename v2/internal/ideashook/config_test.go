package ideashook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRenderHookConfigCreatesFourSeparateOfficialGroupsWithoutWriting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	executable := testHookExecutable(t, t.TempDir())
	options := HookConfigOptions{Scope: HookScopeUser, ExecutablePath: executable}

	preview, result, err := RenderHookConfig(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != HookConfigInstalled || !result.Changed || result.ManagedEvents != 4 || result.Activation != HookActivationState {
		t.Fatalf("result=%+v", result)
	}
	if _, err := os.Stat(filepath.Join(home, "hooks.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("render wrote hooks.json: %v", err)
	}

	document, err := decodeHookDocument(preview)
	if err != nil {
		t.Fatal(err)
	}
	wantCommand, err := managedHookCommand(executable)
	if err != nil {
		t.Fatal(err)
	}
	if state, count := inspectHookDocument(document, wantCommand); state != HookConfigInstalled || count != 4 {
		t.Fatalf("state=%s count=%d", state, count)
	}
	for _, event := range managedHookEvents {
		groups := document.hooks[event]
		if len(groups) != 1 {
			t.Fatalf("%s groups=%d", event, len(groups))
		}
		group, handlers := parseHookGroup(groups[0])
		if _, exists := group["matcher"]; exists || len(handlers) != 1 || !managedHandlerExact(handlers[0], wantCommand) {
			t.Fatalf("%s group=%s", event, groups[0])
		}
		if rawString(handlers[0]["command"]) != wantCommand || rawString(handlers[0]["commandWindows"]) != wantCommand {
			t.Fatalf("%s command mismatch", event)
		}
	}
}

func TestRenderPreservesUnrelatedHooksAndInstallNeverRewritesExistingConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	executable := testHookExecutable(t, t.TempDir())
	path := filepath.Join(home, "hooks.json")
	original := `{
  "description": "owner config",
  "hooks": {
    "PreToolUse": [
      {"matcher":"Write|Edit","hooks":[{"type":"command","command":"owner-check","timeout":9,"async":true,"statusMessage":"owner hook"}]}
    ]
  }
}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	options := HookConfigOptions{Scope: HookScopeUser, ExecutablePath: executable}
	preview, result, err := RenderHookConfig(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != HookConfigInstalled || !result.Changed {
		t.Fatalf("result=%+v", result)
	}
	document, err := decodeHookDocument(preview)
	if err != nil {
		t.Fatal(err)
	}
	if document.description == nil || *document.description != "owner config" || len(document.hooks["PreToolUse"]) != 1 || !bytes.Contains(document.hooks["PreToolUse"][0], []byte("owner-check")) {
		t.Fatalf("unrelated configuration was not preserved in preview: %s", preview)
	}
	if _, err := InstallHookConfig(context.Background(), options); err == nil || CodeOf(err) != CodeIdentityConflict {
		t.Fatalf("existing config install err=%v code=%s", err, CodeOf(err))
	}
	afterConflict, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(afterConflict, []byte(original)) {
		t.Fatalf("existing config changed: %s err=%v", afterConflict, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	installedResult, err := InstallHookConfig(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if installedResult.State != HookConfigInstalled || !installedResult.Changed {
		t.Fatalf("installed result=%+v", installedResult)
	}
	installed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	markerTime := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(path, markerTime, markerTime); err != nil {
		t.Fatal(err)
	}
	second, err := InstallHookConfig(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed || second.State != HookConfigInstalled {
		t.Fatalf("second=%+v", second)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(installed, after) || !info.ModTime().Equal(markerTime) {
		t.Fatalf("exact reinstall changed bytes or mtime: before=%s after=%s mtime=%s", installed, after, info.ModTime())
	}
}

func TestInstallHookConfigSemanticReinstallPreservesOriginalBytesAndTime(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	executable := testHookExecutable(t, t.TempDir())
	options := HookConfigOptions{Scope: HookScopeUser, ExecutablePath: executable}
	preview, _, err := RenderHookConfig(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	var semantic any
	if err := json.Unmarshal(preview, &semantic); err != nil {
		t.Fatal(err)
	}
	minified, err := json.Marshal(semantic)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(preview, minified) {
		t.Fatal("semantic fixture did not change representation")
	}
	path := filepath.Join(home, "hooks.json")
	if err := os.WriteFile(path, minified, 0o600); err != nil {
		t.Fatal(err)
	}
	markerTime := time.Unix(1_710_000_000, 0)
	if err := os.Chtimes(path, markerTime, markerTime); err != nil {
		t.Fatal(err)
	}
	result, err := InstallHookConfig(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != HookConfigInstalled || result.Changed || result.Activation != HookActivationState {
		t.Fatalf("result=%+v", result)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, minified) || !info.ModTime().Equal(markerTime) {
		t.Fatalf("semantic no-op changed bytes or mtime: bytes=%t mtime=%s", bytes.Equal(after, minified), info.ModTime())
	}
}

func TestRenderHookConfigUpgradesOldExecutableAtOriginalGroupIndex(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	executable := testHookExecutable(t, t.TempDir())
	oldExecutable := filepath.Join(t.TempDir(), "mindweaver")
	if runtime.GOOS == "windows" {
		oldExecutable += ".exe"
	}
	oldCommand := `"` + oldExecutable + `" ideas hook codex`
	path := filepath.Join(home, "hooks.json")
	original := `{"hooks":{"SessionStart":[` +
		`{"hooks":[{"type":"command","command":"before"}]},` +
		`{"hooks":[{"type":"command","command":"` + escapeJSONString(oldCommand) + `","commandWindows":"` + escapeJSONString(oldCommand) + `","timeout":4,"async":false,"statusMessage":"` + HookConfigMarker + `"}]},` +
		`{"hooks":[{"type":"command","command":"after"}]}` +
		`],"Stop":[{"hooks":[{"type":"command","command":"owner-stop"}]}]}}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	options := HookConfigOptions{Scope: HookScopeUser, ExecutablePath: executable}
	before, err := InspectHookConfig(context.Background(), options)
	if err != nil || before.State != HookConfigDrifted {
		t.Fatalf("before=%+v err=%v", before, err)
	}
	preview, result, err := RenderHookConfig(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != HookConfigInstalled || !result.Changed {
		t.Fatalf("result=%+v", result)
	}
	document, err := decodeHookDocument(preview)
	if err != nil {
		t.Fatal(err)
	}
	groups := document.hooks["SessionStart"]
	if len(groups) != 3 || !bytes.Contains(groups[0], []byte("before")) || !bytes.Contains(groups[2], []byte("after")) {
		t.Fatalf("group order changed: %s", groups)
	}
	_, handlers := parseHookGroup(groups[1])
	command, err := managedHookCommand(executable)
	if err != nil || len(handlers) != 1 || !managedHandlerExact(handlers[0], command) {
		t.Fatalf("managed group=%s commandErr=%v", groups[1], err)
	}
	stopGroups := document.hooks["Stop"]
	if len(stopGroups) != 2 || !bytes.Contains(stopGroups[0], []byte("owner-stop")) {
		t.Fatalf("missing managed group was not appended: %s", stopGroups)
	}
	_, stopHandlers := parseHookGroup(stopGroups[1])
	if len(stopHandlers) != 1 || !managedHandlerExact(stopHandlers[0], command) {
		t.Fatalf("appended managed group=%s", stopGroups[1])
	}
}

func TestInspectHookConfigClassifiesAbsentPartialDriftConflictInvalidAndInstalled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	executable := testHookExecutable(t, t.TempDir())
	options := HookConfigOptions{Scope: HookScopeUser, ExecutablePath: executable}
	path := filepath.Join(home, "hooks.json")
	assertState := func(want HookConfigState) {
		t.Helper()
		result, err := InspectHookConfig(context.Background(), options)
		if err != nil {
			t.Fatal(err)
		}
		if result.State != want {
			t.Fatalf("state=%s want=%s result=%+v", result.State, want, result)
		}
	}
	assertState(HookConfigAbsent)

	preview, _, err := RenderHookConfig(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]any
	if err := json.Unmarshal(preview, &top); err != nil {
		t.Fatal(err)
	}
	hooks := top["hooks"].(map[string]any)
	delete(hooks, "Stop")
	delete(hooks, "SessionEnd")
	writeJSONFile(t, path, top)
	assertState(HookConfigPartial)

	groups := hooks["SessionStart"].([]any)
	handler := groups[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	handler["timeout"] = float64(4)
	writeJSONFile(t, path, top)
	assertState(HookConfigDrifted)

	handler["timeout"] = float64(3)
	handler["statusMessage"] = "owner marker"
	writeJSONFile(t, path, top)
	assertState(HookConfigConflict)
	if _, _, err := RenderHookConfig(context.Background(), options); err == nil || CodeOf(err) != CodeIdentityConflict {
		t.Fatalf("conflicting render err=%v code=%s", err, CodeOf(err))
	}

	if err := os.WriteFile(path, []byte(`{"hooks":{"sessionStart":[]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	assertState(HookConfigInvalid)
	if _, _, err := RenderHookConfig(context.Background(), options); err == nil || CodeOf(err) != CodeInvalidInput {
		t.Fatalf("invalid render err=%v code=%s", err, CodeOf(err))
	}

	if err := os.WriteFile(path, preview, 0o600); err != nil {
		t.Fatal(err)
	}
	assertState(HookConfigInstalled)
}

func TestHookConfigConflictRejectsDuplicateMarkerSharedGroupAndUnmarkedCallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	executable := testHookExecutable(t, t.TempDir())
	command, err := managedHookCommand(executable)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "hooks.json")
	options := HookConfigOptions{Scope: HookScopeUser, ExecutablePath: executable}
	cases := []string{
		`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"other","statusMessage":"` + HookConfigMarker + `"}]}]}}`,
		`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"` + escapeJSONString(command) + `","commandWindows":"` + escapeJSONString(command) + `","timeout":3,"async":false,"statusMessage":"` + HookConfigMarker + `"},{"type":"agent"}]}]}}`,
		`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"` + escapeJSONString(command) + `","commandWindows":"` + escapeJSONString(command) + `","timeout":3,"async":false,"statusMessage":"` + HookConfigMarker + `"}]},{"hooks":[{"type":"command","command":"other","statusMessage":"` + HookConfigMarker + `"}]}]}}`,
		`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"other","statusMessage":"` + HookConfigMarker + `"}]}]}}`,
		`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"` + escapeJSONString(command) + `"}]}]}}`,
	}
	for index, input := range cases {
		if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := InspectHookConfig(context.Background(), options)
		if err != nil || result.State != HookConfigConflict {
			t.Fatalf("case %d result=%+v err=%v input=%s", index, result, err, input)
		}
	}
}

func TestHookConfigReadIsBoundedAndExplicitCodexHomeMustExist(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	executable := testHookExecutable(t, t.TempDir())
	path := filepath.Join(home, "hooks.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, maxHookConfigBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := InspectHookConfig(context.Background(), HookConfigOptions{Scope: HookScopeUser, ExecutablePath: executable})
	if err != nil || result.State != HookConfigInvalid {
		t.Fatalf("result=%+v err=%v", result, err)
	}

	missing := filepath.Join(t.TempDir(), "missing-codex-home")
	t.Setenv("CODEX_HOME", missing)
	if _, _, err := RenderHookConfig(context.Background(), HookConfigOptions{Scope: HookScopeUser, ExecutablePath: executable}); err == nil || CodeOf(err) != CodeInvalidInput {
		t.Fatalf("missing CODEX_HOME err=%v code=%s", err, CodeOf(err))
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing CODEX_HOME was created: %v", err)
	}
}

func TestHookConfigRejectsDuplicateUnknownAndDangerousCommandPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	path := filepath.Join(home, "hooks.json")
	executable := testHookExecutable(t, t.TempDir())
	options := HookConfigOptions{Scope: HookScopeUser, ExecutablePath: executable}
	for _, input := range []string{
		`null`,
		`{"hooks":{},"hooks":{}}`,
		`{"hooks":null}`,
		`{"unknown":true,"hooks":{}}`,
		`{"hooks":{"SessionStart":null}}`,
		`{"hooks":{"SessionStart":[null]}}`,
		`{"hooks":{"SessionStart":[{"hooks":null}]}}`,
		`{"description":"\ud800","hooks":{}}`,
		`{"hooks":{"SessionStart":[{"unknown":true,"hooks":[]}]}}`,
		`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"x","unexpected":true}]}]}}`,
	} {
		if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := InspectHookConfig(context.Background(), options)
		if err != nil || result.State != HookConfigInvalid {
			t.Fatalf("input=%s result=%+v err=%v", input, result, err)
		}
	}

	dangerDirectory := filepath.Join(t.TempDir(), "danger&owner")
	if err := os.Mkdir(dangerDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	danger := testHookExecutable(t, dangerDirectory)
	if _, _, err := RenderHookConfig(context.Background(), HookConfigOptions{Scope: HookScopeUser, ExecutablePath: danger}); err == nil || CodeOf(err) != CodeInvalidInput {
		t.Fatalf("dangerous executable err=%v code=%s", err, CodeOf(err))
	}
	if _, _, err := RenderHookConfig(context.Background(), HookConfigOptions{Scope: "ambient", ExecutablePath: executable}); err == nil || CodeOf(err) != CodeInvalidInput {
		t.Fatalf("ambient scope err=%v code=%s", err, CodeOf(err))
	}
	otherExecutable := filepath.Join(t.TempDir(), "other")
	if runtime.GOOS == "windows" {
		otherExecutable += ".exe"
	}
	if err := os.WriteFile(otherExecutable, []byte("test executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := RenderHookConfig(context.Background(), HookConfigOptions{Scope: HookScopeUser, ExecutablePath: otherExecutable}); err == nil || CodeOf(err) != CodeInvalidInput {
		t.Fatalf("foreign executable err=%v code=%s", err, CodeOf(err))
	}
}

func TestRepoHookScopeUsesMainCheckoutFromLinkedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	parent := t.TempDir()
	mainRoot := filepath.Join(parent, "main")
	linkedRoot := filepath.Join(parent, "linked")
	if err := os.Mkdir(mainRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, mainRoot, "init")
	runGitTest(t, mainRoot, "config", "user.email", "mindweaver@example.invalid")
	runGitTest(t, mainRoot, "config", "user.name", "MindWeaver Test")
	if err := os.WriteFile(filepath.Join(mainRoot, "tracked.txt"), []byte("tracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, mainRoot, "add", "tracked.txt")
	runGitTest(t, mainRoot, "commit", "-m", "base")
	runGitTest(t, mainRoot, "worktree", "add", "-b", "linked-test", linkedRoot)

	executable := testHookExecutable(t, filepath.Join(parent, "bin"))
	result, err := InstallHookConfig(context.Background(), HookConfigOptions{
		Scope: HookScopeRepo, WorkingDirectory: linkedRoot, ExecutablePath: executable,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != HookConfigInstalled {
		t.Fatalf("result=%+v", result)
	}
	if _, err := os.Stat(filepath.Join(mainRoot, ".codex", "hooks.json")); err != nil {
		t.Fatalf("main checkout config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(linkedRoot, ".codex", "hooks.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("linked worktree received config: %v", err)
	}
}

func TestRepoHookScopeIgnoresGitAuthorityEnvironmentAndRejectsNonMainLayouts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	other := filepath.Join(root, "other")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "init")
	runGitTest(t, other, "init")
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	resolved, err := resolveMainCheckoutRoot(context.Background(), repo)
	if err != nil || !samePath(resolved, repo) {
		t.Fatalf("resolved=%q err=%v", resolved, err)
	}
	for _, entry := range sanitizedGitEnvironment() {
		key := strings.SplitN(entry, "=", 2)[0]
		if strings.HasPrefix(strings.ToUpper(key), "GIT_") && !strings.EqualFold(key, "GIT_TERMINAL_PROMPT") {
			t.Fatalf("authority environment retained: %s", key)
		}
	}
	t.Setenv("GIT_DIR", "")
	t.Setenv("GIT_WORK_TREE", "")

	separateWork := filepath.Join(root, "separate-work")
	separateGit := filepath.Join(root, "separate-metadata")
	runGitTest(t, root, "init", "--separate-git-dir", separateGit, separateWork)
	if _, err := resolveMainCheckoutRoot(context.Background(), separateWork); err == nil {
		t.Fatal("separate-git-dir checkout accepted")
	}

	source := filepath.Join(root, "source")
	parent := filepath.Join(root, "parent")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, source, "init")
	runGitTest(t, source, "config", "user.email", "mindweaver@example.invalid")
	runGitTest(t, source, "config", "user.name", "MindWeaver Test")
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("tracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, source, "add", "tracked.txt")
	runGitTest(t, source, "commit", "-m", "source")
	runGitTest(t, parent, "init")
	runGitTest(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", source, "module")
	if _, err := resolveMainCheckoutRoot(context.Background(), filepath.Join(parent, "module")); err == nil {
		t.Fatal("submodule checkout accepted")
	}
}

func TestRenderAndInspectAreZeroWriteForMissingRepoConfig(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	repo := t.TempDir()
	runGitTest(t, repo, "init")
	executable := testHookExecutable(t, t.TempDir())
	options := HookConfigOptions{Scope: HookScopeRepo, WorkingDirectory: repo, ExecutablePath: executable}
	result, err := InspectHookConfig(context.Background(), options)
	if err != nil || result.State != HookConfigAbsent {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, _, err := RenderHookConfig(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".codex")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only operations created .codex: %v", err)
	}
}

func TestHookConfigContextCancellationIsContentFree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	executable := testHookExecutable(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := RenderHookConfig(ctx, HookConfigOptions{Scope: HookScopeUser, ExecutablePath: executable})
	if err == nil || CodeOf(err) != CodeCanceled || strings.Contains(err.Error(), executable) {
		t.Fatalf("err=%v code=%s", err, CodeOf(err))
	}
}

func testHookExecutable(t *testing.T, directory string) string {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "mindweaver"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("test executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func escapeJSONString(value string) string {
	data, _ := json.Marshal(value)
	return strings.Trim(string(data), `"`)
}

func runGitTest(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	command.Env = sanitizedGitEnvironment()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
}
