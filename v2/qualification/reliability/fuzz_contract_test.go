package reliability

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	coreFuzzTime       = "1s"
	fuzzCommandTimeout = 2 * time.Minute
	maxFuzzOutput      = 2 << 20
	maxTestSourceBytes = 4 << 20
)

type fuzzTarget struct {
	Package string
	Name    string
}

type reliabilityToolchain struct {
	goTool string
	goRoot string
}

var frozenCoreFuzzTargets = []fuzzTarget{
	{Package: "./internal/blob", Name: "FuzzParseIDCanonical"},
	{Package: "./internal/ingest", Name: "FuzzChunkTextDeterministicAndBounded"},
	{Package: "./internal/ingest", Name: "FuzzReadTextCanonicalAndBounded"},
	{Package: "./internal/pdfextract/protocol", Name: "FuzzDecodeResultRequiresCanonicalFrame"},
}

func TestCoreFuzzTargetClosureAndExecution(t *testing.T) {
	root := reliabilityModuleRoot(t)
	got := discoverProductionFuzzTargets(t, root)
	if !slices.Equal(got, frozenCoreFuzzTargets) {
		t.Fatalf("CORE fuzz target set = %#v, want %#v", got, frozenCoreFuzzTargets)
	}

	toolchain := reliabilityGoTool(t)
	moduleMode := reliabilityModuleMode(t, root)
	for _, target := range frozenCoreFuzzTargets {
		target := target
		t.Run(strings.TrimPrefix(target.Package, "./")+"/"+target.Name, func(t *testing.T) {
			runCoreFuzzTarget(t, root, toolchain, moduleMode, target)
		})
	}
}

