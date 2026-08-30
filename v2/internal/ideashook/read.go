package ideashook

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/sessiondistill"
)

type SessionSummary struct {
	SessionID        string `json:"session_id"`
	EventCount       int    `json:"event_count"`
	VisibleTurnCount int    `json:"visible_turn_count"`
	LastCapturedAt   string `json:"last_captured_at"`
}

type capturedSession struct {
	summary SessionSummary
	records []spoolRecord
}

func ListSessions(ctx context.Context) ([]SessionSummary, error) {
	sessions, err := readCapturedSessions(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]SessionSummary, len(sessions))
	for index := range sessions {
		result[index] = sessions[index].summary
	}
	return result, nil
}

func RequestForSession(ctx context.Context, sessionID string) (sessiondistill.Request, error) {
	if !validOpaque(sessionID, "sha256:") {
		return sessiondistill.Request{}, hookError(CodeInvalidInput, errors.New("session identity"))
	}
	sessions, err := readCapturedSessions(ctx)
	if err != nil {
		return sessiondistill.Request{}, err
	}
	return requestForSessionFromSessions(sessions, sessionID)
}

// RequestForCurrentSession selects the one open captured root session whose
// most recent event belongs to the exact current working directory. It never
// falls back to a globally recent session: no match is not found, and multiple
// matches are an identity conflict that requires explicit -session selection.
func RequestForCurrentSession(ctx context.Context, workingDirectory string) (sessiondistill.Request, error) {
	if !validMetadata(workingDirectory, maxMetadataBytes) || !filepath.IsAbs(workingDirectory) {
		return sessiondistill.Request{}, hookError(CodeInvalidInput, errors.New("current working directory"))
	}
	root, err := resolveSpoolRoot()
	if err != nil {
		return sessiondistill.Request{}, hookError(CodeUnsupported, err)
	}
	return requestForCurrentSessionAtRoot(ctx, root, workingDirectory)
}

func requestForCurrentSessionAtRoot(ctx context.Context, root, workingDirectory string) (sessiondistill.Request, error) {
	if !validMetadata(workingDirectory, maxMetadataBytes) || !filepath.IsAbs(workingDirectory) {
		return sessiondistill.Request{}, hookError(CodeInvalidInput, errors.New("current working directory"))
	}
	var request sessiondistill.Request
	err := inspectCapturedSessionsAtRoot(ctx, root, func(sessions []capturedSession, key []byte) error {
		projectID := projectIDForWorkingDirectory(key, workingDirectory)
		selected, err := requestForCurrentSessionFromSessions(sessions, projectID)
		if err != nil {
			return err
		}
		request = selected
		return nil
	})
	if err != nil {
		return sessiondistill.Request{}, err
	}
	return request, nil
}

func requestForSessionFromSessions(sessions []capturedSession, sessionID string) (sessiondistill.Request, error) {
	for _, session := range sessions {
		if session.summary.SessionID == sessionID {
			return requestFromCapturedSession(session)
		}
	}
	return sessiondistill.Request{}, hookError(CodeNotFound, errors.New("captured session"))
}

func requestForCurrentSessionFromSessions(sessions []capturedSession, projectID string) (sessiondistill.Request, error) {
	if !validOpaque(projectID, "sha256:") {
		return sessiondistill.Request{}, hookError(CodeInvalidInput, errors.New("current project identity"))
	}
	var selected *capturedSession
	for index := range sessions {
		session := &sessions[index]
		if session.summary.VisibleTurnCount == 0 || len(session.records) == 0 {
			continue
		}
		latest := session.records[len(session.records)-1]
		if latest.ProjectID != projectID || latest.HookEvent == "SessionEnd" {
			continue
		}
		if selected != nil {
			return sessiondistill.Request{}, hookError(CodeIdentityConflict, errors.New("multiple open current sessions"))
		}
		selected = session
	}
	if selected == nil {
		return sessiondistill.Request{}, hookError(CodeNotFound, errors.New("open current session"))
	}
	return requestFromCapturedSession(*selected)
}

