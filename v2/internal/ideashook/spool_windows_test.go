//go:build windows

package ideashook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/sessiondistill"
	"golang.org/x/sys/windows"
)

func TestCaptureStoresRedactedVisibleRootEventsAndExactReplay(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	cwd := t.TempDir()
	inputs := []string{
		hookSessionStart("s1", cwd),
		hookPrompt("s1", "t1", cwd, "我的想法 password=private-value 应该保留。"),
		hookStop("s1", "t1", cwd, "最终回答"),
		hookStop("s1", "t1", cwd, "修订后的最终回答"),
		hookSessionEnd("s1", cwd),
	}
	for _, input := range inputs {
		captureJSONAtRoot(t, root, input)
	}
	captureJSONAtRoot(t, root, inputs[1])

	records, data := readOnlyCapturedRecords(t, root, "s1", cwd)
	if len(records) != 5 {
		t.Fatalf("records=%#v", records)
	}
	for index, record := range records {
		if record.Ordinal != uint32(index+1) || record.SessionID == "s1" || record.TurnID == "t1" || record.ProjectID == cwd {
			t.Fatalf("raw identifier leaked: %+v", record)
		}
	}
	joined := strings.Join(data, "\n")
	for _, forbidden := range []string{"private-value", cwd, `"dedupe_id"`, `"transcript_path"`, `"permission_mode"`, `"model"`} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("private metadata leaked: %q in %s", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "[redacted:credential]") {
		t.Fatalf("secret was not redacted before persistence: %s", joined)
	}
	if records[2].TurnID != records[3].TurnID || records[2].Content == records[3].Content {
		t.Fatalf("changed Stop was not retained separately: %#v", records)
	}
}

func TestCaptureAllowsPerEventProjectChangeWithinSession(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	cwdA, cwdB := t.TempDir(), t.TempDir()
	captureJSONAtRoot(t, root, hookPrompt("s1", "t1", cwdA, "我的想法是 A。"))
	captureJSONAtRoot(t, root, hookPrompt("s1", "t2", cwdB, "我的想法是 B。"))
	records, _ := readOnlyCapturedRecords(t, root, "s1", cwdA)
	if len(records) != 2 || records[0].ProjectID == records[1].ProjectID {
		t.Fatalf("project identity was forced across events: %#v", records)
	}
}

func TestRequestForSessionFiltersBoundariesAndRenumbersVisibleTurns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	cwd := t.TempDir()
	for _, input := range []string{
		hookSessionStart("s1", cwd),
		hookPrompt("s1", "t1", cwd, "我的想法是先做 CLI。"),
		hookStop("s1", "t1", cwd, "我的想法是先用规则。"),
		hookSessionEnd("s1", cwd),
	} {
		captureJSONAtRoot(t, root, input)
	}
	captureJSONAtRoot(t, root, hookPrompt("s2", "t2", cwd, "我的想法是另一会话。"))
	sessions, err := readCapturedSessionsAtRoot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	var selected string
	for _, session := range sessions {
		if session.summary.VisibleTurnCount == 2 {
			selected = session.summary.SessionID
		}
	}
	request, err := requestForSessionFromSessions(sessions, selected)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Turns) != 2 || request.Turns[0].Ordinal != 1 || request.Turns[1].Ordinal != 2 ||
		request.Turns[0].Role != "user" || request.Turns[1].Role != "assistant" {
		t.Fatalf("request=%+v", request)
	}
	bundle, err := sessiondistill.NewV1().Distill(context.Background(), request)
	result := bundle.Result()
	if err != nil || len(result.UserItems) != 1 || len(result.AssistantContext) != 1 {
		t.Fatalf("bundle=%+v err=%v", result, err)
	}
	reportDirectory := filepath.Join(t.TempDir(), "report")
	if err := sessiondistill.PublishBundle(context.Background(), reportDirectory, bundle); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"report.json", "report.md"} {
		if info, err := os.Stat(filepath.Join(reportDirectory, name)); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			t.Fatalf("published %s info=%v err=%v", name, info, err)
		}
	}
}

func TestRequestForCurrentSessionUsesExactProjectAndFailsClosedOnParallelOrSealedSessions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	currentCWD, otherCWD := t.TempDir(), t.TempDir()
	captureJSONAtRoot(t, root, hookSessionStart("current", currentCWD))
	captureJSONAtRoot(t, root, hookPrompt("current", "t1", currentCWD, "我的想法是当前任务。"))
	captureJSONAtRoot(t, root, hookPrompt("other", "t1", otherCWD, "我的想法是其他项目。"))

	request, err := requestForCurrentSessionAtRoot(context.Background(), root, currentCWD)
	if err != nil || len(request.Turns) != 1 || request.Turns[0].Text != "我的想法是当前任务。" {
		t.Fatalf("request=%+v err=%v", request, err)
	}

	captureJSONAtRoot(t, root, hookPrompt("parallel", "t1", currentCWD, "我的想法是并行任务。"))
	if _, err := requestForCurrentSessionAtRoot(context.Background(), root, currentCWD); CodeOf(err) != CodeIdentityConflict {
		t.Fatalf("parallel err=%v code=%s", err, CodeOf(err))
	}

	captureJSONAtRoot(t, root, hookSessionEnd("current", currentCWD))
	request, err = requestForCurrentSessionAtRoot(context.Background(), root, currentCWD)
	if err != nil || len(request.Turns) != 1 || request.Turns[0].Text != "我的想法是并行任务。" {
		t.Fatalf("remaining open request=%+v err=%v", request, err)
	}
	captureJSONAtRoot(t, root, hookSessionEnd("parallel", currentCWD))
	if _, err := requestForCurrentSessionAtRoot(context.Background(), root, currentCWD); CodeOf(err) != CodeNotFound {
		t.Fatalf("sealed err=%v code=%s", err, CodeOf(err))
	}
}

