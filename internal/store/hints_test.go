package store

import (
	"errors"
	"testing"
)

func TestHintsForLevel(t *testing.T) {
	s := newTestStore(t)

	res, err := s.db.Exec(
		`INSERT INTO levels (order_index, title, description, flag) VALUES (10, 'L10', 'd', 'flag')`,
	)
	if err != nil {
		t.Fatalf("insert level: %v", err)
	}
	levelID, _ := res.LastInsertId()

	res, err = s.db.Exec(
		`INSERT INTO levels (order_index, title, description, flag) VALUES (20, 'L20', 'd', 'flag')`,
	)
	if err != nil {
		t.Fatalf("insert other level: %v", err)
	}
	otherID, _ := res.LastInsertId()

	// Insert out of order_index order to prove the query sorts, and add a hint
	// on the other level to prove scoping by level_id.
	for _, h := range []struct {
		oi   int
		text string
	}{{30, "third"}, {10, "first"}, {20, "second"}} {
		if _, err := s.db.Exec(
			`INSERT INTO hints (level_id, order_index, text) VALUES (?, ?, ?)`,
			levelID, h.oi, h.text,
		); err != nil {
			t.Fatalf("insert hint: %v", err)
		}
	}
	if _, err := s.db.Exec(
		`INSERT INTO hints (level_id, order_index, text) VALUES (?, 10, 'other-level-hint')`,
		otherID,
	); err != nil {
		t.Fatalf("insert other hint: %v", err)
	}

	got, err := s.HintsForLevel(levelID)
	if err != nil {
		t.Fatalf("hints: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 hints, got %d: %+v", len(got), got)
	}
	if got[0].Text != "first" || got[1].Text != "second" || got[2].Text != "third" {
		t.Fatalf("hints out of order or scoped wrong: %+v", got)
	}

	// A level with no hints yields an empty, non-nil slice.
	res, err = s.db.Exec(
		`INSERT INTO levels (order_index, title, description, flag) VALUES (40, 'L40', 'd', 'flag')`,
	)
	if err != nil {
		t.Fatalf("insert empty level: %v", err)
	}
	emptyID, _ := res.LastInsertId()
	got, err = s.HintsForLevel(emptyID)
	if err != nil {
		t.Fatalf("hints empty: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("want empty non-nil slice, got %+v", got)
	}
}

func TestReplaceHints(t *testing.T) {
	s := newTestStore(t)
	res, err := s.db.Exec(`INSERT INTO levels (order_index, title, description, flag) VALUES (10, 'L', 'd', 'f')`)
	if err != nil {
		t.Fatalf("insert level: %v", err)
	}
	levelID, _ := res.LastInsertId()

	// Replace with three -> stored in order 1,2,3.
	if err := s.ReplaceHints(levelID, []string{"first", "second", "third"}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	got, _ := s.HintsForLevel(levelID)
	if len(got) != 3 || got[0].Text != "first" || got[2].Text != "third" {
		t.Fatalf("after replace: %+v", got)
	}

	// Replace again with a reordered, shorter list -> old ones gone, reindexed.
	if err := s.ReplaceHints(levelID, []string{"only-a", "only-b"}); err != nil {
		t.Fatalf("replace 2: %v", err)
	}
	got, _ = s.HintsForLevel(levelID)
	if len(got) != 2 || got[0].Text != "only-a" || got[1].Text != "only-b" {
		t.Fatalf("after replace 2: %+v", got)
	}

	// Empty slice clears all.
	if err := s.ReplaceHints(levelID, []string{}); err != nil {
		t.Fatalf("replace empty: %v", err)
	}
	if got, _ = s.HintsForLevel(levelID); len(got) != 0 {
		t.Fatalf("want cleared, got %+v", got)
	}

	// Missing level -> ErrNotFound (and nothing inserted).
	if err := s.ReplaceHints(9999, []string{"x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing level: want ErrNotFound, got %v", err)
	}
}
