package offlinevendor

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

type moduleContract struct {
	path            string
	version         string
	h1              string
	licenseFiles    []licenseContract
	provenanceFiles []provenanceContract
}

type licenseContract struct {
	name   string
	size   int64
	sha256 string
	spdx   string
}

type provenanceContract struct {
	name      string
	size      int64
	sha256    string
	statement string
}

const (
	expectedVendorFiles             = 698
	expectedVendorTreeSHA256        = "f935ae79254d0fd1f5f32f4491f0b1f1e28e22fc088cef262bde2e860cf9c254"
	expectedRootWorkflowSHA256      = "d354148491a6add88cab10ef0df3105f512c039cc980698009948b724e706886"
	expectedExtractedWorkflowSHA256 = "d2439b66aeefd1c13849a2ff76b3feeb3fcccfd2ce899c1acc0f61472f02239b"
)

var vendoredPackages = []string{
	"github.com/mgilbir/formalis",
	"github.com/mgilbir/golittlecms",
	"github.com/mgilbir/gopenjpeg",
	"github.com/mgilbir/gopenjpeg/internal/bio",
	"github.com/mgilbir/gopenjpeg/internal/cio",
	"github.com/mgilbir/gopenjpeg/internal/cparams",
	"github.com/mgilbir/gopenjpeg/internal/dwt",
	"github.com/mgilbir/gopenjpeg/internal/event",
	"github.com/mgilbir/gopenjpeg/internal/ht",
	"github.com/mgilbir/gopenjpeg/internal/image",
	"github.com/mgilbir/gopenjpeg/internal/j2k",
	"github.com/mgilbir/gopenjpeg/internal/jp2",
	"github.com/mgilbir/gopenjpeg/internal/mct",
	"github.com/mgilbir/gopenjpeg/internal/mqc",
	"github.com/mgilbir/gopenjpeg/internal/opjmath",
	"github.com/mgilbir/gopenjpeg/internal/pi",
	"github.com/mgilbir/gopenjpeg/internal/sparse",
	"github.com/mgilbir/gopenjpeg/internal/t1",
	"github.com/mgilbir/gopenjpeg/internal/t2",
	"github.com/mgilbir/gopenjpeg/internal/tcd",
	"github.com/mgilbir/gopenjpeg/internal/tgt",
	"github.com/mgilbir/gopenjpeg/internal/tile",
	"github.com/mgilbir/pdf0",
	"github.com/ncruces/go-sqlite3",
	"github.com/ncruces/go-sqlite3/driver",
	"github.com/ncruces/go-sqlite3/ext/fts5",
	"github.com/ncruces/go-sqlite3/internal/dotlk",
	"github.com/ncruces/go-sqlite3/internal/errutil",
	"github.com/ncruces/go-sqlite3/internal/sqlite3_wrap",
	"github.com/ncruces/go-sqlite3/internal/testenv",
	"github.com/ncruces/go-sqlite3/internal/util",
	"github.com/ncruces/go-sqlite3/vfs",
	"github.com/ncruces/go-sqlite3-wasm/v3",
	"github.com/ncruces/go-sqlite3-wasm/v3/fts5",
	"github.com/ncruces/julianday",
	"golang.org/x/sys/unix",
	"golang.org/x/sys/windows",
}

