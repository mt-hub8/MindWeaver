package supplychain

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	inventorySchema                   = "mindweaver.release-inventory.v1"
	outputManifestSchema              = "mindweaver.release-documents.v1"
	fixtureRegenerationNotImplemented = "FIXTURE_REGENERATION_NOT_IMPLEMENTED"
	releaseMetadataMissing            = "RELEASE_METADATA_MISSING"
	sourceRevisionAttestationMissing  = "SOURCE_REVISION_ATTESTATION_MISSING"
	vulnerabilityEvidenceMissing      = "VULNERABILITY_EVIDENCE_MISSING"
	qualificationSourceDateEpoch      = int64(1787935671)
	qualificationRevision             = "8a9195d5a20ddef3acfeacc974a049724fc2d34f"
	qualificationModuleTree           = "git-sha1:40cdfdb1d3bcc0c5138da42d58111d3a6a8915bd"
	qualificationSourceUnion          = "34ccd6e3ab8b15575eef652c16052552b7c14bd067e1ec7c7a62467f655084ff"
	qualificationVersion              = "0.0.0-qualification.8a9195d"
	qualificationGeneratorName        = "MindWeaver supply-chain qualification"
	qualificationGeneratorVersion     = "1.0.0"
	spdxVersion                       = "SPDX-2.3"
)

type releaseInventory struct {
	Schema                string               `json:"schema"`
	Product               inventoryProduct     `json:"product"`
	Source                inventorySource      `json:"source"`
	Version               inventoryVersion     `json:"version"`
	Creation              inventoryCreation    `json:"creation"`
	Artifacts             []inventoryArtifact  `json:"artifacts"`
	Components            []inventoryComponent `json:"components"`
	Evidence              []inventoryEvidence  `json:"evidence"`
	QualificationBlockers []string             `json:"qualificationBlockers"`
}

type inventorySource struct {
	Repository                  string `json:"repository"`
	Revision                    string `json:"revision"`
	ModuleTree                  string `json:"moduleTree"`
	ProductionSourceUnionSHA256 string `json:"productionSourceUnionSha256"`
	Attestation                 string `json:"attestation"`
}

type inventoryVersion struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

type inventoryCreation struct {
	Timestamp string `json:"timestamp"`
	Source    string `json:"source"`
}

type inventoryProduct struct {
	Name       string `json:"name"`
	ModulePath string `json:"modulePath"`
}

type inventoryArtifact struct {
	Name         string   `json:"name"`
	MainPackage  string   `json:"mainPackage"`
	Size         int64    `json:"size"`
	SHA256       string   `json:"sha256"`
	Dependencies []string `json:"dependencies"`
}

type inventoryComponent struct {
	Ref               string   `json:"ref"`
	Kind              string   `json:"kind"`
	Name              string   `json:"name"`
	Version           string   `json:"version"`
	GoModuleH1        string   `json:"goModuleH1,omitempty"`
	LicenseExpression string   `json:"licenseExpression"`
	Evidence          []string `json:"evidence"`
	Dependencies      []string `json:"dependencies"`
}

type inventoryEvidence struct {
	Ref           string `json:"ref"`
	Path          string `json:"path"`
	Kind          string `json:"kind"`
	Size          int64  `json:"size"`
	SHA256        string `json:"sha256"`
	SPDXCandidate string `json:"spdxCandidate,omitempty"`
	Text          string `json:"text"`
}

type generatedDocument struct {
	Name string
	Data []byte
}

type documentManifest struct {
	Schema                string         `json:"schema"`
	InventorySHA256       string         `json:"inventorySha256"`
	Files                 []manifestFile `json:"files"`
	QualificationBlockers []string       `json:"qualificationBlockers"`
}

type manifestFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func assertDerivedSupplyChainDocuments(t *testing.T, root, moduleCache, goTool string, environment []string, artifacts []builtArtifact, prepackageBlockers []string) {
	t.Helper()
	inventory, err := assembleReleaseInventory(root, moduleCache, goTool, environment, artifacts, prepackageBlockers)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateReleaseInventory(inventory); err != nil {
		t.Fatal(err)
	}
	documents, err := renderSupplyChainDocuments(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateGeneratedDocuments(inventory, documents); err != nil {
		t.Fatal(err)
	}
	for _, document := range documents {
		for _, forbidden := range []string{root, goTool} {
			if bytes.Contains(bytes.ToLower(document.Data), bytes.ToLower([]byte(forbidden))) {
				t.Fatalf("generated %s contains a host-local locator", document.Name)
			}
		}
	}
	assertCommittedDocuments(t, root, documents)
	assertOfficialSchemaValidation(t, root, environment)
}

func assertOfficialSchemaValidation(t *testing.T, root string, environment []string) {
	t.Helper()
	powerShell, err := exec.LookPath("pwsh")
	if err != nil {
		t.Fatal("OFFICIAL_SCHEMA_VALIDATOR_UNAVAILABLE")
	}
	info, err := os.Lstat(powerShell)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("OFFICIAL_SCHEMA_VALIDATOR_UNAVAILABLE")
	}
	command := exec.CommandContext(
		t.Context(), powerShell, "-NoLogo", "-NoProfile", "-NonInteractive", "-File",
		filepath.Join(root, "qualification", "supplychain", "validate_official_schemas.ps1"),
		"-ModuleRoot", root,
	)
	command.Env = environmentWith(environment, map[string]string{"POWERSHELL_TELEMETRY_OPTOUT": "1"})
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		t.Fatal("OFFICIAL_SCHEMA_VALIDATION_FAILED")
	}
}

