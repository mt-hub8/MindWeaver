// Package indexing owns the generation cut-over protocol. Physical indexes and
// provider clients live behind shared infrastructure; this package only models
// the atomic product semantics needed by every index implementation.
package indexing

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const GenerationProtocolVersion = "generation/v1"

var generationSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var (
	ErrInvalidGeneration  = errors.New("invalid generation")
	ErrGenerationExists   = errors.New("generation already exists with different identity")
	ErrBuildInProgress    = errors.New("another generation build is in progress")
	ErrTransition         = errors.New("generation transition is not allowed")
	ErrActivationConflict = errors.New("active generation changed before cut-over")
)

type GenerationState string

const (
	GenerationReserved  GenerationState = "RESERVED"
	GenerationBuilding  GenerationState = "BUILDING"
	GenerationReady     GenerationState = "READY_TO_ACTIVATE"
	GenerationActive    GenerationState = "ACTIVE"
	GenerationRetired   GenerationState = "RETIRED"
	GenerationFailed    GenerationState = "FAILED"
	GenerationCancelled GenerationState = "CANCELLED"
)

func (s GenerationState) buildInProgress() bool {
	return s == GenerationReserved || s == GenerationBuilding || s == GenerationReady
}

// Generation is immutable in identity (document, number, source text and
// chunker fingerprint) and mutable only through the methods on Set.
type Generation struct {
	DocumentID          string          `json:"document_id"`
	Number              uint64          `json:"number"`
	State               GenerationState `json:"state"`
	SourceArtifactID    string          `json:"source_artifact_id"`
	SourceTextSHA256    string          `json:"source_text_sha256"`
	ChunkerFingerprint  string          `json:"chunker_fingerprint"`
	IndexFingerprint    string          `json:"index_fingerprint"`
	ChunkCount          uint32          `json:"chunk_count"`
	ChunkManifestSHA256 string          `json:"chunk_manifest_sha256,omitempty"`
	IndexedChunkCount   uint32          `json:"indexed_chunk_count"`
	IndexManifestSHA256 string          `json:"index_manifest_sha256,omitempty"`
	Revision            uint64          `json:"revision"`
}

func (g Generation) Validate() error {
	if strings.TrimSpace(g.DocumentID) == "" || g.Number == 0 || g.Revision == 0 {
		return fmt.Errorf("%w: document, number and revision are required", ErrInvalidGeneration)
	}
	if !generationSHA256Pattern.MatchString(g.SourceTextSHA256) {
		return fmt.Errorf("%w: source text digest must be lowercase sha256", ErrInvalidGeneration)
	}
	if strings.TrimSpace(g.SourceArtifactID) == "" || strings.TrimSpace(g.ChunkerFingerprint) == "" ||
		strings.TrimSpace(g.IndexFingerprint) == "" {
		return fmt.Errorf("%w: artifact and build fingerprints are required", ErrInvalidGeneration)
	}
	if g.IndexedChunkCount > g.ChunkCount {
		return fmt.Errorf("%w: indexed chunks exceed produced chunks", ErrInvalidGeneration)
	}
	switch g.State {
	case GenerationReserved:
		if g.ChunkCount != 0 || g.IndexedChunkCount != 0 || g.ChunkManifestSHA256 != "" || g.IndexManifestSHA256 != "" {
			return fmt.Errorf("%w: reserved generation cannot have materialized chunks", ErrInvalidGeneration)
		}
	case GenerationBuilding, GenerationFailed, GenerationCancelled:
		if g.ChunkCount > 0 && !generationSHA256Pattern.MatchString(g.ChunkManifestSHA256) {
			return fmt.Errorf("%w: materialized chunks require a manifest digest", ErrInvalidGeneration)
		}
		if g.IndexedChunkCount > 0 && !generationSHA256Pattern.MatchString(g.IndexManifestSHA256) {
			return fmt.Errorf("%w: indexed chunks require a manifest digest", ErrInvalidGeneration)
		}
	case GenerationReady, GenerationActive, GenerationRetired:
		if g.ChunkCount == 0 || g.IndexedChunkCount != g.ChunkCount ||
			!generationSHA256Pattern.MatchString(g.ChunkManifestSHA256) ||
			!generationSHA256Pattern.MatchString(g.IndexManifestSHA256) {
			return fmt.Errorf("%w: activatable generation must have a complete non-empty index", ErrInvalidGeneration)
		}
	default:
		return fmt.Errorf("%w: unknown state %q", ErrInvalidGeneration, g.State)
	}
	return nil
}

// Set is the per-document generation aggregate. Activating a candidate and
// retiring the prior active generation happen in one pure transition.
type Set struct {
	Protocol         string       `json:"protocol"`
	DocumentID       string       `json:"document_id"`
	Revision         uint64       `json:"revision"`
	ActiveGeneration uint64       `json:"active_generation"`
	Generations      []Generation `json:"generations"`
}

