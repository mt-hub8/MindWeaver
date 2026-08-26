package production

import (
	"bufio"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const modulePath = "github.com/mt-hub8/MindWeaver/v2"

type artifactContract struct {
	name              string
	target            string
	requiredPackages  []string
	forbiddenPackages []string
	requiredSymbols   []string
	forbiddenSymbols  []string
	modules           []string
}

var shippedArtifacts = []artifactContract{
	{
		name:   "mindweaver.exe",
		target: "./cmd/mindweaver",
		requiredPackages: []string{
			modulePath + "/internal/app",
			modulePath + "/internal/backup",
			modulePath + "/internal/pdfextract/client",
			modulePath + "/internal/pdfextract/protocol",
		},
		forbiddenPackages: []string{
			modulePath + "/internal/pdfextract/parser",
			"github.com/mgilbir/formalis",
			"github.com/mgilbir/golittlecms",
			"github.com/mgilbir/gopenjpeg",
			"github.com/mgilbir/pdf0",
		},
		requiredSymbols: []string{
			modulePath + "/internal/backup.(*Coordinator).Create",
		},
		forbiddenSymbols: []string{
			modulePath + "/internal/pdfextract/parser.",
			"github.com/mgilbir/pdf0.",
		},
		modules: []string{
			"github.com/ncruces/go-sqlite3-wasm/v3@v3.2.35304#h1:5NoQAewtgKNK3G4bjNPxVoGXu6F6NzLXWCTdD5FFAEY=",
			"github.com/ncruces/go-sqlite3@v0.35.3#h1:Ei07Zv1qfV/vyXzelhFsyS5Oh9TArBZHsmFk14Xv3GY=",
			"github.com/ncruces/julianday@v1.0.0#h1:fH0OKwa7NWvniGQtxdJRxAgkBMolni2BjDHaWTxqt7M=",
			"golang.org/x/sys@v0.47.0#h1:o7XGOvZQCADBQQ4Y7VNq2dRWQR7JmOUW8Kxx4ZsNgWs=",
		},
	},
	{
		name:   "mindweaver-pdf.exe",
		target: "./cmd/mindweaver-pdf",
		requiredPackages: []string{
			modulePath + "/internal/pdfextract/parser",
			modulePath + "/internal/pdfextract/protocol",
			"github.com/mgilbir/pdf0",
		},
		forbiddenPackages: []string{
			modulePath + "/internal/app",
			modulePath + "/internal/backup",
			modulePath + "/internal/pdfextract/client",
			"os/exec",
			"golang.org/x/sys/windows",
		},
		requiredSymbols: []string{
			modulePath + "/internal/pdfextract/parser.Run",
			"github.com/mgilbir/pdf0.(*Document).ExtractTextContext",
		},
		forbiddenSymbols: []string{
			modulePath + "/internal/app.",
			modulePath + "/internal/backup.",
			modulePath + "/internal/pdfextract/client.",
		},
		modules: []string{
			"github.com/mgilbir/formalis@v0.3.1#h1:NyYe/EcRYJ2jUjgaZG98lNXgJ7H+jgy6mq7HXOnQxl8=",
			"github.com/mgilbir/golittlecms@v0.0.0-20260727161601-f6af7cfe1556#h1:2ZUsOgMhxpHCYC8jyzeEnJZFLYGbXhqjAJWJBvY8q4U=",
			"github.com/mgilbir/gopenjpeg@v0.0.0-20260727163526-8a139bc479b2#h1:kdDIM4JNxn9gsRk5Zo6mtmcFpBqnl9gTVUwf9t6lIRk=",
			"github.com/mgilbir/pdf0@v0.1.0#h1:rfBK18bcQ4kHQTXBmriAb07TafhG2w1fLflq9lHgaG4=",
		},
	},
}

// TestWindowsAMD64ShippedDualPEClosure qualifies the actual two executable
// artifacts. Source imports alone are insufficient: both final PE files are
// inspected through buildinfo and the Go symbol table after an offline build.
func TestWindowsAMD64ShippedDualPEClosure(t *testing.T) {
	root := moduleRoot(t)
	goTool := goTool(t)
	environment := offlineWindowsAMD64Environment()

	wantCommands := make([]string, 0, len(shippedArtifacts))
	for _, artifact := range shippedArtifacts {
		wantCommands = append(wantCommands, modulePath+"/"+strings.TrimPrefix(artifact.target, "./"))
	}
	sort.Strings(wantCommands)
	gotCommands := commandMainPackages(t, goTool, root, environment)
	if strings.Join(gotCommands, "\n") != strings.Join(wantCommands, "\n") {
		t.Fatalf("shipped command exact-set:\n got %q\nwant %q", gotCommands, wantCommands)
	}
	for _, forbidden := range []string{"mindweaver-migrate", "mindweaver-neutral-export"} {
		for _, command := range gotCommands {
			if strings.Contains(command, forbidden) {
				t.Fatalf("legacy command %q remains in shipped exact-set", command)
			}
		}
		if _, err := os.Lstat(filepath.Join(root, "cmd", forbidden)); !os.IsNotExist(err) {
			t.Fatalf("legacy command path cmd/%s exists: %v", forbidden, err)
		}
	}

	output := t.TempDir()
	for _, artifact := range shippedArtifacts {
		artifact := artifact
		t.Run(artifact.name, func(t *testing.T) {
			graph := packageGraph(t, goTool, root, environment, artifact.target)
			assertGraph(t, artifact.name, graph, artifact.requiredPackages, artifact.forbiddenPackages)

			path := filepath.Join(output, artifact.name)
			runGo(t, goTool, root, environment, "build", "-trimpath", "-buildvcs=false", "-o", path, artifact.target)
			assertPortableExecutable(t, path)
			information, err := buildinfo.ReadFile(path)
			if err != nil {
				t.Fatalf("read buildinfo: %v", err)
			}
			assertBuildInfo(t, artifact, information)
			assertSymbols(t, goTool, environment, path, artifact.requiredSymbols, artifact.forbiddenSymbols)
			t.Logf("%s sha256=%s", artifact.name, fileSHA256(t, path))
		})
	}

	entries, err := os.ReadDir(output)
	if err != nil {
		t.Fatal(err)
	}
	gotArtifacts := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("artifact directory contains unexpected subdirectory %q", entry.Name())
		}
		gotArtifacts = append(gotArtifacts, entry.Name())
	}
	sort.Strings(gotArtifacts)
	wantArtifacts := []string{"mindweaver-pdf.exe", "mindweaver.exe"}
	if strings.Join(gotArtifacts, "\n") != strings.Join(wantArtifacts, "\n") {
		t.Fatalf("built artifact exact-set:\n got %q\nwant %q", gotArtifacts, wantArtifacts)
	}
}

