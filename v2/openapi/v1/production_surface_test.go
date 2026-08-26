package v1_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

const (
	goInspectionTimeout   = 30 * time.Second
	maxGoInspectionOutput = 8 << 20
	maxGoPackages         = 4096
	maxCommandSourceFiles = 512
	maxCommandSourceFile  = 4 << 20
	maxCommandSourceBytes = 32 << 20
)

type goListModule struct {
	Path    string
	Version string
	Sum     string
	Dir     string
	Main    bool
	Replace *goListModule
}

type goListError struct {
	Err string
}

type goListPackage struct {
	Dir        string
	ImportPath string
	Name       string
	Standard   bool
	Incomplete bool
	Error      *goListError
	Module     *goListModule

	GoFiles      []string
	CgoFiles     []string
	CFiles       []string
	CXXFiles     []string
	MFiles       []string
	HFiles       []string
	FFiles       []string
	SFiles       []string
	SwigFiles    []string
	SwigCXXFiles []string
	SysoFiles    []string
	EmbedFiles   []string
}

type boundedCommandBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func (buffer *boundedCommandBuffer) Write(value []byte) (int, error) {
	written := len(value)
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining > 0 {
		if remaining > len(value) {
			remaining = len(value)
		}
		_, _ = buffer.buffer.Write(value[:remaining])
	}
	if remaining < len(value) {
		buffer.exceeded = true
	}
	return written, nil
}

func runOfflineGoList(root string, arguments ...string) ([]byte, error) {
	return runOfflineGoListWithTimeout(root, goInspectionTimeout, arguments...)
}

func runOfflineGoListWithTimeout(root string, timeout time.Duration, arguments ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goBinary += ".exe"
	}
	if info, err := os.Stat(goBinary); err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("frozen Go inspection tool is unavailable")
	}
	command := exec.CommandContext(ctx, goBinary, arguments...)
	command.Dir = root
	command.Env = offlineGoEnvironment(os.Environ())
	stdout := &boundedCommandBuffer{limit: maxGoInspectionOutput}
	stderr := &boundedCommandBuffer{limit: 256 << 10}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if ctx.Err() != nil {
		return nil, errors.New("offline Go inspection exceeded its deadline")
	}
	if stdout.exceeded || stderr.exceeded {
		return nil, errors.New("offline Go inspection exceeded its output bound")
	}
	if err != nil {
		return nil, errors.New("offline Go inspection failed")
	}
	return append([]byte(nil), stdout.buffer.Bytes()...), nil
}

