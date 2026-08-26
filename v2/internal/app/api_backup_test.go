package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/internal/backup"
)

func TestBackupCreateHandlerReturnsAcceptedBeforeCreateCompletes(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	engine := &backupEngineStub{create: func(context.Context, string) (backup.Manifest, error) {
		once.Do(func() { close(entered) })
		<-release
		return backup.Manifest{Database: backup.Artifact{Size: 1}}, nil
	}}
	runtime := newBackupRuntimeForTest(t, engine)
	api := &API{backups: runtime}
	destination := filepath.Join(t.TempDir(), "async-backup")
	body, err := json.Marshal(struct {
		Destination string `json:"destination"`
	}{destination})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/backups", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "async-create")
	response := httptest.NewRecorder()

	api.createBackup(response, request)
	if response.Code != http.StatusAccepted {
		close(release)
		t.Fatalf("create status/body = %d %q", response.Code, response.Body.String())
	}
	var accepted backupOperationStatus
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil || accepted.OperationID == "" ||
		strings.Contains(response.Body.String(), destination) {
		close(release)
		t.Fatalf("create response = %+v, %v, body=%q", accepted, err, response.Body.String())
	}
	select {
	case <-entered:
	default:
		// The goroutine need not have been scheduled before the 202 was written;
		// its completion is deliberately independent from this response.
	}
	close(release)
	if status := waitBackup(t, runtime, accepted.OperationID); status.State != backupStateSucceeded {
		t.Fatalf("terminal status = %+v", status)
	}
}

func TestBackupHandlersRejectNonCanonicalInputAndExposeOnlyStableStatus(t *testing.T) {
	var starts atomic.Int64
	runtime := &backupControlStub{
		start: func(context.Context, string, string) (backupOperationStatus, error) {
			starts.Add(1)
			return backupOperationStatus{OperationID: strings.Repeat("a", 64), State: backupStateAccepted, Phase: "accepted"}, nil
		},
		status: func(string) (backupOperationStatus, error) {
			return backupOperationStatus{OperationID: strings.Repeat("a", 64), State: backupStateFailed, Phase: "complete", FailureCode: "BACKUP_FAILED"}, nil
		},
		cancel: func(string) (backupOperationStatus, error) {
			return backupOperationStatus{OperationID: strings.Repeat("a", 64), State: backupStateRunning, Phase: "creating", CancelRequested: true}, nil
		},
	}
	api := &API{backups: runtime}

	for _, body := range []string{
		`{"destination":"C:\\backup","destination":"C:\\other"}`,
		`{"Destination":"C:\\backup"}`,
		`{"destination":"relative","extra":true}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/backups", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "strict-input")
		response := httptest.NewRecorder()
		api.createBackup(response, request)
		// Direct handler tests do not have localhttp's private safe response
		// wrapper, so Problem bodies fail closed as 500 here. The assertion is
		// that malformed input never reaches admission or returns success.
		if response.Code < http.StatusBadRequest {
			t.Fatalf("body %q status/body = %d %q", body, response.Code, response.Body.String())
		}
	}
	if starts.Load() != 0 {
		t.Fatalf("invalid requests reached runtime %d times", starts.Load())
	}

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/v1/backups/status?operationId="+strings.Repeat("a", 64), nil)
	statusResponse := httptest.NewRecorder()
	api.backupStatus(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK || !strings.Contains(statusResponse.Body.String(), `"failureCode":"BACKUP_FAILED"`) {
		t.Fatalf("status response = %d %q", statusResponse.Code, statusResponse.Body.String())
	}

	cancelRequest := httptest.NewRequest(http.MethodPost, "/api/v1/backups/cancel",
		strings.NewReader(`{"operationId":"`+strings.Repeat("a", 64)+`"}`))
	cancelRequest.Header.Set("Content-Type", "application/json")
	cancelResponse := httptest.NewRecorder()
	api.cancelBackup(cancelResponse, cancelRequest)
	if cancelResponse.Code != http.StatusAccepted || !strings.Contains(cancelResponse.Body.String(), `"cancelRequested":true`) {
		t.Fatalf("cancel response = %d %q", cancelResponse.Code, cancelResponse.Body.String())
	}
}

type backupControlStub struct {
	start  func(context.Context, string, string) (backupOperationStatus, error)
	status func(string) (backupOperationStatus, error)
	cancel func(string) (backupOperationStatus, error)
}

func (stub *backupControlStub) Start(ctx context.Context, key, destination string) (backupOperationStatus, error) {
	return stub.start(ctx, key, destination)
}

func (stub *backupControlStub) Status(id string) (backupOperationStatus, error) {
	return stub.status(id)
}

func (stub *backupControlStub) Cancel(id string) (backupOperationStatus, error) {
	return stub.cancel(id)
}
