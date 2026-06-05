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
	Key         string `json:"key"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

type seedLevel struct {
	OrderIndex  int      `json:"order_index"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Flag        string   `json:"flag"`
	UnlocksTool *string  `json:"unlocks_tool"` // nil / null = unlocks nothing
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

	// Wipe in FK-safe order: hints and levels are children/referrers; levels
	// reference tools (RESTRICT), so levels must go before tools. Deleting levels
	// cascades to user_progress.
	for _, table := range []string{"hints", "levels", "tools"} {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}
	if _, err := tx.Exec(
		`DELETE FROM sqlite_sequence WHERE name IN ('tools', 'levels', 'hints')`,
	); err != nil {
		return fmt.Errorf("reset id counters: %w", err)
	}

	// Tools first; remember each key's new id so levels can reference it.
	toolID := map[string]int64{}
	for _, t := range sf.Tools {
		if t.Key == "" {
			return fmt.Errorf("tool %q has no key", t.Title)
		}
		if _, dup := toolID[t.Key]; dup {
			return fmt.Errorf("duplicate tool key %q", t.Key)
		}
		res, err := tx.Exec(
			`INSERT INTO tools (type, title, description, content) VALUES (?, ?, ?, ?)`,
			t.Type, t.Title, t.Description, t.Content,
		)
		if err != nil {
			return fmt.Errorf("insert tool %q: %w", t.Key, err)
		}
		id, _ := res.LastInsertId()
		toolID[t.Key] = id
	}

	for _, l := range sf.Levels {
		var unlocks any // NULL unless the level names a known tool
		if l.UnlocksTool != nil && *l.UnlocksTool != "" {
			id, ok := toolID[*l.UnlocksTool]
			if !ok {
				return fmt.Errorf("level %q unlocks unknown tool %q", l.Title, *l.UnlocksTool)
			}
			unlocks = id
		}
		res, err := tx.Exec(
			`INSERT INTO levels (order_index, title, description, flag, unlocks_tool_id)
			 VALUES (?, ?, ?, ?, ?)`,
			l.OrderIndex, l.Title, l.Description, l.Flag, unlocks,
		)
		if err != nil {
			return fmt.Errorf("insert level %q: %w", l.Title, err)
		}
		levelID, _ := res.LastInsertId()

		for i, text := range l.Hints {
			if _, err := tx.Exec(
				`INSERT INTO hints (level_id, order_index, text) VALUES (?, ?, ?)`,
				levelID, i+1, text,
			); err != nil {
				return fmt.Errorf("insert hint %d for level %q: %w", i+1, l.Title, err)
			}
		}
	}

	return tx.Commit()
}