func assembleReleaseInventory(root, moduleCache, goTool string, environment []string, artifacts []builtArtifact, prepackageBlockers []string) (releaseInventory, error) {
	inventory := releaseInventory{
		Schema:  inventorySchema,
		Product: inventoryProduct{Name: "MindWeaver", ModulePath: modulePath},
		Source: inventorySource{
			Repository: "github.com/mt-hub8/MindWeaver", Revision: qualificationRevision,
			ModuleTree: qualificationModuleTree, ProductionSourceUnionSHA256: qualificationSourceUnion,
			Attestation: "reviewed qualification input; not independently derived from Git in standalone validation",
		},
		Version: inventoryVersion{
			Value:  qualificationVersion,
			Source: "qualification-only reviewed input; real release metadata is unavailable",
		},
		Creation: inventoryCreation{
			Timestamp: time.Unix(qualificationSourceDateEpoch, 0).UTC().Format(time.RFC3339),
			Source:    "qualification fixture SOURCE_DATE_EPOCH=1787935671 bound to source revision; not a release publication time",
		},
		QualificationBlockers: append(append([]string(nil), prepackageBlockers...),
			fixtureRegenerationNotImplemented, releaseMetadataMissing, sourceRevisionAttestationMissing, vulnerabilityEvidenceMissing),
	}
	sort.Strings(inventory.QualificationBlockers)

	evidenceByPath := make(map[string]string)
	addEvidence := func(path, absolute string, contract fileContract) (string, error) {
		if _, exists := evidenceByPath[path]; exists {
			return "", fmt.Errorf("duplicate inventory evidence path %s", path)
		}
		if err := validateFile(absolute, contract); err != nil {
			return "", err
		}
		contents, err := os.ReadFile(absolute)
		if err != nil {
			return "", err
		}
		ref := stableRef("evidence", path+"\n"+contract.sha256)
		inventory.Evidence = append(inventory.Evidence, inventoryEvidence{
			Ref: ref, Path: filepath.ToSlash(path), Kind: contract.kind, Size: int64(len(contents)),
			SHA256: contract.sha256, SPDXCandidate: contract.spdxCandidate, Text: string(contents),
		})
		evidenceByPath[path] = ref
		return ref, nil
	}

	componentByKey := make(map[string]string, len(moduleContracts))
	for _, module := range moduleContracts {
		component := inventoryComponent{
			Ref: componentRef(module.path, module.version), Kind: "go-module", Name: module.path,
			Version: module.version, GoModuleH1: module.h1, LicenseExpression: moduleLicenseExpression(module),
			Dependencies: []string{},
		}
		for _, contract := range module.files {
			relative := filepath.ToSlash(filepath.Join("module-cache", filepath.FromSlash(module.path+"@"+module.version), contract.name))
			ref, err := addEvidence(relative, filepath.Join(moduleCacheDirectory(moduleCache, module), contract.name), contract)
			if err != nil {
				return releaseInventory{}, fmt.Errorf("inventory %s: %w", relative, err)
			}
			component.Evidence = append(component.Evidence, ref)
		}
		componentByKey[module.path+"@"+module.version] = component.Ref
		inventory.Components = append(inventory.Components, component)
	}

	goRoot, err := selectedGoRoot(goTool, environment)
	if err != nil {
		return releaseInventory{}, err
	}
	runtimeComponent := inventoryComponent{
		Ref: "pkg:generic/golang@1.27.0", Kind: "runtime",
		Name: "Go runtime", Version: goRuntimeContract.goVersion, LicenseExpression: "BSD-3-Clause",
		Dependencies: []string{},
	}
	for _, contract := range goRuntimeContract.files {
		relative := filepath.ToSlash(filepath.Join("toolchain", goRuntimeContract.goVersion, contract.name))
		ref, err := addEvidence(relative, filepath.Join(goRoot, contract.name), contract)
		if err != nil {
			return releaseInventory{}, fmt.Errorf("inventory %s: %w", relative, err)
		}
		runtimeComponent.Evidence = append(runtimeComponent.Evidence, ref)
	}
	inventory.Components = append(inventory.Components, runtimeComponent)

	sqliteComponent := inventoryComponent{
		Ref: "pkg:generic/sqlite@" + sqliteUpstreamVersion, Kind: "native-upstream",
		Name: "SQLite", Version: sqliteUpstreamVersion,
		LicenseExpression: "LicenseRef-SQLite-Public-Domain", Dependencies: []string{},
	}
	for _, contract := range sqliteUpstreamFiles {
		fileContract := fileContract{
			name: contract.name, size: contract.size, sha256: contract.sha256,
			kind: "UPSTREAM_PROVENANCE",
		}
		if contract.name == "LICENSE.md" {
			fileContract.kind = "UPSTREAM_LICENSE"
			fileContract.spdxCandidate = "LicenseRef-SQLite-Public-Domain"
		}
		relative := filepath.ToSlash(filepath.Join("third_party", "sqlite", sqliteUpstreamVersion, contract.name))
		ref, err := addEvidence(relative, filepath.Join(root, filepath.FromSlash(relative)), fileContract)
		if err != nil {
			return releaseInventory{}, fmt.Errorf("inventory %s: %w", relative, err)
		}
		sqliteComponent.Evidence = append(sqliteComponent.Evidence, ref)
	}
	inventory.Components = append(inventory.Components, sqliteComponent)

	wasmRef := componentByKey["github.com/ncruces/go-sqlite3-wasm/v3@v3.2.35304"]
	for index := range inventory.Components {
		if inventory.Components[index].Ref == wasmRef {
			inventory.Components[index].Dependencies = []string{sqliteComponent.Ref}
		}
	}

	if len(artifacts) != len(artifactContracts) {
		return releaseInventory{}, fmt.Errorf("built artifact count = %d, want %d", len(artifacts), len(artifactContracts))
	}
	contractByName := make(map[string]artifactContract, len(artifactContracts))
	for _, contract := range artifactContracts {
		contractByName[contract.name] = contract
	}
	for _, artifact := range artifacts {
		contract, ok := contractByName[artifact.name]
		if !ok {
			return releaseInventory{}, fmt.Errorf("unreviewed built artifact %s", artifact.name)
		}
		_, stem, err := artifactTarget(contract)
		if err != nil {
			return releaseInventory{}, err
		}
		dependencies := []string{runtimeComponent.Ref}
		for _, module := range artifact.modules {
			ref, ok := componentByKey[module]
			if !ok {
				return releaseInventory{}, fmt.Errorf("artifact %s has no component evidence for %s", artifact.name, module)
			}
			dependencies = append(dependencies, ref)
		}
		sort.Strings(dependencies)
		inventory.Artifacts = append(inventory.Artifacts, inventoryArtifact{
			Name: artifact.name, MainPackage: modulePath + "/cmd/" + stem,
			Size: artifact.size, SHA256: artifact.sha256, Dependencies: dependencies,
		})
	}
	sort.Slice(inventory.Artifacts, func(i, j int) bool { return inventory.Artifacts[i].Name < inventory.Artifacts[j].Name })
	sort.Slice(inventory.Components, func(i, j int) bool { return inventory.Components[i].Ref < inventory.Components[j].Ref })
	sort.Slice(inventory.Evidence, func(i, j int) bool { return inventory.Evidence[i].Path < inventory.Evidence[j].Path })
	return inventory, nil
}

func selectedGoRoot(goTool string, environment []string) (string, error) {
	command := exec.Command(goTool, "env", "GOROOT")
	command.Env = environment
	output, err := command.Output()
	if err != nil {
		return "", errors.New("selected Go tool did not expose its runtime root")
	}
	root := strings.TrimSpace(string(output))
	if root == "" || !filepath.IsAbs(root) {
		return "", errors.New("selected Go tool exposed an invalid runtime root")
	}
	return filepath.Clean(root), nil
}

func componentRef(path, version string) string {
	return "pkg:golang/" + path + "@" + version
}

func stableRef(kind, value string) string {
	digest := sha256.Sum256([]byte(value))
	return kind + ":" + hex.EncodeToString(digest[:12])
}

func moduleLicenseExpression(module moduleContract) string {
	for _, evidence := range module.files {
		if evidence.kind == "LICENSE" || evidence.kind == "MODULE_AUTHORED_LICENSE" || evidence.kind == "LICENSE_NOTICE_PATENT_STATEMENT" {
			return evidence.spdxCandidate
		}
	}
	return "NOASSERTION"
}

