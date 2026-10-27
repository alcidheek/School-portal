package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func RunMigrations(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		return fmt.Errorf("find migrations: %w", err)
	}

	sort.Strings(files)

	for _, file := range files {
		version := filepath.Base(file)

		var exists bool

		err := db.QueryRow(
			`SELECT EXISTS(
				SELECT 1
				FROM schema_migrations
				WHERE version = $1
			)`,
			version,
		).Scan(&exists)

		if err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}

		if exists {
			continue
		}

		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", version, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("execute migration %s: %w", version, err)
		}

		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (version) VALUES ($1)`,
			version,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", version, err)
		}

		fmt.Printf("Migration applied: %s\n", version)
	}

	return nil
}
