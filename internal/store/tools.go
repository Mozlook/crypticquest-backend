package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// Tool is the player-facing view of a toolkit entry. Tools carry no secrets
// (no flag here, unlike levels), so every column is safe to expose. Description
// is nullable in the schema; COALESCE flattens NULL to "" so it stays a plain
// Go string instead of needing a sql.NullString.
type Tool struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Content     string `json:"content"`
	// UnlocksAtLevelID is the level whose solve unlocks this tool. It is part of
	// the admin surface only; player reads (UnlockedTools) leave it nil and the
	// omitempty tag keeps it out of the player payload entirely.
	UnlocksAtLevelID *int64 `json:"unlocks_at_level_id,omitempty"`
}

// UnlockedTools returns the tools the user has unlocked: every tool whose
// unlocks_at_level_id is a level they have solved. Tools with a NULL
// unlocks_at_level_id are never unlocked (the JOIN drops them). Ordered by tool
// id for a stable, deterministic toolkit.
func (s *Store) UnlockedTools(userID int64) ([]Tool, error) {
	rows, err := s.db.Query(
		`SELECT t.id, t.type, t.title, COALESCE(t.description, ''), t.content
		 FROM tools t
		 JOIN user_progress up ON up.level_id = t.unlocks_at_level_id AND up.user_id = ?
		 ORDER BY t.id`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("unlocked tools: %w", err)
	}
	defer rows.Close()

	tools := []Tool{} // non-nil so JSON encodes [] not null
	for rows.Next() {
		var t Tool
		if err := rows.Scan(&t.ID, &t.Type, &t.Title, &t.Description, &t.Content); err != nil {
			return nil, fmt.Errorf("scan tool: %w", err)
		}
		tools = append(tools, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tools: %w", err)
	}
	return tools, nil
}

// IsToolFileUnlocked reports whether the user may download the toolkit file at
// the given path. A file is unlocked when some tool the user has earned exposes
// it via tool.content — the contract is that a file tool's content holds the
// path relative to files/tools/ (the same segment the URL carries). The JOIN is
// the same earned-tools shape as UnlockedTools; link-type tools store a URL in
// content, so they simply never match a file path.
func (s *Store) IsToolFileUnlocked(userID int64, path string) (bool, error) {
	var unlocked bool
	if err := s.db.QueryRow(
		`SELECT EXISTS(
		   SELECT 1 FROM tools t
		   JOIN user_progress up ON up.level_id = t.unlocks_at_level_id AND up.user_id = ?
		   WHERE t.content = ?
		 )`,
		userID, path,
	).Scan(&unlocked); err != nil {
		return false, fmt.Errorf("is tool file unlocked: %w", err)
	}
	return unlocked, nil
}

// --- Admin surface ----------------------------------------------------------
//
// Tools carry no secrets, so the admin and player views share the Tool struct;
// the difference is only scope (all tools vs unlocked) and the write methods.

// ToolInput is the writable subset of a tool (no id, which the DB owns).
// UnlocksAtLevelID is nil when the tool is not tied to any level (never auto-
// unlocked); a non-nil value must name an existing level or the write fails.
type ToolInput struct {
	Type             string
	Title            string
	Description      string
	Content          string
	UnlocksAtLevelID *int64
}

// ListAllTools returns every tool ordered by id, for the admin list.
func (s *Store) ListAllTools() ([]Tool, error) {
	rows, err := s.db.Query(
		`SELECT id, type, title, COALESCE(description, ''), content, unlocks_at_level_id
		 FROM tools ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all tools: %w", err)
	}
	defer rows.Close()

	tools := []Tool{} // non-nil so JSON encodes [] not null
	for rows.Next() {
		t, err := scanAdminTool(rows)
		if err != nil {
			return nil, err
		}
		tools = append(tools, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tools: %w", err)
	}
	return tools, nil
}

// ToolByID returns one tool, or ErrNotFound.
func (s *Store) ToolByID(id int64) (Tool, error) {
	t, err := scanAdminTool(s.db.QueryRow(
		`SELECT id, type, title, COALESCE(description, ''), content, unlocks_at_level_id
		 FROM tools WHERE id = ?`,
		id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Tool{}, ErrNotFound
	}
	if err != nil {
		return Tool{}, fmt.Errorf("tool by id: %w", err)
	}
	return t, nil
}

// scanAdminTool scans one tool row including the nullable unlocks_at_level_id,
// flattening it into a *int64. Works for both *sql.Row and *sql.Rows.
func scanAdminTool(sc interface{ Scan(...any) error }) (Tool, error) {
	var t Tool
	var levelID sql.NullInt64
	if err := sc.Scan(&t.ID, &t.Type, &t.Title, &t.Description, &t.Content, &levelID); err != nil {
		return Tool{}, err
	}
	if levelID.Valid {
		t.UnlocksAtLevelID = &levelID.Int64
	}
	return t, nil
}

// CreateTool inserts a tool and returns its new id. A bad unlocks_at_level_id
// (no such level) maps to ErrInvalidReference so the handler can answer 400.
func (s *Store) CreateTool(in ToolInput) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO tools (type, title, description, content, unlocks_at_level_id)
		 VALUES (?, ?, ?, ?, ?)`,
		in.Type, in.Title, in.Description, in.Content, in.UnlocksAtLevelID,
	)
	if err != nil {
		return 0, mapToolWriteErr(err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// UpdateTool overwrites a tool by id. Returns ErrNotFound if no such tool, and
// the same mapped reference error as CreateTool.
func (s *Store) UpdateTool(id int64, in ToolInput) error {
	res, err := s.db.Exec(
		`UPDATE tools SET type = ?, title = ?, description = ?, content = ?, unlocks_at_level_id = ?
		 WHERE id = ?`,
		in.Type, in.Title, in.Description, in.Content, in.UnlocksAtLevelID, id,
	)
	if err != nil {
		return mapToolWriteErr(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTool removes a tool by id. Returns ErrNotFound if there was nothing to
// delete. Nothing references a tool any more, so a delete never has to be
// blocked — a level that pointed here is unaffected (the relation lives on the
// tool).
func (s *Store) DeleteTool(id int64) error {
	res, err := s.db.Exec(`DELETE FROM tools WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete tool: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// mapToolWriteErr translates SQLite constraint failures on a tool write into the
// store's sentinels. The only outbound constraint is the unlocks_at_level_id
// foreign key.
func mapToolWriteErr(err error) error {
	if isForeignKeyViolation(err) {
		return ErrInvalidReference
	}
	return fmt.Errorf("tool write: %w", err)
}