func expectedInventoryComponents() map[string]inventoryComponent {
	expected := make(map[string]inventoryComponent, len(moduleContracts)+2)
	for _, module := range moduleContracts {
		component := inventoryComponent{
			Ref: componentRef(module.path, module.version), Kind: "go-module", Name: module.path,
			Version: module.version, GoModuleH1: module.h1, LicenseExpression: moduleLicenseExpression(module),
			Dependencies: []string{},
		}
		for _, contract := range module.files {
			path := filepath.ToSlash(filepath.Join("module-cache", filepath.FromSlash(module.path+"@"+module.version), contract.name))
			component.Evidence = append(component.Evidence, stableRef("evidence", path+"\n"+contract.sha256))
		}
		expected[component.Ref] = component
	}
	runtime := inventoryComponent{
		Ref: "pkg:generic/golang@1.27.0", Kind: "runtime", Name: "Go runtime",
		Version: goRuntimeContract.goVersion, LicenseExpression: "BSD-3-Clause", Dependencies: []string{},
	}
	for _, contract := range goRuntimeContract.files {
		path := filepath.ToSlash(filepath.Join("toolchain", goRuntimeContract.goVersion, contract.name))
		runtime.Evidence = append(runtime.Evidence, stableRef("evidence", path+"\n"+contract.sha256))
	}
	expected[runtime.Ref] = runtime

	sqlite := inventoryComponent{
		Ref: "pkg:generic/sqlite@" + sqliteUpstreamVersion, Kind: "native-upstream", Name: "SQLite",
		Version: sqliteUpstreamVersion, LicenseExpression: "LicenseRef-SQLite-Public-Domain", Dependencies: []string{},
	}
	for _, contract := range sqliteUpstreamFiles {
		path := filepath.ToSlash(filepath.Join("third_party", "sqlite", sqliteUpstreamVersion, contract.name))
		sqlite.Evidence = append(sqlite.Evidence, stableRef("evidence", path+"\n"+contract.sha256))
	}
	expected[sqlite.Ref] = sqlite

	wasmRef := componentRef("github.com/ncruces/go-sqlite3-wasm/v3", "v3.2.35304")
	wasm := expected[wasmRef]
	wasm.Dependencies = []string{sqlite.Ref}
	expected[wasmRef] = wasm
	return expected
}

func expectedInventoryEvidence() map[string]inventoryEvidence {
	expected := make(map[string]inventoryEvidence)
	add := func(path string, contract fileContract) {
		ref := stableRef("evidence", path+"\n"+contract.sha256)
		expected[ref] = inventoryEvidence{
			Ref: ref, Path: path, Kind: contract.kind, Size: contract.size,
			SHA256: contract.sha256, SPDXCandidate: contract.spdxCandidate,
		}
	}
	for _, module := range moduleContracts {
		for _, contract := range module.files {
			add(filepath.ToSlash(filepath.Join("module-cache", filepath.FromSlash(module.path+"@"+module.version), contract.name)), contract)
		}
	}
	for _, contract := range goRuntimeContract.files {
		add(filepath.ToSlash(filepath.Join("toolchain", goRuntimeContract.goVersion, contract.name)), contract)
	}
	for _, contract := range sqliteUpstreamFiles {
		file := fileContract{name: contract.name, size: contract.size, sha256: contract.sha256, kind: "UPSTREAM_PROVENANCE"}
		if contract.name == "LICENSE.md" {
			file.kind = "UPSTREAM_LICENSE"
			file.spdxCandidate = "LicenseRef-SQLite-Public-Domain"
		}
		add(filepath.ToSlash(filepath.Join("third_party", "sqlite", sqliteUpstreamVersion, contract.name)), file)
	}
	return expected
}

func validateReleaseInventory(inventory releaseInventory) error {
	if inventory.Schema != inventorySchema || inventory.Product != (inventoryProduct{Name: "MindWeaver", ModulePath: modulePath}) {
		return errors.New("inventory header does not match the frozen schema")
	}
	wantSource := inventorySource{
		Repository: "github.com/mt-hub8/MindWeaver", Revision: qualificationRevision,
		ModuleTree: qualificationModuleTree, ProductionSourceUnionSHA256: qualificationSourceUnion,
		Attestation: "reviewed qualification input; not independently derived from Git in standalone validation",
	}
	if inventory.Source != wantSource || !validRawSHA256(inventory.Source.ProductionSourceUnionSHA256) {
		return errors.New("inventory source identity is not the explicit qualification revision")
	}
	wantVersion := inventoryVersion{
		Value:  qualificationVersion,
		Source: "qualification-only reviewed input; real release metadata is unavailable",
	}
	if inventory.Version != wantVersion || inventory.Version.Value == "(devel)" {
		return errors.New("inventory version is not the explicit qualification input")
	}
	wantCreation := inventoryCreation{
		Timestamp: time.Unix(qualificationSourceDateEpoch, 0).UTC().Format(time.RFC3339),
		Source:    "qualification fixture SOURCE_DATE_EPOCH=1787935671 bound to source revision; not a release publication time",
	}
	if inventory.Creation != wantCreation {
		return errors.New("inventory creation input is not the explicit qualification fixture")
	}
	if !slices.Equal(inventory.QualificationBlockers, []string{fixtureRegenerationNotImplemented, projectLicenseMissing, releaseMetadataMissing, sourceRevisionAttestationMissing, vulnerabilityEvidenceMissing}) {
		return fmt.Errorf("inventory qualification blockers = %q", inventory.QualificationBlockers)
	}
	if len(inventory.Artifacts) != 2 || inventory.Artifacts[0].Name != "mindweaver-pdf.exe" || inventory.Artifacts[1].Name != "mindweaver.exe" {
		return errors.New("inventory artifact exact set is not the shipped dual PE set")
	}
	componentRefs := make(map[string]bool, len(inventory.Components))
	evidenceRefs := make(map[string]bool, len(inventory.Evidence))
	evidencePaths := make(map[string]bool, len(inventory.Evidence))
	expectedEvidence := expectedInventoryEvidence()
	wantEvidenceCount := len(goRuntimeContract.files) + len(sqliteUpstreamFiles)
	for _, module := range moduleContracts {
		wantEvidenceCount += len(module.files)
	}
	if len(inventory.Components) != len(moduleContracts)+2 || len(inventory.Evidence) != wantEvidenceCount {
		return errors.New("inventory component or evidence exact set mismatch")
	}
	for _, evidence := range inventory.Evidence {
		localPath := filepath.Clean(filepath.FromSlash(evidence.Path))
		if evidence.Ref == "" || evidenceRefs[evidence.Ref] || evidence.Path == "" || evidencePaths[strings.ToLower(evidence.Path)] ||
			filepath.IsAbs(localPath) || localPath == ".." || strings.HasPrefix(localPath, ".."+string(filepath.Separator)) ||
			evidence.Size <= 0 || int64(len([]byte(evidence.Text))) != evidence.Size || !validRawSHA256(evidence.SHA256) {
			return fmt.Errorf("invalid inventory evidence %s", evidence.Path)
		}
		digest := sha256.Sum256([]byte(evidence.Text))
		if hex.EncodeToString(digest[:]) != evidence.SHA256 {
			return fmt.Errorf("inventory evidence %s text hash mismatch", evidence.Path)
		}
		want, ok := expectedEvidence[evidence.Ref]
		if !ok || evidence.Path != want.Path || evidence.Kind != want.Kind || evidence.Size != want.Size ||
			evidence.SHA256 != want.SHA256 || evidence.SPDXCandidate != want.SPDXCandidate {
			return fmt.Errorf("inventory evidence %s does not match its committed qualification contract", evidence.Path)
		}
		evidenceRefs[evidence.Ref] = true
		evidencePaths[strings.ToLower(evidence.Path)] = true
	}
	referencedEvidence := make(map[string]bool, len(inventory.Evidence))
	expectedComponents := expectedInventoryComponents()
	for _, component := range inventory.Components {
		if component.Ref == "" || componentRefs[component.Ref] || component.Name == "" || component.Version == "" || component.LicenseExpression == "" {
			return fmt.Errorf("invalid or duplicate inventory component %s", component.Ref)
		}
		if component.Kind == "go-module" {
			if !validGoModuleH1(component.GoModuleH1) || validRawSHA256(component.GoModuleH1) {
				return fmt.Errorf("component %s does not distinguish Go h1 from raw SHA-256", component.Ref)
			}
		} else if component.GoModuleH1 != "" {
			return fmt.Errorf("non-module component %s carries Go h1", component.Ref)
		}
		if err := rejectDuplicateStrings(component.Evidence, "component evidence"); err != nil {
			return fmt.Errorf("component %s: %w", component.Ref, err)
		}
		if err := rejectDuplicateStrings(component.Dependencies, "component dependency"); err != nil {
			return fmt.Errorf("component %s: %w", component.Ref, err)
		}
		want, ok := expectedComponents[component.Ref]
		if !ok || component.Kind != want.Kind || component.Name != want.Name || component.Version != want.Version ||
			component.GoModuleH1 != want.GoModuleH1 || component.LicenseExpression != want.LicenseExpression ||
			!slices.Equal(component.Evidence, want.Evidence) || !slices.Equal(component.Dependencies, want.Dependencies) {
			return fmt.Errorf("component %s does not match its committed qualification contract", component.Ref)
		}
		for _, ref := range component.Evidence {
			if !evidenceRefs[ref] || referencedEvidence[ref] {
				return fmt.Errorf("component %s references unknown evidence %s", component.Ref, ref)
			}
			referencedEvidence[ref] = true
		}
		componentRefs[component.Ref] = true
	}
	referencedComponents := make(map[string]bool, len(inventory.Components))
	for _, component := range inventory.Components {
		for _, dependency := range component.Dependencies {
			if !componentRefs[dependency] {
				return fmt.Errorf("component %s references unknown dependency %s", component.Ref, dependency)
			}
			referencedComponents[dependency] = true
		}
	}
	for _, artifact := range inventory.Artifacts {
		if artifact.Size <= 0 || !validRawSHA256(artifact.SHA256) || artifact.MainPackage == "" {
			return fmt.Errorf("invalid artifact %s", artifact.Name)
		}
		if err := rejectDuplicateStrings(artifact.Dependencies, "artifact dependency"); err != nil {
			return fmt.Errorf("artifact %s: %w", artifact.Name, err)
		}
		var contract *artifactContract
		for index := range artifactContracts {
			if artifactContracts[index].name == artifact.Name {
				contract = &artifactContracts[index]
				break
			}
		}
		if contract == nil {
			return fmt.Errorf("artifact %s has no committed qualification contract", artifact.Name)
		}
		_, stem, err := artifactTarget(*contract)
		if err != nil || artifact.MainPackage != modulePath+"/cmd/"+stem {
			return fmt.Errorf("artifact %s main package does not match its committed qualification contract", artifact.Name)
		}
		if artifact.Size != contract.size || artifact.SHA256 != contract.sha256 {
			return fmt.Errorf("artifact %s identity does not match its committed qualification contract", artifact.Name)
		}
		wantDependencies := []string{"pkg:generic/golang@1.27.0"}
		for _, moduleKey := range contract.modules {
			for _, module := range moduleContracts {
				if module.path+"@"+module.version == moduleKey {
					wantDependencies = append(wantDependencies, componentRef(module.path, module.version))
				}
			}
		}
		sort.Strings(wantDependencies)
		if !slices.Equal(artifact.Dependencies, wantDependencies) {
			return fmt.Errorf("artifact %s dependencies do not match its committed qualification contract", artifact.Name)
		}
		for _, dependency := range artifact.Dependencies {
			if !componentRefs[dependency] {
				return fmt.Errorf("artifact %s references unknown dependency %s", artifact.Name, dependency)
			}
			referencedComponents[dependency] = true
		}
	}
	if len(referencedComponents) != len(componentRefs) || len(referencedEvidence) != len(evidenceRefs) {
		return errors.New("inventory contains unreachable component or evidence")
	}
	return nil
}

