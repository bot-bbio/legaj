package store_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

var ctx = context.Background()

// clock is a controllable time source.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newClock() *clock                   { return &clock{t: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)} }
func migrations(files map[string]string) fstest.MapFS {
	fs := fstest.MapFS{}
	for name, body := range files {
		fs[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fs
}

const mig1 = `CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT NOT NULL) STRICT;`

func mustOpen(t *testing.T, opts store.Options) *store.DB {
	t.Helper()
	db, st, err := store.Open(ctx, opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if st.Kind != store.StartupReady {
		t.Fatalf("startup = %+v, want ready", st)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertNote(t *testing.T, db *store.DB, body string) {
	t.Helper()
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO notes(body) VALUES (?)`, body)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func countNotes(t *testing.T, db *store.DB) int {
	t.Helper()
	var n int
	if err := db.Reader().QueryRowContext(ctx, `SELECT count(*) FROM notes`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestR2_FreshFolderCreatesDBWithEmbeddedMigrations(t *testing.T) {
	dir := testenv.Isolate(t)
	db := mustOpen(t, store.Options{Dir: dir})

	if _, err := os.Stat(filepath.Join(dir, store.FileName)); err != nil {
		t.Fatalf("legaj.db not created: %v", err)
	}
	applied, err := db.AppliedVersions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) == 0 || applied[0] != 1 {
		t.Fatalf("applied = %v, want the embedded migrations starting at 1", applied)
	}
}

func TestR2_MigrationsApplyInOrder(t *testing.T) {
	dir := testenv.Isolate(t)
	// Not zero-padded on purpose: "10_" sorts before "2_" as text.
	fs := migrations(map[string]string{
		"1_log.sql":  `CREATE TABLE log (seq INTEGER PRIMARY KEY, v INTEGER NOT NULL) STRICT; INSERT INTO log(v) VALUES (1);`,
		"10_ten.sql": `INSERT INTO log(v) VALUES (10);`,
		"2_two.sql":  `INSERT INTO log(v) VALUES (2);`,
	})
	db := mustOpen(t, store.Options{Dir: dir, Migrations: fs})

	rows, err := db.Reader().QueryContext(ctx, `SELECT v FROM log ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		got = append(got, v)
	}
	if want := []int{1, 2, 10}; !equal(got, want) {
		t.Fatalf("migration order = %v, want %v", got, want)
	}
}

func TestR2_OnlyNewMigrationsRun(t *testing.T) {
	dir := testenv.Isolate(t)
	first := migrations(map[string]string{"1_notes.sql": mig1})
	db, _, err := store.Open(ctx, store.Options{Dir: dir, Migrations: first})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// Re-running 1 would fail (table exists), so success proves it was skipped.
	second := migrations(map[string]string{
		"1_notes.sql": mig1,
		"2_tags.sql":  `CREATE TABLE tags (id INTEGER PRIMARY KEY) STRICT;`,
	})
	db = mustOpen(t, store.Options{Dir: dir, Migrations: second})
	applied, err := db.AppliedVersions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !equal(applied, []int{1, 2}) {
		t.Fatalf("applied = %v, want [1 2]", applied)
	}
}

func TestR2_DuplicateMigrationVersionRejected(t *testing.T) {
	dir := testenv.Isolate(t)
	fs := migrations(map[string]string{"1_a.sql": mig1, "0001_b.sql": mig1})
	if _, _, err := store.Open(ctx, store.Options{Dir: dir, Migrations: fs}); err == nil {
		t.Fatal("Open accepted two migrations with version 1")
	}
}

func TestR2_TxRollbackLeavesNoRows(t *testing.T) {
	dir := testenv.Isolate(t)
	db := mustOpen(t, store.Options{Dir: dir, Migrations: migrations(map[string]string{"1_notes.sql": mig1})})

	boom := errors.New("boom")
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		for _, b := range []string{"a", "b"} {
			if _, err := tx.ExecContext(ctx, `INSERT INTO notes(body) VALUES (?)`, b); err != nil {
				return err
			}
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Tx error = %v, want boom", err)
	}
	if n := countNotes(t, db); n != 0 {
		t.Fatalf("%d rows persisted after rollback", n)
	}
}

func TestR2_TxPanicRollsBackAndRepanics(t *testing.T) {
	dir := testenv.Isolate(t)
	db := mustOpen(t, store.Options{Dir: dir, Migrations: migrations(map[string]string{"1_notes.sql": mig1})})

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("panic was swallowed")
			}
		}()
		_ = db.Tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `INSERT INTO notes(body) VALUES ('x')`); err != nil {
				return err
			}
			panic("mid-transaction")
		})
	}()
	if n := countNotes(t, db); n != 0 {
		t.Fatalf("%d rows persisted after panic", n)
	}
	// The writer connection must still be usable.
	insertNote(t, db, "after")
	if n := countNotes(t, db); n != 1 {
		t.Fatalf("writer unusable after panic: %d rows", n)
	}
}

