package production

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

const modulePath = "github.com/mt-hub8/MindWeaver/v2"
const expectedSourceUnionSHA256 = "0c8c2e0f101b632a073bea6600c103304f7ad48d824573242308e8e5e94e1d07"

type artifactContract struct {
	name              string
	target            string
	firstParty        []string
	requiredPackages  []string
	forbiddenPackages []string
	requiredSymbols   []string
	forbiddenSymbols  []string
	modules           []string
	sourceSHA256      string
}

var shippedArtifacts = []artifactContract{
	{
		name:   "mindweaver.exe",
		target: "./cmd/mindweaver",
		firstParty: []string{
			modulePath + "/cmd/mindweaver",
			modulePath + "/internal/app",
			modulePath + "/internal/backup",
			modulePath + "/internal/blob",
			modulePath + "/internal/ingest",
			modulePath + "/internal/lifecycle",
			modulePath + "/internal/localhttp",
			modulePath + "/internal/ollama",
			modulePath + "/internal/pdfextract/client",
			modulePath + "/internal/pdfextract/protocol",
			modulePath + "/internal/rag",
			modulePath + "/internal/store/sqlite",
			modulePath + "/internal/transport",
			modulePath + "/internal/vault",
			modulePath + "/internal/webui",
			modulePath + "/internal/workbench",
			modulePath + "/platform",
			modulePath + "/platform/apperror",
			modulePath + "/platform/config",
			modulePath + "/platform/version",
		},
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
		sourceSHA256: "b7d8ca3e12648fd472c5f9eb1d774fe5aef7b91b694080b2a813380295ddc748",
	},
	{
		name:   "mindweaver-pdf.exe",
		target: "./cmd/mindweaver-pdf",
		firstParty: []string{
			modulePath + "/cmd/mindweaver-pdf",
			modulePath + "/internal/pdfextract/parser",
			modulePath + "/internal/pdfextract/protocol",
		},
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
		sourceSHA256: "bf8badaa11f215a4acd100a839d6e360017bdbc5d9ae18cbbb67e9222ab8849a",
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
	sources := make(map[string][]sourceEntry, len(shippedArtifacts))
	for _, artifact := range shippedArtifacts {
		artifact := artifact
		t.Run(artifact.name, func(t *testing.T) {
			graph := packageGraph(t, goTool, root, environment, artifact.target)
			assertGraph(t, artifact, graph)
			entries, digest := commandSourceManifest(t, goTool, root, environment, artifact)
			if digest != artifact.sourceSHA256 {
				t.Errorf("source manifest SHA-256 = %s, want %s", digest, artifact.sourceSHA256)
			}
			sources[artifact.name] = entries
			t.Logf("%s source files=%d sha256=%s", artifact.name, len(entries), digest)

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
	unionDigest, unionFiles := sourceUnionDigest(t, sources)
	if unionDigest != expectedSourceUnionSHA256 {
		t.Errorf("dual-PE source union SHA-256 = %s, want %s", unionDigest, expectedSourceUnionSHA256)
	}
	t.Logf("dual-PE source union files=%d sha256=%s", unionFiles, unionDigest)
}

type listedSourcePackage struct {
	Dir        string
	ImportPath string
	Standard   bool
	Module     *struct {
		Path string
		Main bool
	}
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

type sourceEntry struct {
	importPath string
	kind       string
	relative   string
	size       int
	digest     string
}

func commandSourceManifest(
	t *testing.T,
	goTool, root string,
	environment []string,
	artifact artifactContract,
) ([]sourceEntry, string) {
	t.Helper()
	raw := runGo(t, goTool, root, environment, "list", "-deps", "-json", artifact.target)
	decoder := json.NewDecoder(strings.NewReader(raw))
	var entries []sourceEntry
	seen := make(map[string]bool)
	var totalBytes int64
	for {
		var record listedSourcePackage
		if err := decoder.Decode(&record); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("decode source package graph: %v", err)
		}
		if record.Standard || record.Module == nil || !record.Module.Main || record.Module.Path != modulePath {
			continue
		}
		packageRelative, err := filepath.Rel(root, record.Dir)
		if err != nil || packageRelative == ".." || strings.HasPrefix(packageRelative, ".."+string(filepath.Separator)) {
			t.Fatalf("first-party package directory escapes module root: %s", record.ImportPath)
		}
		packageRelative = filepath.ToSlash(packageRelative)
		if record.ImportPath != modulePath+"/"+packageRelative {
			t.Fatalf("first-party import path %s does not match directory %s", record.ImportPath, packageRelative)
		}
		categories := []struct {
			kind  string
			files []string
			raw   bool
		}{
			{"go", record.GoFiles, false}, {"cgo", record.CgoFiles, false},
			{"c", record.CFiles, false}, {"cxx", record.CXXFiles, false},
			{"objective-c", record.MFiles, false}, {"header", record.HFiles, false},
			{"fortran", record.FFiles, false}, {"assembly", record.SFiles, false},
			{"swig", record.SwigFiles, false}, {"swig-cxx", record.SwigCXXFiles, false},
			{"syso", record.SysoFiles, true}, {"embed", record.EmbedFiles, true},
		}
		for _, category := range categories {
			for _, name := range category.files {
				filename := filepath.Join(record.Dir, filepath.FromSlash(name))
				relative, err := filepath.Rel(root, filename)
				if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
					t.Fatalf("source file escapes module root: %s", name)
				}
				relative = filepath.ToSlash(relative)
				if seen[strings.ToLower(relative)] {
					t.Fatalf("source manifest contains duplicate or case-colliding file %s", relative)
				}
				seen[strings.ToLower(relative)] = true
				contents := readBoundedSource(t, root, filename, relative)
				if !category.raw {
					contents = canonicalText(t, relative, contents)
				}
				totalBytes += int64(len(contents))
				if totalBytes > 32<<20 {
					t.Fatalf("source manifest bytes exceed 32 MiB")
				}
				digest := sha256.Sum256(contents)
				entries = append(entries, sourceEntry{
					importPath: record.ImportPath,
					kind:       category.kind, relative: relative, size: len(contents),
					digest: hex.EncodeToString(digest[:]),
				})
			}
		}
	}
	if len(entries) == 0 || len(entries) > 512 {
		t.Fatalf("source manifest file count = %d, want 1..512", len(entries))
	}
	sortSourceEntries(entries)
	var manifest strings.Builder
	manifest.WriteString("mindweaver.command-source.v1\n")
	fmt.Fprintf(&manifest, "command\t%s/cmd/%s\n", modulePath, strings.TrimPrefix(artifact.target, "./cmd/"))
	manifest.WriteString("target\twindows\tamd64\tcgo=0\n")
	for _, entry := range entries {
		fmt.Fprintf(&manifest, "file\t%s\t%s\t%s\t%d\t%s\n",
			entry.importPath, entry.kind, entry.relative, entry.size, entry.digest)
	}
	digest := sha256.Sum256([]byte(manifest.String()))
	return entries, hex.EncodeToString(digest[:])
}

func readBoundedSource(t *testing.T, root, filename, relative string) []byte {
	t.Helper()
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	fileResolved, err := filepath.EvalSymlinks(filename)
	if err != nil {
		t.Fatalf("resolve source file %s: %v", relative, err)
	}
	resolvedRelative, err := filepath.Rel(rootResolved, fileResolved)
	if err != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(filepath.Separator)) {
		t.Fatalf("source file %s resolves outside module root", relative)
	}
	info, err := os.Lstat(filename)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > 4<<20 {
		t.Fatalf("source file %s is missing, linked, non-regular, or oversized", relative)
	}
	file, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open source file %s: %v", relative, err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		t.Fatalf("source file %s identity changed before read", relative)
	}
	contents, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
	if err != nil || len(contents) > 4<<20 || int64(len(contents)) != opened.Size() {
		t.Fatalf("read source file %s within bound: %v", relative, err)
	}
	current, err := os.Lstat(filename)
	if err != nil || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, current) {
		t.Fatalf("source file %s identity changed during read", relative)
	}
	return contents
}

