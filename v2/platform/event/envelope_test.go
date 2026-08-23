package event_test

import (
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/platform"
	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
	"github.com/mt-hub8/MindWeaver/v2/platform/event"
)

func TestNewNormalizesTimeAndClonesPayload(t *testing.T) {
	eventID, _ := platform.ParseID("00000000-0000-4000-8000-000000000001")
	streamID, _ := platform.ParseID("00000000-0000-4000-8000-000000000002")
	payload := []byte(`{"status":"PENDING"}`)
	wantTime := time.Date(2026, 8, 23, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	got, err := event.New(event.Envelope{
		ID: eventID, StreamID: streamID, StreamKind: "job", Sequence: 1,
		Type: "job.created", SchemaVersion: 1, OccurredAt: wantTime, Data: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload[2] = 'X'
	if !got.OccurredAt.Equal(wantTime) || got.OccurredAt.Location() != time.UTC {
		t.Fatalf("OccurredAt = %v", got.OccurredAt)
	}
	if string(got.Data) != `{"status":"PENDING"}` {
		t.Fatalf("Data was not cloned: %s", got.Data)
	}
}

func TestValidateRejectsInvalidPayload(t *testing.T) {
	err := (event.Envelope{}).Validate()
	if !apperror.IsKind(err, apperror.KindInvalid) {
		t.Fatalf("Validate() error = %v", err)
	}
}
