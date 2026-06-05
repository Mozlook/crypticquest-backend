package store

import "testing"

func TestUnlockedTools(t *testing.T) {
	s := newTestStore(t)
	uid, err := s.CreateUser("alice", "hash1")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Two tools; one has a NULL description to exercise the COALESCE.
	res, err := s.db.Exec(
		`INSERT INTO tools (type, title, description, content) VALUES ('cipher', 'Caesar wheel', 'rotates letters', 'wheel-data')`,
	)
	if err != nil {
		t.Fatalf("insert tool: %v", err)
	}
	toolA, _ := res.LastInsertId()

	res, err = s.db.Exec(
		`INSERT INTO tools (type, title, content) VALUES ('ref', 'ASCII table', 'ascii-data')`,
	)
	if err != nil {
		t.Fatalf("insert tool: %v", err)
	}
	toolB, _ := res.LastInsertId()

	// L10 unlocks toolA, L20 unlocks toolB, L30 unlocks nothing (NULL).
	levelID := map[int]int64{}
	insLevel := func(oi int, unlocks any) int64 {
		res, err := s.db.Exec(
			`INSERT INTO levels (order_index, title, description, flag, unlocks_tool_id) VALUES (?, ?, 'd', 'flag', ?)`,
			oi, "L", unlocks,
		)
		if err != nil {
			t.Fatalf("insert level oi=%d: %v", oi, err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	levelID[10] = insLevel(10, toolA)
	levelID[20] = insLevel(20, toolB)
	levelID[30] = insLevel(30, nil)

	// Fresh account: nothing solved, so no tools unlocked. Want [] not nil.
	got, err := s.UnlockedTools(uid)
	if err != nil {
		t.Fatalf("unlocked: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("fresh account: want empty non-nil slice, got %+v", got)
	}

	// Solve L10 -> only toolA unlocked, with description carried through.
	if _, err := s.db.Exec(
		`INSERT INTO user_progress (user_id, level_id) VALUES (?, ?)`,
		uid, levelID[10],
	); err != nil {
		t.Fatalf("solve L10: %v", err)
	}
	got, err = s.UnlockedTools(uid)
	if err != nil {
		t.Fatalf("unlocked: %v", err)
	}
	if len(got) != 1 || got[0].ID != toolA {
		t.Fatalf("after L10: want only toolA, got %+v", got)
	}
	if got[0].Title != "Caesar wheel" || got[0].Type != "cipher" ||
		got[0].Description != "rotates letters" || got[0].Content != "wheel-data" {
		t.Fatalf("toolA fields wrong: %+v", got[0])
	}

	// Solve L20 (unlocks toolB) and L30 (unlocks nothing) -> toolA + toolB only.
	for _, oi := range []int{20, 30} {
		if _, err := s.db.Exec(
			`INSERT INTO user_progress (user_id, level_id) VALUES (?, ?)`,
			uid, levelID[oi],
		); err != nil {
			t.Fatalf("solve L%d: %v", oi, err)
		}
	}
	got, err = s.UnlockedTools(uid)
	if err != nil {
		t.Fatalf("unlocked: %v", err)
	}
	if len(got) != 2 || got[0].ID != toolA || got[1].ID != toolB {
		t.Fatalf("after L20+L30: want toolA,toolB ordered, got %+v", got)
	}
	// toolB had a NULL description -> COALESCE should give "".
	if got[1].Description != "" {
		t.Fatalf("toolB null description should flatten to empty, got %q", got[1].Description)
	}
}