func requestFromCapturedSession(session capturedSession) (sessiondistill.Request, error) {
	turns := make([]sessiondistill.VisibleTurn, 0, session.summary.VisibleTurnCount)
	for _, record := range session.records {
		if record.Kind != "message" {
			continue
		}
		turns = append(turns, sessiondistill.VisibleTurn{
			EventID: record.EventID,
			Ordinal: uint32(len(turns) + 1),
			Role:    sessiondistill.Role(record.Role),
			Text:    record.Content,
		})
	}
	if len(turns) == 0 {
		return sessiondistill.Request{}, hookError(CodeNoVisibleEvents, errors.New("no visible captured events"))
	}
	return sessiondistill.Request{SchemaVersion: sessiondistill.SchemaVersion, SessionID: session.summary.SessionID, Turns: turns}, nil
}

func readCapturedSessions(ctx context.Context) (result []capturedSession, resultErr error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	root, err := resolveSpoolRoot()
	if err != nil {
		return nil, hookError(CodeUnsupported, err)
	}
	if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
		return nil, hookError(CodeNotFound, err)
	} else if err != nil {
		return nil, hookError(CodeStorage, err)
	}
	return readCapturedSessionsAtRoot(ctx, root)
}

func readCapturedSessionsAtRoot(ctx context.Context, root string) (result []capturedSession, resultErr error) {
	err := inspectCapturedSessionsAtRoot(ctx, root, func(sessions []capturedSession, _ []byte) error {
		result = sessions
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// inspectCapturedSessionsAtRoot keeps the root lock, spool key, retained
// sessions directory, verified records, and the consumer's selection in one
// critical section. The consumer must not retain or mutate the supplied key.
func inspectCapturedSessionsAtRoot(ctx context.Context, root string, consume func([]capturedSession, []byte) error) (resultErr error) {
	if err := contextError(ctx); err != nil {
		return err
	}
	if consume == nil {
		return hookError(CodeStorage, errors.New("captured session consumer"))
	}
	if err := prepareSpoolRoot(root); err != nil {
		return hookError(CodeIntegrity, err)
	}
	lock, err := acquireSpoolLock(ctx, root)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, lock.Close()) }()
	key, err := loadSpoolKey(root)
	if err != nil {
		return hookError(CodeIntegrity, err)
	}
	sessionsRoot := filepath.Join(root, "sessions")
	if _, err := os.Lstat(sessionsRoot); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return hookError(CodeNotFound, err)
		}
		return hookError(CodeIntegrity, err)
	}
	if _, err := inspectGlobalSpool(sessionsRoot, "sha256:"+strings.Repeat("0", 64)); err != nil {
		return err
	}
	sessionsHandle, entries, err := openBoundedOwnerOnlyDirectory(sessionsRoot, maxGlobalSessions)
	if err != nil {
		return hookError(CodeIntegrity, err)
	}
	defer func() { resultErr = errors.Join(resultErr, sessionsHandle.Close()) }()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	result := make([]capturedSession, 0, len(entries))
	for _, entry := range entries {
		sessionID := "sha256:" + entry.Name()
		records, _, err := readSessionRecords(filepath.Join(sessionsRoot, entry.Name()), sessionID, key)
		if err != nil {
			return err
		}
		if len(records) == 0 {
			continue
		}
		visible, last := 0, time.Time{}
		for _, record := range records {
			if record.Kind == "message" {
				visible++
			}
			captured, _ := time.Parse(time.RFC3339Nano, record.CapturedAt)
			if captured.After(last) {
				last = captured
			}
		}
		result = append(result, capturedSession{
			summary: SessionSummary{
				SessionID:        sessionID,
				EventCount:       len(records),
				VisibleTurnCount: visible,
				LastCapturedAt:   last.UTC().Format(time.RFC3339Nano),
			},
			records: records,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].summary.LastCapturedAt != result[j].summary.LastCapturedAt {
			return result[i].summary.LastCapturedAt > result[j].summary.LastCapturedAt
		}
		return result[i].summary.SessionID < result[j].summary.SessionID
	})
	if len(result) == 0 {
		return hookError(CodeNotFound, errors.New("no captured sessions"))
	}
	if err := consume(result, key); err != nil {
		return err
	}
	return nil
}
