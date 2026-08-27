package supplychain

import (
	"bufio"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"sort"
	"strings"
	"testing"
)

const (
	modulePath                         = "github.com/mt-hub8/MindWeaver/v2"
	projectLicenseMissing              = "PROJECT_LICENSE_MISSING"
	projectLicenseUnreviewed           = "PROJECT_LICENSE_UNREVIEWED"
	sqliteTranslationProvenanceMissing = "SQLITE_TRANSLATION_UPSTREAM_PROVENANCE_MISSING"
)

type artifactContract struct {
	name    string
	modules []string
}

type fileContract struct {
	name          string
	size          int64
	sha256        string
	kind          string
	spdxCandidate string
}

type moduleContract struct {
	path              string
	version           string
	h1                string
	files             []fileContract
	provenanceBlocker string
}

type runtimeContract struct {
	goVersion string
	files     []fileContract
}

// This contract is a pre-package input boundary, not a release result. It
// derives the shipped module sets from real PE build information and then
// keeps authority-owned inputs as explicit blockers. It never emits an SBOM,
// NOTICE, signature, installer, or release PASS artifact.

var artifactContracts = []artifactContract{
	{
		name: "mindweaver.exe",
		modules: []string{
			"github.com/ncruces/go-sqlite3-wasm/v3@v3.2.35304",
			"github.com/ncruces/go-sqlite3@v0.35.3",
			"github.com/ncruces/julianday@v1.0.0",
			"golang.org/x/sys@v0.47.0",
		},
	},
	{
		name: "mindweaver-pdf.exe",
		modules: []string{
			"github.com/mgilbir/formalis@v0.3.1",
			"github.com/mgilbir/golittlecms@v0.0.0-20260727161601-f6af7cfe1556",
			"github.com/mgilbir/gopenjpeg@v0.0.0-20260727163526-8a139bc479b2",
			"github.com/mgilbir/pdf0@v0.1.0",
		},
	},
}

var moduleContracts = []moduleContract{
	{
		path: "github.com/mgilbir/formalis", version: "v0.3.1",
		h1: "h1:NyYe/EcRYJ2jUjgaZG98lNXgJ7H+jgy6mq7HXOnQxl8=",
		files: []fileContract{
			{"LICENSE", 1082, "4e9651455e1b761ed462c50f60c4618c8985f46404e8db467def14848e77725a", "LICENSE", "MIT"},
		},
	},
	{
		path: "github.com/mgilbir/golittlecms", version: "v0.0.0-20260727161601-f6af7cfe1556",
		h1: "h1:2ZUsOgMhxpHCYC8jyzeEnJZFLYGbXhqjAJWJBvY8q4U=",
		files: []fileContract{
			{"LICENSE", 1199, "4b0b89edd67872e0507e20e03032e4dc4eb194f88082f80acee13a13fb73317c", "LICENSE", "MIT"},
		},
	},
	{
		path: "github.com/mgilbir/gopenjpeg", version: "v0.0.0-20260727163526-8a139bc479b2",
		h1: "h1:kdDIM4JNxn9gsRk5Zo6mtmcFpBqnl9gTVUwf9t6lIRk=",
		files: []fileContract{
			{"LICENSE", 2167, "958dc940b3916ca8b4d373f24027e26e29623828f41205de09e9c680e5539f78", "LICENSE_NOTICE_PATENT_STATEMENT", "BSD-2-Clause"},
		},
	},
	{
		path: "github.com/mgilbir/pdf0", version: "v0.1.0",
		h1: "h1:rfBK18bcQ4kHQTXBmriAb07TafhG2w1fLflq9lHgaG4=",
		files: []fileContract{
			{"LICENSE", 1082, "4e9651455e1b761ed462c50f60c4618c8985f46404e8db467def14848e77725a", "LICENSE", "MIT"},
		},
	},
	{
		path: "github.com/ncruces/go-sqlite3-wasm/v3", version: "v3.2.35304",
		h1: "h1:5NoQAewtgKNK3G4bjNPxVoGXu6F6NzLXWCTdD5FFAEY=",
		files: []fileContract{
			{"LICENSE", 918, "13219037ddf63dbbcf174bf59525d602df7a2e30083f63be566715c858fcb19e", "MODULE_AUTHORED_LICENSE", "MIT-0"},
			{"README.md", 435, "fb8084fccb5733ccc4af421ff026812da7852e91516bdf7c70d007eb46744383", "UPSTREAM_LICENSE_RETENTION_STATEMENT", ""},
		},
		provenanceBlocker: sqliteTranslationProvenanceMissing,
	},
	{
		path: "github.com/ncruces/go-sqlite3", version: "v0.35.3",
		h1: "h1:Ei07Zv1qfV/vyXzelhFsyS5Oh9TArBZHsmFk14Xv3GY=",
		files: []fileContract{
			{"LICENSE", 1068, "8929c89a593807bf0c042272adc7301376a661050876bd702ebdd36cd7c55d2b", "LICENSE", "MIT"},
		},
	},
	{
		path: "github.com/ncruces/julianday", version: "v1.0.0",
		h1: "h1:fH0OKwa7NWvniGQtxdJRxAgkBMolni2BjDHaWTxqt7M=",
		files: []fileContract{
			{"LICENSE", 1068, "38ae43959daf953a393a585b2988672cb65a5a541aca0d0be5e72595a0a16883", "LICENSE", "MIT"},
		},
	},
	{
		path: "golang.org/x/sys", version: "v0.47.0",
		h1: "h1:o7XGOvZQCADBQQ4Y7VNq2dRWQR7JmOUW8Kxx4ZsNgWs=",
		files: []fileContract{
			{"LICENSE", 1453, "911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad", "LICENSE", "BSD-3-Clause"},
			{"PATENTS", 1303, "96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc", "PATENT_GRANT", "LicenseRef-Go-Patent-Grant"},
		},
	},
}

