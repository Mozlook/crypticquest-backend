package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// Hint is the player-facing view of a single hint. Hints carry no secrets, so
// the text is safe to expose. The raw order_index is internal (like a level's),
// so it is not serialized — callers receive hints already ordered, and the slice
// order is the contract the frontend reveals against (timers live on the front).
type Hint struct {
	ID   int64  `json:"id"`
	Text string `json:"text"`
}

// HintsForLevel returns every hint of a level, ordered by order_index. Access is
// the caller's responsibility (gate on the level before calling). A level with
// no hints yields an empty, non-nil slice.
func (s *Store) HintsForLevel(levelID int64) ([]Hint, error) {
	rows, err := s.db.Query(
		`SELECT id, text FROM hints WHERE level_id = ? ORDER BY order_index`,
		levelID,
	)
	if err != nil {
		return nil, fmt.Errorf("hints for level: %w", err)
	}
	defer rows.Close()

	hints := []Hint{} // non-nil so JSON encodes [] not null
	for rows.Next() {
		var h Hint
		if err := rows.Scan(&h.ID, &h.Text); err != nil {
			return nil, fmt.Errorf("scan hint: %w", err)
		}
		hints = append(hints, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hints: %w", err)
	}
	return hints, nil
}

// ReplaceHints replaces all of a level's hints with the given ordered texts, in
// one transaction: the slice position becomes order_index (1-based), so this one
// call covers add, remove, and reorder. An empty slice clears all hints. Returns
// ErrNotFound if the level does not exist (so the admin handler can answer 404).
func (s *Store) ReplaceHints(levelID int64, texts []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("replace hints: %w", err)
	}
	defer tx.Rollback() // no-op after a successful Commit

	var one int
	err = tx.QueryRow(`SELECT 1 FROM levels WHERE id = ?`, levelID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("replace hints: lookup level: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM hints WHERE level_id = ?`, levelID); err != nil {
		return fmt.Errorf("replace hints: clear: %w", err)
	}
	for i, text := range texts {
		if _, err := tx.Exec(
			`INSERT INTO hints (level_id, order_index, text) VALUES (?, ?, ?)`,
			levelID, i+1, text,
		); err != nil {
			return fmt.Errorf("replace hints: insert %d: %w", i+1, err)
		}
	}
	return tx.Commit()
}
