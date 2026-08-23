// Package collection defines collection membership and exact retrieval scopes.
// Membership is a true many-to-many edge and is never copied onto chunk rows.
package collection

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

const ProtocolVersion = "collection-membership/v1"

var (
	ErrInvalidMembership = errors.New("invalid collection membership")
	ErrInvalidScope      = errors.New("invalid collection scope")
)

type Membership struct {
	CollectionID string `json:"collection_id"`
	DocumentID   string `json:"document_id"`
}

func (m Membership) Validate() error {
	if strings.TrimSpace(m.CollectionID) == "" || strings.TrimSpace(m.DocumentID) == "" {
		return fmt.Errorf("%w: collection and document ids are required", ErrInvalidMembership)
	}
	return nil
}

func (m Membership) key() string {
	return m.CollectionID + "\x00" + m.DocumentID
}

// State contains a minimal collection catalog so an unknown collection and a
// known empty collection cannot collapse into the same scope result.
type State struct {
	Protocol           string       `json:"protocol"`
	Revision           uint64       `json:"revision"`
	MembershipRevision uint64       `json:"membership_revision"`
	Collections        []string     `json:"collections"`
	Memberships        []Membership `json:"memberships"`
}

func New() State {
	return State{
		Protocol:           ProtocolVersion,
		Revision:           1,
		MembershipRevision: 1,
		Collections:        []string{},
		Memberships:        []Membership{},
	}
}

func (s State) Validate() error {
	if s.Protocol != ProtocolVersion || s.Revision == 0 || s.MembershipRevision == 0 {
		return fmt.Errorf("%w: protocol and revisions are required", ErrInvalidMembership)
	}
	if !strictlySorted(s.Collections) {
		return fmt.Errorf("%w: collection catalog must be unique and sorted", ErrInvalidMembership)
	}
	known := make(map[string]struct{}, len(s.Collections))
	for _, collectionID := range s.Collections {
		known[collectionID] = struct{}{}
	}
	previous := ""
	for i, membership := range s.Memberships {
		if err := membership.Validate(); err != nil {
			return err
		}
		if _, ok := known[membership.CollectionID]; !ok {
			return fmt.Errorf("%w: unknown collection %q", ErrInvalidMembership, membership.CollectionID)
		}
		key := membership.key()
		if i > 0 && key <= previous {
			return fmt.Errorf("%w: memberships must be unique and sorted", ErrInvalidMembership)
		}
		previous = key
	}
	return nil
}

// Create registers stable collection identity. Display metadata belongs to the
// future collection store adapter and is not needed for scope correctness.
func (s State) Create(collectionID string) (State, error) {
	if err := s.Validate(); err != nil {
		return State{}, err
	}
	collectionID = strings.TrimSpace(collectionID)
	if collectionID == "" {
		return State{}, fmt.Errorf("%w: collection id is required", ErrInvalidMembership)
	}
	if containsSorted(s.Collections, collectionID) {
		return s, nil
	}
	next := clone(s)
	next.Collections = append(next.Collections, collectionID)
	sort.Strings(next.Collections)
	next.Revision++
	return next, next.Validate()
}

func (s State) Assign(collectionID, documentID string) (State, error) {
	if err := s.Validate(); err != nil {
		return State{}, err
	}
	membership := Membership{
		CollectionID: strings.TrimSpace(collectionID),
		DocumentID:   strings.TrimSpace(documentID),
	}
	if err := membership.Validate(); err != nil {
		return State{}, err
	}
	if !containsSorted(s.Collections, membership.CollectionID) {
		return State{}, fmt.Errorf("%w: collection %q is not registered", ErrInvalidMembership, membership.CollectionID)
	}
	for _, existing := range s.Memberships {
		if existing == membership {
			return s, nil
		}
	}
	next := clone(s)
	next.Memberships = append(next.Memberships, membership)
	sortMemberships(next.Memberships)
	next.Revision++
	next.MembershipRevision++
	return next, next.Validate()
}

func (s State) Remove(collectionID, documentID string) (State, error) {
	if err := s.Validate(); err != nil {
		return State{}, err
	}
	target := Membership{
		CollectionID: strings.TrimSpace(collectionID),
		DocumentID:   strings.TrimSpace(documentID),
	}
	if err := target.Validate(); err != nil {
		return State{}, err
	}
	if !containsSorted(s.Collections, target.CollectionID) {
		return State{}, fmt.Errorf("%w: collection %q is not registered", ErrInvalidMembership, target.CollectionID)
	}
	next := clone(s)
	for i, existing := range next.Memberships {
		if existing != target {
			continue
		}
		next.Memberships = append(next.Memberships[:i], next.Memberships[i+1:]...)
		next.Revision++
		next.MembershipRevision++
		return next, next.Validate()
	}
	return s, nil
}

func (s State) CollectionsFor(documentID string) []string {
	seen := make(map[string]struct{})
	for _, membership := range s.Memberships {
		if membership.DocumentID == documentID {
			seen[membership.CollectionID] = struct{}{}
		}
	}
	return sortedKeys(seen)
}

func (s State) DocumentsIn(collectionID string) []string {
	seen := make(map[string]struct{})
	for _, membership := range s.Memberships {
		if membership.CollectionID == collectionID {
			seen[membership.DocumentID] = struct{}{}
		}
	}
	return sortedKeys(seen)
}

