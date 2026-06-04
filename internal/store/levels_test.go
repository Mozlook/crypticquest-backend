package store

import (
	"errors"
	"fmt"
	"testing"
)

func TestListAccessibleLevels(t *testing.T) {
	s := newTestStore(t)
	uid, err := s.CreateUser("alice", "hash1")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	levelID := map[int]int64{}
	for _, oi := range []int{10, 20, 30, 40} {
		res, err := s.db.Exec(
			`INSERT INTO levels (order_index, title, description, flag) VALUES (?, ?, 'd', 'flag')`,
			oi, fmt.Sprintf("L%d", oi),
		)
		if err != nil {
			t.Fatalf("insert level oi=%d: %v", oi, err)
		}
		id, _ := res.LastInsertId()
		levelID[oi] = id
	}

	// Fresh account: only the first level, not solved.
	got, err := s.ListAccessibleLevels(uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Title != "L10" || got[0].Solved {
		t.Fatalf("fresh account: unexpected list %+v", got)
	}

	// Solve the first two -> current = 3 -> three levels visible.
	for _, oi := range []int{10, 20} {
		if _, err := s.db.Exec(
			`INSERT INTO user_progress (user_id, level_id) VALUES (?, ?)`,
			uid, levelID[oi],
		); err != nil {
			t.Fatalf("solve oi=%d: %v", oi, err)
		}
	}

	got, err = s.ListAccessibleLevels(uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 levels, got %d: %+v", len(got), got)
	}
	if got[0].Title != "L10" || got[1].Title != "L20" || got[2].Title != "L30" {
		t.Fatalf("levels out of order: %+v", got)
	}
	if !got[0].Solved || !got[1].Solved || got[2].Solved {
		t.Fatalf("solved flags wrong: %+v", got)
	}
}

func TestLevelByID(t *testing.T) {
	s := newTestStore(t)
	uid, err := s.CreateUser("alice", "hash1")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	res, err := s.db.Exec(
		`INSERT INTO levels (order_index, title, description, flag) VALUES (10, 'Caesar', 'narrative', 'flag{x}')`,
	)
	if err != nil {
		t.Fatalf("insert level: %v", err)
	}
	id, _ := res.LastInsertId()

	d, err := s.LevelByID(uid, id)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if d.ID != id || d.Title != "Caesar" || d.Description != "narrative" || d.OrderIndex != 10 {
		t.Fatalf("unexpected detail: %+v", d)
	}
	if d.Solved {
		t.Fatalf("level should be unsolved")
	}

	if _, err := s.LevelByID(uid, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing level: want ErrNotFound, got %v", err)
	}
}

func TestLevelForSubmit(t *testing.T) {
	s := newTestStore(t)
	res, err := s.db.Exec(
		`INSERT INTO levels (order_index, title, description, flag) VALUES (10, 't', 'd', 'flag{x}')`,
	)
	if err != nil {
		t.Fatalf("insert level: %v", err)
	}
	id, _ := res.LastInsertId()

	oi, flag, err := s.LevelForSubmit(id)
	if err != nil {
		t.Fatalf("for submit: %v", err)
	}
	if oi != 10 || flag != "flag{x}" {
		t.Fatalf("unexpected: oi=%d flag=%q", oi, flag)
	}

	if _, _, err := s.LevelForSubmit(9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing level: want ErrNotFound, got %v", err)
	}
}
