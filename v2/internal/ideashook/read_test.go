package ideashook

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectIdentityPreservesCaseAndCleansLexicalPath(t *testing.T) {
	key := bytes.Repeat([]byte{0x5a}, spoolKeyBytes)
	parent := t.TempDir()
	upper := filepath.Join(parent, "Repo")
	lower := filepath.Join(parent, "repo")
	if projectIDForWorkingDirectory(key, upper) == projectIDForWorkingDirectory(key, lower) {
		t.Fatal("case-distinct working directories collided")
	}
	if projectIDForWorkingDirectory(key, filepath.Join(upper, ".")) != projectIDForWorkingDirectory(key, upper) {
		t.Fatal("lexically equivalent clean paths diverged")
	}
}

func TestCurrentSessionSelectionRequiresOneOpenVisibleExactProjectMatch(t *testing.T) {
	projectA := "sha256:" + strings.Repeat("a", 64)
	projectB := "sha256:" + strings.Repeat("b", 64)
	message := func(eventCharacter, sessionCharacter, projectID, content string) spoolRecord {
		return spoolRecord{
			EventID:   "sha256:" + strings.Repeat(eventCharacter, 64),
			SessionID: "sha256:" + strings.Repeat(sessionCharacter, 64),
			ProjectID: projectID, Kind: "message", HookEvent: "UserPromptSubmit",
			Role: "user", Content: content,
		}
	}
	closed := capturedSession{
		summary: SessionSummary{SessionID: "sha256:" + strings.Repeat("c", 64), VisibleTurnCount: 1},
		records: []spoolRecord{
			message("d", "c", projectA, "我的想法是已关闭会话。"),
			{ProjectID: projectA, HookEvent: "SessionEnd", Kind: "boundary"},
		},
	}
	otherProject := capturedSession{
		summary: SessionSummary{SessionID: "sha256:" + strings.Repeat("e", 64), VisibleTurnCount: 1},
		records: []spoolRecord{message("f", "e", projectB, "我的想法是其他项目。")},
	}
	current := capturedSession{
		summary: SessionSummary{SessionID: "sha256:" + strings.Repeat("1", 64), VisibleTurnCount: 1},
		records: []spoolRecord{message("2", "1", projectA, "我的想法是当前项目。")},
	}

	request, err := requestForCurrentSessionFromSessions([]capturedSession{closed, otherProject, current}, projectA)
	if err != nil {
		t.Fatal(err)
	}
	if request.SessionID != current.summary.SessionID || len(request.Turns) != 1 || request.Turns[0].Text != "我的想法是当前项目。" {
		t.Fatalf("request=%+v", request)
	}

	ambiguous := current
	ambiguous.summary.SessionID = "sha256:" + strings.Repeat("3", 64)
	ambiguous.records = []spoolRecord{message("4", "3", projectA, "我的想法是并行任务。")}
	if _, err := requestForCurrentSessionFromSessions([]capturedSession{current, ambiguous}, projectA); CodeOf(err) != CodeIdentityConflict {
		t.Fatalf("ambiguous err=%v code=%s", err, CodeOf(err))
	}
	if _, err := requestForCurrentSessionFromSessions([]capturedSession{closed, otherProject}, projectA); CodeOf(err) != CodeNotFound {
		t.Fatalf("missing err=%v code=%s", err, CodeOf(err))
	}
}
