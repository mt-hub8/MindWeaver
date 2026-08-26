package knowledge_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	"github.com/mt-hub8/MindWeaver/v2/internal/lifecycle"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

const (
	boundaryBaseline  = "d61a158afdc7c86914c70a847ce4e9d6fe686517"
	boundaryName      = "CORE_KEYWORD_SEARCH_ONLY_NATURAL_QUESTION_EXPLICITLY_LIMITED"
	boundaryCorpusSHA = "3e81a3ed29077cac9bb5f6be3d6e81bb9bd757799f0e8233544535f16a58e132"
)

type boundaryContract struct {
	SchemaVersion   int                  `json:"schema_version"`
	BaselineCommit  string               `json:"baseline_commit"`
	ProductBoundary string               `json:"product_boundary"`
	Collections     []boundaryCollection `json:"collections"`
	Documents       []boundaryDocument   `json:"documents"`
	Probes          []boundaryProbe      `json:"probes"`
}

type boundaryCollection struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type boundaryDocument struct {
	Key                string   `json:"key"`
	Title              string   `json:"title"`
	Filename           string   `json:"filename"`
	Content            string   `json:"content"`
	State              string   `json:"state"`
	Collections        []string `json:"collections"`
	PreTransitionQuery string   `json:"pre_transition_query,omitempty"`
}

type boundaryProbe struct {
	ID                string   `json:"id"`
	Class             string   `json:"class"`
	Query             string   `json:"query"`
	Scope             string   `json:"scope"`
	ExpectedDocuments []string `json:"expected_documents"`
	ExpectedError     string   `json:"expected_error,omitempty"`
	FactDocument      string   `json:"fact_document,omitempty"`
	FactAnchors       []string `json:"fact_anchors,omitempty"`
}

func TestCoreKeywordBoundaryContract(t *testing.T) {
	contract, raw := readBoundaryContract(t)
	if contract.SchemaVersion != 1 || contract.BaselineCommit != boundaryBaseline || contract.ProductBoundary != boundaryName {
		t.Fatalf("contract identity = version %d, baseline %q, boundary %q",
			contract.SchemaVersion, contract.BaselineCommit, contract.ProductBoundary)
	}
	for _, forbidden := range []string{
		`"selection"`, `"ranking"`, `"postings"`, `"tokenizer"`,
		`"migration"`, `"relational"`, `"hybrid"`, `"embedding"`, `"vector"`,
	} {
		if bytes.Contains(bytes.ToLower(raw), []byte(forbidden)) {
			t.Fatalf("boundary contract contains candidate field %s", forbidden)
		}
	}

	collections := make(map[string]struct{}, len(contract.Collections))
	for _, collection := range contract.Collections {
		if collection.Key == "" || collection.Name == "" || strings.TrimSpace(collection.Name) != collection.Name {
			t.Fatalf("invalid collection: %#v", collection)
		}
		if _, duplicate := collections[collection.Key]; duplicate {
			t.Fatalf("duplicate collection key %q", collection.Key)
		}
		collections[collection.Key] = struct{}{}
	}
	if len(collections) != 3 {
		t.Fatalf("collection count = %d, want 3", len(collections))
	}

	documents := make(map[string]boundaryDocument, len(contract.Documents))
	states := make(map[string]int)
	for _, document := range contract.Documents {
		if document.Key == "" || document.Title == "" || document.Filename == "" || document.Content == "" {
			t.Fatalf("incomplete document: %#v", document)
		}
		if _, duplicate := documents[document.Key]; duplicate {
			t.Fatalf("duplicate document key %q", document.Key)
		}
		if !slices.Contains([]string{"active", "trashed", "inactive"}, document.State) {
			t.Fatalf("document %q state = %q", document.Key, document.State)
		}
		if document.State == "inactive" && len(document.Collections) != 0 {
			t.Fatalf("inactive document %q invents membership", document.Key)
		}
		for _, key := range document.Collections {
			if _, exists := collections[key]; !exists {
				t.Fatalf("document %q references collection %q", document.Key, key)
			}
		}
		documents[document.Key] = document
		states[document.State]++
	}
	if len(documents) != 4 || states["active"] != 2 || states["trashed"] != 1 || states["inactive"] != 1 {
		t.Fatalf("document states = %#v", states)
	}

	classes := make(map[string]int)
	probeIDs := make(map[string]struct{}, len(contract.Probes))
	for _, probe := range contract.Probes {
		if probe.ID == "" || probe.Query == "" {
			t.Fatalf("incomplete probe: %#v", probe)
		}
		if _, duplicate := probeIDs[probe.ID]; duplicate {
			t.Fatalf("duplicate probe %q", probe.ID)
		}
		probeIDs[probe.ID] = struct{}{}
		if probe.Scope != "global" {
			if _, exists := collections[probe.Scope]; !exists {
				t.Fatalf("probe %q references scope %q", probe.ID, probe.Scope)
			}
		}
		switch probe.Class {
		case "supported_exact":
			if utf8.RuneCountInString(probe.Query) < 3 || len(probe.ExpectedDocuments) == 0 || probe.ExpectedError != "" {
				t.Fatalf("invalid supported probe: %#v", probe)
			}
			for _, key := range probe.ExpectedDocuments {
				document, exists := documents[key]
				if !exists || document.State != "active" || !strings.Contains(document.Content, probe.Query) {
					t.Fatalf("probe %q expected document %q is not an active source-positive control", probe.ID, key)
				}
			}
		case "scope_exclusion", "lifecycle_exclusion":
			if len(probe.ExpectedDocuments) != 0 || probe.ExpectedError != "" {
				t.Fatalf("invalid exclusion probe: %#v", probe)
			}
		case "natural_question_limitation":
			document, exists := documents[probe.FactDocument]
			if !exists || len(probe.ExpectedDocuments) != 0 || len(probe.FactAnchors) == 0 || strings.Contains(document.Content, probe.Query) {
				t.Fatalf("invalid natural-question limitation: %#v", probe)
			}
			for _, anchor := range probe.FactAnchors {
				if !strings.Contains(document.Content, anchor) {
					t.Fatalf("probe %q fact lacks anchor %q", probe.ID, anchor)
				}
			}
		case "unsupported_short":
			if utf8.RuneCountInString(probe.Query) != 2 || probe.ExpectedError != "QUERY_TOO_SHORT" || len(probe.ExpectedDocuments) != 0 {
				t.Fatalf("invalid short probe: %#v", probe)
			}
		default:
			t.Fatalf("probe %q class = %q", probe.ID, probe.Class)
		}
		classes[probe.Class]++
	}
	if len(contract.Probes) != 8 || classes["supported_exact"] != 3 || classes["scope_exclusion"] != 1 ||
		classes["lifecycle_exclusion"] != 2 || classes["natural_question_limitation"] != 1 || classes["unsupported_short"] != 1 {
		t.Fatalf("probe classes = %#v", classes)
	}
}

