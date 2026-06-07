package store

import (
	"errors"
	"testing"
)

func TestIsToolFileUnlocked(t *testing.T) {
	s := newTestStore(t)
	uid, err := s.CreateUser("alice", "hash1")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Two levels; tools hang off them.
	res, err := s.db.Exec(`INSERT INTO levels (order_index, title, description, flag) VALUES (10, 'L10', 'd', 'flag')`)
	if err != nil {
		t.Fatalf("insert level: %v", err)
	}
	l10, _ := res.LastInsertId()
	res, err = s.db.Exec(`INSERT INTO levels (order_index, title, description, flag) VALUES (20, 'L20', 'd', 'flag')`)
	if err != nil {
		t.Fatalf("insert locked level: %v", err)
	}
	l20, _ := res.LastInsertId()

	// A pdf tool unlocked at L10, another at L20.
	if _, err := s.db.Exec(
		`INSERT INTO tools (type, title, content, unlocks_at_level_id) VALUES ('pdf', 'nmap guide', 'nmap-cheatsheet.pdf', ?)`,
		l10,
	); err != nil {
		t.Fatalf("insert pdf tool: %v", err)
	}
	if _, err := s.db.Exec(
		`INSERT INTO tools (type, title, content, unlocks_at_level_id) VALUES ('pdf', 'sha guide', 'sha-explained.pdf', ?)`,
		l20,
	); err != nil {
		t.Fatalf("insert second pdf tool: %v", err)
	}

	// Nothing solved yet -> nothing unlocked.
	if ok, err := s.IsToolFileUnlocked(uid, "nmap-cheatsheet.pdf"); err != nil || ok {
		t.Fatalf("before solving: want false, got ok=%v err=%v", ok, err)
	}

	// Solve L10 -> nmap pdf unlocked, sha pdf still locked, unknown path false.
	if _, err := s.db.Exec(
		`INSERT INTO user_progress (user_id, level_id) VALUES (?, ?)`, uid, l10,
	); err != nil {
		t.Fatalf("solve L10: %v", err)
	}
	if ok, err := s.IsToolFileUnlocked(uid, "nmap-cheatsheet.pdf"); err != nil || !ok {
		t.Fatalf("nmap after L10: want true, got ok=%v err=%v", ok, err)
	}
	if ok, err := s.IsToolFileUnlocked(uid, "sha-explained.pdf"); err != nil || ok {
		t.Fatalf("sha (level not solved): want false, got ok=%v err=%v", ok, err)
	}
	if ok, err := s.IsToolFileUnlocked(uid, "does-not-exist.pdf"); err != nil || ok {
		t.Fatalf("unknown path: want false, got ok=%v err=%v", ok, err)
	}
}

func TestUnlockedTools(t *testing.T) {
	s := newTestStore(t)
	uid, err := s.CreateUser("alice", "hash1")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Three levels.
	levelID := map[int]int64{}
	insLevel := func(oi int) int64 {
		res, err := s.db.Exec(
			`INSERT INTO levels (order_index, title, description, flag) VALUES (?, 'L', 'd', 'flag')`, oi,
		)
		if err != nil {
			t.Fatalf("insert level oi=%d: %v", oi, err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	levelID[10] = insLevel(10)
	levelID[20] = insLevel(20)
	levelID[30] = insLevel(30)

	// toolA unlocks at L10 (has a description), toolB at L20 (NULL description to
	// exercise COALESCE), toolC unlocks at no level (NULL -> never unlocked).
	res, err := s.db.Exec(
		`INSERT INTO tools (type, title, description, content, unlocks_at_level_id) VALUES ('cipher', 'Caesar wheel', 'rotates letters', 'wheel-data', ?)`,
		levelID[10],
	)
	if err != nil {
		t.Fatalf("insert toolA: %v", err)
	}
	toolA, _ := res.LastInsertId()
	res, err = s.db.Exec(
		`INSERT INTO tools (type, title, content, unlocks_at_level_id) VALUES ('ref', 'ASCII table', 'ascii-data', ?)`,
		levelID[20],
	)
	if err != nil {
		t.Fatalf("insert toolB: %v", err)
	}
	toolB, _ := res.LastInsertId()
	if _, err := s.db.Exec(
		`INSERT INTO tools (type, title, content, unlocks_at_level_id) VALUES ('ref', 'orphan', 'orphan-data', NULL)`,
	); err != nil {
		t.Fatalf("insert toolC: %v", err)
	}

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

	// Solve L20 (unlocks toolB) and L30 (unlocks nothing) -> toolA + toolB only;
	// toolC stays locked because it is tied to no level.
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

func TestAdminToolCRUD(t *testing.T) {
	s := newTestStore(t)

	// A level for the tool to unlock at.
	levelID, err := s.CreateLevel(AdminLevelInput{OrderIndex: 10, Title: "L10", Description: "d", Flag: "f"})
	if err != nil {
		t.Fatalf("create level: %v", err)
	}

	id, err := s.CreateTool(ToolInput{Type: "link", Title: "CyberChef", Description: "swiss army", Content: "http://x", UnlocksAtLevelID: &levelID})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := s.ToolByID(id)
	if err != nil || got.Title != "CyberChef" || got.Type != "link" || got.Description != "swiss army" ||
		got.UnlocksAtLevelID == nil || *got.UnlocksAtLevelID != levelID {
		t.Fatalf("by id: %+v err=%v", got, err)
	}

	// A bad level reference -> ErrInvalidReference.
	bad := int64(9999)
	if _, err := s.CreateTool(ToolInput{Type: "link", Title: "x", Content: "y", UnlocksAtLevelID: &bad}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("bad level ref: want ErrInvalidReference, got %v", err)
	}

	// Update: change fields and drop the level link.
	if err := s.UpdateTool(id, ToolInput{Type: "pdf", Title: "Guide", Description: "", Content: "guide.pdf"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = s.ToolByID(id)
	if got.Type != "pdf" || got.Title != "Guide" || got.Description != "" || got.Content != "guide.pdf" || got.UnlocksAtLevelID != nil {
		t.Fatalf("after update: %+v", got)
	}

	if err := s.UpdateTool(9999, ToolInput{Type: "link", Title: "x", Content: "y"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing: want ErrNotFound, got %v", err)
	}

	all, err := s.ListAllTools()
	if err != nil || len(all) != 1 || all[0].ID != id {
		t.Fatalf("list all: %+v err=%v", all, err)
	}

	// Deleting a level a tool points at un-assigns the tool (ON DELETE SET NULL),
	// it does not block.
	if err := s.UpdateTool(id, ToolInput{Type: "pdf", Title: "Guide", Content: "guide.pdf", UnlocksAtLevelID: &levelID}); err != nil {
		t.Fatalf("re-link: %v", err)
	}
	if err := s.DeleteLevel(levelID); err != nil {
		t.Fatalf("delete level: %v", err)
	}
	got, _ = s.ToolByID(id)
	if got.UnlocksAtLevelID != nil {
		t.Fatalf("after level delete: tool should be un-assigned, got %+v", got)
	}

	// Deleting the tool always succeeds; a second delete is 404.
	if err := s.DeleteTool(id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteTool(id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete again: want ErrNotFound, got %v", err)
	}
}