func NewSet(documentID string) (Set, error) {
	s := Set{
		Protocol:   GenerationProtocolVersion,
		DocumentID: strings.TrimSpace(documentID),
		Revision:   1,
	}
	if err := s.Validate(); err != nil {
		return Set{}, err
	}
	return s, nil
}

func (s Set) Validate() error {
	if s.Protocol != GenerationProtocolVersion {
		return fmt.Errorf("%w: protocol must be %q", ErrInvalidGeneration, GenerationProtocolVersion)
	}
	if strings.TrimSpace(s.DocumentID) == "" || s.Revision == 0 {
		return fmt.Errorf("%w: set document and revision are required", ErrInvalidGeneration)
	}
	activeCount := 0
	inProgressCount := 0
	var previous uint64
	for i, generation := range s.Generations {
		if generation.DocumentID != s.DocumentID {
			return fmt.Errorf("%w: generation belongs to another document", ErrInvalidGeneration)
		}
		if err := generation.Validate(); err != nil {
			return err
		}
		if i > 0 && generation.Number <= previous {
			return fmt.Errorf("%w: generations must be unique and sorted", ErrInvalidGeneration)
		}
		previous = generation.Number
		if generation.State == GenerationActive {
			activeCount++
			if generation.Number != s.ActiveGeneration {
				return fmt.Errorf("%w: active marker does not match active generation", ErrInvalidGeneration)
			}
		}
		if generation.State.buildInProgress() {
			inProgressCount++
		}
	}
	if activeCount > 1 || inProgressCount > 1 {
		return fmt.Errorf("%w: at most one active and one candidate generation are allowed", ErrInvalidGeneration)
	}
	if (s.ActiveGeneration == 0) != (activeCount == 0) {
		return fmt.Errorf("%w: active generation pointer is inconsistent", ErrInvalidGeneration)
	}
	return nil
}

// Reserve creates the next build candidate. Replaying the same immutable
// identity is a no-op; reusing a number for different source/config is rejected.
func (s Set) Reserve(number uint64, sourceArtifactID, sourceTextSHA256, chunkerFingerprint, indexFingerprint string) (Set, error) {
	if err := s.Validate(); err != nil {
		return Set{}, err
	}
	for _, existing := range s.Generations {
		if existing.Number == number {
			if existing.SourceArtifactID == sourceArtifactID && existing.SourceTextSHA256 == sourceTextSHA256 &&
				existing.ChunkerFingerprint == chunkerFingerprint && existing.IndexFingerprint == indexFingerprint {
				return s, nil
			}
			return Set{}, ErrGenerationExists
		}
		if existing.State.buildInProgress() {
			return Set{}, ErrBuildInProgress
		}
		if existing.Number >= number {
			return Set{}, fmt.Errorf("%w: number must exceed every prior generation", ErrInvalidGeneration)
		}
	}
	candidate := Generation{
		DocumentID:         s.DocumentID,
		Number:             number,
		State:              GenerationReserved,
		SourceArtifactID:   strings.TrimSpace(sourceArtifactID),
		SourceTextSHA256:   sourceTextSHA256,
		ChunkerFingerprint: strings.TrimSpace(chunkerFingerprint),
		IndexFingerprint:   strings.TrimSpace(indexFingerprint),
		Revision:           1,
	}
	if err := candidate.Validate(); err != nil {
		return Set{}, err
	}
	next := cloneSet(s)
	next.Generations = append(next.Generations, candidate)
	sort.Slice(next.Generations, func(i, j int) bool {
		return next.Generations[i].Number < next.Generations[j].Number
	})
	next.Revision++
	return next, next.Validate()
}

func (s Set) Start(number uint64) (Set, error) {
	return s.mutate(number, func(g Generation) (Generation, bool, error) {
		switch g.State {
		case GenerationReserved:
			g.State = GenerationBuilding
			return g, true, nil
		case GenerationBuilding:
			return g, false, nil
		default:
			return Generation{}, false, fmt.Errorf("%w: cannot start %s", ErrTransition, g.State)
		}
	})
}