func rejectDuplicateStrings(values []string, label string) error {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			return fmt.Errorf("duplicate or empty %s %q", label, value)
		}
		seen[value] = true
	}
	return nil
}

func validRawSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func validGoModuleH1(value string) bool {
	if !strings.HasPrefix(value, "h1:") {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "h1:"))
	return err == nil && len(decoded) == sha256.Size && "h1:"+base64.StdEncoding.EncodeToString(decoded) == value
}

func renderSupplyChainDocuments(inventory releaseInventory) ([]generatedDocument, error) {
	if err := validateReleaseInventory(inventory); err != nil {
		return nil, err
	}
	inventoryBytes, err := canonicalJSON(inventory)
	if err != nil {
		return nil, err
	}
	inventorySHA := rawSHA256(inventoryBytes)
	spdxBytes, err := renderSPDX(inventory, inventorySHA)
	if err != nil {
		return nil, err
	}
	noticeBytes := renderNotice(inventory, inventorySHA)
	documents := []generatedDocument{
		{Name: "inventory.json", Data: inventoryBytes},
		{Name: "sbom.spdx.json", Data: spdxBytes},
		{Name: "NOTICE.txt", Data: noticeBytes},
	}
	manifest := documentManifest{Schema: outputManifestSchema, InventorySHA256: inventorySHA, QualificationBlockers: append([]string(nil), inventory.QualificationBlockers...)}
	for _, document := range documents {
		manifest.Files = append(manifest.Files, manifestFile{Name: document.Name, Size: int64(len(document.Data)), SHA256: rawSHA256(document.Data)})
	}
	manifestBytes, err := canonicalJSON(manifest)
	if err != nil {
		return nil, err
	}
	documents = append(documents, generatedDocument{Name: "manifest.json", Data: manifestBytes})
	return documents, nil
}

func canonicalJSON(value any) ([]byte, error) {
	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(contents, '\n'), nil
}

func rawSHA256(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}

type spdxDocument struct {
	SPDXVersion            string                     `json:"spdxVersion"`
	DataLicense            string                     `json:"dataLicense"`
	SPDXID                 string                     `json:"SPDXID"`
	Name                   string                     `json:"name"`
	DocumentNamespace      string                     `json:"documentNamespace"`
	CreationInfo           spdxCreationInfo           `json:"creationInfo"`
	Packages               []spdxPackage              `json:"packages"`
	Relationships          []spdxRelationship         `json:"relationships"`
	ExtractedLicensingInfo []spdxExtractedLicenseInfo `json:"hasExtractedLicensingInfos"`
}

type spdxCreationInfo struct {
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
}

type spdxPackage struct {
	Name             string            `json:"name"`
	SPDXID           string            `json:"SPDXID"`
	VersionInfo      string            `json:"versionInfo"`
	DownloadLocation string            `json:"downloadLocation"`
	FilesAnalyzed    bool              `json:"filesAnalyzed"`
	LicenseConcluded string            `json:"licenseConcluded"`
	LicenseDeclared  string            `json:"licenseDeclared"`
	CopyrightText    string            `json:"copyrightText"`
	Checksums        []spdxChecksum    `json:"checksums,omitempty"`
	ExternalRefs     []spdxExternalRef `json:"externalRefs,omitempty"`
	Comment          string            `json:"comment,omitempty"`
	SourceInfo       string            `json:"sourceInfo,omitempty"`
}

