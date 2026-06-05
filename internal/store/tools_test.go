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

	// A pdf tool whose content is the file path, and a link tool (URL content).
	res, err := s.db.Exec(
		`INSERT INTO tools (type, title, content) VALUES ('pdf', 'nmap guide', 'nmap-cheatsheet.pdf')`,
	)
	if err != nil {
		t.Fatalf("insert pdf tool: %v", err)
	}
	pdfTool, _ := res.LastInsertId()
	res, err = s.db.Exec(
		`INSERT INTO tools (type, title, content) VALUES ('pdf', 'sha guide', 'sha-explained.pdf')`,
	)
	if err != nil {
		t.Fatalf("insert second pdf tool: %v", err)
	}
	lockedTool, _ := res.LastInsertId()

	// L10 unlocks the nmap pdf, L20 unlocks the sha pdf.
	res, err = s.db.Exec(
		`INSERT INTO levels (order_index, title, description, flag, unlocks_tool_id) VALUES (10, 'L10', 'd', 'flag', ?)`,
		pdfTool,
	)
	if err != nil {
		t.Fatalf("insert level: %v", err)
	}
	l10, _ := res.LastInsertId()
	if _, err := s.db.Exec(
		`INSERT INTO levels (order_index, title, description, flag, unlocks_tool_id) VALUES (20, 'L20', 'd', 'flag', ?)`,
		lockedTool,
	); err != nil {
		t.Fatalf("insert locked level: %v", err)
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

func TestAdminToolCRUD(t *testing.T) {
	s := newTestStore(t)

	id, err := s.CreateTool(ToolInput{Type: "link", Title: "CyberChef", Description: "swiss army", Content: "http://x"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := s.ToolByID(id)
	if err != nil || got.Title != "CyberChef" || got.Type != "link" || got.Description != "swiss army" {
		t.Fatalf("by id: %+v err=%v", got, err)
	}

	if err := s.UpdateTool(id, ToolInput{Type: "pdf", Title: "Guide", Description: "", Content: "guide.pdf"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = s.ToolByID(id)
	if got.Type != "pdf" || got.Title != "Guide" || got.Description != "" || got.Content != "guide.pdf" {
		t.Fatalf("after update: %+v", got)
	}

	if err := s.UpdateTool(9999, ToolInput{Type: "link", Title: "x", Content: "y"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing: want ErrNotFound, got %v", err)
	}

	all, err := s.ListAllTools()
	if err != nil || len(all) != 1 || all[0].ID != id {
		t.Fatalf("list all: %+v err=%v", all, err)
	}

	// A level unlocking the tool blocks its deletion (RESTRICT FK -> ErrReferenced).
	if _, err := s.db.Exec(
		`INSERT INTO levels (order_index, title, description, flag, unlocks_tool_id) VALUES (10, 'L', 'd', 'f', ?)`, id,
	); err != nil {
		t.Fatalf("insert referencing level: %v", err)
	}
	if err := s.DeleteTool(id); !errors.Is(err, ErrReferenced) {
		t.Fatalf("delete referenced: want ErrReferenced, got %v", err)
	}

	// Remove the reference, then delete succeeds; second delete is 404.
	if _, err := s.db.Exec(`UPDATE levels SET unlocks_tool_id = NULL WHERE unlocks_tool_id = ?`, id); err != nil {
		t.Fatalf("clear ref: %v", err)
	}
	if err := s.DeleteTool(id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteTool(id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete again: want ErrNotFound, got %v", err)
	}
}