var goRuntimeContract = runtimeContract{
	goVersion: "go1.27.0",
	files: []fileContract{
		{"LICENSE", 1453, "911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad", "LICENSE", "BSD-3-Clause"},
		{"PATENTS", 1303, "96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc", "PATENT_GRANT", "LicenseRef-Go-Patent-Grant"},
		{"VERSION", 35, "964a484ac6011bfc2770d672c3ba1c466f64258fcb0441c8147adb1ac2ee3b30", "TOOLCHAIN_IDENTITY", ""},
	},
}

func TestPrepackageSupplyChainInputClosure(t *testing.T) {
	root := moduleRoot(t)
	goTool := selectedGoTool(t)
	environment, moduleCache := offlineBuildEnvironment(t, goTool)

	if err := validateContractShape(moduleContracts, goRuntimeContract); err != nil {
		t.Fatal(err)
	}
	moduleSums, err := readModuleSums(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateVendoredEvidence(root, moduleContracts, moduleSums); err != nil {
		t.Fatal(err)
	}
	if err := validateGoRuntime(t, goTool, environment, goRuntimeContract); err != nil {
		t.Fatal(err)
	}

	actual := make(map[string][]string, len(artifactContracts))
	for _, artifact := range artifactContracts {
		modules, err := buildArtifactModules(t, goTool, root, environment, artifact)
		if err != nil {
			t.Fatal(err)
		}
		actual[artifact.name] = modules
	}
	if err := validateArtifactMapping(actual, artifactContracts, moduleContracts); err != nil {
		t.Fatal(err)
	}
	if err := requireEmptyDirectory(moduleCache); err != nil {
		t.Fatalf("offline vendored build populated GOMODCACHE: %v", err)
	}

	blockers, err := assessPrepackageBlockers(root, moduleContracts)
	if err != nil {
		t.Fatal(err)
	}
	wantBlockers := []string{projectLicenseMissing, sqliteTranslationProvenanceMissing}
	if !slices.Equal(blockers, wantBlockers) {
		t.Fatalf("pre-package blockers = %q, want %q", blockers, wantBlockers)
	}
}

func TestPrepackageInventoryStructuralMutationsFailClosed(t *testing.T) {
	t.Run("unknown module", func(t *testing.T) {
		mutantArtifacts := cloneArtifactContracts(artifactContracts)
		mutantArtifacts[0].modules = append(mutantArtifacts[0].modules, "example.invalid/unknown@v1.0.0")
		actual := expectedArtifactMapping(mutantArtifacts)
		requireErrorContains(t, validateArtifactMapping(actual, mutantArtifacts, moduleContracts), "unknown module")
	})

	t.Run("wrong PE mapping", func(t *testing.T) {
		actual := expectedArtifactMapping(artifactContracts)
		actual["mindweaver.exe"][0] = "github.com/mgilbir/pdf0@v0.1.0"
		requireErrorContains(t, validateArtifactMapping(actual, artifactContracts, moduleContracts), "module mapping")
	})

	t.Run("module replacement", func(t *testing.T) {
		artifact := artifactContracts[0]
		information := validBuildInfo(artifact)
		information.Deps[0].Replace = &debug.Module{Path: "example.invalid/replacement", Version: "v1.0.1"}
		_, err := modulesFromBuildInfo(information, artifact)
		requireErrorContains(t, err, "replacement")
	})

	t.Run("wrong main package path", func(t *testing.T) {
		artifact := artifactContracts[0]
		information := validBuildInfo(artifact)
		information.Path = modulePath + "/cmd/not-mindweaver"
		_, err := modulesFromBuildInfo(information, artifact)
		requireErrorContains(t, err, "main package")
	})

	t.Run("h1 used as raw SHA-256", func(t *testing.T) {
		mutant := cloneModuleContracts(moduleContracts)
		mutant[0].files[0].sha256 = mutant[0].h1
		requireErrorContains(t, validateContractShape(mutant, goRuntimeContract), "raw evidence SHA-256")
	})
}

func buildArtifactModules(t *testing.T, goTool, root string, environment []string, artifact artifactContract) ([]string, error) {
	t.Helper()
	target, _, err := artifactTarget(artifact)
	if err != nil {
		return nil, err
	}
	output := filepath.Join(t.TempDir(), artifact.name)
	command := exec.CommandContext(t.Context(), goTool, "build", "-trimpath", "-buildvcs=false", "-o", output, target)
	command.Dir = root
	command.Env = environment
	if combined, err := command.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("build %s: %w: %s", artifact.name, err, combined)
	}
	file, err := os.Open(output)
	if err != nil {
		return nil, err
	}
	var magic [2]byte
	_, readErr := io.ReadFull(file, magic[:])
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || string(magic[:]) != "MZ" {
		return nil, fmt.Errorf("%s is not a readable Windows PE", artifact.name)
	}
	information, err := buildinfo.ReadFile(output)
	if err != nil {
		return nil, err
	}
	if information.GoVersion != goRuntimeContract.goVersion {
		return nil, fmt.Errorf("%s Go version = %q, want %q", artifact.name, information.GoVersion, goRuntimeContract.goVersion)
	}
	return modulesFromBuildInfo(information, artifact)
}