type spdxChecksum struct {
	Algorithm     string `json:"algorithm"`
	ChecksumValue string `json:"checksumValue"`
}

type spdxExternalRef struct {
	ReferenceCategory string `json:"referenceCategory"`
	ReferenceType     string `json:"referenceType"`
	ReferenceLocator  string `json:"referenceLocator"`
}

type spdxRelationship struct {
	SPDXElementID      string `json:"spdxElementId"`
	RelationshipType   string `json:"relationshipType"`
	RelatedSPDXElement string `json:"relatedSpdxElement"`
}

type spdxExtractedLicenseInfo struct {
	LicenseID     string `json:"licenseId"`
	ExtractedText string `json:"extractedText"`
	Name          string `json:"name"`
}

func renderSPDX(inventory releaseInventory, inventorySHA string) ([]byte, error) {
	namespace, err := spdxDocumentNamespace(inventorySHA, spdxVersion, qualificationGeneratorName, qualificationGeneratorVersion)
	if err != nil {
		return nil, err
	}
	document := spdxDocument{
		SPDXVersion: spdxVersion, DataLicense: "CC0-1.0", SPDXID: "SPDXRef-DOCUMENT",
		Name: "MindWeaver dual-PE SBOM", DocumentNamespace: namespace,
		CreationInfo: spdxCreationInfo{Created: inventory.Creation.Timestamp, Creators: []string{qualificationToolCreator()}},
	}
	productID := "SPDXRef-Product"
	document.Packages = append(document.Packages, spdxPackage{
		Name: inventory.Product.Name, SPDXID: productID, VersionInfo: inventory.Version.Value, DownloadLocation: "NOASSERTION",
		FilesAnalyzed: false, LicenseConcluded: "NOASSERTION", LicenseDeclared: "NOASSERTION", CopyrightText: "NOASSERTION",
		Comment: "Pre-package supply-chain qualification blocked: " + strings.Join(inventory.QualificationBlockers, ","), SourceInfo: sourceBinding(inventory),
	})

	for _, artifact := range inventory.Artifacts {
		id := spdxID("artifact:" + artifact.Name)
		document.Packages = append(document.Packages, spdxPackage{
			Name: artifact.Name, SPDXID: id, VersionInfo: inventory.Version.Value, DownloadLocation: "NOASSERTION",
			FilesAnalyzed: false, LicenseConcluded: "NOASSERTION", LicenseDeclared: "NOASSERTION", CopyrightText: "NOASSERTION",
			Checksums:  []spdxChecksum{{Algorithm: "SHA256", ChecksumValue: artifact.SHA256}},
			SourceInfo: sourceBinding(inventory) + "; mainPackage=" + artifact.MainPackage,
		})
	}
	for _, component := range inventory.Components {
		document.Packages = append(document.Packages, spdxPackage{
			Name: component.Name, SPDXID: spdxID(component.Ref), VersionInfo: component.Version,
			DownloadLocation: "NOASSERTION", FilesAnalyzed: false, LicenseConcluded: "NOASSERTION",
			LicenseDeclared: component.LicenseExpression, CopyrightText: "NOASSERTION", Comment: spdxComponentComment(component, inventory.Evidence),
			ExternalRefs: []spdxExternalRef{{ReferenceCategory: "PACKAGE-MANAGER", ReferenceType: "purl", ReferenceLocator: component.Ref}},
		})
	}
	for _, evidence := range inventory.Evidence {
		if evidence.SPDXCandidate == "LicenseRef-SQLite-Public-Domain" {
			document.ExtractedLicensingInfo = []spdxExtractedLicenseInfo{{
				LicenseID: evidence.SPDXCandidate, ExtractedText: evidence.Text,
				Name: "SQLite public-domain dedication and blessing",
			}}
			break
		}
	}
	document.Relationships = expectedSPDXRelationships(inventory)
	sort.Slice(document.Packages, func(i, j int) bool { return document.Packages[i].SPDXID < document.Packages[j].SPDXID })
	return canonicalJSON(document)
}

func qualificationToolCreator() string {
	return "Tool: " + qualificationGeneratorName + "-" + qualificationGeneratorVersion
}

func spdxDocumentNamespace(inventorySHA, version, generatorName, generatorVersion string) (string, error) {
	if !validRawSHA256(inventorySHA) || version == "" || generatorName == "" || generatorVersion == "" ||
		strings.ContainsAny(version, "\r\n") || strings.ContainsAny(generatorName, "\r\n") || strings.ContainsAny(generatorVersion, "\r\n") {
		return "", errors.New("SPDX namespace inputs are invalid")
	}
	seed := "inventory-sha256:" + inventorySHA + "\n" +
		"spdx-version:" + version + "\n" +
		"generator-name:" + generatorName + "\n" +
		"generator-version:" + generatorVersion + "\n"
	digest := sha256.Sum256([]byte(seed))
	return "https://spdx.org/spdxdocs/mindweaver-" + hex.EncodeToString(digest[:]), nil
}

func spdxComponentComment(component inventoryComponent, evidence []inventoryEvidence) string {
	comment := "License expression is a reviewed candidate from hash-bound upstream evidence, not a legal conclusion."
	if component.GoModuleH1 != "" {
		comment += " Go module h1 (not a raw SHA-256): " + component.GoModuleH1
	}
	byRef := make(map[string]inventoryEvidence, len(evidence))
	for _, item := range evidence {
		byRef[item.Ref] = item
	}
	bindings := make([]string, 0, len(component.Evidence))
	for _, ref := range component.Evidence {
		item := byRef[ref]
		bindings = append(bindings, item.Path+"@sha256:"+item.SHA256)
	}
	sort.Strings(bindings)
	comment += " Evidence inventory bindings (not analyzed package files): " + strings.Join(bindings, ",")
	return comment
}

func sourceBinding(inventory releaseInventory) string {
	return "repository=" + inventory.Source.Repository +
		"; revision=" + inventory.Source.Revision +
		"; moduleTree=" + inventory.Source.ModuleTree +
		"; productionSourceUnionSHA256=" + inventory.Source.ProductionSourceUnionSHA256 +
		"; attestation=" + inventory.Source.Attestation
}

func expectedSPDXRelationships(inventory releaseInventory) []spdxRelationship {
	relationships := []spdxRelationship{{"SPDXRef-DOCUMENT", "DESCRIBES", "SPDXRef-Product"}}
	for _, artifact := range inventory.Artifacts {
		artifactID := spdxID("artifact:" + artifact.Name)
		relationships = append(relationships, spdxRelationship{"SPDXRef-Product", "CONTAINS", artifactID})
		for _, dependency := range artifact.Dependencies {
			relationships = append(relationships, spdxRelationship{artifactID, "DEPENDS_ON", spdxID(dependency)})
		}
	}
	for _, component := range inventory.Components {
		componentID := spdxID(component.Ref)
		for _, dependency := range component.Dependencies {
			relationships = append(relationships, spdxRelationship{componentID, "DEPENDS_ON", spdxID(dependency)})
		}
	}
	sortSPDXRelationships(relationships)
	return relationships
}

