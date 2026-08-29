package moduleintegrity

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

var expectedModuleSums = map[string]string{
	"github.com/mgilbir/formalis v0.3.1":                                       "h1:NyYe/EcRYJ2jUjgaZG98lNXgJ7H+jgy6mq7HXOnQxl8=",
	"github.com/mgilbir/formalis v0.3.1/go.mod":                                "h1:M82rAk4SewJbe1B3bhgQKw0w7ojFOHus7Nm1roqZYEk=",
	"github.com/mgilbir/golittlecms v0.0.0-20260727161601-f6af7cfe1556":        "h1:2ZUsOgMhxpHCYC8jyzeEnJZFLYGbXhqjAJWJBvY8q4U=",
	"github.com/mgilbir/golittlecms v0.0.0-20260727161601-f6af7cfe1556/go.mod": "h1:8SmvcTVpsTeqGaLqnhlCphGYaBeoeedJQB6AsKNNQxo=",
	"github.com/mgilbir/gopenjpeg v0.0.0-20260727163526-8a139bc479b2":          "h1:kdDIM4JNxn9gsRk5Zo6mtmcFpBqnl9gTVUwf9t6lIRk=",
	"github.com/mgilbir/gopenjpeg v0.0.0-20260727163526-8a139bc479b2/go.mod":   "h1:J72BFvnlK6fgFlVEtFycG+kbNYLX04tjzVI6eVmDAho=",
	"github.com/mgilbir/pdf0 v0.1.0":                                           "h1:rfBK18bcQ4kHQTXBmriAb07TafhG2w1fLflq9lHgaG4=",
	"github.com/mgilbir/pdf0 v0.1.0/go.mod":                                    "h1:ePtVea4gtSqYy2oNb3xJUwdmQD+5n8GwOwah6nJ9GMw=",
	"github.com/ncruces/go-sqlite3 v0.35.3":                                    "h1:Ei07Zv1qfV/vyXzelhFsyS5Oh9TArBZHsmFk14Xv3GY=",
	"github.com/ncruces/go-sqlite3 v0.35.3/go.mod":                             "h1:i1rhym/NIiB5xeEfzbN+e24Y+i7NGUpf7C2xZ3Dpwks=",
	"github.com/ncruces/go-sqlite3-wasm/v3 v3.2.35304":                         "h1:5NoQAewtgKNK3G4bjNPxVoGXu6F6NzLXWCTdD5FFAEY=",
	"github.com/ncruces/go-sqlite3-wasm/v3 v3.2.35304/go.mod":                  "h1:o8gr9w/50fXA5TDskg6bNUjvqmFfw4KaXth4q+yDSjg=",
	"github.com/ncruces/julianday v1.0.0":                                      "h1:fH0OKwa7NWvniGQtxdJRxAgkBMolni2BjDHaWTxqt7M=",
	"github.com/ncruces/julianday v1.0.0/go.mod":                               "h1:Dusn2KvZrrovOMJuOt0TNXL6tB7U2E8kvza5fFc9G7g=",
	"golang.org/x/sys v0.47.0":                                                 "h1:o7XGOvZQCADBQQ4Y7VNq2dRWQR7JmOUW8Kxx4ZsNgWs=",
	"golang.org/x/sys v0.47.0/go.mod":                                          "h1:4GL1E5IUh+htKOUEOaiffhrAeqysfVGipDYzABqnCmw=",
	"golang.org/x/text v0.40.0":                                                "h1:Ub2Z6/xjgF1WrYQz2nuITOEegKFtiIy+rieRJ5lHZKs=",
	"golang.org/x/text v0.40.0/go.mod":                                         "h1:hpnzDAfGV753zIKo+wk3u1bVKCGPbrnF7+7LBF/UHVY=",
}

func TestReadonlyModuleBuildContract(t *testing.T) {
	root := moduleRoot(t)
	if _, err := os.Lstat(filepath.Join(root, "vendor")); err == nil {
		t.Fatal("checked-in vendor tree is forbidden")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal("inspect forbidden vendor tree")
	}
	assertGoModulePolicy(t, root)
	assertGoSumContract(t, filepath.Join(root, "go.sum"))
	assertWorkflowExactSets(t, root)
	assertBuildPolicyFiles(t, root)
	verifyDownloadedModules(t, root)
}

func TestBuildPolicyRejectsVendoredOrNetworkDisabledConfiguration(t *testing.T) {
	for _, mutant := range []string{
		"GOFLAGS=-mod=vendor",
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOVCS=*:off",
		"go mod vendor",
		"vendor/** -text -whitespace",
	} {
		if err := validateReadonlyBuildPolicy([]byte(mutant)); err == nil {
			t.Fatalf("build policy accepted %q", mutant)
		}
	}
	for _, valid := range []string{
		"GOFLAGS=-mod=readonly -trimpath -buildvcs=false\ngo mod download\ngo mod verify\n",
		"cache: true\ncache-dependency-path: go.sum\n",
	} {
		if err := validateReadonlyBuildPolicy([]byte(valid)); err != nil {
			t.Fatalf("build policy rejected readonly module configuration: %v", err)
		}
	}
}

func assertGoModulePolicy(t *testing.T, root string) {
	t.Helper()
	contents := readBounded(t, filepath.Join(root, "go.mod"), 64<<10)
	text := string(contents)
	for _, required := range []string{
		"module github.com/mt-hub8/MindWeaver/v2\n",
		"\ngo 1.26\n",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("go.mod lacks %q", required)
		}
	}
	for _, forbidden := range []string{"\nreplace ", "\nexclude ", "\ntoolchain "} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("go.mod contains forbidden directive %q", strings.TrimSpace(forbidden))
		}
	}
}

