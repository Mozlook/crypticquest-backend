package store

import "testing"

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