func sortSPDXRelationships(relationships []spdxRelationship) {
	sort.Slice(relationships, func(i, j int) bool {
		return spdxRelationshipKey(relationships[i]) < spdxRelationshipKey(relationships[j])
	})
}

func spdxRelationshipKey(relationship spdxRelationship) string {
	return relationship.SPDXElementID + "\x00" + relationship.RelationshipType + "\x00" + relationship.RelatedSPDXElement
}

func spdxID(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "SPDXRef-" + hex.EncodeToString(digest[:12])
}

func renderNotice(inventory releaseInventory, inventorySHA string) []byte {
	var output strings.Builder
	fmt.Fprintln(&output, "MindWeaver third-party notice candidate")
	fmt.Fprintln(&output, "Inventory-SHA256:", inventorySHA)
	fmt.Fprintln(&output, "Qualification-Version:", inventory.Version.Value)
	fmt.Fprintln(&output, "Source-Revision:", inventory.Source.Revision)
	fmt.Fprintln(&output, "Source-Module-Tree:", inventory.Source.ModuleTree)
	fmt.Fprintln(&output, "Production-Source-Union-SHA256:", inventory.Source.ProductionSourceUnionSHA256)
	fmt.Fprintln(&output, "Creation-Input:", inventory.Creation.Timestamp)
	fmt.Fprintln(&output, "Supply-Chain-Qualification-Blockers:", strings.Join(inventory.QualificationBlockers, ","))
	fmt.Fprintln(&output, "This deterministic file is not a release authorization or legal conclusion.")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "Artifacts")
	for _, artifact := range inventory.Artifacts {
		fmt.Fprintf(&output, "- %s %d bytes SHA256:%s\n", artifact.Name, artifact.Size, artifact.SHA256)
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "Components")
	for _, component := range inventory.Components {
		fmt.Fprintf(&output, "- %s %s license:%s", component.Name, component.Version, component.LicenseExpression)
		if component.GoModuleH1 != "" {
			fmt.Fprintf(&output, " go-module-h1:%s", component.GoModuleH1)
		}
		fmt.Fprintln(&output)
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "Bound legal and upstream evidence")
	for _, evidence := range inventory.Evidence {
		fmt.Fprintf(&output, "\n----- BEGIN %s -----\n", evidence.Path)
		fmt.Fprintf(&output, "Kind: %s\nRaw-SHA256: %s\n", evidence.Kind, evidence.SHA256)
		if evidence.SPDXCandidate != "" {
			fmt.Fprintf(&output, "SPDX-Candidate: %s\n", evidence.SPDXCandidate)
		}
		fmt.Fprintln(&output)
		noticeText := normalizeNoticeText(evidence.Text)
		output.WriteString(noticeText)
		if !strings.HasSuffix(noticeText, "\n") {
			fmt.Fprintln(&output)
		}
		fmt.Fprintf(&output, "----- END %s -----\n", evidence.Path)
	}
	return []byte(output.String())
}

func normalizeNoticeText(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for index := range lines {
		lines[index] = strings.TrimRight(lines[index], " \t")
	}
	return strings.Join(lines, "\n")
}

func validateGeneratedDocuments(inventory releaseInventory, documents []generatedDocument) error {
	if len(documents) != 4 {
		return fmt.Errorf("generated document count = %d, want 4", len(documents))
	}
	byName := make(map[string][]byte, len(documents))
	for _, document := range documents {
		if _, exists := byName[document.Name]; exists || len(document.Data) == 0 {
			return fmt.Errorf("duplicate or empty generated document %s", document.Name)
		}
		byName[document.Name] = document.Data
	}
	var decodedInventory releaseInventory
	if err := strictJSON(byName["inventory.json"], &decodedInventory); err != nil {
		return fmt.Errorf("inventory schema: %w", err)
	}
	if err := validateReleaseInventory(decodedInventory); err != nil {
		return err
	}
	if !reflect.DeepEqual(inventory, decodedInventory) {
		return errors.New("encoded inventory does not round-trip exactly")
	}
	inventorySHA := rawSHA256(byName["inventory.json"])
	var spdx spdxDocument
	if err := strictJSON(byName["sbom.spdx.json"], &spdx); err != nil {
		return fmt.Errorf("SPDX schema: %w", err)
	}
	if err := validateSPDXDocument(inventory, inventorySHA, spdx); err != nil {
		return err
	}
	if !bytes.Equal(byName["NOTICE.txt"], renderNotice(inventory, inventorySHA)) {
		return errors.New("NOTICE is not derived from the canonical inventory")
	}
	var manifest documentManifest
	if err := strictJSON(byName["manifest.json"], &manifest); err != nil {
		return fmt.Errorf("document manifest schema: %w", err)
	}
	if manifest.Schema != outputManifestSchema || manifest.InventorySHA256 != inventorySHA || !slices.Equal(manifest.QualificationBlockers, inventory.QualificationBlockers) {
		return errors.New("document manifest header mismatch")
	}
	if len(manifest.Files) != 3 {
		return errors.New("document manifest file exact set mismatch")
	}
	wantManifestNames := []string{"NOTICE.txt", "inventory.json", "sbom.spdx.json"}
	gotManifestNames := make([]string, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		gotManifestNames = append(gotManifestNames, file.Name)
		contents, ok := byName[file.Name]
		if !ok || int64(len(contents)) != file.Size || rawSHA256(contents) != file.SHA256 || !validRawSHA256(file.SHA256) {
			return fmt.Errorf("document manifest hash mismatch for %s", file.Name)
		}
	}
	sort.Strings(gotManifestNames)
	if !slices.Equal(gotManifestNames, wantManifestNames) {
		return errors.New("document manifest file exact set mismatch")
	}
	return nil
}