func modulesFromBuildInfo(information *buildinfo.BuildInfo, artifact artifactContract) ([]string, error) {
	_, stem, err := artifactTarget(artifact)
	if err != nil {
		return nil, err
	}
	wantMainPackage := modulePath + "/cmd/" + stem
	if information.Path != wantMainPackage {
		return nil, fmt.Errorf("%s main package = %q, want %q", artifact.name, information.Path, wantMainPackage)
	}
	if information.Main.Path != modulePath {
		return nil, fmt.Errorf("%s main module = %q, want %q", artifact.name, information.Main.Path, modulePath)
	}
	settings := make(map[string]string)
	for _, setting := range information.Settings {
		if strings.HasPrefix(setting.Key, "vcs") {
			return nil, fmt.Errorf("%s retained forbidden VCS build setting %s", artifact.name, setting.Key)
		}
		settings[setting.Key] = setting.Value
	}
	for key, want := range map[string]string{
		"GOOS": "windows", "GOARCH": "amd64", "CGO_ENABLED": "0", "-trimpath": "true",
	} {
		if settings[key] != want {
			return nil, fmt.Errorf("%s build setting %s = %q, want %q", artifact.name, key, settings[key], want)
		}
	}
	modules := make([]string, 0, len(information.Deps))
	for _, dependency := range information.Deps {
		if dependency.Replace != nil {
			return nil, fmt.Errorf("%s contains replacement for %s", artifact.name, dependency.Path)
		}
		if dependency.Sum != "" {
			return nil, fmt.Errorf("%s vendored buildinfo unexpectedly carries sum for %s", artifact.name, dependency.Path)
		}
		modules = append(modules, dependency.Path+"@"+dependency.Version)
	}
	sort.Strings(modules)
	return modules, nil
}

