package store

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Group is a host grouping (e.g., "Production", "Staging").
type Group struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sortOrder"`
	CreatedAt int64  `json:"createdAt"`
}

// CreateGroup inserts a new group with a fresh UUID. Returns an error if a
// group with the same name already exists (UNIQUE constraint).
func (s *Store) CreateGroup(name string) (Group, error) {
	g := Group{
		ID:        uuid.NewString(),
		Name:      name,
		SortOrder: 0,
		CreatedAt: time.Now().Unix(),
	}
	_, err := s.db.Exec(
		`INSERT INTO groups(id, name, sort_order, created_at) VALUES(?, ?, ?, ?)`,
		g.ID, g.Name, g.SortOrder, g.CreatedAt,
	)
	if err != nil {
		return Group{}, fmt.Errorf("store: create group: %w", err)
	}
	return g, nil
}

// ListGroups returns all groups, ordered by sort_order then name.
func (s *Store) ListGroups() ([]Group, error) {
	rows, err := s.db.Query(`SELECT id, name, sort_order, created_at FROM groups ORDER BY sort_order, name`)
	if err != nil {
		return nil, fmt.Errorf("store: list groups: %w", err)
	}
	defer rows.Close()

	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.SortOrder, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// RenameGroup updates the name of an existing group.
func (s *Store) RenameGroup(id, name string) error {
	res, err := s.db.Exec(`UPDATE groups SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return fmt.Errorf("store: rename group: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("store: rename group: id %q not found", id)
	}
	return nil
}

// DeleteGroup removes a group. Hosts in the deleted group have their group_id
// set to NULL via the foreign-key cascade.
func (s *Store) DeleteGroup(id string) error {
	_, err := s.db.Exec(`DELETE FROM groups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete group: %w", err)
	}
	return nil
}