func commandMainPackages(t *testing.T, goTool, root string, environment []string) []string {
	t.Helper()
	output := runGo(t, goTool, root, environment, "list", "-f={{if eq .Name \"main\"}}{{.ImportPath}}{{end}}", "./cmd/...")
	commands := nonEmptyLines(output)
	sort.Strings(commands)
	return commands
}

func packageGraph(t *testing.T, goTool, root string, environment []string, target string) map[string]bool {
	t.Helper()
	output := runGo(t, goTool, root, environment, "list", "-deps", "-f={{.ImportPath}}", target)
	graph := make(map[string]bool)
	for _, imported := range nonEmptyLines(output) {
		graph[imported] = true
	}
	return graph
}

func assertGraph(t *testing.T, name string, graph map[string]bool, required, forbidden []string) {
	t.Helper()
	for _, imported := range required {
		if !graph[imported] {
			t.Errorf("%s package graph is missing %s", name, imported)
		}
	}
	for _, imported := range forbidden {
		if graph[imported] {
			t.Errorf("%s package graph contains forbidden %s", name, imported)
		}
	}
}

func assertBuildInfo(t *testing.T, artifact artifactContract, information *buildinfo.BuildInfo) {
	t.Helper()
	if information.Main.Path != modulePath {
		t.Errorf("main module = %q, want %q", information.Main.Path, modulePath)
	}
	settings := make(map[string]string)
	for _, setting := range information.Settings {
		settings[setting.Key] = setting.Value
		if strings.HasPrefix(setting.Key, "vcs") {
			t.Errorf("buildinfo contains forbidden VCS setting %q", setting.Key)
		}
	}
	for key, want := range map[string]string{
		"GOOS": "windows", "GOARCH": "amd64", "CGO_ENABLED": "0", "-trimpath": "true",
	} {
		if settings[key] != want {
			t.Errorf("build setting %s = %q, want %q", key, settings[key], want)
		}
	}
	gotModules := make([]string, 0, len(information.Deps))
	for _, dependency := range information.Deps {
		if dependency.Replace != nil {
			t.Fatalf("module replacement retained for %s", dependency.Path)
		}
		gotModules = append(gotModules, dependency.Path+"@"+dependency.Version+"#"+dependency.Sum)
	}
	sort.Strings(gotModules)
	wantModules := append([]string(nil), artifact.modules...)
	sort.Strings(wantModules)
	if strings.Join(gotModules, "\n") != strings.Join(wantModules, "\n") {
		t.Errorf("module closure:\n got %q\nwant %q", gotModules, wantModules)
	}
}