var vendoredModules = []moduleContract{
	{
		path: "github.com/mgilbir/formalis", version: "v0.3.1",
		h1:           "h1:NyYe/EcRYJ2jUjgaZG98lNXgJ7H+jgy6mq7HXOnQxl8=",
		licenseFiles: []licenseContract{{"LICENSE", 1082, "4e9651455e1b761ed462c50f60c4618c8985f46404e8db467def14848e77725a", "MIT"}},
	},
	{
		path: "github.com/mgilbir/golittlecms", version: "v0.0.0-20260727161601-f6af7cfe1556",
		h1:           "h1:2ZUsOgMhxpHCYC8jyzeEnJZFLYGbXhqjAJWJBvY8q4U=",
		licenseFiles: []licenseContract{{"LICENSE", 1199, "4b0b89edd67872e0507e20e03032e4dc4eb194f88082f80acee13a13fb73317c", "MIT"}},
	},
	{
		path: "github.com/mgilbir/gopenjpeg", version: "v0.0.0-20260727163526-8a139bc479b2",
		h1:           "h1:kdDIM4JNxn9gsRk5Zo6mtmcFpBqnl9gTVUwf9t6lIRk=",
		licenseFiles: []licenseContract{{"LICENSE", 2167, "958dc940b3916ca8b4d373f24027e26e29623828f41205de09e9c680e5539f78", "BSD-2-Clause"}},
	},
	{
		path: "github.com/mgilbir/pdf0", version: "v0.1.0",
		h1:           "h1:rfBK18bcQ4kHQTXBmriAb07TafhG2w1fLflq9lHgaG4=",
		licenseFiles: []licenseContract{{"LICENSE", 1082, "4e9651455e1b761ed462c50f60c4618c8985f46404e8db467def14848e77725a", "MIT"}},
	},
	{
		path: "github.com/ncruces/go-sqlite3-wasm/v3", version: "v3.2.35304",
		h1: "h1:5NoQAewtgKNK3G4bjNPxVoGXu6F6NzLXWCTdD5FFAEY=",
		licenseFiles: []licenseContract{
			{"LICENSE", 918, "13219037ddf63dbbcf174bf59525d602df7a2e30083f63be566715c858fcb19e", "MIT-0"},
		},
		provenanceFiles: []provenanceContract{
			{"README.md", 435, "fb8084fccb5733ccc4af421ff026812da7852e91516bdf7c70d007eb46744383", "MACHINE_TRANSLATION_RETAINS_UPSTREAM_LICENSES"},
		},
	},
	{
		path: "github.com/ncruces/go-sqlite3", version: "v0.35.3",
		h1:           "h1:Ei07Zv1qfV/vyXzelhFsyS5Oh9TArBZHsmFk14Xv3GY=",
		licenseFiles: []licenseContract{{"LICENSE", 1068, "8929c89a593807bf0c042272adc7301376a661050876bd702ebdd36cd7c55d2b", "MIT"}},
	},
	{
		path: "github.com/ncruces/julianday", version: "v1.0.0",
		h1:           "h1:fH0OKwa7NWvniGQtxdJRxAgkBMolni2BjDHaWTxqt7M=",
		licenseFiles: []licenseContract{{"LICENSE", 1068, "38ae43959daf953a393a585b2988672cb65a5a541aca0d0be5e72595a0a16883", "MIT"}},
	},
	{
		path: "golang.org/x/sys", version: "v0.47.0",
		h1: "h1:o7XGOvZQCADBQQ4Y7VNq2dRWQR7JmOUW8Kxx4ZsNgWs=",
		licenseFiles: []licenseContract{
			{"LICENSE", 1453, "911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad", "BSD-3-Clause"},
			{"PATENTS", 1303, "96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc", "LicenseRef-Go-Patent-Grant"},
		},
	},
}

func TestVendoredModuleAndLicenseContract(t *testing.T) {
	root := moduleRoot(t)
	assertRepositoryAttributes(t, filepath.Join(root, ".gitattributes"))
	assertWorkflowContracts(t, root)
	assertOfflineEntrypointPolicies(t, root)
	assertToolchainAndNoReplace(t, filepath.Join(root, "go.mod"))
	assertVendoredModuleSet(t, filepath.Join(root, "vendor", "modules.txt"))
	assertModuleSums(t, filepath.Join(root, "go.sum"))
	assertVendoredLicenseSet(t, root)
	assertVendoredProvenanceBoundary(t, root)
	assertVendorTreeIdentity(t, filepath.Join(root, "vendor"))
}