func artifactTarget(artifact artifactContract) (target, stem string, err error) {
	if filepath.Ext(artifact.name) != ".exe" {
		return "", "", fmt.Errorf("artifact name %q does not have the exact .exe suffix", artifact.name)
	}
	stem = strings.TrimSuffix(artifact.name, ".exe")
	if stem == "" || stem != filepath.Base(stem) || strings.ToLower(stem) != stem {
		return "", "", fmt.Errorf("artifact name %q does not derive one safe command stem", artifact.name)
	}
	return "./cmd/" + stem, stem, nil
}

func validBuildInfo(artifact artifactContract) *buildinfo.BuildInfo {
	_, stem, err := artifactTarget(artifact)
	if err != nil {
		panic(err)
	}
	information := &buildinfo.BuildInfo{
		GoVersion: goRuntimeContract.goVersion,
		Path:      modulePath + "/cmd/" + stem,
		Main:      debug.Module{Path: modulePath, Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "GOOS", Value: "windows"},
			{Key: "GOARCH", Value: "amd64"},
			{Key: "CGO_ENABLED", Value: "0"},
			{Key: "-trimpath", Value: "true"},
		},
	}
	for _, key := range artifact.modules {
		path, version, found := strings.Cut(key, "@")
		if !found {
			panic("invalid test artifact module " + key)
		}
		information.Deps = append(information.Deps, &debug.Module{Path: path, Version: version})
	}
	return information
}

func requireErrorContains(t *testing.T, err error, fragment string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), fragment) {
		t.Fatalf("error = %v, want target reason containing %q", err, fragment)
	}
}

func validateArtifactMapping(actual map[string][]string, artifacts []artifactContract, modules []moduleContract) error {
	wantNames := make([]string, 0, len(artifacts))
	commandStems := make(map[string]bool, len(artifacts))
	knownModules := make(map[string]bool, len(modules))
	for _, module := range modules {
		knownModules[module.path+"@"+module.version] = true
	}
	shipped := make(map[string]bool, len(modules))
	for _, artifact := range artifacts {
		_, stem, err := artifactTarget(artifact)
		if err != nil {
			return err
		}
		if commandStems[stem] {
			return fmt.Errorf("duplicate artifact command stem %q", stem)
		}
		commandStems[stem] = true
		wantNames = append(wantNames, artifact.name)
		got, ok := actual[artifact.name]
		if !ok {
			return fmt.Errorf("missing artifact inventory %s", artifact.name)
		}
		got = append([]string(nil), got...)
		want := append([]string(nil), artifact.modules...)
		sort.Strings(got)
		sort.Strings(want)
		if !slices.Equal(got, want) {
			return fmt.Errorf("%s module mapping = %q, want %q", artifact.name, got, want)
		}
		for _, key := range got {
			if !knownModules[key] {
				return fmt.Errorf("%s contains unknown module %s", artifact.name, key)
			}
			shipped[key] = true
		}
	}
	sort.Strings(wantNames)
	gotNames := make([]string, 0, len(actual))
	for name := range actual {
		gotNames = append(gotNames, name)
	}
	sort.Strings(gotNames)
	if !slices.Equal(gotNames, wantNames) {
		return fmt.Errorf("artifact inventory exact-set = %q, want %q", gotNames, wantNames)
	}
	if len(shipped) != len(knownModules) {
		return errors.New("legal evidence contains a module that is not linked into either shipped PE")
	}
	return nil
}