func TestR2_PragmasSet(t *testing.T) {
	dir := testenv.Isolate(t)
	db := mustOpen(t, store.Options{Dir: dir})
	var fk, timeout int
	var mode string
	r := db.Reader()
	if err := r.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if err := r.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if err := r.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&timeout); err != nil {
		t.Fatal(err)
	}
	if fk != 1 || mode != "wal" || timeout < 1000 {
		t.Fatalf("foreign_keys=%d journal_mode=%s busy_timeout=%d", fk, mode, timeout)
	}
}

func TestR2_ReaderCannotWrite(t *testing.T) {
	dir := testenv.Isolate(t)
	db := mustOpen(t, store.Options{Dir: dir, Migrations: migrations(map[string]string{"1_notes.sql": mig1})})
	if _, err := db.Reader().ExecContext(ctx, `INSERT INTO notes(body) VALUES ('x')`); err == nil {
		t.Fatal("reader pool accepted a write; all writes must go through Tx")
	}
}

func TestR2_BackupKeepsNewest7(t *testing.T) {
	dir := testenv.Isolate(t)
	c := newClock()
	db := mustOpen(t, store.Options{Dir: dir, Now: c.now, Migrations: migrations(map[string]string{"1_notes.sql": mig1})})

	var first, last time.Time
	for day := 0; day < 8; day++ {
		created, err := db.DailyBackup(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !created {
			t.Fatalf("day %d: no backup taken", day)
		}
		// A second call on the same day does nothing.
		if again, err := db.DailyBackup(ctx); err != nil || again {
			t.Fatalf("day %d: second backup on the same day (created=%v err=%v)", day, again, err)
		}
		if day == 0 {
			first = c.now()
		}
		last = c.now()
		c.advance(24 * time.Hour)
	}

	backups, err := store.ListBackups(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 7 {
		t.Fatalf("%d backups kept, want 7: %+v", len(backups), backups)
	}
	if !backups[0].TakenAt.Equal(last) {
		t.Fatalf("newest backup %v, want %v", backups[0].TakenAt, last)
	}
	for _, b := range backups {
		if b.TakenAt.Equal(first) {
			t.Fatal("oldest backup (day 1) was not pruned")
		}
	}
}

func TestR2_BackupIsAUsableCopy(t *testing.T) {
	dir := testenv.Isolate(t)
	c := newClock()
	db := mustOpen(t, store.Options{Dir: dir, Now: c.now, Migrations: migrations(map[string]string{"1_notes.sql": mig1})})
	insertNote(t, db, "keep me")
	if _, err := db.DailyBackup(ctx); err != nil {
		t.Fatal(err)
	}
	backups, err := store.ListBackups(ctx, dir)
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %+v, err = %v", backups, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Restore(dir, backups[0].Name, c.now); err != nil {
		t.Fatal(err)
	}
	db = mustOpen(t, store.Options{Dir: dir, Now: c.now, Migrations: migrations(map[string]string{"1_notes.sql": mig1})})
	if n := countNotes(t, db); n != 1 {
		t.Fatalf("restored DB has %d notes, want 1", n)
	}
}

func TestR2_CorruptDBNeverRecreatedEmpty(t *testing.T) {
	dir := testenv.Isolate(t)
	c := newClock()
	fs := migrations(map[string]string{"1_notes.sql": mig1})
	db := mustOpen(t, store.Options{Dir: dir, Now: c.now, Migrations: fs})
	insertNote(t, db, "precious")
	if _, err := db.DailyBackup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// A broken backup must never be offered.
	if err := os.WriteFile(filepath.Join(dir, store.BackupDir, "legaj-20261031T090000Z.db"), []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(dir, store.FileName)
	garbage := []byte(strings.Repeat("this is not a sqlite file ", 400))
	if err := os.WriteFile(dbPath, garbage, 0o600); err != nil {
		t.Fatal(err)
	}
	c.advance(time.Hour)

	got, st, err := store.Open(ctx, store.Options{Dir: dir, Now: c.now, Migrations: fs})
	if err != nil {
		t.Fatalf("Open returned an error instead of a corrupt state: %v", err)
	}
	if got != nil {
		_ = got.Close()
		t.Fatal("Open returned a usable DB for a corrupt file")
	}
	if st.Kind != store.StartupCorrupt {
		t.Fatalf("startup kind = %q, want corrupt", st.Kind)
	}
	if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a new legaj.db exists after corruption; it must never be recreated empty")
	}
	moved, err := os.ReadFile(filepath.Join(dir, st.MovedAside))
	if err != nil {
		t.Fatalf("corrupt file not moved aside to %q: %v", st.MovedAside, err)
	}
	if string(moved) != string(garbage) {
		t.Fatal("moved-aside file is not the original corrupt file")
	}
	if !strings.HasPrefix(st.MovedAside, store.FileName+".corrupt-") {
		t.Fatalf("moved-aside name %q, want legaj.db.corrupt-<ts>", st.MovedAside)
	}
	if len(st.Backups) != 1 {
		t.Fatalf("restore offer lists %d backups, want only the 1 valid one: %+v", len(st.Backups), st.Backups)
	}

	// Choosing the offered backup brings the data back.
	if err := store.Restore(dir, st.Backups[0].Name, c.now); err != nil {
		t.Fatal(err)
	}
	db = mustOpen(t, store.Options{Dir: dir, Now: c.now, Migrations: fs})
	if n := countNotes(t, db); n != 1 {
		t.Fatalf("restored DB has %d notes, want 1", n)
	}
}

func TestR2_BackupTakenBeforeMigrations(t *testing.T) {
	dir := testenv.Isolate(t)
	c := newClock()
	v1 := migrations(map[string]string{"1_notes.sql": mig1})
	db := mustOpen(t, store.Options{Dir: dir, Now: c.now, Migrations: v1})
	insertNote(t, db, "before upgrade")
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if b, _ := store.ListBackups(ctx, dir); len(b) != 0 {
		t.Fatalf("a fresh DB should need no pre-migration backup, found %d", len(b))
	}

	c.advance(time.Minute)
	v2 := migrations(map[string]string{"1_notes.sql": mig1, "2_tags.sql": `CREATE TABLE tags (id INTEGER PRIMARY KEY) STRICT;`})
	db = mustOpen(t, store.Options{Dir: dir, Now: c.now, Migrations: v2})
	_ = db

	backups, err := store.ListBackups(ctx, dir)
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %+v (err %v), want 1 pre-migration backup", backups, err)
	}
	// The backup holds the pre-migration schema and data.
	versions, notes := inspectBackup(t, filepath.Join(dir, store.BackupDir, backups[0].Name))
	if !equal(versions, []int{1}) || notes != 1 {
		t.Fatalf("backup has versions %v and %d notes, want [1] and 1", versions, notes)
	}
}

