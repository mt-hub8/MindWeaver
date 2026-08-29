package client

import (
	"bufio"
	"bytes"
	"debug/buildinfo"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/mt-hub8/MindWeaver/v2"

var expectedWindowsModules = map[string][]string{
	"mindweaver.exe": {
		"github.com/ncruces/go-sqlite3@v0.35.3",
		"github.com/ncruces/go-sqlite3-wasm/v3@v3.2.35304",
		"github.com/ncruces/julianday@v1.0.0",
		"golang.org/x/sys@v0.47.0",
	},
	"mindweaver-pdf.exe": {
		"github.com/mgilbir/formalis@v0.3.1",
		"github.com/mgilbir/golittlecms@v0.0.0-20260727161601-f6af7cfe1556",
		"github.com/mgilbir/gopenjpeg@v0.0.0-20260727163526-8a139bc479b2",
		"github.com/mgilbir/pdf0@v0.1.0",
	},
}

var expectedWindowsModuleSums = map[string]string{
	"github.com/mgilbir/formalis@v0.3.1":                                "h1:NyYe/EcRYJ2jUjgaZG98lNXgJ7H+jgy6mq7HXOnQxl8=",
	"github.com/mgilbir/golittlecms@v0.0.0-20260727161601-f6af7cfe1556": "h1:2ZUsOgMhxpHCYC8jyzeEnJZFLYGbXhqjAJWJBvY8q4U=",
	"github.com/mgilbir/gopenjpeg@v0.0.0-20260727163526-8a139bc479b2":   "h1:kdDIM4JNxn9gsRk5Zo6mtmcFpBqnl9gTVUwf9t6lIRk=",
	"github.com/mgilbir/pdf0@v0.1.0":                                    "h1:rfBK18bcQ4kHQTXBmriAb07TafhG2w1fLflq9lHgaG4=",
	"github.com/ncruces/go-sqlite3-wasm/v3@v3.2.35304":                  "h1:5NoQAewtgKNK3G4bjNPxVoGXu6F6NzLXWCTdD5FFAEY=",
	"github.com/ncruces/go-sqlite3@v0.35.3":                             "h1:Ei07Zv1qfV/vyXzelhFsyS5Oh9TArBZHsmFk14Xv3GY=",
	"github.com/ncruces/julianday@v1.0.0":                               "h1:fH0OKwa7NWvniGQtxdJRxAgkBMolni2BjDHaWTxqt7M=",
	"golang.org/x/sys@v0.47.0":                                          "h1:o7XGOvZQCADBQQ4Y7VNq2dRWQR7JmOUW8Kxx4ZsNgWs=",
}

func TestPDFExtractionDirectImportAllowlist(t *testing.T) {
	root := moduleRoot(t)
	allowed := map[string]map[string]bool{
		"github.com/mgilbir/pdf0": {
			"internal/pdfextract/parser/bounded_content.go":  true,
			"internal/pdfextract/parser/bounded_document.go": true,
		},
		"golang.org/x/sys/windows": {
			"internal/pdfextract/client/command_windows.go": true,
		},
		modulePath + "/internal/pdfextract/parser": {
			"cmd/mindweaver-pdf/main.go": true,
		},
	}
	seen := make(map[string]map[string]bool)
	for dependency := range allowed {
		seen[dependency] = make(map[string]bool)
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			name, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if name == "golang.org/x/sys/windows" && !strings.HasPrefix(relative, "internal/pdfextract/") {
				continue
			}
			files, controlled := allowed[name]
			if !controlled {
				continue
			}
			if !files[relative] {
				return fmt.Errorf("%s imports %s outside its allowlist", relative, name)
			}
			seen[name][relative] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for dependency, files := range allowed {
		for file := range files {
			if !seen[dependency][file] {
				t.Errorf("required dependency edge missing: %s -> %s", file, dependency)
			}
		}
	}

	entries, err := os.ReadDir(filepath.Join(root, "internal", "pdfextract"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			t.Errorf("root pdfextract facade is forbidden: internal/pdfextract/%s", entry.Name())
		}
	}
}

func TestWindowsAMD64PDFDependencyGraph(t *testing.T) {
	root := moduleRoot(t)
	goTool := nestedGoTool(t)
	environment := hermeticBuildEnvironment("windows", "amd64")

	protocolGraph := listDependencyGraph(t, goTool, root, environment, "./internal/pdfextract/protocol")
	assertPackageBoundary(t, protocolGraph,
		[]string{modulePath + "/internal/pdfextract/protocol"},
		nil,
	)
	clientGraph := listDependencyGraph(t, goTool, root, environment, "./internal/pdfextract/client")
	assertPackageBoundary(t, clientGraph,
		[]string{modulePath + "/internal/pdfextract/client", modulePath + "/internal/pdfextract/protocol"},
		[]string{"golang.org/x/sys"},
	)
	assertGraphContains(t, clientGraph, "os/exec", "golang.org/x/sys/windows")
	assertGraphExcludes(t, clientGraph, modulePath+"/internal/pdfextract/parser", "github.com/mgilbir/pdf0")
	parserGraph := listDependencyGraph(t, goTool, root, environment, "./internal/pdfextract/parser")
	assertPackageBoundary(t, parserGraph,
		[]string{modulePath + "/internal/pdfextract/parser", modulePath + "/internal/pdfextract/protocol"},
		[]string{
			"github.com/mgilbir/formalis",
			"github.com/mgilbir/golittlecms",
			"github.com/mgilbir/gopenjpeg",
			"github.com/mgilbir/pdf0",
		},
	)
	assertGraphExcludes(t, parserGraph, modulePath+"/internal/pdfextract/client", "net/http", "os/exec", "golang.org/x/sys/windows")

	mainGraph := listDependencyGraph(t, goTool, root, environment, "./cmd/mindweaver")
	assertGraphContains(t, mainGraph,
		modulePath+"/internal/pdfextract/client",
		modulePath+"/internal/pdfextract/protocol",
	)
	assertGraphExcludes(t, mainGraph,
		modulePath+"/internal/pdfextract/parser",
		"github.com/mgilbir/pdf0",
	)

	helperGraph := listDependencyGraph(t, goTool, root, environment, "./cmd/mindweaver-pdf")
	assertGraphContains(t, helperGraph,
		modulePath+"/internal/pdfextract/parser",
		modulePath+"/internal/pdfextract/protocol",
		"github.com/mgilbir/pdf0",
	)
	assertGraphExcludes(t, helperGraph,
		modulePath+"/internal/pdfextract/client",
		modulePath+"/qualification/pdf",
		"net/http",
		"os/exec",
		"golang.org/x/sys/windows",
	)
	for imported := range helperGraph {
		if strings.HasPrefix(imported, "golang.org/x/sys/") {
			t.Errorf("helper graph contains forbidden x/sys package %s", imported)
		}
	}
	assertNoHelperRuntimeEscape(t, helperGraph)

	temporary := t.TempDir()
	artifacts := []struct {
		name   string
		target string
	}{
		{name: "mindweaver.exe", target: "./cmd/mindweaver"},
		{name: "mindweaver-pdf.exe", target: "./cmd/mindweaver-pdf"},
	}
	for _, artifact := range artifacts {
		t.Run(artifact.name, func(t *testing.T) {
			path := filepath.Join(temporary, artifact.name)
			command := exec.Command(goTool, "build", "-trimpath", "-buildvcs=false", "-o", path, artifact.target)
			command.Dir = root
			command.Env = environment
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("build %s: %v\n%s", artifact.name, err, output)
			}
			information, err := buildinfo.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s buildinfo: %v", artifact.name, err)
			}
			if information.Main.Path != modulePath {
				t.Fatalf("%s main module = %q", artifact.name, information.Main.Path)
			}
			assertBuildSettings(t, artifact.name, information)
			got := make([]string, 0, len(information.Deps))
			for _, dependency := range information.Deps {
				if dependency.Replace != nil {
					t.Fatalf("%s contains module replacement for %s", artifact.name, dependency.Path)
				}
				key := dependency.Path + "@" + dependency.Version
				if dependency.Sum != expectedWindowsModuleSums[key] {
					t.Fatalf("%s module sum for %s = %q", artifact.name, key, dependency.Sum)
				}
				got = append(got, key)
			}
			sort.Strings(got)
			want := append([]string(nil), expectedWindowsModules[artifact.name]...)
			sort.Strings(want)
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("%s module closure:\n got %q\nwant %q", artifact.name, got, want)
			}
			t.Logf("%s module closure: %s", artifact.name, strings.Join(got, ", "))
			if artifact.name == "mindweaver-pdf.exe" {
				assertNoHelperFilesystemSymbols(t, goTool, path, environment)
			}
		})
	}
}

