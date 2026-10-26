package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.sql
var files embed.FS

type migration struct {
	version int64
	name    string
	up      string
	down    string
}

var migrationFile = regexp.MustCompile(`^([0-9]+)_(.+)\.(up|down)\.sql$`)

func Up(ctx context.Context, db *sql.DB) error {
	all, err := load()
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("initialize migration tracking: %w", err)
	}
	for _, item := range all {
		var applied bool
		if err := db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", item.version).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %d: %w", item.version, err)
		}
		if applied {
			continue
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", item.version, err)
		}
		if _, err := tx.ExecContext(ctx, item.up); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d (%s): %w", item.version, item.name, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version, name) VALUES ($1, $2)", item.version, item.name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", item.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", item.version, err)
		}
	}
	return nil
}

func Down(ctx context.Context, db *sql.DB, steps int) error {
	if steps < 1 {
		return fmt.Errorf("migration down steps must be positive")
	}
	all, err := load()
	if err != nil {
		return err
	}
	for step := 0; step < steps; step++ {
		var version int64
		err := db.QueryRowContext(ctx, "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&version)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read latest migration: %w", err)
		}
		var item *migration
		for i := range all {
			if all[i].version == version {
				item = &all[i]
				break
			}
		}
		if item == nil {
			return fmt.Errorf("migration %d is applied but no source migration exists", version)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin rollback of migration %d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, item.down); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("rollback migration %d (%s): %w", version, item.name, err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = $1", version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("remove migration record %d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit rollback of migration %d: %w", version, err)
		}
	}
	return nil
}

func load() ([]migration, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	items := make(map[int64]*migration)
	for _, entry := range entries {
		match := migrationFile.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		version, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse migration version in %q: %w", entry.Name(), err)
		}
		item := items[version]
		if item == nil {
			item = &migration{version: version, name: match[2]}
			items[version] = item
		}
		if item.name != match[2] {
			return nil, fmt.Errorf("migration version %d has inconsistent names", version)
		}
		content, err := files.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		switch match[3] {
		case "up":
			item.up = string(content)
		case "down":
			item.down = string(content)
		}
	}
	result := make([]migration, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.up) == "" || strings.TrimSpace(item.down) == "" {
			return nil, fmt.Errorf("migration %d must define both up and down", item.version)
		}
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].version < result[j].version })
	for i := 1; i < len(result); i++ {
		if result[i-1].version == result[i].version {
			return nil, fmt.Errorf("duplicate migration version %d", result[i].version)
		}
	}
	return result, nil
}
