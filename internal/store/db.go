// Package store owns LeGaJ's SQLite database (R2, D6): opening it safely,
// numbered migrations, transactions, daily backups, and corruption recovery.
//
// Invariants, all of which exist to stop V1-style data loss:
//   - All writes go through Tx on a single writer connection, so writes are
//     serialized and can't fail with SQLITE_BUSY. Reads use a separate
//     query-only pool, which can never write.
//   - A corrupt database is moved aside and NEVER replaced by an empty one;
//     Open returns a typed Startup state offering a verified backup instead.
//   - A backup is taken before pending migrations run, and each migration
//     runs in its own transaction with its version record.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver
)

// FileName is the database file inside the data folder.
const FileName = "legaj.db"

// Connection settings shared by both pools: enforce foreign keys, WAL so
// readers never block the writer, and wait instead of failing on a lock.
const commonPragmas = "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"

// Options configures Open.
type Options struct {
	// Dir is the data folder (apphome.Dir()).
	Dir string
	// Now is the clock used for backup names and dates. Default time.Now.
	Now func() time.Time
	// Migrations overrides the embedded migrations (tests only).
	Migrations fs.FS
}

// DB is an open, migrated database.
type DB struct {
	dir    string
	now    func() time.Time
	writer *sql.DB
	reader *sql.DB
}

// StartupKind says whether the database is ready, or what the user must decide.
type StartupKind string

const (
	// StartupReady means the DB is open and fully migrated.
	StartupReady StartupKind = "ready"
	// StartupCorrupt means legaj.db failed its integrity check and was moved
	// aside. The user may restore one of Backups or quit.
	StartupCorrupt StartupKind = "corrupt"
	// StartupMigrationFailed means an upgrade failed and was rolled back.
	// The user may restore one of Backups or quit.
	StartupMigrationFailed StartupKind = "migration_failed"
)

// Startup is the outcome of Open, rendered by the frontend (plan item 10).
type Startup struct {
	Kind StartupKind `json:"kind"`
	// MovedAside is the file name the corrupt DB was renamed to.
	MovedAside string `json:"movedAside,omitempty"`
	// Backups are the backups that passed an integrity check, newest first.
	Backups []BackupInfo `json:"backups"`
}

// Open opens (or creates) the database in opts.Dir and applies pending
// migrations. When the result is not StartupReady, the returned *DB is nil
// and err is nil: the caller shows the restore/quit choice. err is non-nil
// only for unexpected failures (permissions, a locked file), in which case
// nothing on disk was changed.
func Open(ctx context.Context, opts Options) (*DB, Startup, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Migrations == nil {
		opts.Migrations = embeddedMigrations()
	}
	migs, err := loadMigrations(opts.Migrations)
	if err != nil {
		return nil, Startup{}, err
	}
	if err := os.MkdirAll(opts.Dir, 0o700); err != nil {
		return nil, Startup{}, fmt.Errorf("create data folder: %w", err)
	}
	path := filepath.Join(opts.Dir, FileName)
	_, statErr := os.Stat(path)
	existed := statErr == nil

	d, err := openPools(path)
	if err != nil {
		return nil, Startup{}, err
	}
	d.dir, d.now = opts.Dir, opts.Now

	if existed {
		healthy, err := d.integrityOK(ctx)
		if err != nil {
			_ = d.Close()
			return nil, Startup{}, err
		}
		if !healthy {
			_ = d.Close()
			return quarantine(ctx, opts.Dir, opts.Now)
		}
	}

	pending, err := d.pending(ctx, migs)
	if err != nil {
		_ = d.Close()
		return nil, Startup{}, err
	}
	if len(pending) > 0 && existed {
		if _, err := d.Backup(ctx, "premigrate"); err != nil {
			_ = d.Close()
			return nil, Startup{}, fmt.Errorf("backup before migration: %w", err)
		}
	}
	for _, m := range pending {
		if err := d.apply(ctx, m); err != nil {
			_ = d.Close()
			backups, listErr := ListBackups(ctx, opts.Dir)
			if listErr != nil {
				return nil, Startup{}, listErr
			}
			return nil, Startup{Kind: StartupMigrationFailed, Backups: backups}, nil
		}
	}
	return d, Startup{Kind: StartupReady, Backups: []BackupInfo{}}, nil
}

// openPools opens the single-connection writer and the query-only reader.
func openPools(path string) (*DB, error) {
	// _txlock=immediate: BEGIN IMMEDIATE takes the write lock up front, so a
	// transaction never fails halfway when it first writes.
	writer, err := sql.Open("sqlite", path+"?"+commonPragmas+"&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)
	writer.SetConnMaxLifetime(0)

	reader, err := sql.Open("sqlite", path+"?"+commonPragmas+"&_pragma=query_only(1)")
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	reader.SetMaxOpenConns(4)
	return &DB{writer: writer, reader: reader}, nil
}

// Close closes both pools. The OS file handles are released on return, so
// the file can be renamed afterwards (Windows forbids renaming open files).
func (d *DB) Close() error {
	if d == nil {
		return nil
	}
	return errors.Join(d.reader.Close(), d.writer.Close())
}

// Reader is the query-only pool. Writes through it fail.
func (d *DB) Reader() *sql.DB {
	return d.reader
}

// Tx runs fn in a write transaction on the single writer connection. It
// commits if fn returns nil and rolls back otherwise, including on panic
// (which is then re-raised). fn must use only tx, never d.Reader(): the
// writer is held for the whole call.
func (d *DB) Tx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := d.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return errors.Join(err, rbErr)
		}
		return err
	}
	return tx.Commit()
}