func TestExtractedWorkflowContractIgnoresParentWorkflows(t *testing.T) {
	root := moduleRoot(t)
	parent := t.TempDir()
	extracted := filepath.Join(parent, "extracted")
	writeTestFile(t, filepath.Join(parent, ".github", "workflows", "evil.yml"), []byte("invalid parent input\n"))
	writeTestFile(t, filepath.Join(extracted, ".github", "workflows", "ci.yml"),
		readRegularFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), 1<<20))

	for _, declaredRoot := range []string{"", extracted} {
		if err := validateWorkflowContracts(extracted, declaredRoot, extracted); err != nil {
			t.Fatalf("extracted workflow contract with declared root %q depended on parent: %v", declaredRoot, err)
		}
	}
}

func TestMonorepoWorkflowContractRejectsExtraWorkflow(t *testing.T) {
	root := moduleRoot(t)
	repository := t.TempDir()
	extracted := filepath.Join(repository, "v2")
	writeTestFile(t, filepath.Join(extracted, ".github", "workflows", "ci.yml"),
		readRegularFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), 1<<20))
	writeTestFile(t, filepath.Join(repository, ".github", "workflows", "go-ci.yml"),
		[]byte("placeholder; exact-set rejection must happen before identity validation\n"))
	writeTestFile(t, filepath.Join(repository, ".github", "workflows", "java-ci.yml"), []byte("legacy workflow\n"))

	if err := validateWorkflowContracts(extracted, repository, repository); err == nil {
		t.Fatal("monorepo workflow contract accepted an extra Java workflow")
	}
}

func TestWorkflowIdentityRejectsDisabledGate(t *testing.T) {
	root := moduleRoot(t)
	repository := t.TempDir()
	contents := string(readRegularFile(t, filepath.Join(root, ".github", "workflows", "ci.yml"), 1<<20))
	contents = strings.Replace(contents,
		"      - name: Empty-cache vendored CI\n",
		"      - name: Empty-cache vendored CI\n        if: false\n", 1)
	writeTestFile(t, filepath.Join(repository, ".github", "workflows", "ci.yml"), []byte(contents))

	if err := validateWorkflowContracts(repository, repository, repository); err == nil {
		t.Fatal("workflow identity accepted a disabled product gate")
	}
}

func assertOfflineEntrypointPolicies(t *testing.T, root string) {
	t.Helper()
	ciPowerShell := string(readRegularFile(t, filepath.Join(root, "scripts", "ci.ps1"), 1<<20))
	for _, required := range []string{
		"$env:GOROOT = $mwGoRoot",
		"$mwSavedGoRoot = [Environment]::GetEnvironmentVariable('GOROOT', 'Process')",
		"$env:GO111MODULE = 'on'",
	} {
		if strings.Count(ciPowerShell, required) != 1 {
			t.Fatalf("ci.ps1 must contain exactly one %q", required)
		}
	}

	ciShell := string(readRegularFile(t, filepath.Join(root, "scripts", "ci.sh"), 1<<20))
	if strings.Contains(ciShell, "command -v gofmt") {
		t.Fatal("ci.sh must not fall back to an ambient gofmt")
	}
	for _, required := range []string{
		`mw_go_root=$(GOROOT= "$mw_go" env GOROOT)`,
		`export GOROOT="$mw_go_root"`,
		"export GO111MODULE=on",
	} {
		if strings.Count(ciShell, required) != 1 {
			t.Fatalf("ci.sh must contain exactly one %q", required)
		}
	}

	browser := string(readRegularFile(t, filepath.Join(root, "scripts", "test-browser.ps1"), 1<<20))
	for _, required := range []string{
		"$sourceRelative -ceq '.'",
		"$sourceRelative -ceq 'v2'",
		`$env:GOFLAGS = "-mod=vendor -trimpath -buildvcs=false"`,
		`$env:GOMODCACHE = $moduleCache`,
		`$env:GOCACHE = $buildCache`,
		`$env:GOTMPDIR = $goTemp`,
		`$env:GOROOT = $goRoot`,
		`$env:GOVCS = "*:off"`,
	} {
		if strings.Count(browser, required) != 1 {
			t.Fatalf("test-browser.ps1 must contain exactly one %q", required)
		}
	}
	if strings.Contains(ciPowerShell+ciShell, "spikes/sqlite") {
		t.Fatal("SQLite spike must not be a CUT-002 delivery entry point")
	}
}