func offlineGoEnvironment(environment []string) []string {
	overrides := map[string]string{
		"CGO_ENABLED": "0",
		"GO111MODULE": "on",
		"GOAMD64":     "v1",
		"GOARCH":      "amd64",
		"GOENV":       "off",
		// An empty GOEXPERIMENT selects the defaults built into the exact
		// runtime.GOROOT toolchain. "none" is not equivalent in Go 1.27: it
		// disables the default jsonv2 experiment required by this dependency
		// graph.
		"GOEXPERIMENT": "",
		"GOFIPS140":    "off",
		"GOFLAGS":      "-mod=readonly -buildvcs=false",
		"GOOS":         "windows",
		"GOPROXY":      "off",
		"GOSUMDB":      "off",
		"GOTOOLCHAIN":  "local",
		"GOVCS":        "*:off",
		"GOWORK":       "off",
		"GOROOT":       runtime.GOROOT(),
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

func decodeGoListPackages(raw []byte) ([]goListPackage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	packages := make([]goListPackage, 0, 64)
	seen := map[string]struct{}{}
	caseFolded := map[string]string{}
	for {
		var record goListPackage
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("malformed Go package inventory")
		}
		if len(packages) >= maxGoPackages || record.ImportPath == "" || len(record.ImportPath) > 1024 || len(record.Dir) > 4096 || len(record.Name) > 256 {
			return nil, errors.New("Go package inventory is outside its bounds")
		}
		if record.Error != nil || record.Incomplete {
			return nil, errors.New("Go package inventory is incomplete")
		}
		if record.Module != nil && record.Module.Replace != nil {
			return nil, errors.New("Go package inventory contains a module replacement")
		}
		if _, duplicate := seen[record.ImportPath]; duplicate {
			return nil, errors.New("Go package inventory contains a duplicate import path")
		}
		folded := strings.ToLower(record.ImportPath)
		if previous, collision := caseFolded[folded]; collision && previous != record.ImportPath {
			return nil, errors.New("Go package inventory contains an import-path case collision")
		}
		seen[record.ImportPath] = struct{}{}
		caseFolded[folded] = record.ImportPath
		packages = append(packages, record)
	}
	if len(packages) == 0 {
		return nil, errors.New("Go package inventory is empty")
	}
	return packages, nil
}

type commandProductionInventory struct {
	packages        []string
	externalModules []string
	sourceDigest    string
	sourceManifest  []byte
	selectedFiles   []commandSourceEntry
}

func inspectCommandProduction(root, module, command string, libraryRoots, excludedRoots []string) (commandProductionInventory, error) {
	raw, err := runOfflineGoList(root, "list", "-deps", "-json", "./cmd/"+command)
	if err != nil {
		return commandProductionInventory{}, err
	}
	records, err := decodeGoListPackages(raw)
	if err != nil {
		return commandProductionInventory{}, err
	}
	localPackages := map[string]struct{}{}
	externalModules := map[string]struct{}{}
	localCase := map[string]string{}
	moduleCase := map[string]string{}
	var sourceEntries []commandSourceEntry
	sourceBudget := commandSourceBudget{}
	commandImport := module + "/cmd/" + command
	seenCommand := false
	for _, record := range records {
		if record.Standard {
			continue
		}
		local, relative, err := moduleRelativeDirectory(root, record.Dir)
		if err != nil {
			return commandProductionInventory{}, err
		}
		if local {
			if record.Module == nil || !record.Module.Main || record.Module.Path != module {
				return commandProductionInventory{}, errors.New("production command links a package from an unowned module inside the source root")
			}
			if record.ImportPath != module+"/"+relative {
				return commandProductionInventory{}, errors.New("module-local import path does not match its physical directory")
			}
			if record.ImportPath == commandImport {
				seenCommand = true
			} else {
				if !hasRelativePrefix(relative, libraryRoots) || hasRelativePrefix(relative, excludedRoots) {
					return commandProductionInventory{}, errors.New("production command links an excluded or undeclared first-party root")
				}
				folded := strings.ToLower(relative)
				if previous, collision := localCase[folded]; collision && previous != relative {
					return commandProductionInventory{}, errors.New("production package closure contains a path case collision")
				}
				localCase[folded] = relative
				localPackages[relative] = struct{}{}
			}
			entries, err := sourceEntriesForPackage(root, record, &sourceBudget)
			if err != nil {
				return commandProductionInventory{}, err
			}
			sourceEntries = append(sourceEntries, entries...)
			continue
		}
		if record.Module == nil || record.Module.Main || strings.HasPrefix(record.ImportPath, module+"/") {
			return commandProductionInventory{}, errors.New("first-party package resolves outside the source root")
		}
		if record.Module.Path == "" || record.Module.Version == "" || record.Module.Sum == "" {
			return commandProductionInventory{}, errors.New("external production module identity is incomplete")
		}
		identity := record.Module.Path + "@" + record.Module.Version + "#" + record.Module.Sum
		folded := strings.ToLower(record.Module.Path)
		if previous, collision := moduleCase[folded]; collision && previous != record.Module.Path {
			return commandProductionInventory{}, errors.New("external production module path has a case collision")
		}
		moduleCase[folded] = record.Module.Path
		externalModules[identity] = struct{}{}
	}
	if !seenCommand {
		return commandProductionInventory{}, errors.New("production command package is absent from its dependency closure")
	}
	digest, manifest, entries, err := buildCommandSourceManifest(module, command, sourceEntries)
	if err != nil {
		return commandProductionInventory{}, err
	}
	return commandProductionInventory{
		packages:        sortedKeys(localPackages),
		externalModules: sortedKeys(externalModules),
		sourceDigest:    digest,
		sourceManifest:  manifest,
		selectedFiles:   entries,
	}, nil
}

func moduleRelativeDirectory(root, directory string) (bool, string, error) {
	rootAbsolute, err := filepath.Abs(root)
	if err != nil {
		return false, "", errors.New("resolve module root")
	}
	directoryAbsolute, err := filepath.Abs(directory)
	if err != nil {
		return false, "", errors.New("resolve package directory")
	}
	if !strings.EqualFold(filepath.VolumeName(rootAbsolute), filepath.VolumeName(directoryAbsolute)) {
		return false, "", nil
	}
	relative, err := filepath.Rel(rootAbsolute, directoryAbsolute)
	if err != nil {
		return false, "", errors.New("compare package directory with module root")
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false, "", nil
	}
	if relative == "." || filepath.IsAbs(relative) {
		return false, "", errors.New("production package has an invalid module-relative directory")
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootAbsolute)
	if err != nil {
		return false, "", errors.New("resolve module-root identity")
	}
	resolvedDirectory, err := filepath.EvalSymlinks(directoryAbsolute)
	if err != nil {
		return false, "", errors.New("resolve package-directory identity")
	}
	resolvedRelative, err := filepath.Rel(resolvedRoot, resolvedDirectory)
	if err != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) || filepath.IsAbs(resolvedRelative) {
		return false, "", errors.New("package directory escapes the resolved module root")
	}
	if !strings.EqualFold(filepath.Clean(resolvedRelative), filepath.Clean(relative)) {
		return false, "", errors.New("package path traverses a link or reparse alias")
	}
	return true, filepath.ToSlash(relative), nil
}

func hasRelativePrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if value == prefix || strings.HasPrefix(value, prefix+"/") {
			return true
		}
	}
	return false
}

func moduleMainPackages(root, module string) ([]string, error) {
	moduleRoots, err := discoverGoModuleRoots(root)
	if err != nil {
		return nil, err
	}
	mainPackages := map[string]struct{}{}
	caseFolded := map[string]string{}
	for _, moduleRoot := range moduleRoots {
		raw, err := runOfflineGoList(moduleRoot, "list", "-json", "./...")
		if err != nil {
			return nil, err
		}
		packages, err := decodeGoListPackages(raw)
		if err != nil {
			return nil, err
		}
		for _, record := range packages {
			if record.Module == nil || !record.Module.Main || record.Module.Replace != nil {
				return nil, errors.New("source module inventory contains an unexpected module identity")
			}
			if moduleRoot == root && record.Module.Path != module {
				return nil, errors.New("primary module identity drift")
			}
			if record.Name != "main" {
				continue
			}
			local, relative, err := moduleRelativeDirectory(root, record.Dir)
			if err != nil || !local {
				return nil, errors.New("main package resolves outside the source root")
			}
			folded := strings.ToLower(relative)
			if previous, collision := caseFolded[folded]; collision && previous != relative {
				return nil, errors.New("main package path has a case collision")
			}
			caseFolded[folded] = relative
			mainPackages[relative] = struct{}{}
		}
	}
	return sortedKeys(mainPackages), nil
}

