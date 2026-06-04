package store

import "testing"

func TestRecordSolvedIdempotent(t *testing.T) {
	s := newTestStore(t)
	uid, err := s.CreateUser("alice", "hash1")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	res, err := s.db.Exec(`INSERT INTO levels (order_index, title, description, flag) VALUES (10, 't', 'd', 'flag')`)
	if err != nil {
		t.Fatalf("insert level: %v", err)
	}
	lid, _ := res.LastInsertId()

	for i := 0; i < 3; i++ {
		if err := s.RecordSolved(uid, lid); err != nil {
			t.Fatalf("record solved (call %d): %v", i, err)
		}
	}

	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM user_progress WHERE user_id = ?`, uid).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("want exactly 1 progress row after repeated solves, got %d", count)
	}
}

func TestIsLevelAccessible(t *testing.T) {
	s := newTestStore(t)
	uid, err := s.CreateUser("alice", "hash1")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Four levels with gapped order_index, as the app uses internally.
	levelID := map[int]int64{}
	for _, oi := range []int{10, 20, 30, 40} {
		res, err := s.db.Exec(
			`INSERT INTO levels (order_index, title, description, flag) VALUES (?, 't', 'd', 'flag')`,
			oi,
		)
		if err != nil {
			t.Fatalf("insert level oi=%d: %v", oi, err)
		}
		id, _ := res.LastInsertId()
		levelID[oi] = id
	}

	check := func(oi int, want bool) {
		t.Helper()
		got, err := s.IsLevelAccessible(uid, oi)
		if err != nil {
			t.Fatalf("accessible oi=%d: %v", oi, err)
		}
		if got != want {
			t.Fatalf("oi=%d: want accessible=%v, got %v", oi, want, got)
		}
	}

	// Fresh account (current = 1): only the first level is unlocked.
	check(10, true)
	check(20, false)
	check(30, false)
	check(40, false)

	// Solve the first two (current becomes 3): up to the third is unlocked.
	for _, oi := range []int{10, 20} {
		if _, err := s.db.Exec(
			`INSERT INTO user_progress (user_id, level_id) VALUES (?, ?)`,
			uid, levelID[oi],
		); err != nil {
			t.Fatalf("solve oi=%d: %v", oi, err)
		}
	}
	check(10, true)  // revisiting a solved level stays free
	check(20, true)
	check(30, true)  // the next unsolved level
	check(40, false) // still locked
}
