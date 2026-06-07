// Command seed loads developer content (tools, levels, hints) from a seed.json
// into the database, rebuilding it from scratch each run. It is a dev/test
// convenience only — in production content is managed through the admin panel.
//
// Usage: seed [-seed seed/seed.json]   (DB path comes from DB_PATH, like the server)
//
// "Rebuild from scratch": the content tables are wiped and their autoincrement
// counters reset before inserting, so level ids are deterministic (1, 2, 3 ...)
// across reseeds — which is what the files/levels/{id}/ folders depend on. Note
// this also clears user_progress (it cascades off levels); acceptable for dev.
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"crypticquest"
	"crypticquest/internal/config"
	"crypticquest/internal/db"
)

type seedFile struct {
	Tools  []seedTool  `json:"tools"`
	Levels []seedLevel `json:"levels"`
}

type seedTool struct {
	Type           string  `json:"type"`
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	Content        string  `json:"content"`
	UnlocksAtLevel *string `json:"unlocks_at_level"` // level key; nil / null = unlocks nothing
}

type seedLevel struct {
	Key         string   `json:"key"` // referenced by a tool's unlocks_at_level
	OrderIndex  int      `json:"order_index"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Flag        string   `json:"flag"`
	Hints       []string `json:"hints"`
}

func main() {
	seedPath := flag.String("seed", "seed/seed.json", "path to the seed JSON file")
	flag.Parse()

	cfg := config.Load()

	data, err := os.ReadFile(*seedPath)
	if err != nil {
		log.Fatalf("read seed file: %v", err)
	}
	var sf seedFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields() // catch typos in the seed file early
	if err := dec.Decode(&sf); err != nil {
		log.Fatalf("parse seed file: %v", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer database.Close()
	if err := db.Migrate(database, crypticquest.MigrationsFS); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	if err := load(database, sf); err != nil {
		log.Fatalf("seed: %v", err)
	}
	log.Printf("seeded %s: %d tools, %d levels", cfg.DBPath, len(sf.Tools), len(sf.Levels))
}

// load wipes the content tables and inserts the seed inside one transaction, so
// a failure leaves the database untouched rather than half-seeded.
func load(database *sql.DB, sf seedFile) error {
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after a successful Commit

	// Wipe in FK-safe order: hints cascade off levels, tools reference levels
	// (SET NULL). Delete tools before levels so no tool transiently dangles.
	for _, table := range []string{"hints", "tools", "levels"} {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}
	if _, err := tx.Exec(
		`DELETE FROM sqlite_sequence WHERE name IN ('tools', 'levels', 'hints')`,
	); err != nil {
		return fmt.Errorf("reset id counters: %w", err)
	}

	// Levels first; remember each key's new id so tools can reference it.
	levelID := map[string]int64{}
	for _, l := range sf.Levels {
		if l.Key == "" {
			return fmt.Errorf("level %q has no key", l.Title)
		}
		if _, dup := levelID[l.Key]; dup {
			return fmt.Errorf("duplicate level key %q", l.Key)
		}
		res, err := tx.Exec(
			`INSERT INTO levels (order_index, title, description, flag)
			 VALUES (?, ?, ?, ?)`,
			l.OrderIndex, l.Title, l.Description, l.Flag,
		)
		if err != nil {
			return fmt.Errorf("insert level %q: %w", l.Title, err)
		}
		id, _ := res.LastInsertId()
		levelID[l.Key] = id

		for i, text := range l.Hints {
			if _, err := tx.Exec(
				`INSERT INTO hints (level_id, order_index, text) VALUES (?, ?, ?)`,
				id, i+1, text,
			); err != nil {
				return fmt.Errorf("insert hint %d for level %q: %w", i+1, l.Title, err)
			}
		}
	}

	for _, t := range sf.Tools {
		var unlocks any // NULL unless the tool names a known level
		if t.UnlocksAtLevel != nil && *t.UnlocksAtLevel != "" {
			id, ok := levelID[*t.UnlocksAtLevel]
			if !ok {
				return fmt.Errorf("tool %q unlocks at unknown level %q", t.Title, *t.UnlocksAtLevel)
			}
			unlocks = id
		}
		if _, err := tx.Exec(
			`INSERT INTO tools (type, title, description, content, unlocks_at_level_id)
			 VALUES (?, ?, ?, ?, ?)`,
			t.Type, t.Title, t.Description, t.Content, unlocks,
		); err != nil {
			return fmt.Errorf("insert tool %q: %w", t.Title, err)
		}
	}

	return tx.Commit()
}