func discoverProductionFuzzTargets(t *testing.T, root string) []fuzzTarget {
	t.Helper()
	var targets []fuzzTarget
	seen := make(map[fuzzTarget]struct{})
	for _, sourceRoot := range []string{"cmd", "internal", "platform"} {
		walkRoot := filepath.Join(root, sourceRoot)
		if err := filepath.WalkDir(walkRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("production fuzz inventory encountered a symlink: %s", filepath.Base(path))
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			if !info.Mode().IsRegular() || info.Size() > maxTestSourceBytes {
				return fmt.Errorf("production test source is not a bounded regular file: %s", filepath.Base(path))
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
			if err != nil {
				return fmt.Errorf("parse production test source %s: %w", filepath.Base(path), err)
			}
			for _, declaration := range parsed.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || !isGoFuzzTarget(function) {
					continue
				}
				relativePackage, err := filepath.Rel(root, filepath.Dir(path))
				if err != nil || relativePackage == "." || strings.HasPrefix(relativePackage, "..") {
					return errors.New("production fuzz target escaped the module")
				}
				target := fuzzTarget{Package: "./" + filepath.ToSlash(relativePackage), Name: function.Name.Name}
				if _, duplicate := seen[target]; duplicate {
					return fmt.Errorf("duplicate production fuzz target %s in %s", target.Name, target.Package)
				}
				seen[target] = struct{}{}
				targets = append(targets, target)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	sort.Slice(targets, func(left, right int) bool {
		if targets[left].Package != targets[right].Package {
			return targets[left].Package < targets[right].Package
		}
		return targets[left].Name < targets[right].Name
	})
	return targets
}

// isGoFuzzTarget mirrors cmd/go's syntactic fuzz-target recognition. In
// particular, Go accepts *F and *alias.F because import/type resolution occurs
// only when the generated test main is compiled.
func isGoFuzzTarget(function *ast.FuncDecl) bool {
	if function.Recv != nil || !isGoTestName(function.Name.Name, "Fuzz") ||
		function.Type.Params == nil || len(function.Type.Params.List) != 1 ||
		function.Type.Results != nil && len(function.Type.Results.List) != 0 ||
		function.Type.TypeParams != nil && function.Type.TypeParams.NumFields() != 0 {
		return false
	}
	parameter := function.Type.Params.List[0]
	if len(parameter.Names) > 1 {
		return false
	}
	pointer, ok := parameter.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	if selector, ok := pointer.X.(*ast.SelectorExpr); ok {
		return selector.Sel.Name == "F"
	}
	name, ok := pointer.X.(*ast.Ident)
	return ok && name.Name == "F"
}

func isGoTestName(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) {
		return true
	}
	character, _ := utf8.DecodeRuneInString(name[len(prefix):])
	return !unicode.IsLower(character)
}

func TestGoFuzzTargetClassifier(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		declaration string
		want        bool
	}{
		{declaration: "func FuzzAlias(*F) {}", want: true},
		{declaration: "func FuzzQualified(value *external.F) {}", want: true},
		{declaration: "func Fuzzlower(*testing.F) {}", want: false},
		{declaration: "func FuzzGeneric[T any](*testing.F) {}", want: false},
		{declaration: "func FuzzTwo(left, right *testing.F) {}", want: false},
		{declaration: "func FuzzResult(*testing.F) error { return nil }", want: false},
	} {
		fixture := fixture
		t.Run(fixture.declaration, func(t *testing.T) {
			parsed, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture\n"+fixture.declaration, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			function := parsed.Decls[0].(*ast.FuncDecl)
			if got := isGoFuzzTarget(function); got != fixture.want {
				t.Fatalf("isGoFuzzTarget = %t, want %t", got, fixture.want)
			}
		})
	}
}

func TestReliabilityGoToolPrefersConfiguredIdentity(t *testing.T) {
	configured := strings.TrimSpace(os.Getenv("MW_GO"))
	if configured == "" {
		configured = filepath.Join(runtime.GOROOT(), "bin", "go.exe")
	}
	resolved, err := filepath.EvalSymlinks(configured)
	if err != nil {
		t.Fatal(err)
	}
	configuredEnvironmentPath := resolved
	if runtime.GOOS == "windows" {
		configuredEnvironmentPath = strings.ToUpper(resolved)
	}
	t.Setenv("MW_GO", configuredEnvironmentPath)
	t.Setenv("GOROOT", filepath.Join(t.TempDir(), "bogus-runtime-root"))
	toolchain := reliabilityGoTool(t)
	actualInfo, err := os.Stat(toolchain.goTool)
	if err != nil {
		t.Fatal(err)
	}
	wantInfo, err := os.Stat(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(actualInfo, wantInfo) || !strings.EqualFold(toolchain.goRoot, filepath.Dir(filepath.Dir(resolved))) {
		t.Fatalf("resolved toolchain = %#v, want executable %q", toolchain, resolved)
	}
}

func TestReliabilityOfflineEnvironmentReplacesMixedCaseToolVariables(t *testing.T) {
	toolchain := reliabilityToolchain{
		goTool: `C:\Pinned\Go\bin\go.exe`,
		goRoot: `C:\Pinned\Go`,
	}
	got := reliabilityOfflineEnvironment([]string{
		"Path=preserved",
		"goRoot=C:\\bogus",
		"Mw_Go=C:\\bogus\\go.exe",
		"GoFlAgS=-mod=mod",
		"GoPrOxY=https://proxy.invalid",
	}, toolchain)
	values := make(map[string][]string)
	for _, entry := range got {
		name, value, found := strings.Cut(entry, "=")
		if !found {
			t.Fatalf("malformed environment entry %q", entry)
		}
		key := strings.ToUpper(name)
		values[key] = append(values[key], value)
	}
	for key, want := range map[string]string{
		"GOROOT":  toolchain.goRoot,
		"MW_GO":   toolchain.goTool,
		"GOFLAGS": "-trimpath -buildvcs=false",
		"GOPROXY": "off",
	} {
		if len(values[key]) != 1 || values[key][0] != want {
			t.Fatalf("%s entries = %#v, want only %q", key, values[key], want)
		}
	}
	if len(values["PATH"]) != 1 || values["PATH"][0] != "preserved" {
		t.Fatalf("unrelated environment = %#v", values["PATH"])
	}
}

func TestValidateReliabilityGoToolRejectsIncompleteRoot(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "bin", "go.exe")
	if err := os.MkdirAll(filepath.Dir(fake), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fake, []byte("not a toolchain"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateReliabilityGoTool(fake, "go.exe"); err == nil {
		t.Fatal("incomplete Go toolchain root was accepted")
	}
}

func runCoreFuzzTarget(t *testing.T, root string, toolchain reliabilityToolchain, moduleMode string, target fuzzTarget) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), fuzzCommandTimeout)
	defer cancel()
	arguments := []string{
		"test", "-mod=" + moduleMode, "-count=1", "-run=^$",
		"-fuzz=^" + regexp.QuoteMeta(target.Name) + "$", "-fuzztime=" + coreFuzzTime,
		"-parallel=1", "-timeout=90s", target.Package,
	}
	command := exec.CommandContext(ctx, toolchain.goTool, arguments...)
	command.Dir = root
	command.Env = reliabilityOfflineEnvironment(os.Environ(), toolchain)
	output := &boundedFuzzOutput{limit: maxFuzzOutput}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()
	if ctx.Err() != nil {
		t.Fatalf("%s exceeded its execution bound", target.Name)
	}
	if output.exceeded {
		t.Fatalf("%s exceeded its output bound", target.Name)
	}
	text := output.String()
	if err != nil {
		t.Fatalf("direct Go fuzz command for %s failed: %v\n%s", target.Name, err, text)
	}
	if !strings.Contains(text, "fuzz:") || !strings.Contains(text, "execs:") || !strings.Contains(text, "PASS") {
		t.Fatalf("direct Go fuzz command for %s did not prove mutation and PASS:\n%s", target.Name, text)
	}
}