func expectedArtifactMapping(artifacts []artifactContract) map[string][]string {
	result := make(map[string][]string, len(artifacts))
	for _, artifact := range artifacts {
		result[artifact.name] = append([]string(nil), artifact.modules...)
	}
	return result
}

func cloneArtifactContracts(source []artifactContract) []artifactContract {
	result := make([]artifactContract, len(source))
	copy(result, source)
	for index := range result {
		result[index].modules = append([]string(nil), source[index].modules...)
	}
	return result
}

func validateVendoredEvidence(root string, modules []moduleContract, sums map[string]string) error {
	for _, module := range modules {
		key := module.path + "@" + module.version
		if sums[key] != module.h1 {
			return fmt.Errorf("go.sum %s = %q, want %q", key, sums[key], module.h1)
		}
		for _, evidence := range module.files {
			path := filepath.Join(root, "vendor", filepath.FromSlash(module.path), evidence.name)
			if err := validateFile(path, evidence); err != nil {
				return fmt.Errorf("%s evidence %s: %w", key, evidence.name, err)
			}
		}
	}
	return nil
}

func validateGoRuntime(t *testing.T, goTool string, environment []string, contract runtimeContract) error {
	t.Helper()
	command := exec.CommandContext(t.Context(), goTool, "version")
	command.Env = environment
	versionBytes, err := command.Output()
	if err != nil {
		return err
	}
	wantVersion := "go version " + contract.goVersion + " windows/amd64"
	if strings.TrimSpace(string(versionBytes)) != wantVersion {
		return fmt.Errorf("selected toolchain = %q, want %q", strings.TrimSpace(string(versionBytes)), wantVersion)
	}
	command = exec.CommandContext(t.Context(), goTool, "env", "GOROOT")
	command.Env = environment
	rootBytes, err := command.Output()
	if err != nil {
		return err
	}
	root := strings.TrimSpace(string(rootBytes))
	if root == "" || !filepath.IsAbs(root) {
		return fmt.Errorf("selected toolchain returned invalid GOROOT %q", root)
	}
	for _, evidence := range contract.files {
		if err := validateFile(filepath.Join(root, evidence.name), evidence); err != nil {
			return fmt.Errorf("Go runtime evidence %s: %w", evidence.name, err)
		}
	}
	return nil
}

func validateFile(path string, contract fileContract) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != contract.size {
		return fmt.Errorf("not the expected bounded regular file")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(contents)
	if hex.EncodeToString(digest[:]) != contract.sha256 {
		return fmt.Errorf("raw SHA-256 mismatch")
	}
	return nil
}

func validateContractShape(modules []moduleContract, runtime runtimeContract) error {
	seen := make(map[string]bool, len(modules))
	for _, module := range modules {
		key := module.path + "@" + module.version
		moduleFilePath := filepath.FromSlash(module.path)
		if seen[key] || module.path == "" || module.version == "" || !strings.HasPrefix(module.h1, "h1:") ||
			filepath.IsAbs(moduleFilePath) || filepath.Clean(moduleFilePath) == ".." || strings.HasPrefix(filepath.Clean(moduleFilePath), ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid or duplicate module evidence contract %q", key)
		}
		seen[key] = true
		for _, evidence := range module.files {
			if err := validateFileContractShape(evidence); err != nil {
				return fmt.Errorf("%s evidence %s: %w", key, evidence.name, err)
			}
		}
	}
	if runtime.goVersion != "go1.27.0" {
		return fmt.Errorf("runtime contract selects %q", runtime.goVersion)
	}
	for _, evidence := range runtime.files {
		if err := validateFileContractShape(evidence); err != nil {
			return fmt.Errorf("runtime evidence %s: %w", evidence.name, err)
		}
	}
	return nil
}