func TestR2_FailedMigrationLeavesSchemaUnchanged(t *testing.T) {
	dir := testenv.Isolate(t)
	c := newClock()
	v1 := migrations(map[string]string{"1_notes.sql": mig1})
	db := mustOpen(t, store.Options{Dir: dir, Now: c.now, Migrations: v1})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	c.advance(time.Minute)
	bad := migrations(map[string]string{
		"1_notes.sql": mig1,
		// The first statement succeeds; the second fails.
		"2_bad.sql": `CREATE TABLE half (x INTEGER) STRICT; INSERT INTO no_such_table VALUES (1);`,
	})
	got, st, err := store.Open(ctx, store.Options{Dir: dir, Now: c.now, Migrations: bad})
	if err != nil {
		t.Fatalf("Open returned an error instead of a migration_failed state: %v", err)
	}
	if got != nil {
		_ = got.Close()
		t.Fatal("Open returned a usable DB after a failed migration")
	}
	if st.Kind != store.StartupMigrationFailed {
		t.Fatalf("startup kind = %q, want migration_failed", st.Kind)
	}
	if len(st.Backups) == 0 {
		t.Fatal("no restore option offered after a failed migration")
	}

	db = mustOpen(t, store.Options{Dir: dir, Now: c.now, Migrations: v1})
	applied, err := db.AppliedVersions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !equal(applied, []int{1}) {
		t.Fatalf("schema version moved to %v after a failed migration", applied)
	}
	var n int
	if err := db.Reader().QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE name = 'half'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("table from the failed migration's first statement persisted")
	}
}

