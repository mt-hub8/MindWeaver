package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maxSearchResults    = 100
	maxSearchQueryBytes = 1024
)

// ErrQueryTooShort tells the caller to provide at least three characters for
// the portable trigram index.
var ErrQueryTooShort = errors.New("sqlite: search text must contain at least 3 characters")

// ChunkHit is one active-revision search result. Lower Rank values are better.
type ChunkHit struct {
	ChunkID    string
	DocumentID string
	RevisionID string
	Ordinal    int
	Content    string
	Rank       float64
}

// Search searches active revisions of active documents. text is one literal
// continuous source phrase, never an FTS5 expression or a natural-language
// query to split, rewrite, or expand. The trigram index requires at least 3
// runes; shorter queries return ErrQueryTooShort instead of silently scanning
// or omitting part of the scope.
func (s *Store) Search(ctx context.Context, text string, limit int) ([]ChunkHit, error) {
	query, err := prepareSearch(text, limit)
	if err != nil {
		return nil, err
	}
	return s.searchRows(ctx, `
		SELECT c.id, c.document_id, c.revision_id, c.ordinal, c.content,
			bm25(chunks_fts) AS score
		FROM chunks_fts
		JOIN chunks AS c ON c.row_id = chunks_fts.rowid
		JOIN documents AS d ON d.id = c.document_id
		JOIN document_revisions AS r ON r.id = c.revision_id
		WHERE chunks_fts MATCH ?
			AND d.status = 'active'
			AND r.is_active = 1
		ORDER BY score, c.row_id
		LIMIT ?
	`, ftsPhrase(query), limit)
}

// SearchCollection searches only documents explicitly joined to collectionID.
// Missing and empty collections return no hits and never widen globally.
func (s *Store) SearchCollection(ctx context.Context, collectionID, text string, limit int) ([]ChunkHit, error) {
	if err := validateIdentifier("collection id", collectionID); err != nil {
		return nil, err
	}
	query, err := prepareSearch(text, limit)
	if err != nil {
		return nil, err
	}
	return s.searchRows(ctx, `
		SELECT c.id, c.document_id, c.revision_id, c.ordinal, c.content,
			bm25(chunks_fts) AS score
		FROM chunks_fts
		JOIN chunks AS c ON c.row_id = chunks_fts.rowid
		JOIN documents AS d ON d.id = c.document_id
		JOIN document_revisions AS r ON r.id = c.revision_id
		JOIN collection_documents AS cd
			ON cd.document_id = c.document_id AND cd.collection_id = ?
		WHERE chunks_fts MATCH ?
			AND d.status = 'active'
			AND r.is_active = 1
		ORDER BY score, c.row_id
		LIMIT ?
	`, collectionID, ftsPhrase(query), limit)
}

func (s *Store) searchRows(ctx context.Context, query string, args ...any) ([]ChunkHit, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: search: %w", err)
	}
	defer rows.Close()
	hits := make([]ChunkHit, 0)
	for rows.Next() {
		var hit ChunkHit
		if err := rows.Scan(
			&hit.ChunkID, &hit.DocumentID, &hit.RevisionID,
			&hit.Ordinal, &hit.Content, &hit.Rank,
		); err != nil {
			return nil, fmt.Errorf("sqlite: scan search result: %w", err)
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate search results: %w", err)
	}
	return hits, nil
}

func prepareSearch(text string, limit int) (string, error) {
	query := strings.TrimSpace(text)
	if query == "" {
		return "", errors.New("sqlite: search text is required")
	}
	if len(query) > maxSearchQueryBytes {
		return "", fmt.Errorf("sqlite: search text exceeds %d bytes", maxSearchQueryBytes)
	}
	if !utf8.ValidString(query) {
		return "", errors.New("sqlite: search text is not valid UTF-8")
	}
	if utf8.RuneCountInString(query) < 3 {
		return "", ErrQueryTooShort
	}
	if limit < 1 || limit > maxSearchResults {
		return "", fmt.Errorf("sqlite: search limit must be between 1 and %d", maxSearchResults)
	}
	return query, nil
}

func ftsPhrase(text string) string {
	return `"` + strings.ReplaceAll(text, `"`, `""`) + `"`
}