func TestCurrentSessionSelectionAndCaptureShareOneSpoolLock(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	currentCWD := t.TempDir()
	captureJSONAtRoot(t, root, hookPrompt("current", "t1", currentCWD, "我的想法是当前任务。"))

	selectionEntered := make(chan struct{})
	releaseSelection := make(chan struct{})
	selectionDone := make(chan error, 1)
	go func() {
		selectionDone <- inspectCapturedSessionsAtRoot(context.Background(), root, func(sessions []capturedSession, key []byte) error {
			if _, err := requestForCurrentSessionFromSessions(sessions, projectIDForWorkingDirectory(key, currentCWD)); err != nil {
				return err
			}
			close(selectionEntered)
			<-releaseSelection
			return nil
		})
	}()
	select {
	case <-selectionEntered:
	case <-time.After(time.Second):
		t.Fatal("selection did not reach its locked snapshot")
	}

	event, err := decodeHookInput(strings.NewReader(hookSessionEnd("current", currentCWD)))
	if err != nil {
		t.Fatal(err)
	}
	blockedContext, cancelBlocked := context.WithTimeout(context.Background(), 50*time.Millisecond)
	err = captureAtRoot(blockedContext, root, event, time.Now)
	cancelBlocked()
	if CodeOf(err) != CodeCanceled {
		t.Fatalf("capture did not block on the selection lock: %v code=%s", err, CodeOf(err))
	}
	close(releaseSelection)
	if err := <-selectionDone; err != nil {
		t.Fatal(err)
	}
	if err := captureAtRoot(context.Background(), root, event, time.Now); err != nil {
		t.Fatal(err)
	}
	if _, err := requestForCurrentSessionAtRoot(context.Background(), root, currentCWD); CodeOf(err) != CodeNotFound {
		t.Fatalf("post-seal err=%v code=%s", err, CodeOf(err))
	}
}

func TestCurrentSessionWaitsForCaptureCommitBeforeSelecting(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	currentCWD := t.TempDir()
	captureJSONAtRoot(t, root, hookPrompt("current", "t1", currentCWD, "我的想法是当前任务。"))

	captureEntered := make(chan struct{})
	releaseCapture := make(chan struct{})
	captureDone := make(chan error, 1)
	event, err := decodeHookInput(strings.NewReader(hookSessionEnd("current", currentCWD)))
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		captureDone <- captureAtRootWithOptions(context.Background(), root, event, time.Now, captureSpoolOptions{
			limits: productionSpoolAdmissionLimits(),
			hooks: captureSpoolHooks{afterSessionsRootRetained: func() error {
				close(captureEntered)
				<-releaseCapture
				return nil
			}},
		})
	}()
	select {
	case <-captureEntered:
	case <-time.After(time.Second):
		t.Fatal("capture did not reach its locked write")
	}

	blockedContext, cancelBlocked := context.WithTimeout(context.Background(), 50*time.Millisecond)
	_, blockedErr := requestForCurrentSessionAtRoot(blockedContext, root, currentCWD)
	cancelBlocked()
	if CodeOf(blockedErr) != CodeCanceled {
		t.Fatalf("current selection did not block on the capture lock: %v code=%s", blockedErr, CodeOf(blockedErr))
	}
	close(releaseCapture)
	if err := <-captureDone; err != nil {
		t.Fatal(err)
	}
	if _, err := requestForCurrentSessionAtRoot(context.Background(), root, currentCWD); CodeOf(err) != CodeNotFound {
		t.Fatalf("request err=%v code=%s", err, CodeOf(err))
	}
}

