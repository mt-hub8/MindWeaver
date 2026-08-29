package runtimequalification_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const committedRuntimeEvidenceSHA256 = "ba1b4088933599db779b0699a8b5fc6b8fa3edd85d47225e8585227e214e46f5"

const (
	committedRuntimeEvidenceCommit = "6c76b3d25a9bb8c30baeab989e03174fe9f90833"
	committedMindWeaverSHA256      = "8af2fb0eb8e5c35f0e9e4474351eed9962d805ca4b1648871a549c4fd5f01329"
	committedPDFBlockerSHA256      = "766493d8a50f09ab909dbceea63855995b6c6c70d1dbff46e7779e0a76c1f1cb"
)

func TestCommittedRuntimeInterruptionEvidenceContract(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "testdata", "qualification", "runtime", "evidence-v1.json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 64<<10 {
		t.Fatalf("runtime evidence must be a regular non-link file within 64 KiB: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || int64(len(raw)) != info.Size() {
		t.Fatal("read complete runtime evidence")
	}
	digest := sha256.Sum256(raw)
	if got := hex.EncodeToString(digest[:]); got != committedRuntimeEvidenceSHA256 {
		t.Fatalf("runtime evidence SHA-256 = %s, want %s", got, committedRuntimeEvidenceSHA256)
	}

	report, err := decodeCanonicalEvidence(raw)
	if err != nil {
		t.Fatalf("decode strict runtime evidence: %v", err)
	}
	if report.SchemaVersion != 1 || report.Qualification != "mindweaver.runtime-interruption/v1" ||
		report.TestedCommit != committedRuntimeEvidenceCommit || report.Binary.Name != "mindweaver.exe" ||
		report.Binary.Toolchain != "go version go1.27.0 windows/amd64" || report.Binary.GOOS != "windows" ||
		report.Binary.GOARCH != "amd64" || report.Binary.SHA256 != committedMindWeaverSHA256 ||
		report.Fixture.Role != "qualification-only-pdf-blocker" || report.Fixture.SHA256 != committedPDFBlockerSHA256 {
		t.Fatal("runtime evidence top-level identity is outside the frozen contract")
	}

	want := []struct {
		protocolCase
		checkpoint evidenceCounts
		final      evidenceCounts
	}{
		{
			protocolCase: protocolCase{CaseCode: "ANSWER_PROVIDER_RECEIVED_BEFORE_TERMINAL", CheckpointCode: "PROVIDER_REQUEST_RECEIVED_ANSWER_PENDING", FinalCode: "ANSWER_FAILED_OUTCOME_UNCERTAIN"},
			checkpoint: evidenceCounts{
				"answer_citations": 0, "answer_sources": 1, "ask_requests": 1, "chunks": 1,
				"conversations": 1, "distinct_blobs": 1, "documents": 1, "fts_rows": 1,
				"fts_term_matches": 1, "messages": 2, "provider_attempts": 1,
				"reconciled_answers": 0, "revisions": 1,
			},
			final: evidenceCounts{
				"answer_citations": 0, "answer_sources": 1, "ask_requests": 1, "chunks": 1,
				"conversations": 1, "distinct_blobs": 1, "documents": 1, "fts_rows": 1,
				"fts_term_matches": 1, "messages": 2, "provider_attempts": 1,
				"reconciled_answers": 1, "revisions": 1,
			},
		},
		{
			protocolCase: protocolCase{CaseCode: "INGESTION_ACCEPTED_WHILE_WORKER_OCCUPIED", CheckpointCode: "BLOCKER_RUNNING_TARGET_QUEUED", FinalCode: "TARGET_SUCCEEDED_SEARCHABLE_NO_DUPLICATES"},
			checkpoint: evidenceCounts{
				"active_revisions": 0, "blob_files": 2, "blocker_attempts": 1,
				"blocker_fts_term_matches": 0, "chunks": 0, "distinct_blobs": 2,
				"documents": 2, "duplicate_chunk_slots": 0, "fts_rows": 0, "ingestions": 2,
				"jobs": 2, "recovered_jobs": 0, "revisions": 2, "target_attempts": 0,
				"target_chunks": 0, "target_fts_rows": 0, "target_fts_term_matches": 0,
				"terminal_jobs": 0,
			},
			final: evidenceCounts{
				"active_revisions": 2, "blob_files": 2, "blocker_attempts": 2,
				"blocker_fts_term_matches": 1, "chunks": 2, "distinct_blobs": 2,
				"documents": 2, "duplicate_chunk_slots": 0, "fts_rows": 2, "ingestions": 2,
				"jobs": 2, "recovered_jobs": 1, "revisions": 2, "target_attempts": 1,
				"target_chunks": 1, "target_fts_rows": 1, "target_fts_term_matches": 1,
				"terminal_jobs": 2,
			},
		},
	}
	if len(report.Cases) != len(want) {
		t.Fatalf("runtime evidence cases = %d, want %d", len(report.Cases), len(want))
	}
	for caseIndex, contract := range want {
		observed := report.Cases[caseIndex]
		if observed.CaseCode != contract.CaseCode || observed.Repetitions != 10 || len(observed.Runs) != 10 {
			t.Fatalf("runtime case %d is incomplete", caseIndex)
		}
		seenRunIdentity := make(map[string]struct{}, len(observed.Runs))
		for runIndex, run := range observed.Runs {
			if run.Run != runIndex+1 || run.Checkpoint.Code != contract.CheckpointCode ||
				run.Final.Code != contract.FinalCode || !equalCounts(run.Checkpoint.Counts, contract.checkpoint) ||
				!equalCounts(run.Final.Counts, contract.final) {
				t.Fatalf("runtime case %d run %d violates the frozen state contract", caseIndex, runIndex+1)
			}
			assertEvidenceHashes(t, caseIndex, run.Checkpoint.IDHashes)
			assertEvidenceHashes(t, caseIndex, run.Final.IDHashes)
			if run.Checkpoint.IDHashes != run.Final.IDHashes {
				t.Fatalf("runtime case %d run %d changed durable identities", caseIndex, runIndex+1)
			}
			if caseIndex == 1 && run.Final.IDHashes.Blob == run.Final.IDHashes.BlockerBlob {
				t.Fatalf("runtime case %d run %d collapsed distinct source blobs", caseIndex, runIndex+1)
			}
			runIdentity := run.Final.IDHashes.Answer
			if caseIndex == 1 {
				runIdentity = run.Final.IDHashes.Job
			}
			if _, duplicate := seenRunIdentity[runIdentity]; duplicate {
				t.Fatalf("runtime case %d reused one durable identity across runs", caseIndex)
			}
			seenRunIdentity[runIdentity] = struct{}{}
		}
	}

	text := string(raw)
	for _, forbidden := range []string{"http://", "https://", `:\\`, "runtime-model", "interruptionanswer", "interruptiontarget"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("runtime evidence contains forbidden raw category %q", forbidden)
		}
	}
}