type listedPackage struct {
	ImportPath string
	Standard   bool
	Dir        string
	GoFiles    []string
	Module     *struct {
		Path string
	}
}

func assertNoHelperRuntimeEscape(t *testing.T, graph map[string]listedPackage) {
	t.Helper()
	for imported, record := range graph {
		if record.Standard || record.Module == nil || record.Dir == "" {
			continue
		}
		for _, name := range record.GoFiles {
			filename := filepath.Join(record.Dir, name)
			parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
			if err != nil {
				t.Fatalf("parse helper source %s: %v", filename, err)
			}
			aliases := make(map[string]string)
			for _, declaration := range parsed.Imports {
				dependency, err := strconv.Unquote(declaration.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if dependency == "net" || dependency == "net/http" || dependency == "os/exec" || dependency == "syscall" {
					t.Errorf("helper package %s directly imports forbidden runtime capability %s", imported, dependency)
				}
				alias := path.Base(dependency)
				if declaration.Name != nil {
					alias = declaration.Name.Name
				}
				aliases[alias] = dependency
			}
			ast.Inspect(parsed, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				identifier, ok := selector.X.(*ast.Ident)
				if !ok {
					return true
				}
				switch aliases[identifier.Name] {
				case "os":
					switch selector.Sel.Name {
					case "CreateTemp", "MkdirTemp", "TempDir":
						t.Errorf("helper package %s uses forbidden temporary-path API os.%s", imported, selector.Sel.Name)
					}
				case "io/ioutil":
					switch selector.Sel.Name {
					case "TempDir", "TempFile":
						t.Errorf("helper package %s uses forbidden temporary-path API ioutil.%s", imported, selector.Sel.Name)
					}
				}
				return true
			})
		}
	}
}