func TestR2_MissingDBWithBackupsOffersRestoreNotEmptyDB(t *testing.T) {
	dir := testenv.Isolate(t)
	c := newClock()
	fs := migrations(map[string]string{"1_notes.sql": mig1})
	db := mustOpen(t, store.Options{Dir: dir, Now: c.now, Migrations: fs})
	insertNote(t, db, "precious")
	if _, err := db.DailyBackup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// e.g. the user chose Quit on the corrupt screen, so legaj.db is now absent.
	for _, f := range []string{store.FileName, store.FileName + "-wal", store.FileName + "-shm"} {
		_ = os.Remove(filepath.Join(dir, f))
	}

	got, st, err := store.Open(ctx, store.Options{Dir: dir, Now: c.now, Migrations: fs})
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		_ = got.Close()
		t.Fatal("an empty database was opened although backups exist")
	}
	if st.Kind != store.StartupMissing || len(st.Backups) != 1 {
		t.Fatalf("startup = %+v, want missing with the backup offered", st)
	}
	if _, err := os.Stat(filepath.Join(dir, store.FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("legaj.db was created empty")
	}
	if err := store.Restore(dir, st.Backups[0].Name, c.now); err != nil {
		t.Fatal(err)
	}
	db = mustOpen(t, store.Options{Dir: dir, Now: c.now, Migrations: fs})
	if n := countNotes(t, db); n != 1 {
		t.Fatalf("restored %d notes, want 1", n)
	}
}

func TestR2_PremigrateBackupsDontPushOutDailies(t *testing.T) {
	dir := testenv.Isolate(t)
	c := newClock()
	db := mustOpen(t, store.Options{Dir: dir, Now: c.now, Migrations: migrations(map[string]string{"1_notes.sql": mig1})})
	for range 7 {
		if _, err := db.DailyBackup(ctx); err != nil {
			t.Fatal(err)
		}
		c.advance(24 * time.Hour)
	}
	// A migration that keeps failing: every retry takes a pre-migration copy.
	for range 5 {
		c.advance(time.Minute)
		if _, err := db.Backup(ctx, "premigrate"); err != nil {
			t.Fatal(err)
		}
	}
	backups, err := store.ListBackups(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	daily, pre := 0, 0
	for _, b := range backups {
		if strings.Contains(b.Name, "premigrate") {
			pre++
		} else {
			daily++
		}
	}
	if daily != 7 || pre != 3 {
		t.Fatalf("kept %d daily and %d pre-migration backups, want 7 and 3", daily, pre)
	}
	// A pre-migration copy taken today doesn't count as today's daily backup.
	if created, err := db.DailyBackup(ctx); err != nil || !created {
		t.Fatalf("daily backup skipped because of a pre-migration copy: %v %v", created, err)
	}
}

func TestR2_RestoreRejectsPathTraversal(t *testing.T) {
	dir := testenv.Isolate(t)
	for _, name := range []string{`..\legaj.db`, `../x.db`, `C:\Windows\x.db`, `legaj-x.db`, ``} {
		if err := store.Restore(dir, name, time.Now); err == nil {
			t.Errorf("Restore accepted %q", name)
		}
	}
}

// inspectBackup opens a backup read-only and returns its applied versions
// and note count.
func inspectBackup(t *testing.T, path string) ([]int, int) {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_pragma=query_only(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var versions []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, v)
	}
	var notes int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM notes`).Scan(&notes); err != nil {
		t.Fatal(err)
	}
	return versions, notes
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