func assertGoSumContract(t *testing.T, filename string) {
	t.Helper()
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	actual := make(map[string]string)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 64<<10)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 || !strings.HasPrefix(fields[2], "h1:") {
			t.Fatalf("invalid go.sum line %q", scanner.Text())
		}
		key := fields[0] + " " + fields[1]
		if _, duplicate := actual[key]; duplicate {
			t.Fatalf("duplicate go.sum key %s", key)
		}
		actual[key] = fields[2]
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(actual) != len(expectedModuleSums) {
		t.Fatalf("go.sum entries = %d, want %d", len(actual), len(expectedModuleSums))
	}
	for key, want := range expectedModuleSums {
		if actual[key] != want {
			t.Fatalf("go.sum %s = %q, want %q", key, actual[key], want)
		}
	}
}

func assertWorkflowExactSets(t *testing.T, root string) {
	t.Helper()
	repositoryRoot := filepath.Dir(root)
	contracts := []struct {
		directory string
		want      []string
	}{{filepath.Join(root, ".github", "workflows"), []string{"ci.yml"}}}
	if _, err := os.Stat(filepath.Join(repositoryRoot, "v2", "go.mod")); err == nil {
		contracts = append(contracts, struct {
			directory string
			want      []string
		}{filepath.Join(repositoryRoot, ".github", "workflows"), []string{"go-ci.yml"}})
	}
	for _, contract := range contracts {
		entries, err := os.ReadDir(contract.directory)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, entry := range entries {
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				t.Fatalf("workflow directory contains non-file %s", entry.Name())
			}
			got = append(got, entry.Name())
		}
		sort.Strings(got)
		if !slices.Equal(got, contract.want) {
			t.Fatalf("workflow exact set = %q, want %q", got, contract.want)
		}
	}
}

func assertBuildPolicyFiles(t *testing.T, root string) {
	t.Helper()
	repositoryRoot := filepath.Dir(root)
	contracts := []struct {
		path     string
		required []string
	}{
		{filepath.Join(root, ".github", "workflows", "ci.yml"), []string{"name: MindWeaver Go CI", "cache: true", "cache-dependency-path: go.sum", "go mod download", "go mod verify", "-mod=readonly"}},
		{filepath.Join(root, "scripts", "ci.ps1"), []string{"mod download", "mod verify", "-mod=readonly"}},
		{filepath.Join(root, "scripts", "ci.sh"), []string{"mod download", "mod verify", "-mod=readonly"}},
		{filepath.Join(root, "Makefile"), []string{"mod download", "mod verify", "-mod=readonly"}},
		{filepath.Join(root, ".gitattributes"), []string{"*.mod text eol=lf", "*.sum text eol=lf"}},
	}
	if _, err := os.Stat(filepath.Join(repositoryRoot, "v2", "go.mod")); err == nil {
		contracts = append(contracts, struct {
			path     string
			required []string
		}{filepath.Join(repositoryRoot, ".github", "workflows", "go-ci.yml"), []string{"name: MindWeaver Go CI", "cache: true", "cache-dependency-path: v2/go.sum", "go mod download", "go mod verify", "-mod=readonly"}})
	}
	for _, contract := range contracts {
		contents := readBounded(t, contract.path, 256<<10)
		if err := validateReadonlyBuildPolicy(contents); err != nil {
			t.Fatalf("%s: %v", contract.path, err)
		}
		for _, required := range contract.required {
			if !bytes.Contains(contents, []byte(required)) {
				t.Fatalf("%s lacks %q", contract.path, required)
			}
		}
	}
}

func validateReadonlyBuildPolicy(contents []byte) error {
	lower := strings.ToLower(string(contents))
	for _, forbidden := range []string{
		"-mod=vendor", "goproxy=off", "gosumdb=off", "govcs=*:off",
		"go mod vendor", "vendor/** -text -whitespace",
	} {
		if strings.Contains(lower, forbidden) {
			return errors.New("vendored or network-disabled module policy is forbidden")
		}
	}
	return nil
}

func verifyDownloadedModules(t *testing.T, root string) {
	t.Helper()
	goTool := strings.TrimSpace(os.Getenv("MW_GO"))
	if goTool == "" {
		name := "go"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		goTool = filepath.Join(runtime.GOROOT(), "bin", name)
	}
	for _, arguments := range [][]string{{"mod", "download"}, {"mod", "verify"}} {
		command := exec.CommandContext(t.Context(), goTool, arguments...)
		command.Dir = root
		command.Env = readonlyEnvironment(os.Environ())
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("go %s failed: %v bytes=%d", strings.Join(arguments, " "), err, len(output))
		}
	}
}

func readonlyEnvironment(base []string) []string {
	overrides := map[string]string{"GOFLAGS": "-mod=readonly -trimpath -buildvcs=false", "GOTOOLCHAIN": "local", "GOWORK": "off"}
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, replaced := overrides[strings.ToUpper(key)]; replaced {
				continue
			}
		}
		result = append(result, entry)
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

func readBounded(t *testing.T, filename string, limit int64) []byte {
	t.Helper()
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > limit {
		t.Fatalf("invalid bounded regular file %s", filename)
	}
	contents, err := os.ReadFile(filename)
	if err != nil || int64(len(contents)) != info.Size() || !utf8.Valid(contents) {
		t.Fatalf("read bounded UTF-8 file %s", filename)
	}
	return contents
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(working, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatal("resolve module root")
	}
	return root
}