// RecordProgress persists a replay-safe build checkpoint. Chunk count is fixed
// after first materialization and indexed count may only advance.
func (s Set) RecordProgress(number uint64, chunks uint32, chunkManifestSHA256 string, indexed uint32, indexManifestSHA256 string) (Set, error) {
	return s.mutate(number, func(g Generation) (Generation, bool, error) {
		if g.State != GenerationBuilding {
			return Generation{}, false, fmt.Errorf("%w: progress requires BUILDING", ErrTransition)
		}
		if chunks == 0 || indexed > chunks {
			return Generation{}, false, fmt.Errorf("%w: invalid build counts", ErrInvalidGeneration)
		}
		if g.ChunkCount != 0 && g.ChunkCount != chunks {
			return Generation{}, false, fmt.Errorf("%w: chunk count changed during replay", ErrInvalidGeneration)
		}
		if !generationSHA256Pattern.MatchString(chunkManifestSHA256) ||
			(indexed > 0 && !generationSHA256Pattern.MatchString(indexManifestSHA256)) {
			return Generation{}, false, fmt.Errorf("%w: build manifests must be lowercase sha256", ErrInvalidGeneration)
		}
		if g.ChunkManifestSHA256 != "" && g.ChunkManifestSHA256 != chunkManifestSHA256 {
			return Generation{}, false, fmt.Errorf("%w: chunk manifest changed during replay", ErrInvalidGeneration)
		}
		if indexed < g.IndexedChunkCount {
			return Generation{}, false, fmt.Errorf("%w: indexed progress regressed", ErrInvalidGeneration)
		}
		if g.IndexedChunkCount == indexed && g.IndexManifestSHA256 != "" && g.IndexManifestSHA256 != indexManifestSHA256 {
			return Generation{}, false, fmt.Errorf("%w: index manifest changed at same checkpoint", ErrInvalidGeneration)
		}
		if g.ChunkCount == chunks && g.ChunkManifestSHA256 == chunkManifestSHA256 &&
			g.IndexedChunkCount == indexed && g.IndexManifestSHA256 == indexManifestSHA256 {
			return g, false, nil
		}
		g.ChunkCount = chunks
		g.ChunkManifestSHA256 = chunkManifestSHA256
		g.IndexedChunkCount = indexed
		g.IndexManifestSHA256 = indexManifestSHA256
		return g, true, nil
	})
}

func (s Set) MarkReady(number uint64) (Set, error) {
	return s.mutate(number, func(g Generation) (Generation, bool, error) {
		switch g.State {
		case GenerationBuilding:
			if g.ChunkCount == 0 || g.IndexedChunkCount != g.ChunkCount {
				return Generation{}, false, fmt.Errorf("%w: generation is not fully indexed", ErrTransition)
			}
			g.State = GenerationReady
			return g, true, nil
		case GenerationReady:
			return g, false, nil
		default:
			return Generation{}, false, fmt.Errorf("%w: cannot mark %s ready", ErrTransition, g.State)
		}
	})
}

func (s Set) Activate(number, expectedActive uint64) (Set, error) {
	if err := s.Validate(); err != nil {
		return Set{}, err
	}
	if s.ActiveGeneration == number {
		return s, nil
	}
	if s.ActiveGeneration != expectedActive {
		return Set{}, ErrActivationConflict
	}
	target := -1
	for i := range s.Generations {
		if s.Generations[i].Number == number {
			target = i
			break
		}
	}
	if target < 0 || s.Generations[target].State != GenerationReady {
		return Set{}, fmt.Errorf("%w: target must be READY_TO_ACTIVATE", ErrTransition)
	}
	next := cloneSet(s)
	for i := range next.Generations {
		switch next.Generations[i].State {
		case GenerationActive:
			next.Generations[i].State = GenerationRetired
			next.Generations[i].Revision++
		case GenerationReady:
			if next.Generations[i].Number == number {
				next.Generations[i].State = GenerationActive
				next.Generations[i].Revision++
			}
		}
	}
	next.ActiveGeneration = number
	next.Revision++
	return next, next.Validate()
}

func (s Set) Fail(number uint64) (Set, error) {
	return s.finishCandidate(number, GenerationFailed)
}

func (s Set) Cancel(number uint64) (Set, error) {
	return s.finishCandidate(number, GenerationCancelled)
}

func (s Set) finishCandidate(number uint64, terminal GenerationState) (Set, error) {
	return s.mutate(number, func(g Generation) (Generation, bool, error) {
		if g.State == terminal {
			return g, false, nil
		}
		if !g.State.buildInProgress() {
			return Generation{}, false, fmt.Errorf("%w: cannot finish %s as %s", ErrTransition, g.State, terminal)
		}
		g.State = terminal
		return g, true, nil
	})
}

func (s Set) mutate(number uint64, change func(Generation) (Generation, bool, error)) (Set, error) {
	if err := s.Validate(); err != nil {
		return Set{}, err
	}
	next := cloneSet(s)
	for i, generation := range next.Generations {
		if generation.Number != number {
			continue
		}
		updated, changed, err := change(generation)
		if err != nil {
			return Set{}, err
		}
		if !changed {
			return s, nil
		}
		updated.Revision++
		next.Generations[i] = updated
		next.Revision++
		return next, next.Validate()
	}
	return Set{}, fmt.Errorf("%w: generation %d not found", ErrInvalidGeneration, number)
}

func cloneSet(s Set) Set {
	next := s
	next.Generations = append([]Generation(nil), s.Generations...)
	return next
}
