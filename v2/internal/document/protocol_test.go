package document

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

type documentGolden struct {
	Initial State `json:"initial"`
	Steps   []struct {
		Name        string  `json:"name"`
		Command     Command `json:"command"`
		Retrievable bool    `json:"retrievable"`
		Want        State   `json:"want"`
		WantError   string  `json:"want_error"`
	} `json:"steps"`
}

func TestDocumentLifecycleGolden(t *testing.T) {
	fixtureBytes, err := os.ReadFile("testdata/golden/document_lifecycle.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture documentGolden
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatal(err)
	}
	created, err := New(fixture.Initial.ID, fixture.Initial.Lineage)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(created, fixture.Initial) {
		t.Fatalf("constructor drifted from golden:\n got: %#v\nwant: %#v", created, fixture.Initial)
	}

	state := fixture.Initial
	for _, step := range fixture.Steps {
		t.Run(step.Name, func(t *testing.T) {
			before := state
			got, err := state.Apply(step.Command)
			if step.WantError != "" {
				if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(step.WantError)) {
					t.Fatalf("got error %v, want substring %q", err, step.WantError)
				}
				if !reflect.DeepEqual(state, before) {
					t.Fatal("failed transition mutated source state")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, step.Want) {
				t.Fatalf("state drifted from golden:\n got: %#v\nwant: %#v", got, step.Want)
			}
			if got.Retrievable() != step.Retrievable {
				t.Fatalf("Retrievable()=%v, want %v", got.Retrievable(), step.Retrievable)
			}
			state = got
		})
	}
}

func TestBlobLineageRejectsMutableIdentity(t *testing.T) {
	lineage := BlobLineage{
		Source:    BlobRef{ID: "source", SHA256: strings.Repeat("A", 64)},
		Text:      BlobRef{ID: "text", SHA256: strings.Repeat("b", 64)},
		Extractor: "txt@1",
	}
	if _, err := New("doc", lineage); err == nil {
		t.Fatal("uppercase/non-canonical source digest must be rejected")
	}
}