func assertNoHelperFilesystemSymbols(t *testing.T, goTool, executable string, environment []string) {
	t.Helper()
	command := exec.Command(goTool, "tool", "nm", executable)
	command.Env = environment
	output, err := command.Output()
	if err != nil {
		t.Fatalf("inspect helper symbols: %v", err)
	}
	for _, forbidden := range []string{
		"github.com/mgilbir/golittlecms.(*Context).CreateDeviceLinkFromCubeFile",
		"github.com/mgilbir/golittlecms.(*Context).OpenIOhandlerFromFile",
		"github.com/mgilbir/golittlecms.(*Context).OpenProfileFromFile",
		"github.com/mgilbir/golittlecms.(*IT8).SaveToFile",
		"github.com/mgilbir/golittlecms.(*Profile).SaveProfileToFile",
		"github.com/mgilbir/golittlecms.CreateDeviceLinkFromCubeFile",
		"github.com/mgilbir/golittlecms.IT8LoadFromFile",
		"github.com/mgilbir/golittlecms.OpenProfileFromFile",
		"github.com/mgilbir/golittlecms.removeFile",
		"os.Create",
		"os.Remove",
		"os.WriteFile",
	} {
		if bytes.Contains(output, []byte(forbidden)) {
			t.Errorf("helper PE retains forbidden filesystem symbol %s", forbidden)
		}
	}
}

