// Package event defines the stable envelope used by durable stores and future
// outbox transports. It intentionally contains no broker-specific concepts.
package event

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/platform"
	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
)

// Envelope is an immutable-by-convention versioned domain event record.
type Envelope struct {
	ID            platform.ID     `json:"id"`
	StreamID      platform.ID     `json:"stream_id"`
	StreamKind    string          `json:"stream_kind"`
	Sequence      uint64          `json:"sequence"`
	Type          string          `json:"type"`
	SchemaVersion uint32          `json:"schema_version"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Data          json.RawMessage `json:"data"`
	CorrelationID platform.ID     `json:"correlation_id,omitempty"`
	CausationID   platform.ID     `json:"causation_id,omitempty"`
}

// New validates an envelope and clones its JSON payload.
func New(envelope Envelope) (Envelope, error) {
	envelope.Data = append(json.RawMessage(nil), envelope.Data...)
	envelope.OccurredAt = envelope.OccurredAt.UTC()
	if err := envelope.Validate(); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// Validate checks fields needed for deterministic replay and optimistic append.
func (e Envelope) Validate() error {
	if e.ID.IsZero() || e.StreamID.IsZero() {
		return apperror.New(apperror.KindInvalid, "event.id_required", "event id and stream id are required")
	}
	if strings.TrimSpace(e.StreamKind) == "" || strings.TrimSpace(e.Type) == "" {
		return apperror.New(apperror.KindInvalid, "event.type_required", "event stream kind and type are required")
	}
	if e.Sequence == 0 || e.SchemaVersion == 0 {
		return apperror.New(apperror.KindInvalid, "event.version_invalid", "event sequence and schema version must be positive")
	}
	if e.OccurredAt.IsZero() {
		return apperror.New(apperror.KindInvalid, "event.time_required", "event occurred_at is required")
	}
	if len(e.Data) == 0 || !json.Valid(e.Data) {
		return apperror.New(apperror.KindInvalid, "event.data_invalid", "event data must be valid JSON")
	}
	return nil
}
