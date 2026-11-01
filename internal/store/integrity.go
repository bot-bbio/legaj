package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"modernc.org/sqlite"
)

// SQLite primary result codes that mean "this file is damaged".
const (
	sqliteCorrupt = 11 // SQLITE_CORRUPT
	sqliteNotADB  = 26 // SQLITE_NOTADB
)

// isCorruption reports whether err means the file itself is damaged, as
// opposed to a transient condition (a lock, a permission problem) that must
// never cause a healthy database to be moved aside.
func isCorruption(err error) bool {
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return false
	}
	switch se.Code() & 0xff { // strip the extended-code bits
	case sqliteCorrupt, sqliteNotADB:
		return true
	}
	return false
}

// integrityOK runs PRAGMA integrity_check on the writer connection.
// It returns (false, nil) for a damaged file and an error for anything else.
func (d *DB) integrityOK(ctx context.Context) (bool, error) {
	return integrityCheck(ctx, d.writer)
}

func integrityCheck(ctx context.Context, db *sql.DB) (bool, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA integrity_check`)
	if err != nil {
		if isCorruption(err) {
			return false, nil
		}
		return false, fmt.Errorf("integrity check: %w", err)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			if isCorruption(err) {
				return false, nil
			}
			return false, err
		}
		lines = append(lines, s)
	}
	if err := rows.Err(); err != nil {
		if isCorruption(err) {
			return false, nil
		}
		return false, err
	}
	return len(lines) == 1 && lines[0] == "ok", nil
}

// quarantine moves a corrupt legaj.db (and its WAL) aside and returns the
// restore offer. It never creates a new database.
func quarantine(ctx context.Context, dir string, now func() time.Time) (*DB, Startup, error) {
	stamp := now().UTC().Format("20060102T150405Z")
	moved := FileName + ".corrupt-" + stamp
	if err := os.Rename(filepath.Join(dir, FileName), filepath.Join(dir, moved)); err != nil {
		return nil, Startup{}, fmt.Errorf("move corrupt database aside: %w", err)
	}
	if err := setAsideSidecars(dir, moved); err != nil {
		return nil, Startup{}, err
	}
	backups, err := ListBackups(ctx, dir)
	if err != nil {
		return nil, Startup{}, err
	}
	return nil, Startup{Kind: StartupCorrupt, MovedAside: moved, Backups: backups}, nil
}

// setAsideSidecars keeps legaj.db-wal next to the file it belongs to
// (renamed to <movedTo>-wal) and deletes legaj.db-shm, which is only an
// index. A stale WAL left in place would be replayed onto whatever file is
// opened as legaj.db next, corrupting it.
func setAsideSidecars(dir, movedTo string) error {
	wal := filepath.Join(dir, FileName+"-wal")
	if _, err := os.Stat(wal); err == nil {
		if err := os.Rename(wal, filepath.Join(dir, movedTo+"-wal")); err != nil {
			return fmt.Errorf("move WAL aside: %w", err)
		}
	}
	if err := os.Remove(filepath.Join(dir, FileName+"-shm")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove shared-memory file: %w", err)
	}
	return nil
}
