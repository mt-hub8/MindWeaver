package indexing

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type generationGolden struct {
	DocumentID         string `json:"document_id"`
	SourceArtifactID   string `json:"source_artifact_id"`
	SourceTextSHA256   string `json:"source_text_sha256"`
	ChunkerFingerprint string `json:"chunker_fingerprint"`
	IndexFingerprint   string `json:"index_fingerprint"`
	ChunkManifest      string `json:"chunk_manifest_sha256"`
	IndexManifest      string `json:"index_manifest_sha256"`
	Scenarios          []struct {
		Name       string `json:"name"`
		Operations []struct {
			Kind           string `json:"kind"`
			Number         uint64 `json:"number"`
			Chunks         uint32 `json:"chunks"`
			Indexed        uint32 `json:"indexed"`
			ExpectedActive uint64 `json:"expected_active"`
			ChunkManifest  string `json:"chunk_manifest"`
			IndexManifest  string `json:"index_manifest"`
			WantError      string `json:"want_error"`
		} `json:"operations"`
		WantRevision    uint64 `json:"want_revision"`
		WantActive      uint64 `json:"want_active"`
		WantGenerations []struct {
			Number   uint64          `json:"number"`
			State    GenerationState `json:"state"`
			Revision uint64          `json:"revision"`
			Chunks   uint32          `json:"chunks"`
			Indexed  uint32          `json:"indexed"`
		} `json:"want_generations"`
	} `json:"scenarios"`
}

func TestGenerationTransitionsGolden(t *testing.T) {
	fixtureBytes, err := os.ReadFile("testdata/golden/generation_transitions.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture generationGolden
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatal(err)
	}

	for _, scenario := range fixture.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			state, err := NewSet(fixture.DocumentID)
			if err != nil {
				t.Fatal(err)
			}
			for _, operation := range scenario.Operations {
				before := state
				var got Set
				switch operation.Kind {
				case "reserve":
					got, err = state.Reserve(operation.Number, fixture.SourceArtifactID, fixture.SourceTextSHA256, fixture.ChunkerFingerprint, fixture.IndexFingerprint)
				case "start":
					got, err = state.Start(operation.Number)
				case "progress":
					chunkManifest := operation.ChunkManifest
					if chunkManifest == "" {
						chunkManifest = fixture.ChunkManifest
					}
					indexManifest := operation.IndexManifest
					if indexManifest == "" {
						indexManifest = fixture.IndexManifest
					}
					got, err = state.RecordProgress(operation.Number, operation.Chunks, chunkManifest, operation.Indexed, indexManifest)
				case "ready":
					got, err = state.MarkReady(operation.Number)
				case "activate":
					got, err = state.Activate(operation.Number, operation.ExpectedActive)
				case "fail":
					got, err = state.Fail(operation.Number)
				case "cancel":
					got, err = state.Cancel(operation.Number)
				default:
					t.Fatalf("unknown golden operation %q", operation.Kind)
				}
				if operation.WantError != "" {
					if err == nil || !strings.Contains(err.Error(), operation.WantError) {
						t.Fatalf("%s(%d) error=%v, want substring %q", operation.Kind, operation.Number, err, operation.WantError)
					}
					if state.Revision != before.Revision {
						t.Fatal("failed transition mutated generation set")
					}
					continue
				}
				if err != nil {
					t.Fatalf("%s(%d): %v", operation.Kind, operation.Number, err)
				}
				state = got
			}

			if state.Revision != scenario.WantRevision || state.ActiveGeneration != scenario.WantActive {
				t.Fatalf("set revision/active=(%d,%d), want (%d,%d)", state.Revision, state.ActiveGeneration, scenario.WantRevision, scenario.WantActive)
			}
			if len(state.Generations) != len(scenario.WantGenerations) {
				t.Fatalf("got %d generations, want %d", len(state.Generations), len(scenario.WantGenerations))
			}
			for i, want := range scenario.WantGenerations {
				got := state.Generations[i]
				if got.Number != want.Number || got.State != want.State || got.Revision != want.Revision ||
					got.ChunkCount != want.Chunks || got.IndexedChunkCount != want.Indexed {
					t.Fatalf("generation[%d]=%#v, want %#v", i, got, want)
				}
			}
		})
	}
}

func TestCollectionNeverEntersGenerationIdentity(t *testing.T) {
	set, err := NewSet("doc-shared")
	if err != nil {
		t.Fatal(err)
	}
	set, err = set.Reserve(1, "blob-text", strings.Repeat("c", 64), "txt/v1", "lexical/v1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(set.Generations[0].DocumentID), "collection") {
		t.Fatal("generation identity must remain document-scoped")
	}
}
