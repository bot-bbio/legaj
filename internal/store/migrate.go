package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func embeddedMigrations() fs.FS {
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		panic(err) // the embed pattern above guarantees the folder exists
	}
	return sub
}

// migration is one numbered schema change, e.g. 0002_domain.sql.
type migration struct {
	version int
	name    string
	sql     string
}

var migrationName = regexp.MustCompile(`^(\d+)_[a-z0-9_]+\.sql$`)

// loadMigrations reads every NNNN_name.sql file at the root of fsys,
// ordered by numeric version. Duplicate versions are an error.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	seen := map[int]string{}
	var out []migration
	for _, e := range entries {
		m := migrationName.FindStringSubmatch(e.Name())
		if e.IsDir() || m == nil {
			continue
		}
		v, err := strconv.Atoi(m[1])
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("migration %s: bad version", e.Name())
		}
		if prev, dup := seen[v]; dup {
			return nil, fmt.Errorf("migrations %s and %s share version %d", prev, e.Name(), v)
		}
		seen[v] = e.Name()
		body, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, name: e.Name(), sql: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// createVersionTable is the runner's own bookkeeping, outside numbered
// migrations so it exists before migration 1 runs.
const createVersionTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version    INTEGER PRIMARY KEY,
	name       TEXT    NOT NULL,
	applied_at TEXT    NOT NULL
) STRICT`

// AppliedVersions returns the applied migration versions, ascending.
func (d *DB) AppliedVersions(ctx context.Context) ([]int, error) {
	rows, err := d.reader.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// pending returns the migrations newer than the newest applied one. A
// database already past every known migration came from a newer LeGaJ and
// is refused rather than risk running old code against it.
func (d *DB) pending(ctx context.Context, all []migration) ([]migration, error) {
	if _, err := d.writer.ExecContext(ctx, createVersionTable); err != nil {
		return nil, fmt.Errorf("prepare migrations table: %w", err)
	}
	var current int
	if err := d.writer.QueryRowContext(ctx, `SELECT coalesce(max(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return nil, err
	}
	latest := 0
	if len(all) > 0 {
		latest = all[len(all)-1].version
	}
	if current > latest {
		return nil, fmt.Errorf("the database is from a newer version of LeGaJ (schema %d, this version knows %d)", current, latest)
	}
	var out []migration
	for _, m := range all {
		if m.version > current {
			out = append(out, m)
		}
	}
	return out, nil
}

// apply runs one migration and records it, atomically: if any statement
// fails, neither the schema change nor the version record persists.
func (d *DB) apply(ctx context.Context, m migration) error {
	return d.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)`,
			m.version, m.name, d.now().UTC().Format(time.RFC3339))
		return err
	})
}
