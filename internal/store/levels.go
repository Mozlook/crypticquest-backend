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

// LevelForSubmit fetches the data needed to validate a flag submission: the
// order_index (for the access gate) and the server-only flag. This is the one
// player-flow method that reads the flag, kept separate from the player views
// so the flag never leaks into a response struct. Returns ErrNotFound if absent.
func (s *Store) LevelForSubmit(levelID int64) (orderIndex int, flag string, err error) {
	err = s.db.QueryRow(
		`SELECT order_index, flag FROM levels WHERE id = ?`,
		levelID,
	).Scan(&orderIndex, &flag)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	if err != nil {
		return 0, "", fmt.Errorf("level for submit: %w", err)
	}
	return orderIndex, flag, nil
}

// --- Admin surface (includes the flag) -------------------------------------
//
// These are the ONLY level reads/writes that carry the flag besides
// LevelForSubmit. They back the role-gated /api/admin/levels endpoints. Keep the
// flag out of any struct served to players (see LevelListItem / LevelDetail).

// AdminLevel is the full level as an admin sees it — flag included. Which tools a
// level unlocks lives on the tools side now (tools.unlocks_at_level_id), so a
// level no longer carries a tool reference.
type AdminLevel struct {
	ID          int64  `json:"id"`
	OrderIndex  int    `json:"order_index"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Flag        string `json:"flag"`
}

// AdminLevelInput is the writable subset of a level (no id, which the DB owns).
type AdminLevelInput struct {
	OrderIndex  int
	Title       string
	Description string
	Flag        string
}

// ListAllLevels returns every level (flag included) ordered by order_index, for
// the admin list. Unlike ListAccessibleLevels there is no per-user gate.
func (s *Store) ListAllLevels() ([]AdminLevel, error) {
	rows, err := s.db.Query(
		`SELECT id, order_index, title, description, flag
		 FROM levels ORDER BY order_index`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all levels: %w", err)
	}
	defer rows.Close()

	levels := []AdminLevel{} // non-nil so JSON encodes [] not null
	for rows.Next() {
		l, err := scanAdminLevel(rows)
		if err != nil {
			return nil, err
		}
		levels = append(levels, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate levels: %w", err)
	}
	return levels, nil
}

// AdminLevelByID returns one level with its flag, or ErrNotFound.
func (s *Store) AdminLevelByID(id int64) (AdminLevel, error) {
	row := s.db.QueryRow(
		`SELECT id, order_index, title, description, flag
		 FROM levels WHERE id = ?`,
		id,
	)
	l, err := scanAdminLevel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminLevel{}, ErrNotFound
	}
	if err != nil {
		return AdminLevel{}, err
	}
	return l, nil
}

// CreateLevel inserts a level and returns its new id. Maps a duplicate
// order_index to ErrOrderIndexTaken so the handler can answer 409.
func (s *Store) CreateLevel(in AdminLevelInput) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO levels (order_index, title, description, flag)
		 VALUES (?, ?, ?, ?)`,
		in.OrderIndex, in.Title, in.Description, in.Flag,
	)
	if err != nil {
		return 0, mapLevelWriteErr(err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// UpdateLevel overwrites a level by id. Returns ErrNotFound if no such level,
// and the same mapped constraint errors as CreateLevel.
func (s *Store) UpdateLevel(id int64, in AdminLevelInput) error {
	res, err := s.db.Exec(
		`UPDATE levels
		 SET order_index = ?, title = ?, description = ?, flag = ?
		 WHERE id = ?`,
		in.OrderIndex, in.Title, in.Description, in.Flag, id,
	)
	if err != nil {
		return mapLevelWriteErr(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteLevel removes a level by id (cascading to its hints and any
// user_progress). Returns ErrNotFound if there was nothing to delete.
func (s *Store) DeleteLevel(id int64) error {
	res, err := s.db.Exec(`DELETE FROM levels WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete level: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// scanAdminLevel scans one level row. Works for both *sql.Row and *sql.Rows.
func scanAdminLevel(sc interface{ Scan(...any) error }) (AdminLevel, error) {
	var l AdminLevel
	if err := sc.Scan(&l.ID, &l.OrderIndex, &l.Title, &l.Description, &l.Flag); err != nil {
		return AdminLevel{}, err
	}
	return l, nil
}

// mapLevelWriteErr translates SQLite constraint failures on a level write into
// the store's sentinels; anything else is wrapped. A level has no outbound
// foreign key now, so order_index uniqueness is the only constraint to map.
func mapLevelWriteErr(err error) error {
	switch {
	case isUniqueViolation(err):
		return ErrOrderIndexTaken
	default:
		return fmt.Errorf("level write: %w", err)
	}
}