func sourceUnionDigest(t *testing.T, commands map[string][]sourceEntry) (string, int) {
	t.Helper()
	union := make(map[string]sourceEntry)
	for command, entries := range commands {
		for _, entry := range entries {
			key := strings.ToLower(entry.relative)
			if previous, exists := union[key]; exists && previous != entry {
				t.Fatalf("source union disagrees for %s through %s", entry.relative, command)
			}
			union[key] = entry
		}
	}
	entries := make([]sourceEntry, 0, len(union))
	for _, entry := range union {
		entries = append(entries, entry)
	}
	sortSourceEntries(entries)
	var manifest strings.Builder
	manifest.WriteString("mindweaver.dual-pe-source-union.v1\n")
	manifest.WriteString("target\twindows\tamd64\tcgo=0\n")
	for _, artifact := range shippedArtifacts {
		fmt.Fprintf(&manifest, "command\t%s\t%s\n", artifact.name, artifact.sourceSHA256)
	}
	for _, entry := range entries {
		fmt.Fprintf(&manifest, "file\t%s\t%s\t%s\t%d\t%s\n",
			entry.importPath, entry.kind, entry.relative, entry.size, entry.digest)
	}
	digest := sha256.Sum256([]byte(manifest.String()))
	return hex.EncodeToString(digest[:]), len(entries)
}