func TestVendorWhitespacePolicyDoesNotMaskFirstPartyErrors(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("Git is required by the tracked-only extraction gate")
	}
	root := moduleRoot(t)
	repository := t.TempDir()
	attributes := readRegularFile(t, filepath.Join(root, ".gitattributes"), 1<<20)
	writeTestFile(t, filepath.Join(repository, ".gitattributes"), attributes)
	writeTestFile(t, filepath.Join(repository, "firstparty.txt"), []byte("clean\n"))
	writeTestFile(t, filepath.Join(repository, "vendor", "upstream.txt"), []byte("clean\n"))
	runGit(t, git, repository, true, "init", "--quiet")
	runGit(t, git, repository, true, "add", ".gitattributes", "firstparty.txt", "vendor/upstream.txt")

	writeTestFile(t, filepath.Join(repository, "vendor", "upstream.txt"), []byte("upstream trailing space \n"))
	output := runGit(t, git, repository, true, "diff", "--check", "--", "vendor/upstream.txt")
	if len(output) != 0 {
		t.Fatalf("canonical vendor whitespace produced diagnostics: %q", output)
	}

	writeTestFile(t, filepath.Join(repository, "firstparty.txt"), []byte("first-party trailing space \n"))
	output = runGit(t, git, repository, false, "diff", "--check", "--", "firstparty.txt")
	if !strings.Contains(string(output), "firstparty.txt") ||
		!strings.Contains(string(output), "trailing whitespace") ||
		strings.Contains(string(output), "vendor/") {
		t.Fatalf("first-party whitespace diagnostic = %q", output)
	}
}

func runGit(t *testing.T, git, directory string, wantSuccess bool, arguments ...string) []byte {
	t.Helper()
	command := exec.Command(git, arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+filepath.Join(directory, "missing-global-config"),
		"GIT_CONFIG_COUNT=0",
		"GIT_CONFIG_PARAMETERS=",
	)
	output, err := command.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("git %v failed: %v: %s", arguments, err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("git %v unexpectedly succeeded: %s", arguments, output)
	}
	return output
}