func validateSPDXDocument(inventory releaseInventory, inventorySHA string, document spdxDocument) error {
	wantNamespace, err := spdxDocumentNamespace(inventorySHA, spdxVersion, qualificationGeneratorName, qualificationGeneratorVersion)
	if err != nil {
		return err
	}
	if document.SPDXVersion != spdxVersion || document.DataLicense != "CC0-1.0" || document.SPDXID != "SPDXRef-DOCUMENT" ||
		document.Name != "MindWeaver dual-PE SBOM" ||
		document.DocumentNamespace != wantNamespace || document.CreationInfo.Created != inventory.Creation.Timestamp ||
		qualificationGeneratorName == "" || qualificationGeneratorVersion == "" || !slices.Equal(document.CreationInfo.Creators, []string{qualificationToolCreator()}) {
		return errors.New("SPDX document header mismatch")
	}
	if len(document.Packages) != 1+len(inventory.Artifacts)+len(inventory.Components) {
		return errors.New("SPDX package exact set mismatch")
	}
	ids := map[string]bool{"SPDXRef-DOCUMENT": true}
	packages := make(map[string]spdxPackage, len(document.Packages))
	for _, pkg := range document.Packages {
		if ids[pkg.SPDXID] {
			return fmt.Errorf("duplicate SPDX reference %s", pkg.SPDXID)
		}
		ids[pkg.SPDXID] = true
		packages[pkg.SPDXID] = pkg
		for _, checksum := range pkg.Checksums {
			if checksum.Algorithm != "SHA256" || !validRawSHA256(checksum.ChecksumValue) {
				return fmt.Errorf("SPDX package %s has invalid raw SHA-256", pkg.SPDXID)
			}
		}
	}
	product, ok := packages["SPDXRef-Product"]
	if !ok || product.Name != inventory.Product.Name || product.VersionInfo != inventory.Version.Value || product.SourceInfo != sourceBinding(inventory) ||
		product.DownloadLocation != "NOASSERTION" || product.FilesAnalyzed || product.LicenseConcluded != "NOASSERTION" ||
		product.LicenseDeclared != "NOASSERTION" || product.CopyrightText != "NOASSERTION" || len(product.Checksums) != 0 || len(product.ExternalRefs) != 0 ||
		product.Comment != "Pre-package supply-chain qualification blocked: "+strings.Join(inventory.QualificationBlockers, ",") {
		return errors.New("SPDX product is not bound to inventory source and version")
	}
	for _, artifact := range inventory.Artifacts {
		pkg, ok := packages[spdxID("artifact:"+artifact.Name)]
		if !ok || pkg.Name != artifact.Name || pkg.VersionInfo != inventory.Version.Value || pkg.DownloadLocation != "NOASSERTION" ||
			pkg.FilesAnalyzed || pkg.LicenseConcluded != "NOASSERTION" || pkg.LicenseDeclared != "NOASSERTION" ||
			pkg.CopyrightText != "NOASSERTION" || len(pkg.ExternalRefs) != 0 || pkg.Comment != "" ||
			pkg.SourceInfo != sourceBinding(inventory)+"; mainPackage="+artifact.MainPackage ||
			len(pkg.Checksums) != 1 || pkg.Checksums[0] != (spdxChecksum{Algorithm: "SHA256", ChecksumValue: artifact.SHA256}) {
			return fmt.Errorf("SPDX artifact %s is not exactly derived from inventory", artifact.Name)
		}
	}
	for _, component := range inventory.Components {
		pkg, ok := packages[spdxID(component.Ref)]
		wantExternalRefs := []spdxExternalRef{{ReferenceCategory: "PACKAGE-MANAGER", ReferenceType: "purl", ReferenceLocator: component.Ref}}
		if !ok || pkg.Name != component.Name || pkg.VersionInfo != component.Version || pkg.DownloadLocation != "NOASSERTION" ||
			pkg.FilesAnalyzed || pkg.LicenseConcluded != "NOASSERTION" || pkg.LicenseDeclared != component.LicenseExpression ||
			pkg.CopyrightText != "NOASSERTION" || len(pkg.Checksums) != 0 || !slices.Equal(pkg.ExternalRefs, wantExternalRefs) ||
			pkg.Comment != spdxComponentComment(component, inventory.Evidence) || pkg.SourceInfo != "" {
			return fmt.Errorf("SPDX component %s is not exactly derived from inventory", component.Ref)
		}
	}
	relationshipKeys := make(map[string]bool, len(document.Relationships))
	for _, relationship := range document.Relationships {
		if !ids[relationship.SPDXElementID] || !ids[relationship.RelatedSPDXElement] {
			return fmt.Errorf("SPDX relationship contains an unknown reference")
		}
		key := spdxRelationshipKey(relationship)
		if relationshipKeys[key] {
			return fmt.Errorf("duplicate SPDX relationship %s", key)
		}
		relationshipKeys[key] = true
	}
	actualRelationships := append([]spdxRelationship(nil), document.Relationships...)
	sortSPDXRelationships(actualRelationships)
	if !slices.Equal(actualRelationships, expectedSPDXRelationships(inventory)) {
		return errors.New("SPDX relationship graph is not exactly derived from inventory")
	}
	wantExtractedText := ""
	for _, evidence := range inventory.Evidence {
		if evidence.SPDXCandidate == "LicenseRef-SQLite-Public-Domain" {
			wantExtractedText = evidence.Text
		}
	}
	if len(document.ExtractedLicensingInfo) != 1 || document.ExtractedLicensingInfo[0] != (spdxExtractedLicenseInfo{
		LicenseID: "LicenseRef-SQLite-Public-Domain", ExtractedText: wantExtractedText,
		Name: "SQLite public-domain dedication and blessing",
	}) {
		return errors.New("SPDX SQLite extracted license is missing")
	}
	return nil
}

func strictJSON(contents []byte, destination any) error {
	if err := rejectDuplicateJSONNames(contents); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("JSON contains trailing data")
	}
	canonical, err := canonicalJSON(destination)
	if err != nil {
		return err
	}
	if !bytes.Equal(contents, canonical) {
		return errors.New("JSON is not in the canonical exact schema encoding")
	}
	return nil
}

func rejectDuplicateJSONNames(contents []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := make(map[string]bool)
			for decoder.More() {
				nameToken, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := nameToken.(string)
				if !ok || seen[name] {
					return fmt.Errorf("duplicate or invalid JSON object name")
				}
				seen[name] = true
				if err := walk(); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid JSON delimiter")
		}
		closing, err := decoder.Token()
		if err != nil || closing != matchingDelimiter(delimiter) {
			return errors.New("invalid JSON container")
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("JSON contains trailing data")
	}
	return nil
}

func matchingDelimiter(open json.Delim) json.Delim {
	if open == '{' {
		return '}'
	}
	return ']'
}

func assertCommittedDocuments(t *testing.T, root string, documents []generatedDocument) {
	t.Helper()
	directory := filepath.Join(root, "qualification", "supplychain", "testdata", "release-documents")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	wantNames := make([]string, 0, len(documents))
	for _, document := range documents {
		wantNames = append(wantNames, document.Name)
		actual, err := os.ReadFile(filepath.Join(directory, document.Name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(actual, document.Data) {
			t.Fatalf("committed %s is not derived from the real dual-PE inventory", document.Name)
		}
	}
	gotNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			t.Fatalf("release document directory contains non-file %s", entry.Name())
		}
		gotNames = append(gotNames, entry.Name())
	}
	sort.Strings(wantNames)
	sort.Strings(gotNames)
	if err := validateDocumentExactSet(gotNames, wantNames); err != nil {
		t.Fatalf("release document exact set = %q, want %q", gotNames, wantNames)
	}
}

func validateDocumentExactSet(gotNames, wantNames []string) error {
	got := append([]string(nil), gotNames...)
	want := append([]string(nil), wantNames...)
	sort.Strings(got)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		return errors.New("release document exact set mismatch")
	}
	return nil
}