type ScopeMode string

const (
	ScopeAllDocuments ScopeMode = "ALL_DOCUMENTS"
	ScopeCollections  ScopeMode = "COLLECTIONS_ANY"
)

// ResolvedScopeKind is intentionally three-valued. NONE is never encoded as an
// empty allowlist because empty filters historically failed open in Java.
type ResolvedScopeKind string

const (
	ResolvedAll         ResolvedScopeKind = "ALL"
	ResolvedDocumentSet ResolvedScopeKind = "DOCUMENT_SET"
	ResolvedNone        ResolvedScopeKind = "NONE"
)

type ScopeRequest struct {
	Mode          ScopeMode `json:"mode"`
	CollectionIDs []string  `json:"collection_ids"`
}

type ScopedDocument struct {
	DocumentID           string   `json:"document_id"`
	MatchedCollectionIDs []string `json:"matched_collection_ids"`
}

// ScopeSnapshot freezes membership revision and per-document membership proof.
type ScopeSnapshot struct {
	Mode               ScopeMode         `json:"mode"`
	Kind               ResolvedScopeKind `json:"kind"`
	CollectionIDs      []string          `json:"collection_ids"`
	MembershipRevision uint64            `json:"membership_revision"`
	Documents          []ScopedDocument  `json:"documents"`
}

// Resolve intersects membership with the caller's eligible document set. The
// eligible list is where lifecycle and active-generation predicates are applied.
func (s State) Resolve(request ScopeRequest, eligibleDocumentIDs []string) (ScopeSnapshot, error) {
	if err := s.Validate(); err != nil {
		return ScopeSnapshot{}, err
	}
	eligible := make(map[string]struct{}, len(eligibleDocumentIDs))
	for _, id := range eligibleDocumentIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return ScopeSnapshot{}, fmt.Errorf("%w: eligible document id is blank", ErrInvalidScope)
		}
		eligible[id] = struct{}{}
	}
	collections, err := canonicalIDs(request.CollectionIDs)
	if err != nil {
		return ScopeSnapshot{}, err
	}
	for _, collectionID := range collections {
		if !containsSorted(s.Collections, collectionID) {
			return ScopeSnapshot{}, fmt.Errorf("%w: collection %q is not registered", ErrInvalidScope, collectionID)
		}
	}

	documents := make(map[string]map[string]struct{})
	switch request.Mode {
	case ScopeAllDocuments:
		if len(collections) != 0 {
			return ScopeSnapshot{}, fmt.Errorf("%w: ALL_DOCUMENTS cannot name collections", ErrInvalidScope)
		}
		for id := range eligible {
			documents[id] = map[string]struct{}{}
		}
	case ScopeCollections:
		if len(collections) == 0 {
			return ScopeSnapshot{}, fmt.Errorf("%w: collection scope requires at least one id", ErrInvalidScope)
		}
		selected := make(map[string]struct{}, len(collections))
		for _, id := range collections {
			selected[id] = struct{}{}
		}
		for _, membership := range s.Memberships {
			if _, ok := selected[membership.CollectionID]; !ok {
				continue
			}
			if _, ok := eligible[membership.DocumentID]; !ok {
				continue
			}
			if documents[membership.DocumentID] == nil {
				documents[membership.DocumentID] = make(map[string]struct{})
			}
			documents[membership.DocumentID][membership.CollectionID] = struct{}{}
		}
	default:
		return ScopeSnapshot{}, fmt.Errorf("%w: unknown scope mode %q", ErrInvalidScope, request.Mode)
	}

	documentIDs := make([]string, 0, len(documents))
	for documentID := range documents {
		documentIDs = append(documentIDs, documentID)
	}
	sort.Strings(documentIDs)
	scopedDocuments := make([]ScopedDocument, 0, len(documentIDs))
	for _, documentID := range documentIDs {
		scopedDocuments = append(scopedDocuments, ScopedDocument{
			DocumentID:           documentID,
			MatchedCollectionIDs: sortedKeys(documents[documentID]),
		})
	}
	kind := ResolvedNone
	if len(scopedDocuments) > 0 {
		if request.Mode == ScopeAllDocuments {
			kind = ResolvedAll
		} else {
			kind = ResolvedDocumentSet
		}
	}
	return ScopeSnapshot{
		Mode:               request.Mode,
		Kind:               kind,
		CollectionIDs:      collections,
		MembershipRevision: s.MembershipRevision,
		Documents:          scopedDocuments,
	}, nil
}

func clone(s State) State {
	next := s
	next.Collections = append([]string(nil), s.Collections...)
	next.Memberships = append([]Membership(nil), s.Memberships...)
	return next
}

func sortMemberships(memberships []Membership) {
	sort.Slice(memberships, func(i, j int) bool {
		return memberships[i].key() < memberships[j].key()
	})
}

func canonicalIDs(ids []string) ([]string, error) {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, fmt.Errorf("%w: id is blank", ErrInvalidScope)
		}
		seen[id] = struct{}{}
	}
	return sortedKeys(seen), nil
}

func strictlySorted(values []string) bool {
	for i, value := range values {
		if strings.TrimSpace(value) == "" || (i > 0 && values[i-1] >= value) {
			return false
		}
	}
	return true
}

func containsSorted(values []string, target string) bool {
	i := sort.SearchStrings(values, target)
	return i < len(values) && values[i] == target
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