func sourceMainPackages(root string) ([]string, error) {
	mainPackages := map[string]struct{}{}
	caseFolded := map[string]string{}
	entries := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		entries++
		if entries > 50000 {
			return errors.New("source main-package scan exceeds its entry bound")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("source main-package scan encountered a link")
		}
		if entry.IsDir() && (entry.Name() == "vendor" || entry.Name() == ".git") {
			return filepath.SkipDir
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxCommandSourceFile {
			return errors.New("source main-package scan found an unreadable or oversized Go file")
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.PackageClauseOnly)
		if err != nil {
			return errors.New("source main-package scan found malformed Go source")
		}
		if file.Name.Name != "main" {
			return nil
		}
		local, relative, err := moduleRelativeDirectory(root, filepath.Dir(path))
		if err != nil || !local {
			return errors.New("source main package resolves outside the source root")
		}
		folded := strings.ToLower(relative)
		if previous, collision := caseFolded[folded]; collision && previous != relative {
			return errors.New("source main package path has a case collision")
		}
		caseFolded[folded] = relative
		mainPackages[relative] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return sortedKeys(mainPackages), nil
}

func discoverGoModuleRoots(root string) ([]string, error) {
	var roots []string
	entries := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		entries++
		if entries > 50000 {
			return errors.New("source-tree scan exceeds its entry bound")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("source-tree module scan encountered a link")
		}
		if entry.IsDir() && (entry.Name() == "vendor" || entry.Name() == ".git") {
			return filepath.SkipDir
		}
		if !entry.IsDir() && entry.Name() == "go.mod" {
			if len(roots) >= 16 {
				return errors.New("source-tree module count exceeds its bound")
			}
			roots = append(roots, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(roots)
	if len(roots) == 0 || filepath.Clean(roots[0]) != filepath.Clean(root) {
		return nil, errors.New("primary Go module is absent from the source tree")
	}
	return roots, nil
}

type commandSourceEntry struct {
	importPath string
	kind       string
	relative   string
	size       int
	digest     string
	contents   []byte
}

type commandSourceBudget struct {
	files int
	bytes int64
}

func (budget *commandSourceBudget) reserve(size int64) error {
	if budget == nil || size < 0 || size > maxCommandSourceFile {
		return errors.New("command source is outside its per-file bound")
	}
	if budget.files >= maxCommandSourceFiles {
		return errors.New("command source file count exceeds its bound")
	}
	if budget.bytes > int64(maxCommandSourceBytes)-size {
		return errors.New("command source bytes exceed their aggregate bound")
	}
	budget.files++
	budget.bytes += size
	return nil
}

func sourceEntriesForPackage(root string, record goListPackage, budget *commandSourceBudget) ([]commandSourceEntry, error) {
	categories := []struct {
		kind  string
		files []string
		raw   bool
	}{
		{kind: "go", files: record.GoFiles},
		{kind: "cgo", files: record.CgoFiles},
		{kind: "c", files: record.CFiles},
		{kind: "cxx", files: record.CXXFiles},
		{kind: "objective-c", files: record.MFiles},
		{kind: "header", files: record.HFiles},
		{kind: "fortran", files: record.FFiles},
		{kind: "assembly", files: record.SFiles},
		{kind: "swig", files: record.SwigFiles},
		{kind: "swig-cxx", files: record.SwigCXXFiles},
		{kind: "syso", files: record.SysoFiles, raw: true},
		{kind: "embed", files: record.EmbedFiles, raw: true},
	}
	entries := make([]commandSourceEntry, 0, 32)
	for _, category := range categories {
		for _, name := range category.files {
			relative, contents, err := readCommandSource(root, record.Dir, name, category.raw, budget)
			if err != nil {
				return nil, err
			}
			digest := sha256.Sum256(contents)
			entries = append(entries, commandSourceEntry{
				importPath: record.ImportPath,
				kind:       category.kind,
				relative:   relative,
				size:       len(contents),
				digest:     fmt.Sprintf("%x", digest),
				contents:   contents,
			})
		}
	}
	return entries, nil
}

func buildCommandSourceManifest(module, command string, entries []commandSourceEntry) (string, []byte, []commandSourceEntry, error) {
	if len(entries) == 0 {
		return "", nil, nil, errors.New("command source inventory is empty")
	}
	if len(entries) > maxCommandSourceFiles {
		return "", nil, nil, errors.New("command source file count exceeds its bound")
	}
	paths := map[string]string{}
	totalBytes := 0
	for _, entry := range entries {
		folded := strings.ToLower(entry.relative)
		if previous, collision := paths[folded]; collision {
			if previous == entry.relative {
				return "", nil, nil, errors.New("command source inventory contains a duplicate file")
			}
			return "", nil, nil, errors.New("command source inventory contains a path case collision")
		}
		paths[folded] = entry.relative
		totalBytes += len(entry.contents)
		if totalBytes > maxCommandSourceBytes {
			return "", nil, nil, errors.New("command source bytes exceed their aggregate bound")
		}
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].relative != entries[right].relative {
			return entries[left].relative < entries[right].relative
		}
		if entries[left].kind != entries[right].kind {
			return entries[left].kind < entries[right].kind
		}
		return entries[left].importPath < entries[right].importPath
	})
	var manifest strings.Builder
	manifest.WriteString("mindweaver.command-source.v1\n")
	fmt.Fprintf(&manifest, "command\t%s/cmd/%s\n", module, command)
	manifest.WriteString("target\twindows\tamd64\tcgo=0\n")
	for _, entry := range entries {
		fmt.Fprintf(&manifest, "file\t%s\t%s\t%s\t%d\t%s\n", entry.importPath, entry.kind, entry.relative, entry.size, entry.digest)
	}
	canonical := []byte(manifest.String())
	digest := sha256.Sum256(canonical)
	return fmt.Sprintf("%x", digest), canonical, entries, nil
}

func commandSourceManifestDigest(root, module, command string) (string, []byte, error) {
	inventory, err := inspectCommandProduction(root, module, command, []string{"internal", "platform"}, []string{"docs", "openapi", "qualification", "release", "spikes", "testdata"})
	if err != nil {
		return "", nil, err
	}
	return inventory.sourceDigest, inventory.sourceManifest, nil
}

func readCommandSource(root, packageDirectory, name string, raw bool, budget *commandSourceBudget) (string, []byte, error) {
	if name == "" || strings.Contains(name, "\\") || !filepath.IsLocal(filepath.FromSlash(name)) || filepath.ToSlash(filepath.Clean(filepath.FromSlash(name))) != name {
		return "", nil, errors.New("command source path is not canonical and local")
	}
	path := filepath.Join(packageDirectory, filepath.FromSlash(name))
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maxCommandSourceFile {
		return "", nil, errors.New("command source is absent, linked, non-regular, or oversized")
	}
	if err := budget.reserve(info.Size()); err != nil {
		return "", nil, err
	}
	local, relative, err := moduleRelativeDirectory(root, path)
	if err != nil || !local {
		return "", nil, errors.New("command source resolves outside the module root")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", nil, errors.New("open command source")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() != info.Size() || !os.SameFile(info, opened) {
		return "", nil, errors.New("command source identity changed before read")
	}
	contents, err := io.ReadAll(io.LimitReader(file, int64(maxCommandSourceFile)+1))
	if err != nil || int64(len(contents)) != opened.Size() || len(contents) > maxCommandSourceFile {
		return "", nil, errors.New("read command source within its admitted size")
	}
	current, err := os.Lstat(path)
	if err != nil || current.Mode()&os.ModeSymlink != 0 || !current.Mode().IsRegular() || current.Size() != opened.Size() || !os.SameFile(opened, current) {
		return "", nil, errors.New("command source identity changed during read")
	}
	if !raw {
		contents, err = canonicalTextSource(contents)
		if err != nil {
			return "", nil, err
		}
	}
	return relative, contents, nil
}

func canonicalTextSource(contents []byte) ([]byte, error) {
	if !utf8.Valid(contents) {
		return nil, errors.New("command text source is not UTF-8")
	}
	if !bytes.Contains(contents, []byte{'\r'}) {
		return contents, nil
	}
	canonical := make([]byte, 0, len(contents))
	for index := 0; index < len(contents); index++ {
		switch contents[index] {
		case '\r':
			if index+1 >= len(contents) || contents[index+1] != '\n' {
				return nil, errors.New("command text source has mixed or lone carriage returns")
			}
			canonical = append(canonical, '\n')
			index++
		case '\n':
			return nil, errors.New("command text source mixes LF and CRLF")
		default:
			canonical = append(canonical, contents[index])
		}
	}
	return canonical, nil
}

func TestProductionClosureRejectsExcludedFirstPartyImport(t *testing.T) {
	root := newGoModuleFixture(t, "example.invalid/surface", map[string]string{
		"cmd/mindweaver/main.go":         "package main\nimport _ \"example.invalid/surface/release/compatbridge\"\nfunc main() {}\n",
		"release/compatbridge/compat.go": "package compatbridge\n",
	})
	if _, err := inspectCommandProduction(root, "example.invalid/surface", "mindweaver", []string{"internal", "platform"}, []string{"release"}); err == nil {
		t.Fatal("excluded first-party command dependency unexpectedly passed")
	}
}

func TestCommandProductionInventoryPreservesPerCommandOwnership(t *testing.T) {
	root := newGoModuleFixture(t, "example.invalid/surface", map[string]string{
		"cmd/mindweaver/main.go":     "package main\nimport (_ \"example.invalid/surface/internal/mainonly\"; _ \"example.invalid/surface/internal/shared\")\nfunc main() {}\n",
		"cmd/mindweaver-pdf/main.go": "package main\nimport (_ \"example.invalid/surface/internal/helperonly\"; _ \"example.invalid/surface/internal/shared\")\nfunc main() {}\n",
		"internal/helperonly/x.go":   "package helperonly\n",
		"internal/mainonly/x.go":     "package mainonly\n",
		"internal/shared/x.go":       "package shared\n",
	})
	mainInventory, err := inspectCommandProduction(root, "example.invalid/surface", "mindweaver", []string{"internal"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	helperInventory, err := inspectCommandProduction(root, "example.invalid/surface", "mindweaver-pdf", []string{"internal"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflectStringSlices(mainInventory.packages, []string{"internal/mainonly", "internal/shared"}) ||
		!reflectStringSlices(helperInventory.packages, []string{"internal/helperonly", "internal/shared"}) {
		t.Fatalf("per-command ownership drift: main=%#v helper=%#v", mainInventory.packages, helperInventory.packages)
	}
}

func TestSelectedEmbeddedFileRejectsLegacyMigrationToken(t *testing.T) {
	root := newGoModuleFixture(t, "example.invalid/surface", map[string]string{
		"cmd/mindweaver/main.go":      "package main\nimport _ \"example.invalid/surface/internal/payload\"\nfunc main() {}\n",
		"internal/payload/payload.go": "package payload\nimport _ \"embed\"\n//go:embed stale.txt\nvar stale string\n",
		"internal/payload/stale.txt":  "Legacy_Import\n",
	})
	inventory, err := inspectCommandProduction(root, "example.invalid/surface", "mindweaver", []string{"internal"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacyMigrationSelectedFileError(map[string]commandProductionInventory{"mindweaver": inventory}); err == nil {
		t.Fatal("embedded legacy migration token unexpectedly passed")
	}
}

func TestModuleMainPackagesIncludesNestedModulesAndUnexpectedCommandRoot(t *testing.T) {
	root := newGoModuleFixture(t, "example.invalid/surface", map[string]string{
		"cmd/main.go":                "package main\nfunc main() {}\n",
		"cmd/mindweaver/main.go":     "package main\nfunc main() {}\n",
		"tools/go.mod":               "module example.invalid/tool\n\ngo 1.26\n",
		"tools/cmd/evidence/main.go": "package main\nfunc main() {}\n",
	})
	actual, err := moduleMainPackages(root, "example.invalid/surface")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cmd", "cmd/mindweaver", "tools/cmd/evidence"}
	if !reflectStringSlices(actual, want) {
		t.Fatalf("main packages = %#v, want %#v", actual, want)
	}
}

func TestSourceMainPackagesIncludesGoIgnoredDirectories(t *testing.T) {
	root := newGoModuleFixture(t, "example.invalid/surface", map[string]string{
		".hidden/main.go":         "package main\nfunc main() {}\n",
		"_hidden/main.go":         "package main\nfunc main() {}\n",
		"testdata/hidden/main.go": "package main\nfunc main() {}\n",
	})
	actual, err := sourceMainPackages(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".hidden", "_hidden", "testdata/hidden"}
	if !reflectStringSlices(actual, want) {
		t.Fatalf("source main packages = %#v, want %#v", actual, want)
	}
}

func TestCommandSourceManifestBindsCompiledFilesAndCanonicalNewlines(t *testing.T) {
	root := newGoModuleFixture(t, "example.invalid/surface", map[string]string{
		"cmd/mindweaver/main.go": "package main\nimport _ \"example.invalid/surface/internal/core\"\nfunc main() {}\n",
		"internal/core/core.go":  "package core\nconst Value = 1\n",
	})
	first, _, err := commandSourceManifestDigest(root, "example.invalid/surface", "mindweaver")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmd", "mindweaver", "main.go"), []byte("package main\r\nimport _ \"example.invalid/surface/internal/core\"\r\nfunc main() {}\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	crlf, _, err := commandSourceManifestDigest(root, "example.invalid/surface", "mindweaver")
	if err != nil {
		t.Fatal(err)
	}
	if crlf != first {
		t.Fatal("uniform CRLF changed the canonical command source digest")
	}
	if err := os.WriteFile(filepath.Join(root, "cmd", "mindweaver", "extra.go"), []byte("package main\nfunc hidden() string { return \"hidden\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	withExtra, _, err := commandSourceManifestDigest(root, "example.invalid/surface", "mindweaver")
	if err != nil {
		t.Fatal(err)
	}
	if withExtra == first {
		t.Fatal("additional compiled command source did not change the manifest digest")
	}
	if err := os.WriteFile(filepath.Join(root, "cmd", "mindweaver", "main.go"), []byte("package main\nimport _ \"example.invalid/surface/internal/core\"\nfunc main() { if true { hidden() } }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mutated, _, err := commandSourceManifestDigest(root, "example.invalid/surface", "mindweaver")
	if err != nil {
		t.Fatal(err)
	}
	if mutated == withExtra {
		t.Fatal("command control-flow mutation did not change the manifest digest")
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "core", "core.go"), []byte("package core\nconst Value = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	transitive, _, err := commandSourceManifestDigest(root, "example.invalid/surface", "mindweaver")
	if err != nil {
		t.Fatal(err)
	}
	if transitive == mutated {
		t.Fatal("transitive first-party source mutation did not change the manifest digest")
	}
}

func TestCommandSourceAdmissionBoundsBeforeAccumulation(t *testing.T) {
	root := t.TempDir()
	packageDirectory := filepath.Join(root, "internal", "core")
	if err := os.MkdirAll(packageDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	names := make([]string, maxCommandSourceFiles+1)
	for index := range names {
		names[index] = fmt.Sprintf("source-%03d.go", index)
		if err := os.WriteFile(filepath.Join(packageDirectory, names[index]), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	budget := &commandSourceBudget{}
	_, err := sourceEntriesForPackage(root, goListPackage{
		Dir:        packageDirectory,
		ImportPath: "example.invalid/surface/internal/core",
		GoFiles:    names,
	}, budget)
	if err == nil {
		t.Fatal("source inventory above the file-count admission bound unexpectedly passed")
	}
	if budget.files != maxCommandSourceFiles || budget.bytes != 0 {
		t.Fatalf("rejected file-count budget = %#v", budget)
	}

	payload := "payload.bin"
	if err := os.WriteFile(filepath.Join(packageDirectory, payload), []byte{1}, 0o600); err != nil {
		t.Fatal(err)
	}
	budget = &commandSourceBudget{bytes: maxCommandSourceBytes}
	_, err = sourceEntriesForPackage(root, goListPackage{
		Dir:        packageDirectory,
		ImportPath: "example.invalid/surface/internal/core",
		EmbedFiles: []string{payload},
	}, budget)
	if err == nil {
		t.Fatal("source inventory above the aggregate-byte admission bound unexpectedly passed")
	}
	if budget.files != 0 || budget.bytes != maxCommandSourceBytes {
		t.Fatalf("rejected aggregate-byte budget changed: %#v", budget)
	}
}

func TestDecodeGoListPackagesFailsClosed(t *testing.T) {
	for name, raw := range map[string]string{
		"malformed":   "{",
		"duplicate":   "{\"ImportPath\":\"example.invalid/p\",\"Dir\":\"x\",\"Name\":\"p\"}\n{\"ImportPath\":\"example.invalid/p\",\"Dir\":\"x\",\"Name\":\"p\"}\n",
		"replacement": "{\"ImportPath\":\"example.invalid/p\",\"Dir\":\"x\",\"Name\":\"p\",\"Module\":{\"Path\":\"example.invalid/p\",\"Replace\":{\"Path\":\"example.invalid/q\"}}}\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeGoListPackages([]byte(raw)); err == nil {
				t.Fatal("invalid Go package inventory unexpectedly passed")
			}
		})
	}
}

func TestOfflineGoInspectionFailsClosedOnNonzeroAndTimeout(t *testing.T) {
	root := newGoModuleFixture(t, "example.invalid/surface", map[string]string{
		"main.go": "package surface\n",
	})
	if _, err := runOfflineGoList(root, "list", "./missing"); err == nil {
		t.Fatal("nonzero Go inspection unexpectedly passed")
	}
	if _, err := runOfflineGoListWithTimeout(root, time.Nanosecond, "list", "./..."); err == nil {
		t.Fatal("expired Go inspection deadline unexpectedly passed")
	}
}

func TestOfflineGoEnvironmentOverridesHostileAmbientTargetSettings(t *testing.T) {
	environment := offlineGoEnvironment([]string{
		"Path=C:\\safe",
		"goamd64=v4",
		"GOEXPERIMENT=hostile",
		"GoFiPs140=on",
		"GOROOT=C:\\hostile",
	})
	want := map[string]string{
		"GOAMD64":      "v1",
		"GOEXPERIMENT": "",
		"GOFIPS140":    "off",
		"GOROOT":       runtime.GOROOT(),
	}
	counts := map[string]int{}
	for _, item := range environment {
		name, value, found := strings.Cut(item, "=")
		if !found {
			continue
		}
		upper := strings.ToUpper(name)
		if expected, exists := want[upper]; exists {
			counts[upper]++
			if value != expected {
				t.Fatalf("%s = %q, want %q", upper, value, expected)
			}
		}
	}
	for name := range want {
		if counts[name] != 1 {
			t.Fatalf("%s appears %d times, want exactly once", name, counts[name])
		}
	}
}

func newGoModuleFixture(t *testing.T, module string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if _, exists := files["go.mod"]; !exists {
		files["go.mod"] = "module " + module + "\n\ngo 1.26\n"
	}
	for relative, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func reflectStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