func sortSourceEntries(entries []sourceEntry) {
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].relative != entries[right].relative {
			return entries[left].relative < entries[right].relative
		}
		if entries[left].kind != entries[right].kind {
			return entries[left].kind < entries[right].kind
		}
		return entries[left].importPath < entries[right].importPath
	})
}

func canonicalText(t *testing.T, name string, contents []byte) []byte {
	t.Helper()
	if !utf8.Valid(contents) {
		t.Fatalf("text source %s is not UTF-8", name)
	}
	if !bytes.Contains(contents, []byte{'\r'}) {
		return contents
	}
	canonical := make([]byte, 0, len(contents))
	for index := 0; index < len(contents); index++ {
		switch contents[index] {
		case '\r':
			if index+1 >= len(contents) || contents[index+1] != '\n' {
				t.Fatalf("text source %s has a lone carriage return", name)
			}
			canonical = append(canonical, '\n')
			index++
		case '\n':
			t.Fatalf("text source %s mixes LF and CRLF", name)
		default:
			canonical = append(canonical, contents[index])
		}
	}
	return canonical
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

func assertGraph(t *testing.T, artifact artifactContract, graph map[string]bool) {
	t.Helper()
	for _, imported := range artifact.requiredPackages {
		if !graph[imported] {
			t.Errorf("%s package graph is missing %s", artifact.name, imported)
		}
	}
	for _, imported := range artifact.forbiddenPackages {
		if graph[imported] {
			t.Errorf("%s package graph contains forbidden %s", artifact.name, imported)
		}
	}

	var gotFirstParty []string
	for imported := range graph {
		if imported == modulePath || strings.HasPrefix(imported, modulePath+"/") {
			gotFirstParty = append(gotFirstParty, imported)
			assertNoLaterSegment(t, artifact.name, imported)
		}
	}
	sort.Strings(gotFirstParty)
	wantFirstParty := append([]string(nil), artifact.firstParty...)
	sort.Strings(wantFirstParty)
	if strings.Join(gotFirstParty, "\n") != strings.Join(wantFirstParty, "\n") {
		t.Errorf("%s first-party package exact-set:\n got %q\nwant %q", artifact.name, gotFirstParty, wantFirstParty)
	}
}

func assertNoLaterSegment(t *testing.T, artifactName, imported string) {
	t.Helper()
	forbidden := map[string]bool{
		"agent": true, "agents": true, "batch": true, "batches": true,
		"embedding": true, "embeddings": true, "evaluation": true, "evaluations": true,
		"kbhealth": true, "memories": true, "memory": true, "notification": true,
		"notifications": true, "reindex": true, "reindexing": true, "rerank": true,
		"reranker": true, "vector": true, "vectors": true,
	}
	relative := strings.TrimPrefix(strings.TrimPrefix(imported, modulePath), "/")
	for _, segment := range strings.Split(relative, "/") {
		if forbidden[strings.ToLower(segment)] {
			t.Errorf("%s reaches forbidden LATER package %s", artifactName, imported)
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
		lowerPath := strings.ToLower(dependency.Path)
		for _, forbidden := range []string{"mysql", "mariadb", "flyway", "springframework"} {
			if strings.Contains(lowerPath, forbidden) {
				t.Errorf("legacy Java/MySQL dependency %s retained in %s", dependency.Path, artifact.name)
			}
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
