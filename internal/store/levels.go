package store

import (
	"database/sql"
	"errors"
	"fmt"
)

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

// LevelDetail is the player-facing view of a single level. The flag is never
// selected. OrderIndex is carried for the access gate but never serialized
// (raw order_index values are internal; the player only sees ordering).
type LevelDetail struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Solved      bool   `json:"solved"`
	OrderIndex  int    `json:"-"`
}

// LevelByID returns the player view of a level (no flag), with solved computed
// for the given user. Returns ErrNotFound when the level does not exist, so the
// caller can answer 404 vs the 403 it returns for an existing-but-locked level.
func (s *Store) LevelByID(userID, levelID int64) (LevelDetail, error) {
	row := s.db.QueryRow(
		`SELECT l.id, l.title, l.description, l.order_index,
		        CASE WHEN up.level_id IS NOT NULL THEN 1 ELSE 0 END
		 FROM levels l
		 LEFT JOIN user_progress up ON up.level_id = l.id AND up.user_id = ?
		 WHERE l.id = ?`,
		userID, levelID,
	)
	var d LevelDetail
	var solved int
	err := row.Scan(&d.ID, &d.Title, &d.Description, &d.OrderIndex, &solved)
	if errors.Is(err, sql.ErrNoRows) {
		return LevelDetail{}, ErrNotFound
	}
	if err != nil {
		return LevelDetail{}, fmt.Errorf("level by id: %w", err)
	}
	d.Solved = solved != 0
	return d, nil
}