func listDependencyGraph(t *testing.T, goTool, root string, environment []string, target string) map[string]listedPackage {
	t.Helper()
	command := exec.Command(goTool, "list", "-deps", "-json", target)
	command.Dir = root
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
	graph := make(map[string]listedPackage)
	decoder := json.NewDecoder(bufio.NewReader(stdout))
	for {
		var record listedPackage
		if err := decoder.Decode(&record); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("decode go list for %s: %v", target, err)
		}
		graph[record.ImportPath] = record
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("go list %s: %v\n%s", target, err, diagnostics.String())
	}
	return graph
}

func assertGraphContains(t *testing.T, graph map[string]listedPackage, required ...string) {
	t.Helper()
	for _, imported := range required {
		if _, ok := graph[imported]; !ok {
			t.Errorf("dependency graph is missing %s", imported)
		}
	}
}

func assertGraphExcludes(t *testing.T, graph map[string]listedPackage, forbidden ...string) {
	t.Helper()
	for _, imported := range forbidden {
		if _, ok := graph[imported]; ok {
			t.Errorf("dependency graph contains forbidden package %s", imported)
		}
	}
}

func assertPackageBoundary(t *testing.T, graph map[string]listedPackage, firstParty, externalModules []string) {
	t.Helper()
	allowedFirstParty := make(map[string]bool, len(firstParty))
	for _, imported := range firstParty {
		allowedFirstParty[imported] = true
	}
	allowedExternal := make(map[string]bool, len(externalModules))
	for _, module := range externalModules {
		allowedExternal[module] = true
	}
	for imported, record := range graph {
		if record.Standard || record.Module == nil {
			continue
		}
		switch record.Module.Path {
		case modulePath:
			if !allowedFirstParty[imported] {
				t.Errorf("package boundary contains forbidden first-party dependency %s", imported)
			}
		default:
			if !allowedExternal[record.Module.Path] {
				t.Errorf("package boundary contains forbidden external module %s through %s", record.Module.Path, imported)
			}
		}
	}
}

func assertBuildSettings(t *testing.T, name string, information *buildinfo.BuildInfo) {
	t.Helper()
	want := map[string]string{"GOOS": "windows", "GOARCH": "amd64", "CGO_ENABLED": "0", "-trimpath": "true"}
	got := make(map[string]string)
	for _, setting := range information.Settings {
		got[setting.Key] = setting.Value
		if strings.HasPrefix(setting.Key, "vcs") {
			t.Errorf("%s contains forbidden VCS build setting %s", name, setting.Key)
		}
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s build setting %s = %q, want %q", name, key, got[key], value)
		}
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(working, "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	return root
}

func nestedGoTool(t *testing.T) string {
	t.Helper()
	if configured := strings.TrimSpace(os.Getenv("MW_GO")); configured != "" {
		absolute, err := filepath.Abs(configured)
		if err != nil {
			t.Fatal(err)
		}
		return absolute
	}
	path, err := exec.LookPath("go")
	if err != nil {
		t.Fatal("Go tool is required for the dependency graph gate")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

func hermeticBuildEnvironment(goos, goarch string) []string {
	overrides := map[string]string{
		"CGO_ENABLED":  "0",
		"GOARCH":       goarch,
		"GOAMD64":      "v1",
		"GOENV":        "off",
		"GOEXPERIMENT": "",
		"GOFIPS140":    "off",
		"GOFLAGS":      "-mod=readonly -buildvcs=false",
		"GOOS":         goos,
		"GOTOOLCHAIN":  "local",
		"GOTELEMETRY":  "off",
		"GOWORK":       "off",
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
		environment = append(environment, key+"="+value)
	}
	return environment
}
