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
}

// UnlockedTools returns the tools the user has unlocked: the tools referenced by
// the unlocks_tool_id of any level they have solved. Levels with a NULL
// unlocks_tool_id contribute nothing (the JOIN drops them). DISTINCT guards
// against the same tool being unlocked by more than one solved level. Ordered by
// tool id for a stable, deterministic toolkit.
func (s *Store) UnlockedTools(userID int64) ([]Tool, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT t.id, t.type, t.title, COALESCE(t.description, ''), t.content
		 FROM tools t
		 JOIN levels l ON l.unlocks_tool_id = t.id
		 JOIN user_progress up ON up.level_id = l.id AND up.user_id = ?
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
// the given path. A file is unlocked when some tool the user has earned points
// at it via tool.content — the contract is that a file tool's content holds the
// path relative to files/tools/ (the same segment the URL carries). The JOIN is
// the same earned-tools shape as UnlockedTools; link-type tools store a URL in
// content, so they simply never match a file path.
func (s *Store) IsToolFileUnlocked(userID int64, path string) (bool, error) {
	var unlocked bool
	if err := s.db.QueryRow(
		`SELECT EXISTS(
		   SELECT 1 FROM tools t
		   JOIN levels l ON l.unlocks_tool_id = t.id
		   JOIN user_progress up ON up.level_id = l.id AND up.user_id = ?
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
type ToolInput struct {
	Type        string
	Title       string
	Description string
	Content     string
}

// ListAllTools returns every tool ordered by id, for the admin list.
func (s *Store) ListAllTools() ([]Tool, error) {
	rows, err := s.db.Query(
		`SELECT id, type, title, COALESCE(description, ''), content FROM tools ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all tools: %w", err)
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

// ToolByID returns one tool, or ErrNotFound.
func (s *Store) ToolByID(id int64) (Tool, error) {
	var t Tool
	err := s.db.QueryRow(
		`SELECT id, type, title, COALESCE(description, ''), content FROM tools WHERE id = ?`,
		id,
	).Scan(&t.ID, &t.Type, &t.Title, &t.Description, &t.Content)
	if errors.Is(err, sql.ErrNoRows) {
		return Tool{}, ErrNotFound
	}
	if err != nil {
		return Tool{}, fmt.Errorf("tool by id: %w", err)
	}
	return t, nil
}

// CreateTool inserts a tool and returns its new id.
func (s *Store) CreateTool(in ToolInput) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO tools (type, title, description, content) VALUES (?, ?, ?, ?)`,
		in.Type, in.Title, in.Description, in.Content,
	)
	if err != nil {
		return 0, fmt.Errorf("create tool: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// UpdateTool overwrites a tool by id. Returns ErrNotFound if no such tool.
func (s *Store) UpdateTool(id int64, in ToolInput) error {
	res, err := s.db.Exec(
		`UPDATE tools SET type = ?, title = ?, description = ?, content = ? WHERE id = ?`,
		in.Type, in.Title, in.Description, in.Content, id,
	)
	if err != nil {
		return fmt.Errorf("update tool: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTool removes a tool by id. Returns ErrNotFound if there was nothing to
// delete, and ErrReferenced if a level still unlocks it — levels.unlocks_tool_id
// is a RESTRICT foreign key, so the delete is blocked rather than orphaning it.
func (s *Store) DeleteTool(id int64) error {
	res, err := s.db.Exec(`DELETE FROM tools WHERE id = ?`, id)
	if err != nil {
		if isForeignKeyViolation(err) {
			return ErrReferenced
		}
		return fmt.Errorf("delete tool: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