func TestRuntimeInterruptionEvidenceRejectsDuplicateKeys(t *testing.T) {
	duplicate := []byte("{\n  \"schema_version\": 1,\n  \"schema_version\": 1\n}\n")
	if _, err := decodeCanonicalEvidence(duplicate); err == nil {
		t.Fatal("runtime evidence accepted a duplicate JSON key")
	}
}

func TestRuntimeInterruptionEvidenceCountsRequireExactKeys(t *testing.T) {
	want := evidenceCounts{"answer_citations": 0, "provider_attempts": 1}
	forged := evidenceCounts{"forged": 999, "provider_attempts": 1}
	if equalCounts(forged, want) {
		t.Fatal("runtime evidence accepted a missing zero-valued count plus an unknown key")
	}
}

func decodeCanonicalEvidence(raw []byte) (qualificationReport, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var report qualificationReport
	if err := decoder.Decode(&report); err != nil {
		return qualificationReport{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return qualificationReport{}, errors.New("runtime evidence contains trailing JSON")
	}
	canonical, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return qualificationReport{}, err
	}
	canonical = append(canonical, '\n')
	if !bytes.Equal(raw, canonical) {
		return qualificationReport{}, errors.New("runtime evidence is not the canonical JSON encoding")
	}
	return report, nil
}

func equalCounts(got, want evidenceCounts) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		gotValue, exists := got[key]
		if !exists || gotValue != value {
			return false
		}
	}
	return true
}

func assertEvidenceHashes(t *testing.T, caseIndex int, hashes evidenceHashes) {
	t.Helper()
	values := []string{
		hashes.Conversation, hashes.Answer, hashes.SourceSet, hashes.Document,
		hashes.Revision, hashes.Job, hashes.Blob, hashes.BlockerBlob,
	}
	wantNonEmpty := 6
	if caseIndex == 1 {
		wantNonEmpty = 5
	}
	nonEmpty := 0
	for _, value := range values {
		if value != "" {
			nonEmpty++
		}
		if value != "" && !lowerHex(value, 64) {
			t.Fatal("runtime evidence contains a non-canonical identifier hash")
		}
	}
	if nonEmpty != wantNonEmpty {
		t.Fatalf("runtime evidence case %d has %d identity hashes, want %d", caseIndex, nonEmpty, wantNonEmpty)
	}
	if caseIndex == 0 && (hashes.Blob != "" || hashes.BlockerBlob != "") {
		t.Fatal("answer evidence contains ingestion-only identities")
	}
	if caseIndex == 1 && (hashes.Conversation != "" || hashes.Answer != "" || hashes.SourceSet != "") {
		t.Fatal("ingestion evidence contains answer-only identities")
	}
}

func lowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
