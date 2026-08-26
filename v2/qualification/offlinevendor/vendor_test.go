package offlinevendor

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type moduleContract struct {
	path         string
	version      string
	h1           string
	licenseFiles []licenseContract
}

type licenseContract struct {
	name   string
	size   int64
	sha256 string
	spdx   string
}

const (
	expectedVendorFiles      = 698
	expectedVendorTreeSHA256 = "f935ae79254d0fd1f5f32f4491f0b1f1e28e22fc088cef262bde2e860cf9c254"
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
		h1:           "h1:5NoQAewtgKNK3G4bjNPxVoGXu6F6NzLXWCTdD5FFAEY=",
		licenseFiles: []licenseContract{{"LICENSE", 918, "13219037ddf63dbbcf174bf59525d602df7a2e30083f63be566715c858fcb19e", "MIT-0"}},
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
	assertPinnedWorkflow(t, filepath.Join(root, ".github", "workflows", "ci.yml"))
	assertOfflineEntrypointPolicies(t, root)
	assertToolchainAndNoReplace(t, filepath.Join(root, "go.mod"))
	assertVendoredModuleSet(t, filepath.Join(root, "vendor", "modules.txt"))
	assertModuleSums(t, filepath.Join(root, "go.sum"))
	assertVendoredLicenseSet(t, root)
	assertVendorTreeIdentity(t, filepath.Join(root, "vendor"))
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

func assertPinnedWorkflow(t *testing.T, filename string) {
	t.Helper()
	contents := strings.ReplaceAll(string(readRegularFile(t, filename, 1<<20)), "\r\n", "\n")
	required := []string{
		"go-version: 1.27.0",
		"GOFLAGS: -mod=vendor -trimpath -buildvcs=false",
		"GOPROXY: \"off\"",
		"GOSUMDB: \"off\"",
		"GOTOOLCHAIN: local",
		"GOVCS: \"*:off\"",
	}
	for _, line := range required {
		if strings.Count(contents, line) != 1 {
			t.Fatalf("workflow must contain exactly one %q", line)
		}
	}
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