func TestDerivedSupplyChainDocumentMutationsFailClosed(t *testing.T) {
	root := moduleRoot(t)
	directory := filepath.Join(root, "qualification", "supplychain", "testdata", "release-documents")
	read := func(name string) []byte {
		t.Helper()
		contents, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		return contents
	}

	t.Run("duplicate JSON name", func(t *testing.T) {
		mutant := bytes.Replace(read("inventory.json"), []byte("{\n"), []byte("{\n  \"schema\": \"external-pass\",\n"), 1)
		var inventory releaseInventory
		requireErrorContains(t, strictJSON(mutant, &inventory), "duplicate")
	})

	t.Run("h1 cannot be raw SHA256", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		inventory.Evidence[0].SHA256 = inventory.Components[0].GoModuleH1
		requireErrorContains(t, validateReleaseInventory(inventory), "evidence")
	})

	t.Run("source revision is explicit", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		inventory.Source.Revision = strings.Repeat("0", 40)
		requireErrorContains(t, validateReleaseInventory(inventory), "source identity")
	})

	t.Run("qualification version cannot become devel", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		inventory.Version.Value = "(devel)"
		requireErrorContains(t, validateReleaseInventory(inventory), "version")
	})

	t.Run("duplicate artifact dependency", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		inventory.Artifacts[0].Dependencies = append(inventory.Artifacts[0].Dependencies, inventory.Artifacts[0].Dependencies[0])
		requireErrorContains(t, validateReleaseInventory(inventory), "duplicate")
	})

	t.Run("artifact main package is contract exact", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		inventory.Artifacts[0].MainPackage = modulePath + "/cmd/not-shipped"
		requireErrorContains(t, validateReleaseInventory(inventory), "main package")
	})

	t.Run("artifact hash and size are contract exact", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		inventory.Artifacts[0].SHA256 = strings.Repeat("0", 64)
		inventory.Artifacts[0].Size++
		requireErrorContains(t, validateReleaseInventory(inventory), "identity")
	})

	t.Run("artifact dependency set is contract exact", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		inventory.Artifacts[0].Dependencies[0] = inventory.Artifacts[1].Dependencies[1]
		sort.Strings(inventory.Artifacts[0].Dependencies)
		requireErrorContains(t, validateReleaseInventory(inventory), "dependencies")
	})

	t.Run("duplicate component dependency", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		for index := range inventory.Components {
			if len(inventory.Components[index].Dependencies) != 0 {
				inventory.Components[index].Dependencies = append(inventory.Components[index].Dependencies, inventory.Components[index].Dependencies[0])
				requireErrorContains(t, validateReleaseInventory(inventory), "duplicate")
				return
			}
		}
		t.Fatal("fixture has no component dependency to mutate")
	})

	t.Run("duplicate component evidence", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		inventory.Components[0].Evidence = append(inventory.Components[0].Evidence, inventory.Components[0].Evidence[0])
		requireErrorContains(t, validateReleaseInventory(inventory), "duplicate")
	})

	t.Run("component fields are contract exact", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		inventory.Components[0].Name += "/mutated"
		requireErrorContains(t, validateReleaseInventory(inventory), "committed qualification contract")
	})

	t.Run("component license is contract exact", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		inventory.Components[0].LicenseExpression = "Apache-2.0"
		requireErrorContains(t, validateReleaseInventory(inventory), "committed qualification contract")
	})

	t.Run("evidence path is contract exact", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		inventory.Evidence[0].Path = "module-cache/mutated/LICENSE"
		requireErrorContains(t, validateReleaseInventory(inventory), "committed qualification contract")
	})

	t.Run("evidence hash is contract exact", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		inventory.Evidence[0].SHA256 = strings.Repeat("0", 64)
		requireErrorContains(t, validateReleaseInventory(inventory), "text hash")
	})

	t.Run("unknown SPDX field", func(t *testing.T) {
		mutant := bytes.Replace(read("sbom.spdx.json"), []byte("{\n"), []byte("{\n  \"status\": \"PASS\",\n"), 1)
		var document spdxDocument
		requireErrorContains(t, strictJSON(mutant, &document), "unknown field")
	})

	t.Run("SPDX tool creator includes locked generator version", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		var document spdxDocument
		if err := strictJSON(read("sbom.spdx.json"), &document); err != nil {
			t.Fatal(err)
		}
		document.CreationInfo.Creators = []string{"Tool: MindWeaver supply-chain qualification"}
		requireErrorContains(t, validateSPDXDocument(inventory, rawSHA256(read("inventory.json")), document), "header")
	})

	t.Run("SPDX namespace changes with generator version", func(t *testing.T) {
		inventorySHA := rawSHA256(read("inventory.json"))
		current, err := spdxDocumentNamespace(inventorySHA, spdxVersion, qualificationGeneratorName, qualificationGeneratorVersion)
		if err != nil {
			t.Fatal(err)
		}
		next, err := spdxDocumentNamespace(inventorySHA, spdxVersion, qualificationGeneratorName, qualificationGeneratorVersion+".next")
		if err != nil {
			t.Fatal(err)
		}
		if current == next {
			t.Fatal("SPDX namespace did not change with generator version")
		}
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		var document spdxDocument
		if err := strictJSON(read("sbom.spdx.json"), &document); err != nil {
			t.Fatal(err)
		}
		document.DocumentNamespace = next
		requireErrorContains(t, validateSPDXDocument(inventory, inventorySHA, document), "header")
	})

	t.Run("SPDX artifact fields are derived exactly", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		var document spdxDocument
		if err := strictJSON(read("sbom.spdx.json"), &document); err != nil {
			t.Fatal(err)
		}
		artifactID := spdxID("artifact:" + inventory.Artifacts[0].Name)
		for index := range document.Packages {
			if document.Packages[index].SPDXID == artifactID {
				document.Packages[index].LicenseDeclared = "MIT"
				requireErrorContains(t, validateSPDXDocument(inventory, rawSHA256(read("inventory.json")), document), "exactly derived")
				return
			}
		}
		t.Fatal("fixture has no artifact package to mutate")
	})

	t.Run("duplicate SPDX relationship triple", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		var document spdxDocument
		if err := strictJSON(read("sbom.spdx.json"), &document); err != nil {
			t.Fatal(err)
		}
		document.Relationships = append(document.Relationships, document.Relationships[0])
		requireErrorContains(t, validateSPDXDocument(inventory, rawSHA256(read("inventory.json")), document), "duplicate")
	})

	t.Run("SPDX relationship graph is derived exactly", func(t *testing.T) {
		var inventory releaseInventory
		if err := strictJSON(read("inventory.json"), &inventory); err != nil {
			t.Fatal(err)
		}
		var document spdxDocument
		if err := strictJSON(read("sbom.spdx.json"), &document); err != nil {
			t.Fatal(err)
		}
		document.Relationships = document.Relationships[1:]
		requireErrorContains(t, validateSPDXDocument(inventory, rawSHA256(read("inventory.json")), document), "exactly derived")
	})

	t.Run("external PASS is not an accepted document", func(t *testing.T) {
		got := []string{"NOTICE.txt", "external-status-PASS.json", "inventory.json", "manifest.json", "sbom.spdx.json"}
		want := []string{"NOTICE.txt", "inventory.json", "manifest.json", "sbom.spdx.json"}
		requireErrorContains(t, validateDocumentExactSet(got, want), "exact set")
	})

	t.Run("manifest hash", func(t *testing.T) {
		var manifest documentManifest
		if err := strictJSON(read("manifest.json"), &manifest); err != nil {
			t.Fatal(err)
		}
		manifest.Files[0].SHA256 = strings.Repeat("0", 64)
		mutant, err := canonicalJSON(manifest)
		if err != nil {
			t.Fatal(err)
		}
		documents := []generatedDocument{
			{Name: "inventory.json", Data: read("inventory.json")},
			{Name: "sbom.spdx.json", Data: read("sbom.spdx.json")},
			{Name: "NOTICE.txt", Data: read("NOTICE.txt")},
			{Name: "manifest.json", Data: mutant},
		}
		var inventory releaseInventory
		if err := strictJSON(documents[0].Data, &inventory); err != nil {
			t.Fatal(err)
		}
		requireErrorContains(t, validateGeneratedDocuments(inventory, documents), "hash mismatch")
	})
}
