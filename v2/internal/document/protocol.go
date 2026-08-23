// Package document defines the durable product state of an ingested document.
// It deliberately excludes worker/run state: a failed reindex attempt must not
// make a previously active document unavailable.
package document

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const ProtocolVersion = "document/v1"

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var (
	ErrInvalidState       = errors.New("invalid document state")
	ErrInvalidCommand     = errors.New("invalid document command")
	ErrPurged             = errors.New("document is purged")
	ErrLifecycle          = errors.New("document lifecycle does not allow command")
	ErrGeneration         = errors.New("generation must advance monotonically")
	ErrGenerationConflict = errors.New("active generation changed")
)

// Lifecycle controls product visibility. Index data may remain available while
// a document is trashed, but it is never retrievable unless lifecycle is ACTIVE.
type Lifecycle string

const (
	LifecycleActive  Lifecycle = "ACTIVE"
	LifecycleTrashed Lifecycle = "TRASHED"
	LifecyclePurged  Lifecycle = "PURGED"
)

// BlobRef is an immutable, content-addressed artifact reference. Blob bytes are
// owned by the shared blob store; this package only defines lineage semantics.
type BlobRef struct {
	ID        string `json:"id"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
	MediaType string `json:"media_type"`
}

func (r BlobRef) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("%w: blob id is required", ErrInvalidState)
	}
	if !sha256Pattern.MatchString(r.SHA256) {
		return fmt.Errorf("%w: blob %q has invalid sha256", ErrInvalidState, r.ID)
	}
	if r.SizeBytes < 0 {
		return fmt.Errorf("%w: blob %q has negative size", ErrInvalidState, r.ID)
	}
	return nil
}

// BlobLineage pins both the uploaded bytes and the exact extracted text used by
// chunking. Extractor identifies a versioned algorithm/configuration fingerprint.
type BlobLineage struct {
	Source    BlobRef `json:"source"`
	Text      BlobRef `json:"text"`
	Extractor string  `json:"extractor"`
}

func (l BlobLineage) Validate() error {
	if err := l.Source.Validate(); err != nil {
		return fmt.Errorf("source: %w", err)
	}
	if err := l.Text.Validate(); err != nil {
		return fmt.Errorf("text: %w", err)
	}
	if strings.TrimSpace(l.Extractor) == "" {
		return fmt.Errorf("%w: extractor fingerprint is required", ErrInvalidState)
	}
	return nil
}

// State is the durable document aggregate. Revision advances only for a real
// state change; replaying an already-applied command is idempotent.
type State struct {
	Protocol         string      `json:"protocol"`
	ID               string      `json:"id"`
	Revision         uint64      `json:"revision"`
	Lifecycle        Lifecycle   `json:"lifecycle"`
	ActiveGeneration uint64      `json:"active_generation"`
	Lineage          BlobLineage `json:"lineage"`
}

func New(id string, lineage BlobLineage) (State, error) {
	s := State{
		Protocol:  ProtocolVersion,
		ID:        strings.TrimSpace(id),
		Revision:  1,
		Lifecycle: LifecycleActive,
		Lineage:   lineage,
	}
	if err := s.Validate(); err != nil {
		return State{}, err
	}
	return s, nil
}

func (s State) Validate() error {
	if s.Protocol != ProtocolVersion {
		return fmt.Errorf("%w: protocol must be %q", ErrInvalidState, ProtocolVersion)
	}
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("%w: document id is required", ErrInvalidState)
	}
	if s.Revision == 0 {
		return fmt.Errorf("%w: revision must be positive", ErrInvalidState)
	}
	switch s.Lifecycle {
	case LifecycleActive, LifecycleTrashed, LifecyclePurged:
	default:
		return fmt.Errorf("%w: unknown lifecycle %q", ErrInvalidState, s.Lifecycle)
	}
	if s.Lifecycle == LifecyclePurged && s.ActiveGeneration != 0 {
		return fmt.Errorf("%w: purged document cannot retain an active generation", ErrInvalidState)
	}
	if err := s.Lineage.Validate(); err != nil {
		return err
	}
	return nil
}

// Retrievable is the single product predicate used by scope resolution.
func (s State) Retrievable() bool {
	return s.Validate() == nil &&
		s.Lifecycle == LifecycleActive &&
		s.ActiveGeneration != 0
}

type CommandKind string

const (
	CommandActivateGeneration CommandKind = "ACTIVATE_GENERATION"
	CommandTrash              CommandKind = "TRASH"
	CommandRestore            CommandKind = "RESTORE"
	CommandPurge              CommandKind = "PURGE"
)

type Command struct {
	Kind               CommandKind `json:"kind"`
	Generation         uint64      `json:"generation,omitempty"`
	ExpectedGeneration uint64      `json:"expected_generation,omitempty"`
}

// Apply performs a pure state transition. It never records ingestion failures:
// those belong to ingestion.Run, so an unsuccessful reindex leaves this state
// and its active generation untouched.
func (s State) Apply(command Command) (State, error) {
	if err := s.Validate(); err != nil {
		return State{}, err
	}
	next := s
	changed := false

	switch command.Kind {
	case CommandActivateGeneration:
		if s.Lifecycle == LifecyclePurged {
			return State{}, ErrPurged
		}
		if s.Lifecycle != LifecycleActive {
			return State{}, fmt.Errorf("%w: only ACTIVE documents may activate a generation", ErrLifecycle)
		}
		if command.Generation == 0 {
			return State{}, fmt.Errorf("%w: generation must be positive", ErrInvalidCommand)
		}
		if command.Generation == s.ActiveGeneration && s.ActiveGeneration != 0 {
			return s, nil
		}
		if command.ExpectedGeneration != s.ActiveGeneration {
			return State{}, ErrGenerationConflict
		}
		if command.Generation <= s.ActiveGeneration {
			return State{}, ErrGeneration
		}
		next.ActiveGeneration = command.Generation
		changed = true

	case CommandTrash:
		switch s.Lifecycle {
		case LifecycleActive:
			next.Lifecycle = LifecycleTrashed
			changed = true
		case LifecycleTrashed:
			return s, nil
		case LifecyclePurged:
			return State{}, ErrPurged
		}

	case CommandRestore:
		switch s.Lifecycle {
		case LifecycleActive:
			return s, nil
		case LifecycleTrashed:
			next.Lifecycle = LifecycleActive
			changed = true
		case LifecyclePurged:
			return State{}, ErrPurged
		}

	case CommandPurge:
		switch s.Lifecycle {
		case LifecycleActive:
			return State{}, fmt.Errorf("%w: trash a document before purging it", ErrLifecycle)
		case LifecycleTrashed:
			next.Lifecycle = LifecyclePurged
			next.ActiveGeneration = 0
			changed = true
		case LifecyclePurged:
			return s, nil
		}

	default:
		return State{}, fmt.Errorf("%w: unknown command %q", ErrInvalidCommand, command.Kind)
	}

	if changed {
		next.Revision++
	}
	if err := next.Validate(); err != nil {
		return State{}, err
	}
	return next, nil
}