func assertPortableExecutable(t *testing.T, filename string) {
	t.Helper()
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var magic [2]byte
	if _, err := io.ReadFull(file, magic[:]); err != nil {
		t.Fatal(err)
	}
	if string(magic[:]) != "MZ" {
		t.Fatalf("artifact magic = %q, want Windows PE MZ", magic)
	}
}

func assertSymbols(t *testing.T, goTool string, environment []string, filename string, required, forbidden []string) {
	t.Helper()
	command := exec.CommandContext(t.Context(), goTool, "tool", "nm", filename)
	command.Env = environment
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics strings.Builder
	command.Stderr = &diagnostics
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	found := make(map[string]bool, len(required))
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		for _, symbol := range required {
			if strings.Contains(line, symbol) {
				found[symbol] = true
			}
		}
		for _, symbol := range forbidden {
			if strings.Contains(line, symbol) {
				t.Errorf("PE contains forbidden symbol namespace %q", symbol)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("go tool nm: %v: %s", err, diagnostics.String())
	}
	for _, symbol := range required {
		if !found[symbol] {
			t.Errorf("PE is missing required linked symbol %q", symbol)
		}
	}
}

func fileSHA256(t *testing.T, filename string) string {
	t.Helper()
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func runGo(t *testing.T, goTool, root string, environment []string, arguments ...string) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), goTool, arguments...)
	command.Dir = root
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}

func nonEmptyLines(value string) []string {
	var result []string
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(working, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	return root
}

func goTool(t *testing.T) string {
	t.Helper()
	configured := strings.TrimSpace(os.Getenv("MW_GO"))
	if configured == "" {
		var err error
		configured, err = exec.LookPath("go")
		if err != nil {
			t.Fatal("Go tool is required for the dual-PE qualification gate")
		}
	}
	absolute, err := filepath.Abs(configured)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

func offlineWindowsAMD64Environment() []string {
	overrides := map[string]string{
		"CGO_ENABLED": "0",
		"GOARCH":      "amd64",
		"GOFLAGS":     "-mod=readonly -buildvcs=false",
		"GOOS":        "windows",
		"GOPROXY":     "off",
		"GOSUMDB":     "off",
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
	}
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, replaced := overrides[strings.ToUpper(key)]; replaced {
				continue
			}
		}
		environment = append(environment, entry)
	}
	for key, value := range overrides {
		environment = append(environment, fmt.Sprintf("%s=%s", key, value))
	}
	return environment
}
