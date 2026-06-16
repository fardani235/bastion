package store

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Snippet is a saved command body the user can paste into a session.
type Snippet struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

func (s *Store) CreateSnippet(label, body string) (Snippet, error) {
	now := time.Now().Unix()
	sn := Snippet{
		ID:        uuid.NewString(),
		Label:     label,
		Body:      body,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := s.db.Exec(
		`INSERT INTO snippets(id, label, body, created_at, updated_at) VALUES(?,?,?,?,?)`,
		sn.ID, sn.Label, sn.Body, sn.CreatedAt, sn.UpdatedAt,
	)
	if err != nil {
		return Snippet{}, fmt.Errorf("store: create snippet: %w", err)
	}
	return sn, nil
}

func (s *Store) ListSnippets() ([]Snippet, error) {
	rows, err := s.db.Query(`SELECT id, label, body, created_at, updated_at FROM snippets ORDER BY label`)
	if err != nil {
		return nil, fmt.Errorf("store: list snippets: %w", err)
	}
	defer rows.Close()
	var out []Snippet
	for rows.Next() {
		var sn Snippet
		if err := rows.Scan(&sn.ID, &sn.Label, &sn.Body, &sn.CreatedAt, &sn.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, sn)
	}
	return out, rows.Err()
}

func (s *Store) UpdateSnippet(id, label, body string) error {
	res, err := s.db.Exec(
		`UPDATE snippets SET label = ?, body = ?, updated_at = ? WHERE id = ?`,
		label, body, time.Now().Unix(), id,
	)
	if err != nil {
		return fmt.Errorf("store: update snippet: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("store: update snippet: id %q not found", id)
	}
	return nil
}

func (s *Store) DeleteSnippet(id string) error {
	_, err := s.db.Exec(`DELETE FROM snippets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete snippet: %w", err)
	}
	return nil
}