func validateFileContractShape(evidence fileContract) error {
	if evidence.name == "" || evidence.name != filepath.Base(evidence.name) || evidence.name == "." || evidence.name == ".." ||
		evidence.size <= 0 || evidence.kind == "" {
		return errors.New("missing name, size, or evidence kind")
	}
	if len(evidence.sha256) != sha256.Size*2 {
		return errors.New("raw evidence SHA-256 must be 64 lowercase hexadecimal characters")
	}
	decoded, err := hex.DecodeString(evidence.sha256)
	if err != nil || hex.EncodeToString(decoded) != evidence.sha256 {
		return errors.New("raw evidence SHA-256 must be 64 lowercase hexadecimal characters")
	}
	return nil
}

func assessPrepackageBlockers(root string, modules []moduleContract) ([]string, error) {
	blockerSet := make(map[string]bool)
	license := filepath.Join(root, "LICENSE")
	info, err := os.Lstat(license)
	switch {
	case os.IsNotExist(err):
		blockerSet[projectLicenseMissing] = true
	case err != nil:
		return nil, err
	case !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0:
		return nil, errors.New("project LICENSE is not a regular non-link file")
	default:
		blockerSet[projectLicenseUnreviewed] = true
	}
	for _, module := range modules {
		if module.provenanceBlocker != "" {
			blockerSet[module.provenanceBlocker] = true
		}
	}
	blockers := make([]string, 0, len(blockerSet))
	for blocker := range blockerSet {
		blockers = append(blockers, blocker)
	}
	sort.Strings(blockers)
	return blockers, nil
}

func readModuleSums(path string) (map[string]string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect go.sum: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 1<<20 {
		return nil, errors.New("go.sum is not a bounded regular non-link file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 || strings.HasSuffix(fields[1], "/go.mod") {
			continue
		}
		key := fields[0] + "@" + fields[1]
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate module sum %s", key)
		}
		result[key] = fields[2]
	}
	return result, scanner.Err()
}

func cloneModuleContracts(source []moduleContract) []moduleContract {
	result := make([]moduleContract, len(source))
	copy(result, source)
	for index := range result {
		result[index].files = append([]fileContract(nil), source[index].files...)
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

func selectedGoTool(t *testing.T) string {
	t.Helper()
	configured := strings.TrimSpace(os.Getenv("MW_GO"))
	if configured == "" {
		var err error
		configured, err = exec.LookPath("go")
		if err != nil {
			t.Fatal("Go tool is required for pre-package supply-chain qualification")
		}
	}
	absolute, err := filepath.Abs(configured)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

func offlineBuildEnvironment(t *testing.T, goTool string) ([]string, string) {
	t.Helper()
	root := t.TempDir()
	moduleCache := filepath.Join(root, "gomodcache")
	buildCache := filepath.Join(root, "gocache")
	goTemp := filepath.Join(root, "gotmp")
	for _, directory := range []string{moduleCache, buildCache, goTemp} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	goRootCommand := exec.CommandContext(t.Context(), goTool, "env", "GOROOT")
	goRootCommand.Env = environmentWith(os.Environ(), map[string]string{"GOENV": "off", "GOTOOLCHAIN": "local"})
	goRootBytes, err := goRootCommand.Output()
	if err != nil {
		t.Fatal(err)
	}
	overrides := map[string]string{
		"CGO_ENABLED":  "0",
		"GOARCH":       "amd64",
		"GOAMD64":      "v1",
		"GOENV":        "off",
		"GOEXPERIMENT": "",
		"GOFIPS140":    "off",
		"GOFLAGS":      "-mod=vendor -trimpath -buildvcs=false",
		"GOOS":         "windows",
		"GOPROXY":      "off",
		"GOROOT":       strings.TrimSpace(string(goRootBytes)),
		"GOSUMDB":      "off",
		"GOTOOLCHAIN":  "local",
		"GOTELEMETRY":  "off",
		"GOVCS":        "*:off",
		"GOWORK":       "off",
		"GOMODCACHE":   moduleCache,
		"GOCACHE":      buildCache,
		"GOTMPDIR":     goTemp,
	}
	return environmentWith(os.Environ(), overrides), moduleCache
}

func environmentWith(base []string, overrides map[string]string) []string {
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

func requireEmptyDirectory(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		return fmt.Errorf("contains %q", names)
	}
	return nil
}
