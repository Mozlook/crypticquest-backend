package store

import "fmt"

// LevelListItem is the player-facing summary of a level in the level list. It
// deliberately omits the flag (and every other server-only field) so the flag
// can never leak through this representation — it is not even selected.
type LevelListItem struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Solved bool   `json:"solved"`
}

// ListAccessibleLevels returns the player's accessible levels — every solved
// level plus the next unsolved one — ordered by order_index. Implemented
// positionally as "the first `current` levels", matching IsLevelAccessible.
// The flag column is never selected.
func (s *Store) ListAccessibleLevels(userID int64) ([]LevelListItem, error) {
	current, err := s.CurrentLevel(userID)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		`SELECT l.id, l.title,
		        CASE WHEN up.level_id IS NOT NULL THEN 1 ELSE 0 END AS solved
		 FROM levels l
		 LEFT JOIN user_progress up ON up.level_id = l.id AND up.user_id = ?
		 ORDER BY l.order_index
		 LIMIT ?`,
		userID, current,
	)
	if err != nil {
		return nil, fmt.Errorf("list levels: %w", err)
	}
	defer rows.Close()

	items := []LevelListItem{} // non-nil so JSON encodes [] not null
	for rows.Next() {
		var it LevelListItem
		var solved int
		if err := rows.Scan(&it.ID, &it.Title, &solved); err != nil {
			return nil, fmt.Errorf("scan level: %w", err)
		}
		it.Solved = solved != 0
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate levels: %w", err)
	}
	return items, nil
}
