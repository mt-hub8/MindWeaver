package ingestion

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type runGolden struct {
	Spec      Spec `json:"spec"`
	Scenarios []struct {
		Name       string `json:"name"`
		Operations []struct {
			Kind       string     `json:"kind"`
			Fence      uint64     `json:"fence"`
			Checkpoint Checkpoint `json:"checkpoint"`
			WantError  string     `json:"want_error"`
		} `json:"operations"`
		Want struct {
			Status     Status     `json:"status"`
			Checkpoint Checkpoint `json:"checkpoint"`
			Revision   uint64     `json:"revision"`
			Attempt    uint32     `json:"attempt"`
			Fence      uint64     `json:"fence"`
		} `json:"want"`
	} `json:"scenarios"`
}

func TestRunBoundariesGolden(t *testing.T) {
	fixtureBytes, err := os.ReadFile("testdata/golden/run_boundaries.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture runGolden
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range fixture.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			run, err := New(fixture.Spec)
			if err != nil {
				t.Fatal(err)
			}
			for _, operation := range scenario.Operations {
				before := run
				var got Run
				switch operation.Kind {
				case "start":
					got, err = run.Start(operation.Fence)
				case "advance":
					got, err = run.Advance(operation.Fence, operation.Checkpoint)
				case "request_cancel":
					got, err = run.RequestCancel()
				case "ack_cancel":
					got, err = run.AcknowledgeCancel(operation.Fence)
				case "commit":
					got, err = run.CommitSuccess(operation.Fence)
				case "duplicate_text":
					got, err = run.MarkDuplicate(operation.Fence, Duplicate{Kind: DuplicateText, CanonicalDocumentID: "doc-canonical"})
				case "fail":
					got, err = run.Fail(operation.Fence, Failure{Code: "EXTRACT_FAILED", Message: "fixture"})
				default:
					t.Fatalf("unknown operation %q", operation.Kind)
				}
				if operation.WantError != "" {
					if err == nil || !strings.Contains(err.Error(), operation.WantError) {
						t.Fatalf("error=%v, want substring %q", err, operation.WantError)
					}
					if run.Revision != before.Revision {
						t.Fatal("failed transition mutated run")
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				run = got
			}
			if run.Status != scenario.Want.Status || run.Checkpoint != scenario.Want.Checkpoint ||
				run.Revision != scenario.Want.Revision || run.Attempt != scenario.Want.Attempt || run.Fence != scenario.Want.Fence {
				t.Fatalf("run=(%s,%s,%d,%d,%d), want (%s,%s,%d,%d,%d)", run.Status, run.Checkpoint, run.Revision, run.Attempt, run.Fence, scenario.Want.Status, scenario.Want.Checkpoint, scenario.Want.Revision, scenario.Want.Attempt, scenario.Want.Fence)
			}
		})
	}
}

func TestReindexSpecRequiresExpectedActiveGeneration(t *testing.T) {
	spec := Spec{
		ID: "run-r", IdempotencyKey: "r", InputArtifactID: "text", InputFingerprint: strings.Repeat("a", 64),
		Kind: KindReindex, DocumentID: "doc", TargetGeneration: 2,
	}
	if _, err := New(spec); err == nil {
		t.Fatal("reindex without expected active generation must fail")
	}
}