func reliabilityModuleRoot(t *testing.T) string {
	t.Helper()
	current, err := os.Getwd()
	if err != nil {
		t.Fatal("resolve qualification working directory")
	}
	for {
		if info, statErr := os.Lstat(filepath.Join(current, "go.mod")); statErr == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatal("resolve v2 module root")
		}
		current = parent
	}
}

func reliabilityGoTool(t *testing.T) reliabilityToolchain {
	t.Helper()
	name := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidate := strings.TrimSpace(os.Getenv("MW_GO"))
	if candidate == "" {
		candidate = filepath.Join(runtime.GOROOT(), "bin", name)
	}
	toolchain, err := validateReliabilityGoTool(candidate, name)
	if err != nil {
		t.Fatalf("frozen Go fuzz tool is unavailable: %v", err)
	}
	command := exec.CommandContext(t.Context(), toolchain.goTool, "version")
	command.Env = reliabilityOfflineEnvironment(os.Environ(), toolchain)
	output, err := command.Output()
	if err != nil || strings.TrimSpace(string(output)) != "go version go1.27.0 windows/amd64" {
		t.Fatal("frozen Go fuzz tool has an unsupported identity")
	}
	return toolchain
}

func validateReliabilityGoTool(candidate, executableName string) (reliabilityToolchain, error) {
	nameMatches := filepath.Base(candidate) == executableName
	if runtime.GOOS == "windows" {
		nameMatches = strings.EqualFold(filepath.Base(candidate), executableName)
	}
	if strings.TrimSpace(candidate) == "" || !nameMatches {
		return reliabilityToolchain{}, errors.New("Go executable path is invalid")
	}
	absolute, err := filepath.Abs(candidate)
	if err != nil {
		return reliabilityToolchain{}, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return reliabilityToolchain{}, err
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return reliabilityToolchain{}, errors.New("Go executable is not a regular file")
	}
	root := filepath.Dir(filepath.Dir(resolved))
	expected, err := filepath.EvalSymlinks(filepath.Join(root, "bin", executableName))
	if err != nil {
		return reliabilityToolchain{}, err
	}
	expectedInfo, err := os.Stat(expected)
	if err != nil || !os.SameFile(info, expectedInfo) {
		return reliabilityToolchain{}, errors.New("Go executable does not belong to its inferred root")
	}
	for _, required := range []string{
		filepath.Join(root, "bin", "gofmt.exe"),
		filepath.Join(root, "pkg", "tool", "windows_amd64", "compile.exe"),
		filepath.Join(root, "src", "runtime", "proc.go"),
	} {
		requiredInfo, requiredErr := os.Lstat(required)
		if requiredErr != nil || !requiredInfo.Mode().IsRegular() || requiredInfo.Mode()&os.ModeSymlink != 0 {
			return reliabilityToolchain{}, errors.New("Go toolchain root is incomplete")
		}
	}
	return reliabilityToolchain{goTool: resolved, goRoot: root}, nil
}

func reliabilityModuleMode(t *testing.T, root string) string {
	t.Helper()
	manifest := filepath.Join(root, "vendor", "modules.txt")
	info, err := os.Lstat(manifest)
	if err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatal("vendor/modules.txt is not a regular file")
		}
		return "vendor"
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatal("inspect vendor/modules.txt")
	}
	return "readonly"
}

func reliabilityOfflineEnvironment(environment []string, toolchain reliabilityToolchain) []string {
	overrides := map[string]string{
		"CGO_ENABLED": "0", "GO111MODULE": "on", "GOARCH": "amd64",
		"GOAMD64": "v1", "GOENV": "off", "GOEXPERIMENT": "",
		"GOFIPS140": "off", "GOFLAGS": "-trimpath -buildvcs=false",
		"GOOS": "windows", "GOPROXY": "off", "GOROOT": toolchain.goRoot,
		"GOSUMDB": "off", "GOTELEMETRY": "off", "GOTOOLCHAIN": "local",
		"GOVCS": "*:off", "GOWORK": "off", "MW_GO": toolchain.goTool,
	}
	result := make([]string, 0, len(environment)+len(overrides))
	for _, item := range environment {
		name, _, found := strings.Cut(item, "=")
		if !found {
			continue
		}
		if _, replaced := overrides[strings.ToUpper(name)]; !replaced {
			result = append(result, item)
		}
	}
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result = append(result, key+"="+overrides[key])
	}
	return result
}

type boundedFuzzOutput struct {
	mu       sync.Mutex
	data     []byte
	limit    int
	exceeded bool
}

func (output *boundedFuzzOutput) Write(value []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	written := len(value)
	remaining := output.limit - len(output.data)
	if remaining > len(value) {
		remaining = len(value)
	}
	if remaining > 0 {
		output.data = append(output.data, value[:remaining]...)
	}
	if remaining < len(value) {
		output.exceeded = true
	}
	return written, nil
}

func (output *boundedFuzzOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return string(output.data)
}