func writeTestFile(t *testing.T, filename string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, contents, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertRepositoryAttributes(t *testing.T, filename string) {
	t.Helper()
	contents := readRegularFile(t, filename, 1<<20)
	required := map[string]int{
		"*.sql text eol=lf":           0,
		"*.json text eol=lf":          0,
		"*.css text eol=lf":           0,
		"*.js text eol=lf":            0,
		"*.html text eol=lf":          0,
		"vendor/** -text -whitespace": 0,
		"third_party/sqlite/3.53.4/LICENSE.md -text -whitespace": 0,
		"third_party/sqlite/3.53.4/manifest.uuid text eol=lf":    0,
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(contents), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if _, ok := required[line]; ok {
			required[line]++
		}
	}
	for line, count := range required {
		if count != 1 {
			t.Fatalf(".gitattributes must contain exactly one %q rule; got %d", line, count)
		}
	}
}

func assertWorkflowContracts(t *testing.T, moduleRoot string) {
	t.Helper()
	declaredRoot := strings.TrimSpace(os.Getenv("MW_CI_REPOSITORY_ROOT"))
	gitRoot := repositoryTopLevel(t, moduleRoot)
	if err := validateWorkflowContracts(moduleRoot, declaredRoot, gitRoot); err != nil {
		t.Fatal(err)
	}
}

func validateWorkflowContracts(moduleRoot, declaredRoot, gitRoot string) error {
	if err := validateWorkflowDirectory(
		filepath.Join(moduleRoot, ".github", "workflows"),
		[]string{"ci.yml"}, expectedExtractedWorkflowSHA256,
	); err != nil {
		return fmt.Errorf("extracted workflow contract: %w", err)
	}
	if gitRoot == "" {
		if declaredRoot != "" {
			return fmt.Errorf("declared repository root %q has no local Git boundary", declaredRoot)
		}
		return nil
	}
	var err error
	gitRoot, err = filepath.Abs(gitRoot)
	if err != nil {
		return fmt.Errorf("resolve Git repository root: %w", err)
	}
	moduleRoot, err = filepath.Abs(moduleRoot)
	if err != nil {
		return fmt.Errorf("resolve module root: %w", err)
	}
	if declaredRoot != "" {
		declaredRoot, err = filepath.Abs(declaredRoot)
		if err != nil {
			return fmt.Errorf("resolve declared repository root: %w", err)
		}
		if !samePath(declaredRoot, gitRoot) {
			return fmt.Errorf("declared repository root %q is not verified Git top-level %q", declaredRoot, gitRoot)
		}
	}
	if samePath(moduleRoot, gitRoot) {
		return nil
	}
	if !samePath(moduleRoot, filepath.Join(gitRoot, "v2")) {
		return fmt.Errorf("module root %q is neither declared repository root nor its v2 tree", moduleRoot)
	}
	if err := validateWorkflowDirectory(
		filepath.Join(gitRoot, ".github", "workflows"),
		[]string{"go-ci.yml"}, expectedRootWorkflowSHA256,
	); err != nil {
		return fmt.Errorf("monorepo workflow contract: %w", err)
	}
	return nil
}

func validateWorkflowDirectory(workflowRoot string, wantNames []string, wantSHA256 string) error {
	entries, err := os.ReadDir(workflowRoot)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("workflow entry is not a regular file: %s", entry.Name())
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if !slices.Equal(names, wantNames) {
		return fmt.Errorf("workflow exact-set = %q, want %q", names, wantNames)
	}
	filename := filepath.Join(workflowRoot, wantNames[0])
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 1<<20 {
		return fmt.Errorf("workflow must be a bounded regular non-link file: %v", err)
	}
	contents, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	normalized := strings.ReplaceAll(string(contents), "\r\n", "\n")
	digest := sha256.Sum256([]byte(normalized))
	gotSHA256 := hex.EncodeToString(digest[:])
	if gotSHA256 != wantSHA256 {
		return fmt.Errorf("workflow SHA-256 = %s, want %s", gotSHA256, wantSHA256)
	}
	return nil
}

func repositoryTopLevel(t *testing.T, moduleRoot string) string {
	t.Helper()
	candidate := ""
	if hasLocalGitBoundary(t, moduleRoot) {
		candidate = moduleRoot
	} else {
		parent := filepath.Dir(moduleRoot)
		if filepath.Base(moduleRoot) == "v2" && hasLocalGitBoundary(t, parent) {
			candidate = parent
		}
	}
	if candidate == "" {
		return ""
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("local repository boundary requires Git")
	}
	command := exec.Command(git, "-C", candidate, "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("verify local repository root: %v", err)
	}
	top := strings.TrimSpace(string(output))
	if !samePath(candidate, top) {
		t.Fatalf("local Git boundary %q resolved to unexpected top-level %q", candidate, top)
	}
	return top
}

func hasLocalGitBoundary(t *testing.T, root string) bool {
	t.Helper()
	info, err := os.Lstat(filepath.Join(root, ".git"))
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		t.Fatalf("inspect local Git boundary: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.Mode().IsDir() && !info.Mode().IsRegular()) {
		t.Fatalf("local Git boundary is neither a regular file nor directory: %s", root)
	}
	return true
}

func samePath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func assertToolchainAndNoReplace(t *testing.T, filename string) {
	t.Helper()
	contents := readRegularFile(t, filename, 1<<20)
	var goDirective string
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	for scanner.Scan() {
		fields := strings.Fields(strings.TrimSpace(strings.SplitN(scanner.Text(), "//", 2)[0]))
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "go":
			if len(fields) != 2 || goDirective != "" {
				t.Fatalf("go.mod has an ambiguous go directive")
			}
			goDirective = fields[1]
		case "toolchain":
			t.Fatalf("go.mod must not select an alternate toolchain; CI pins GOTOOLCHAIN=local")
		case "replace":
			t.Fatalf("go.mod replacements are forbidden by the offline contract")
		}
		for _, field := range fields {
			if field == "=>" {
				t.Fatalf("go.mod replacements are forbidden by the offline contract")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if goDirective != "1.26" {
		t.Fatalf("go directive = %q, want 1.26", goDirective)
	}
}

func assertVendoredModuleSet(t *testing.T, filename string) {
	t.Helper()
	contents := readRegularFile(t, filename, 1<<20)
	got := make(map[string]string)
	var packages []string
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "=>") {
			t.Fatalf("vendor/modules.txt replacements are forbidden")
		}
		if line == "" || strings.HasPrefix(line, "## ") {
			continue
		}
		if !strings.HasPrefix(line, "# ") {
			if strings.TrimSpace(line) != line || strings.ContainsAny(line, " \t") {
				t.Fatalf("unrecognized vendored package declaration %q", line)
			}
			packages = append(packages, line)
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "# "))
		if len(fields) != 2 || !strings.HasPrefix(fields[1], "v") {
			t.Fatalf("unrecognized vendor module declaration %q", line)
		}
		if _, exists := got[fields[0]]; exists {
			t.Fatalf("duplicate vendored module %s", fields[0])
		}
		got[fields[0]] = fields[1]
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	want := make(map[string]string, len(vendoredModules))
	for _, module := range vendoredModules {
		want[module.path] = module.version
	}
	assertExactMap(t, "vendored modules", got, want)
	wantPackages := append([]string(nil), vendoredPackages...)
	sort.Strings(packages)
	sort.Strings(wantPackages)
	if strings.Join(packages, "\n") != strings.Join(wantPackages, "\n") {
		t.Fatalf("vendored package exact-set:\n got %q\nwant %q", packages, wantPackages)
	}
}

func assertModuleSums(t *testing.T, filename string) {
	t.Helper()
	contents := readRegularFile(t, filename, 1<<20)
	sums := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 || strings.HasSuffix(fields[1], "/go.mod") {
			continue
		}
		key := fields[0] + "@" + fields[1]
		if _, exists := sums[key]; exists {
			t.Fatalf("duplicate module sum for %s", key)
		}
		sums[key] = fields[2]
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	for _, module := range vendoredModules {
		key := module.path + "@" + module.version
		if sums[key] != module.h1 {
			t.Fatalf("module sum %s = %q, want %q", key, sums[key], module.h1)
		}
	}
}

func assertVendoredLicenseSet(t *testing.T, root string) {
	t.Helper()
	want := make(map[string]licenseContract)
	for _, module := range vendoredModules {
		for _, license := range module.licenseFiles {
			relative := filepath.ToSlash(filepath.Join("vendor", filepath.FromSlash(module.path), license.name))
			if _, exists := want[relative]; exists {
				t.Fatalf("duplicate license contract for %s", relative)
			}
			want[relative] = license
		}
	}
	got := make(map[string]bool)
	err := filepath.WalkDir(filepath.Join(root, "vendor"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isLegalFilename(entry.Name()) {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		contract, exists := want[relative]
		if !exists {
			return fmt.Errorf("unreviewed vendored legal file %s", relative)
		}
		contents := readRegularFile(t, path, 1<<20)
		digest := sha256.Sum256(contents)
		if int64(len(contents)) != contract.size || hex.EncodeToString(digest[:]) != contract.sha256 {
			return fmt.Errorf("vendored legal file %s does not match size/SHA-256 contract", relative)
		}
		if contract.spdx == "" {
			return fmt.Errorf("vendored legal file %s has no reviewed SPDX candidate", relative)
		}
		got[relative] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	missing := make([]string, 0)
	for relative := range want {
		if !got[relative] {
			missing = append(missing, relative)
		}
	}
	if len(missing) != 0 {
		sort.Strings(missing)
		t.Fatalf("missing vendored legal files: %v", missing)
	}
}

func assertVendoredProvenanceBoundary(t *testing.T, root string) {
	t.Helper()
	for _, module := range vendoredModules {
		for _, evidence := range module.provenanceFiles {
			if evidence.statement == "" {
				t.Fatalf("vendored provenance file %s/%s has no bounded statement classification", module.path, evidence.name)
			}
			path := filepath.Join(root, "vendor", filepath.FromSlash(module.path), evidence.name)
			contents := readRegularFile(t, path, 1<<20)
			digest := sha256.Sum256(contents)
			if int64(len(contents)) != evidence.size || hex.EncodeToString(digest[:]) != evidence.sha256 {
				t.Fatalf("vendored provenance file %s/%s does not match size/SHA-256 contract", module.path, evidence.name)
			}
		}
	}
}

func isLegalFilename(name string) bool {
	upper := strings.ToUpper(name)
	for _, prefix := range []string{"LICENSE", "NOTICE", "COPYING", "PATENTS"} {
		if upper == prefix || strings.HasPrefix(upper, prefix+".") || strings.HasPrefix(upper, prefix+"-") {
			return true
		}
	}
	return false
}

func assertVendorTreeIdentity(t *testing.T, root string) {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("vendored path is a link: %s", path)
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != expectedVendorFiles {
		t.Fatalf("vendored file count = %d, want %d", len(paths), expectedVendorFiles)
	}
	sort.Slice(paths, func(left, right int) bool {
		leftRelative, _ := filepath.Rel(root, paths[left])
		rightRelative, _ := filepath.Rel(root, paths[right])
		return filepath.ToSlash(leftRelative) < filepath.ToSlash(rightRelative)
	})
	manifest := sha256.New()
	_, _ = fmt.Fprintln(manifest, "mindweaver.vendor-tree.v1")
	var total int64
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		relative = filepath.ToSlash(relative)
		folded := strings.ToLower(relative)
		if seen[folded] {
			t.Fatalf("vendored tree contains a case-colliding path %s", relative)
		}
		seen[folded] = true
		contents := readRegularFile(t, path, 8<<20)
		total += int64(len(contents))
		if total > 24<<20 {
			t.Fatal("vendored tree exceeds 24 MiB")
		}
		digest := sha256.Sum256(contents)
		_, _ = fmt.Fprintf(manifest, "file\t%s\t%d\t%s\n", relative, len(contents), hex.EncodeToString(digest[:]))
	}
	if got := hex.EncodeToString(manifest.Sum(nil)); got != expectedVendorTreeSHA256 {
		t.Fatalf("vendored tree SHA-256 = %s, want %s", got, expectedVendorTreeSHA256)
	}
}

func assertExactMap(t *testing.T, label string, got, want map[string]string) {
	t.Helper()
	var differences []string
	for key, value := range want {
		if got[key] != value {
			differences = append(differences, fmt.Sprintf("%s=%q (want %q)", key, got[key], value))
		}
	}
	for key, value := range got {
		if _, exists := want[key]; !exists {
			differences = append(differences, fmt.Sprintf("unknown %s=%q", key, value))
		}
	}
	if len(differences) != 0 {
		sort.Strings(differences)
		t.Fatalf("%s mismatch: %s", label, strings.Join(differences, "; "))
	}
}

func readRegularFile(t *testing.T, filename string, limit int64) []byte {
	t.Helper()
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > limit {
		t.Fatalf("%s must be a regular non-link file within %d bytes: %v", filename, limit, err)
	}
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
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
		t.Fatalf("resolve module root: %v", err)
	}
	return root
}