func TestCoreKeywordBoundaryUsesProductionSearch(t *testing.T) {
	contract, _ := readBoundaryContract(t)
	fixture := newBoundaryFixture(t, contract)
	for _, probe := range contract.Probes {
		probe := probe
		t.Run(probe.ID, func(t *testing.T) {
			var (
				hits []store.ChunkHit
				err  error
			)
			if probe.Scope == "global" {
				hits, err = fixture.bench.Search(t.Context(), probe.Query, 100)
			} else {
				hits, err = fixture.bench.SearchCollection(t.Context(), fixture.collectionIDs[probe.Scope], probe.Query, 100)
			}
			if probe.ExpectedError == "QUERY_TOO_SHORT" {
				if !errors.Is(err, store.ErrQueryTooShort) || len(hits) != 0 {
					t.Fatalf("short-query result = %#v, err=%v", hits, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := fixture.documentKeys(t, hits)
			want := append([]string(nil), probe.ExpectedDocuments...)
			sort.Strings(want)
			if !slices.Equal(got, want) {
				t.Fatalf("documents = %v, want %v; hits=%#v", got, want, hits)
			}
		})
	}
}

type boundaryFixture struct {
	bench         *workbench.Service
	documentIDs   map[string]string
	documentByID  map[string]string
	collectionIDs map[string]string
}

func newBoundaryFixture(t *testing.T, contract boundaryContract) *boundaryFixture {
	t.Helper()
	root := t.TempDir()
	blobs, err := blob.OpenStore(filepath.Join(root, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), filepath.Join(root, "mindweaver.db"), store.Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	bench, err := workbench.New(database, blobs)
	if err != nil {
		t.Fatal(err)
	}
	lifecycleService, err := lifecycle.New(database, blobs)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &boundaryFixture{
		bench: bench, documentIDs: make(map[string]string), documentByID: make(map[string]string), collectionIDs: make(map[string]string),
	}
	for _, collection := range contract.Collections {
		created, createErr := bench.CreateCollection(t.Context(), collection.Name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		fixture.collectionIDs[collection.Key] = created.ID
	}

	// The inactive control is queued last so every active/trashed control uses
	// the real ingestion worker before the final source deliberately remains
	// without an active revision.
	for _, state := range []string{"active", "trashed", "inactive"} {
		for _, document := range contract.Documents {
			if document.State != state {
				continue
			}
			fixture.addDocument(t, lifecycleService, document)
		}
	}
	return fixture
}

func (fixture *boundaryFixture) addDocument(t *testing.T, lifecycleService *lifecycle.Service, document boundaryDocument) {
	t.Helper()
	upload, err := fixture.bench.Upload(t.Context(), workbench.UploadRequest{
		IdempotencyKey: "qualification-keyword-d61a-" + document.Key,
		Title:          document.Title, Filename: document.Filename, Source: strings.NewReader(document.Content),
	})
	if err != nil || !upload.Created {
		t.Fatalf("upload %q = %#v, err=%v", document.Key, upload, err)
	}
	fixture.documentIDs[document.Key] = upload.DocumentID
	fixture.documentByID[upload.DocumentID] = document.Key
	if document.State == "inactive" {
		projection, getErr := fixture.bench.GetDocument(t.Context(), upload.DocumentID)
		if getErr != nil || projection.ActiveRevisionID != "" || projection.IngestionStatus != store.JobQueued {
			t.Fatalf("inactive control = %#v, err=%v", projection, getErr)
		}
		return
	}
	job, runErr := fixture.bench.RunOne(t.Context(), "qualification-keyword-worker", time.Minute)
	if runErr != nil || job.ID != upload.JobID || job.Status != store.JobSucceeded {
		t.Fatalf("ingest %q = %#v, err=%v", document.Key, job, runErr)
	}
	projection, getErr := fixture.bench.GetDocument(t.Context(), upload.DocumentID)
	if getErr != nil || projection.ActiveRevisionID != upload.RevisionID {
		t.Fatalf("active projection %q = %#v, err=%v", document.Key, projection, getErr)
	}
	for _, key := range document.Collections {
		if addErr := fixture.bench.AddDocumentToCollection(t.Context(), fixture.collectionIDs[key], upload.DocumentID); addErr != nil {
			t.Fatalf("add %q to %q: %v", document.Key, key, addErr)
		}
	}
	if document.PreTransitionQuery != "" {
		hits, searchErr := fixture.bench.Search(t.Context(), document.PreTransitionQuery, 100)
		if searchErr != nil || !containsDocument(hits, upload.DocumentID) {
			t.Fatalf("pre-trash control %q = %#v, err=%v", document.Key, hits, searchErr)
		}
	}
	if document.State == "trashed" {
		trashed, trashErr := lifecycleService.TrashExpected(t.Context(), upload.DocumentID, projection.Revision)
		if trashErr != nil || trashed.Status != "trashed" {
			t.Fatalf("trash %q = %#v, err=%v", document.Key, trashed, trashErr)
		}
	}
}

func (fixture *boundaryFixture) documentKeys(t *testing.T, hits []store.ChunkHit) []string {
	t.Helper()
	set := make(map[string]struct{}, len(hits))
	for _, hit := range hits {
		key, exists := fixture.documentByID[hit.DocumentID]
		if !exists {
			t.Fatalf("unknown document id %q", hit.DocumentID)
		}
		set[key] = struct{}{}
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func containsDocument(hits []store.ChunkHit, documentID string) bool {
	for _, hit := range hits {
		if hit.DocumentID == documentID {
			return true
		}
	}
	return false
}

func readBoundaryContract(t *testing.T) (boundaryContract, []byte) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate boundary qualification")
	}
	path := filepath.Join(filepath.Dir(currentFile), "..", "..", "testdata", "qualification", "knowledge", "core-keyword-boundary.d61a.v1.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if got := hex.EncodeToString(digest[:]); got != boundaryCorpusSHA {
		t.Fatalf("corpus SHA-256 = %s, want %s", got, boundaryCorpusSHA)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var contract boundaryContract
	if err := decoder.Decode(&contract); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("contract contains trailing JSON: %v", err)
	}
	return contract, raw
}