func TestCurrentSessionSecondOpenLinearizesOnBothSidesOfSelection(t *testing.T) {
	t.Run("selection first", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "spool")
		cwd := t.TempDir()
		captureJSONAtRoot(t, root, hookPrompt("first", "t1", cwd, "我的想法是第一个任务。"))
		entered := make(chan struct{})
		release := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- inspectCapturedSessionsAtRoot(context.Background(), root, func(sessions []capturedSession, key []byte) error {
				if _, err := requestForCurrentSessionFromSessions(sessions, projectIDForWorkingDirectory(key, cwd)); err != nil {
					return err
				}
				close(entered)
				<-release
				return nil
			})
		}()
		<-entered
		event, err := decodeHookInput(strings.NewReader(hookPrompt("second", "t1", cwd, "我的想法是第二个任务。")))
		if err != nil {
			t.Fatal(err)
		}
		blockedContext, cancelBlocked := context.WithTimeout(context.Background(), 50*time.Millisecond)
		err = captureAtRoot(blockedContext, root, event, time.Now)
		cancelBlocked()
		if CodeOf(err) != CodeCanceled {
			t.Fatalf("second open did not block: %v code=%s", err, CodeOf(err))
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if err := captureAtRoot(context.Background(), root, event, time.Now); err != nil {
			t.Fatal(err)
		}
		if _, err := requestForCurrentSessionAtRoot(context.Background(), root, cwd); CodeOf(err) != CodeIdentityConflict {
			t.Fatalf("parallel err=%v code=%s", err, CodeOf(err))
		}
	})

	t.Run("capture first", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "spool")
		cwd := t.TempDir()
		captureJSONAtRoot(t, root, hookPrompt("first", "t1", cwd, "我的想法是第一个任务。"))
		event, err := decodeHookInput(strings.NewReader(hookPrompt("second", "t1", cwd, "我的想法是第二个任务。")))
		if err != nil {
			t.Fatal(err)
		}
		entered := make(chan struct{})
		release := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- captureAtRootWithOptions(context.Background(), root, event, time.Now, captureSpoolOptions{
				limits: productionSpoolAdmissionLimits(),
				hooks: captureSpoolHooks{afterSessionsRootRetained: func() error {
					close(entered)
					<-release
					return nil
				}},
			})
		}()
		<-entered
		blockedContext, cancelBlocked := context.WithTimeout(context.Background(), 50*time.Millisecond)
		_, blockedErr := requestForCurrentSessionAtRoot(blockedContext, root, cwd)
		cancelBlocked()
		if CodeOf(blockedErr) != CodeCanceled {
			t.Fatalf("selection did not block: %v code=%s", blockedErr, CodeOf(blockedErr))
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if _, err := requestForCurrentSessionAtRoot(context.Background(), root, cwd); CodeOf(err) != CodeIdentityConflict {
			t.Fatalf("parallel err=%v code=%s", err, CodeOf(err))
		}
	})
}

func TestCurrentSessionUsesLatestEventProjectIdentityOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	firstCWD, latestCWD := t.TempDir(), t.TempDir()
	captureJSONAtRoot(t, root, hookPrompt("moving", "t1", firstCWD, "我的想法是第一个项目。"))
	captureJSONAtRoot(t, root, hookPrompt("moving", "t2", latestCWD, "我的想法是后来切换的项目。"))
	if _, err := requestForCurrentSessionAtRoot(context.Background(), root, firstCWD); CodeOf(err) != CodeNotFound {
		t.Fatalf("earlier project err=%v code=%s", err, CodeOf(err))
	}
	request, err := requestForCurrentSessionAtRoot(context.Background(), root, latestCWD)
	if err != nil || len(request.Turns) != 2 || request.Turns[1].Text != "我的想法是后来切换的项目。" {
		t.Fatalf("latest request=%+v err=%v", request, err)
	}
}

func TestCaptureConcurrentReplayAndDistinctEventsRemainContiguous(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	cwd := t.TempDir()
	const count = 20
	var group sync.WaitGroup
	errorsSeen := make(chan error, count*2)
	for run := 0; run < count; run++ {
		group.Add(2)
		go func() {
			defer group.Done()
			event, err := decodeHookInput(strings.NewReader(hookPrompt("s1", "same", cwd, "我的想法是同一条。")))
			if err == nil {
				err = captureAtRoot(context.Background(), root, event, time.Now)
			}
			errorsSeen <- err
		}()
		go func(index int) {
			defer group.Done()
			event, err := decodeHookInput(strings.NewReader(hookPrompt("s1", fmt.Sprintf("t-%d", index), cwd, fmt.Sprintf("我的想法是第 %d 条。", index))))
			if err == nil {
				err = captureAtRoot(context.Background(), root, event, time.Now)
			}
			errorsSeen <- err
		}(run)
	}
	group.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
	records, _ := readOnlyCapturedRecords(t, root, "s1", cwd)
	if len(records) != count+1 {
		t.Fatalf("record count=%d", len(records))
	}
	for index, record := range records {
		if record.Ordinal != uint32(index+1) {
			t.Fatalf("ordinal gap at %d: %+v", index, record)
		}
	}
}

func TestCaptureLockHonorsCancellation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	if err := prepareSpoolRoot(root); err != nil {
		t.Fatal(err)
	}
	first, err := acquireSpoolLock(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	second, err := acquireSpoolLock(ctx, root)
	if second != nil || CodeOf(err) != CodeCanceled {
		t.Fatalf("lock=%v err=%v code=%s", second, err, CodeOf(err))
	}
}

func TestCaptureRejectsHardlinkedRecordAndInsecureRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	cwd := t.TempDir()
	captureJSONAtRoot(t, root, hookPrompt("s1", "t1", cwd, "我的想法是安全边界。"))
	records, _ := readOnlyCapturedRecords(t, root, "s1", cwd)
	key, err := loadOrCreateSpoolKey(root)
	if err != nil || len(records) != 1 {
		t.Fatal(err)
	}
	session := filepath.Join(root, "sessions", strings.TrimPrefix(records[0].SessionID, "sha256:"))
	entries, _ := os.ReadDir(session)
	if err := os.Link(filepath.Join(session, entries[0].Name()), filepath.Join(session, "00002-"+strings.Repeat("a", 64)+".json")); err != nil {
		t.Fatal(err)
	}
	_, _, err = readSessionRecords(session, records[0].SessionID, key)
	if CodeOf(err) != CodeIntegrity {
		t.Fatalf("hardlink err=%v code=%s", err, CodeOf(err))
	}

	insecure := filepath.Join(t.TempDir(), "already-created")
	if err := os.Mkdir(insecure, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := prepareSpoolRoot(insecure); err == nil {
		t.Fatal("insecure existing root was repaired or accepted")
	}
}

