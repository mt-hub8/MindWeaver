package collection

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

type collectionGolden struct {
	Collections []string     `json:"collections"`
	Assignments []Membership `json:"assignments"`
	WantState   State        `json:"want_state"`
	Scopes      []struct {
		Name                string        `json:"name"`
		Request             ScopeRequest  `json:"request"`
		EligibleDocumentIDs []string      `json:"eligible_document_ids"`
		Want                ScopeSnapshot `json:"want"`
		WantError           string        `json:"want_error"`
	} `json:"scopes"`
	Removal struct {
		CollectionID               string   `json:"collection_id"`
		DocumentID                 string   `json:"document_id"`
		WantRevision               uint64   `json:"want_revision"`
		WantMembershipRevision     uint64   `json:"want_membership_revision"`
		WantCollectionsForDocument []string `json:"want_collections_for_document"`
		WantDocumentsInA           []string `json:"want_documents_in_a"`
	} `json:"removal"`
}

func TestManyToManyScopeGolden(t *testing.T) {
	fixtureBytes, err := os.ReadFile("testdata/golden/many_to_many_scope.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture collectionGolden
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatal(err)
	}
	state := New()
	for _, collectionID := range fixture.Collections {
		state, err = state.Create(collectionID)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, assignment := range fixture.Assignments {
		state, err = state.Assign(assignment.CollectionID, assignment.DocumentID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(state, fixture.WantState) {
		t.Fatalf("membership state drifted:\n got: %#v\nwant: %#v", state, fixture.WantState)
	}

	for _, scope := range fixture.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			got, err := state.Resolve(scope.Request, scope.EligibleDocumentIDs)
			if scope.WantError != "" {
				if err == nil || !strings.Contains(err.Error(), scope.WantError) {
					t.Fatalf("error=%v, want substring %q", err, scope.WantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, scope.Want) {
				t.Fatalf("scope drifted:\n got: %#v\nwant: %#v", got, scope.Want)
			}
		})
	}

	state, err = state.Remove(fixture.Removal.CollectionID, fixture.Removal.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	state, err = state.Remove(fixture.Removal.CollectionID, fixture.Removal.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Revision != fixture.Removal.WantRevision ||
		state.MembershipRevision != fixture.Removal.WantMembershipRevision {
		t.Fatalf("removal revisions=(%d,%d), want (%d,%d)", state.Revision, state.MembershipRevision, fixture.Removal.WantRevision, fixture.Removal.WantMembershipRevision)
	}
	if !reflect.DeepEqual(state.CollectionsFor(fixture.Removal.DocumentID), fixture.Removal.WantCollectionsForDocument) ||
		!reflect.DeepEqual(state.DocumentsIn(fixture.Removal.CollectionID), fixture.Removal.WantDocumentsInA) {
		t.Fatal("removing one edge damaged another collection membership")
	}
}
