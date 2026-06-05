package store

import "fmt"

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