func TestCapturedSessionReaderRejectsInsecureSessionsRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	cwd := t.TempDir()
	captureJSONAtRoot(t, root, hookPrompt("s1", "t1", cwd, "我的想法是验证读取边界。"))
	sessionsRoot := filepath.Join(root, "sessions")
	if err := setTestSpoolDirectoryDACL(sessionsRoot, true); err != nil {
		t.Fatal(err)
	}
	if _, err := readCapturedSessionsAtRoot(context.Background(), root); CodeOf(err) != CodeIntegrity {
		t.Fatalf("reader accepted insecure sessions root: err=%v code=%s", err, CodeOf(err))
	}
}

func TestCaptureRejectsExpandedRecordBeforePublishingSessionDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	cwd := t.TempDir()
	input := hookPrompt("oversized-session", "t1", cwd, strings.Repeat("\t", 170<<10))
	event, err := decodeHookInput(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if err := captureAtRoot(context.Background(), root, event, time.Now); CodeOf(err) != CodeLimitExceeded {
		t.Fatalf("expanded record err=%v code=%s", err, CodeOf(err))
	}
	entries, err := os.ReadDir(filepath.Join(root, "sessions"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed capture left session entries=%v err=%v", entries, err)
	}
	captureJSONAtRoot(t, root, hookPrompt("small-session", "t1", cwd, "我的想法是继续使用安全 spool。"))
	if sessions, err := readCapturedSessionsAtRoot(context.Background(), root); err != nil || len(sessions) != 1 {
		t.Fatalf("spool did not recover: sessions=%v err=%v", sessions, err)
	}
}

func TestOwnerOnlyDirectoryHandlePreventsPathReplacementAndEnumerationIsBounded(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	if err := prepareSpoolRoot(root); err != nil {
		t.Fatal(err)
	}
	sessionsRoot := filepath.Join(root, "sessions")
	if err := ensureOwnerOnlyDirectory(sessionsRoot); err != nil {
		t.Fatal(err)
	}
	directory, err := openOwnerOnlyDirectory(sessionsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(sessionsRoot, filepath.Join(root, "sessions-replaced")); err == nil {
		_ = directory.Close()
		t.Fatal("retained directory handle allowed path replacement")
	}
	if err := directory.Close(); err != nil {
		t.Fatal(err)
	}

	plain := t.TempDir()
	for index := 0; index < 3; index++ {
		if err := os.WriteFile(filepath.Join(plain, fmt.Sprintf("%d", index)), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plainDirectory, err := os.Open(plain)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := readBoundedDirectoryEntries(plainDirectory, 2)
	closeErr := plainDirectory.Close()
	if readErr == nil || closeErr != nil {
		t.Fatalf("bounded enumeration readErr=%v closeErr=%v", readErr, closeErr)
	}
}

func TestCaptureRetainsSpoolNamespaceAcrossReadRenameAndSync(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	cwd := t.TempDir()
	captureJSONAtRoot(t, root, hookPrompt("s1", "t1", cwd, "我的想法是先固定捕获目录。"))
	key, err := loadSpoolKey(root)
	if err != nil {
		t.Fatal(err)
	}
	candidate := candidateForEvent(key, captureEvent{rawSession: "s1", cwd: cwd, event: "SessionStart", boundary: "startup"})
	sessionsRoot := filepath.Join(root, "sessions")
	sessionDirectory := filepath.Join(sessionsRoot, strings.TrimPrefix(candidate.sessionID, "sha256:"))
	assertPinned := func() error {
		for _, path := range []string{sessionDirectory, sessionsRoot} {
			replacement := path + "-race-replacement"
			if err := os.Rename(path, replacement); err == nil {
				_ = os.Rename(replacement, path)
				return fmt.Errorf("retained namespace allowed rename")
			}
		}
		return nil
	}
	assertRootPinned := func() error {
		replacement := sessionsRoot + "-root-only-race-replacement"
		if err := os.Rename(sessionsRoot, replacement); err == nil {
			_ = os.Rename(replacement, sessionsRoot)
			return fmt.Errorf("retained sessions root allowed rename")
		}
		return nil
	}
	calls := 0
	hook := func() error {
		calls++
		return assertPinned()
	}
	renameHook := func(string) error { return hook() }
	event, err := decodeHookInput(strings.NewReader(hookPrompt("s1", "t2", cwd, "我的想法是继续验证发布。")))
	if err != nil {
		t.Fatal(err)
	}
	options := captureSpoolOptions{
		limits: productionSpoolAdmissionLimits(),
		hooks: captureSpoolHooks{
			afterSessionsRootRetained: func() error {
				calls++
				return assertRootPinned()
			},
			afterSessionDirectoryRetained: hook,
			beforeRecordRename:            renameHook,
			beforeDirectorySync:           hook,
		},
	}
	if err := captureAtRootWithOptions(context.Background(), root, event, time.Now, options); err != nil {
		t.Fatalf("capture failed: code=%s", CodeOf(err))
	}
	if calls != 4 {
		t.Fatalf("retention hooks=%d", calls)
	}

	// The same renames work after Capture returns, proving the failures above
	// came from the production retained handles rather than ambient state.
	for _, path := range []string{sessionDirectory, sessionsRoot} {
		replacement := path + "-after-capture"
		if err := os.Rename(path, replacement); err != nil {
			t.Fatalf("rename after capture %s: %v", filepath.Base(path), err)
		}
		if err := os.Rename(replacement, path); err != nil {
			t.Fatalf("restore after capture %s: %v", filepath.Base(path), err)
		}
	}
	records, _ := readOnlyCapturedRecords(t, root, "s1", cwd)
	if len(records) != 2 {
		t.Fatalf("records=%d", len(records))
	}
}

func TestCaptureRetainsPendingRecordIdentityThroughHandleRelativeRename(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	cwd := t.TempDir()
	captureJSONAtRoot(t, root, hookPrompt("s1", "t1", cwd, "我的想法是先写入第一条。"))

	hookCalled := false
	event := decodeHookForTest(t, hookPrompt("s1", "t2", cwd, "我的想法是保留临时记录身份。"))
	options := captureSpoolOptions{
		limits: productionSpoolAdmissionLimits(),
		hooks: captureSpoolHooks{beforeRecordRename: func(temporaryPath string) error {
			hookCalled = true
			movedPath := temporaryPath + ".moved"
			if err := os.Rename(temporaryPath, movedPath); err == nil {
				_ = os.Rename(movedPath, temporaryPath)
				return errors.New("retained pending handle allowed rename")
			}
			if err := os.Remove(temporaryPath); err == nil {
				return errors.New("retained pending handle allowed delete")
			}
			writer, err := os.OpenFile(temporaryPath, os.O_WRONLY, 0)
			if err == nil {
				_ = writer.Close()
				return errors.New("retained pending handle allowed a second writer")
			}
			return nil
		}},
	}
	if err := captureAtRootWithOptions(context.Background(), root, event, time.Now, options); err != nil {
		t.Fatalf("capture failed: code=%s", CodeOf(err))
	}
	if !hookCalled {
		t.Fatal("pending identity hook was not called")
	}
	records, names := readOnlyCapturedRecords(t, root, "s1", cwd)
	if len(records) != 2 || records[1].Content != "我的想法是保留临时记录身份。" {
		t.Fatalf("published records=%+v", records)
	}
	for _, name := range names {
		if strings.HasPrefix(name, ".pending-") || strings.HasSuffix(name, ".moved") {
			t.Fatalf("temporary namespace residue=%q", name)
		}
	}
}

func TestCaptureHandleRelativeRenameNeverOverwritesLateTarget(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	cwd := t.TempDir()
	captureJSONAtRoot(t, root, hookPrompt("s1", "t1", cwd, "我的想法是先写入第一条。"))

	lateTarget := []byte("late target must not be replaced")
	var targetPath string
	event := decodeHookForTest(t, hookPrompt("s1", "t2", cwd, "我的想法是验证不覆盖发布。"))
	options := captureSpoolOptions{
		limits: productionSpoolAdmissionLimits(),
		hooks: captureSpoolHooks{beforeRecordRename: func(temporaryPath string) error {
			name := strings.TrimPrefix(filepath.Base(temporaryPath), ".pending-")
			if len(name) <= 33 || name[len(name)-33] != '-' {
				return errors.New("unexpected pending name")
			}
			targetPath = filepath.Join(filepath.Dir(temporaryPath), name[:len(name)-33]+".json")
			file, err := createOwnerOnlyFile(targetPath)
			if err != nil {
				return err
			}
			return errors.Join(writeAll(file, lateTarget), file.Sync(), file.Close())
		}},
	}
	err := captureAtRootWithOptions(context.Background(), root, event, time.Now, options)
	if CodeOf(err) != CodeIdentityConflict {
		t.Fatalf("late target err=%v code=%s", err, CodeOf(err))
	}
	data, readErr := os.ReadFile(targetPath)
	if readErr != nil || string(data) != string(lateTarget) {
		t.Fatalf("late target changed: data=%q err=%v", data, readErr)
	}
	entries, readDirErr := os.ReadDir(filepath.Dir(targetPath))
	if readDirErr != nil {
		t.Fatal(readDirErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".pending-") {
			t.Fatalf("failed no-replace left pending residue %q", entry.Name())
		}
	}
}

func TestCaptureAdmissionLimitsAreExactAndOverLimitLeavesNoPartialWrite(t *testing.T) {
	defaults := productionSpoolAdmissionLimits()
	if defaults.sessionEvents != maxSessionEvents || defaults.sessionBytes != maxSessionBytes ||
		defaults.globalSessions != maxGlobalSessions || defaults.globalEvents != maxGlobalEvents || defaults.globalBytes != maxGlobalBytes {
		t.Fatalf("production limits=%+v", defaults)
	}
	fixedTime := time.Date(2026, 8, 30, 8, 9, 10, 123456789, time.UTC)
	now := func() time.Time { return fixedTime }

	t.Run("per_session_events", func(t *testing.T) {
		root, cwd := filepath.Join(t.TempDir(), "spool"), t.TempDir()
		limits := defaults
		limits.sessionEvents = 2
		for index := 1; index <= 2; index++ {
			captureJSONAtRootWithOptions(t, root, hookPrompt("s1", fmt.Sprintf("t%d", index), cwd, fmt.Sprintf("我的想法是第 %d 条。", index)), now, limits)
		}
		assertCaptureLimitLeavesCounts(t, root, hookPrompt("s1", "t3", cwd, "我的想法是越界。"), now, limits, 1, 2)
	})

	t.Run("global_sessions", func(t *testing.T) {
		root, cwd := filepath.Join(t.TempDir(), "spool"), t.TempDir()
		limits := defaults
		limits.globalSessions = 2
		for index := 1; index <= 2; index++ {
			captureJSONAtRootWithOptions(t, root, hookPrompt(fmt.Sprintf("s%d", index), "t1", cwd, "我的想法是会话边界。"), now, limits)
		}
		assertCaptureLimitLeavesCounts(t, root, hookPrompt("s3", "t1", cwd, "我的想法是第三个会话。"), now, limits, 2, 2)
	})

	t.Run("global_events", func(t *testing.T) {
		root, cwd := filepath.Join(t.TempDir(), "spool"), t.TempDir()
		limits := defaults
		limits.globalEvents = 2
		captureJSONAtRootWithOptions(t, root, hookPrompt("s1", "t1", cwd, "我的想法是全局第一条。"), now, limits)
		captureJSONAtRootWithOptions(t, root, hookPrompt("s2", "t1", cwd, "我的想法是全局第二条。"), now, limits)
		assertCaptureLimitLeavesCounts(t, root, hookPrompt("s3", "t1", cwd, "我的想法是全局越界。"), now, limits, 2, 2)
	})

	t.Run("per_session_bytes", func(t *testing.T) {
		root, cwd := filepath.Join(t.TempDir(), "spool"), t.TempDir()
		first := decodeHookForTest(t, hookPrompt("s1", "t1", cwd, "我的想法是字节一。"))
		captureEventAtRootWithOptions(t, root, first, now, defaults)
		records, total := readCapturedRecordsAndBytes(t, root, "s1", cwd)
		second := decodeHookForTest(t, hookPrompt("s1", "t2", cwd, "我的想法是字节二。"))
		limits := defaults
		limits.sessionBytes = total + projectedRecordSize(t, root, second, uint32(len(records)+1), fixedTime)
		captureEventAtRootWithOptions(t, root, second, now, limits)
		_, exact := readCapturedRecordsAndBytes(t, root, "s1", cwd)
		if exact != limits.sessionBytes {
			t.Fatalf("session bytes=%d limit=%d", exact, limits.sessionBytes)
		}
		assertCaptureLimitLeavesCounts(t, root, hookPrompt("s1", "t3", cwd, "我的想法是字节越界。"), now, limits, 1, 2)
	})

	t.Run("global_bytes", func(t *testing.T) {
		root, cwd := filepath.Join(t.TempDir(), "spool"), t.TempDir()
		first := decodeHookForTest(t, hookPrompt("s1", "t1", cwd, "我的想法是全局字节一。"))
		captureEventAtRootWithOptions(t, root, first, now, defaults)
		_, total := readCapturedRecordsAndBytes(t, root, "s1", cwd)
		second := decodeHookForTest(t, hookPrompt("s2", "t1", cwd, "我的想法是全局字节二。"))
		limits := defaults
		limits.globalBytes = total + projectedRecordSize(t, root, second, 1, fixedTime)
		captureEventAtRootWithOptions(t, root, second, now, limits)
		inspection, err := inspectGlobalSpoolRetained(filepath.Join(root, "sessions"), "sha256:"+strings.Repeat("0", 64), nil)
		if err != nil {
			t.Fatal(err)
		}
		exact := inspection.usage.bytes
		if err := inspection.Close(); err != nil {
			t.Fatal(err)
		}
		if exact != limits.globalBytes {
			t.Fatalf("global bytes=%d limit=%d", exact, limits.globalBytes)
		}
		assertCaptureLimitLeavesCounts(t, root, hookPrompt("s3", "t1", cwd, "我的想法是全局字节越界。"), now, limits, 2, 2)
	})
}

func TestCaptureCleansPendingAndEmptyCrashResidueBeforeAdmission(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	if err := prepareSpoolRoot(root); err != nil {
		t.Fatal(err)
	}
	sessionsRoot := filepath.Join(root, "sessions")
	if err := ensureOwnerOnlyDirectory(sessionsRoot); err != nil {
		t.Fatal(err)
	}
	emptyDirectory := filepath.Join(sessionsRoot, strings.Repeat("a", 64))
	pendingDirectory := filepath.Join(sessionsRoot, strings.Repeat("b", 64))
	for _, path := range []string{emptyDirectory, pendingDirectory} {
		if _, err := ensureOwnerOnlyDirectoryCreated(path); err != nil {
			t.Fatal(err)
		}
	}
	pendingName := ".pending-00001-" + strings.Repeat("c", 64) + "-" + strings.Repeat("d", 32)
	pending, err := createOwnerOnlyFile(filepath.Join(pendingDirectory, pendingName))
	if err != nil {
		t.Fatal(err)
	}
	if err := errors.Join(writeAll(pending, []byte("crash residue")), pending.Sync(), pending.Close()); err != nil {
		t.Fatal(err)
	}

	cwd := t.TempDir()
	limits := productionSpoolAdmissionLimits()
	limits.globalSessions = 1
	captureJSONAtRootWithOptions(t, root, hookPrompt("live", "t1", cwd, "我的想法是清理后继续捕获。"), time.Now, limits)
	for _, removed := range []string{emptyDirectory, pendingDirectory} {
		if _, err := os.Lstat(removed); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("crash residue remains: err=%v", err)
		}
	}
	entries, err := os.ReadDir(sessionsRoot)
	if err != nil || len(entries) != 1 {
		t.Fatalf("session entries=%v err=%v", entries, err)
	}
}

func TestCaptureRejectsMalformedPendingResidueWithoutPartialCleanup(t *testing.T) {
	root := filepath.Join(t.TempDir(), "spool")
	if err := prepareSpoolRoot(root); err != nil {
		t.Fatal(err)
	}
	sessionsRoot := filepath.Join(root, "sessions")
	if err := ensureOwnerOnlyDirectory(sessionsRoot); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(sessionsRoot, strings.Repeat("a", 64))
	if _, err := ensureOwnerOnlyDirectoryCreated(directory); err != nil {
		t.Fatal(err)
	}
	validName := ".pending-00001-" + strings.Repeat("b", 64) + "-" + strings.Repeat("c", 32)
	for _, name := range []string{validName, ".pending-malformed"} {
		file, err := createOwnerOnlyFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	event := decodeHookForTest(t, hookPrompt("live", "t1", t.TempDir(), "我的想法是不能绕过损坏。"))
	if err := captureAtRoot(context.Background(), root, event, time.Now); CodeOf(err) != CodeIntegrity {
		t.Fatalf("malformed pending err=%v code=%s", err, CodeOf(err))
	}
	for _, name := range []string{validName, ".pending-malformed"} {
		if _, err := os.Lstat(filepath.Join(directory, name)); err != nil {
			t.Fatalf("partial cleanup removed %s: %v", name, err)
		}
	}
}

func TestCapturedRecordIntegrityBindsEventIdentityAndCaptureTime(t *testing.T) {
	for _, mutation := range []struct {
		name   string
		change func(*spoolRecord)
	}{
		{name: "event_id", change: func(record *spoolRecord) { record.EventID = "sha256:" + strings.Repeat("a", 64) }},
		{name: "captured_at", change: func(record *spoolRecord) { record.CapturedAt = "2000-01-01T00:00:00Z" }},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "spool")
			cwd := t.TempDir()
			captureJSONAtRoot(t, root, hookPrompt("s1", "t1", cwd, "我的想法是绑定完整记录。"))
			key, err := loadSpoolKey(root)
			if err != nil {
				t.Fatal(err)
			}
			candidate := candidateForEvent(key, captureEvent{rawSession: "s1", cwd: cwd, event: "SessionStart", boundary: "startup"})
			directory := filepath.Join(root, "sessions", strings.TrimPrefix(candidate.sessionID, "sha256:"))
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 1 {
				t.Fatalf("entries=%v err=%v", entries, err)
			}
			path := filepath.Join(directory, entries[0].Name())
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var record spoolRecord
			if err := json.Unmarshal(data, &record); err != nil {
				t.Fatal(err)
			}
			mutation.change(&record)
			data, err = encodeRecord(record)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := readSessionRecords(directory, candidate.sessionID, key); CodeOf(err) != CodeIntegrity {
				t.Fatalf("mutated record accepted: err=%v code=%s", err, CodeOf(err))
			}
		})
	}
}

func decodeHookForTest(t *testing.T, input string) captureEvent {
	t.Helper()
	event, err := decodeHookInput(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func captureEventAtRootWithOptions(t *testing.T, root string, event captureEvent, now func() time.Time, limits spoolAdmissionLimits) {
	t.Helper()
	if err := captureAtRootWithOptions(context.Background(), root, event, now, captureSpoolOptions{limits: limits}); err != nil {
		t.Fatal(err)
	}
}

func captureJSONAtRootWithOptions(t *testing.T, root, input string, now func() time.Time, limits spoolAdmissionLimits) {
	t.Helper()
	captureEventAtRootWithOptions(t, root, decodeHookForTest(t, input), now, limits)
}

func projectedRecordSize(t *testing.T, root string, event captureEvent, ordinal uint32, capturedAt time.Time) int64 {
	t.Helper()
	key, err := loadSpoolKey(root)
	if err != nil {
		t.Fatal(err)
	}
	candidate := candidateForEvent(key, event)
	record := spoolRecord{
		SchemaVersion: spoolSchemaVersion,
		EventID:       "sha256:" + strings.Repeat("a", 64),
		SessionID:     candidate.sessionID,
		ProjectID:     candidate.projectID,
		TurnID:        candidate.turnID,
		Ordinal:       ordinal,
		HookEvent:     candidate.hookEvent,
		Kind:          candidate.kind,
		Role:          candidate.role,
		Content:       candidate.content,
		Boundary:      candidate.boundary,
		CapturedAt:    capturedAt.UTC().Format(time.RFC3339Nano),
	}
	record.RecordMAC = recordIntegrityMAC(key, record)
	encoded, err := encodeRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	return int64(len(encoded))
}

func readCapturedRecordsAndBytes(t *testing.T, root, rawSession, cwd string) ([]spoolRecord, int64) {
	t.Helper()
	key, err := loadSpoolKey(root)
	if err != nil {
		t.Fatal(err)
	}
	candidate := candidateForEvent(key, captureEvent{rawSession: rawSession, cwd: cwd, event: "SessionStart", boundary: "startup"})
	records, total, err := readSessionRecords(filepath.Join(root, "sessions", strings.TrimPrefix(candidate.sessionID, "sha256:")), candidate.sessionID, key)
	if err != nil {
		t.Fatal(err)
	}
	return records, total
}

func assertCaptureLimitLeavesCounts(t *testing.T, root, input string, now func() time.Time, limits spoolAdmissionLimits, wantSessions, wantEvents int) {
	t.Helper()
	event := decodeHookForTest(t, input)
	if err := captureAtRootWithOptions(context.Background(), root, event, now, captureSpoolOptions{limits: limits}); CodeOf(err) != CodeLimitExceeded {
		t.Fatalf("limit err=%v code=%s", err, CodeOf(err))
	}
	inspection, err := inspectGlobalSpoolRetained(filepath.Join(root, "sessions"), "sha256:"+strings.Repeat("0", 64), nil)
	if err != nil {
		t.Fatal(err)
	}
	usage := inspection.usage
	if err := inspection.Close(); err != nil {
		t.Fatal(err)
	}
	if usage.sessions != wantSessions || usage.events != wantEvents {
		t.Fatalf("usage=%+v want sessions=%d events=%d", usage, wantSessions, wantEvents)
	}
	err = filepath.WalkDir(filepath.Join(root, "sessions"), func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.HasPrefix(entry.Name(), ".pending-") {
			return fmt.Errorf("pending record remains")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func captureJSONAtRoot(t *testing.T, root, input string) {
	t.Helper()
	event, err := decodeHookInput(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if err := captureAtRoot(context.Background(), root, event, time.Now); err != nil {
		t.Fatal(err)
	}
}

func readOnlyCapturedRecords(t *testing.T, root, rawSession, cwd string) ([]spoolRecord, []string) {
	t.Helper()
	key, err := loadOrCreateSpoolKey(root)
	if err != nil {
		t.Fatal(err)
	}
	candidate := candidateForEvent(key, captureEvent{rawSession: rawSession, cwd: cwd, event: "SessionStart", boundary: "startup"})
	directory := filepath.Join(root, "sessions", strings.TrimPrefix(candidate.sessionID, "sha256:"))
	records, _, err := readSessionRecords(directory, candidate.sessionID, key)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]string, 0, len(entries))
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, string(content))
	}
	return records, data
}

func hookSessionStart(session, cwd string) string {
	return fmt.Sprintf(`{"session_id":%q,"transcript_path":null,"cwd":%q,"hook_event_name":"SessionStart","model":"gpt","permission_mode":"default","source":"startup"}`, session, cwd)
}

func hookPrompt(session, turn, cwd, prompt string) string {
	return fmt.Sprintf(`{"session_id":%q,"turn_id":%q,"transcript_path":null,"cwd":%q,"hook_event_name":"UserPromptSubmit","model":"gpt","permission_mode":"default","prompt":%q}`, session, turn, cwd, prompt)
}

func hookStop(session, turn, cwd, message string) string {
	return fmt.Sprintf(`{"session_id":%q,"turn_id":%q,"transcript_path":null,"cwd":%q,"hook_event_name":"Stop","model":"gpt","permission_mode":"default","stop_hook_active":false,"last_assistant_message":%q}`, session, turn, cwd, message)
}

func hookSessionEnd(session, cwd string) string {
	return fmt.Sprintf(`{"session_id":%q,"transcript_path":null,"cwd":%q,"hook_event_name":"SessionEnd","reason":"other"}`, session, cwd)
}

func setTestSpoolDirectoryDACL(path string, includeWorld bool) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		return fmt.Errorf("resolve test owner: %w", err)
	}
	sddl := "O:" + user.User.Sid.String() + "D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")"
	if includeWorld {
		sddl += "(A;OICI;GR;;;WD)"
	}
	descriptor, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|
			windows.PROTECTED_DACL_SECURITY_INFORMATION,
		user.User.Sid, nil, dacl, nil)
}

func TestPrepareSpoolRootRejectsReparseWhenAvailable(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "link")
	if err := os.Symlink(target, link); err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skip("Windows symlink privilege unavailable")
		}
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := prepareSpoolRoot(filepath.Join(link, "spool")); err == nil {
		t.Fatal("reparse parent was accepted")
	}
}
