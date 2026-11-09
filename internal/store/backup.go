package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// BackupDir is the folder of backups inside the data folder.
const BackupDir = "backups"

// Backups are rotated per kind, so a migration that keeps failing (each
// retry takes a pre-migration copy) can never push out the daily history.
const (
	keepDaily      = 7 // R2
	keepPremigrate = 3
)

// BackupInfo describes one backup file.
type BackupInfo struct {
	Name      string    `json:"name"`
	TakenAt   time.Time `json:"takenAt"`
	SizeBytes int64     `json:"sizeBytes"`
}

const stampLayout = "20060102T150405Z"

// backupName matches legaj-<UTC stamp>[-reason][-n].db. Restore only ever
// accepts names that match this, which rules out path traversal.
var backupName = regexp.MustCompile(`^legaj-(\d{8}T\d{6}Z)(-[a-z]+)?(-\d+)?\.db$`)

// reasonOf returns a backup file's reason ("" for a daily backup).
func reasonOf(name string) string {
	m := backupName.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	return strings.TrimPrefix(m[2], "-")
}

// DailyBackup takes a daily backup unless one already exists from today
// (in the clock's local zone). It reports whether a backup was taken.
func (d *DB) DailyBackup(ctx context.Context) (bool, error) {
	all, err := scanBackups(d.dir)
	if err != nil {
		return false, err
	}
	now := d.now()
	for _, b := range all {
		if reasonOf(b.Name) != "" {
			continue // a pre-migration copy isn't today's daily backup
		}
		if sameDay(b.TakenAt.In(now.Location()), now) {
			return false, nil
		}
		break // newest daily is older than today
	}
	if _, err := d.Backup(ctx, ""); err != nil {
		return false, err
	}
	return true, nil
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// Backup writes a consistent copy of the database with VACUUM INTO, then
// prunes to the newest 7. The copy is written to a temp name and renamed,
// so a crash never leaves a half-written file under a backup name.
func (d *DB) Backup(ctx context.Context, reason string) (BackupInfo, error) {
	dir := filepath.Join(d.dir, BackupDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return BackupInfo{}, err
	}
	taken := d.now().UTC().Truncate(time.Second)
	base := "legaj-" + taken.Format(stampLayout)
	if reason != "" {
		base += "-" + reason
	}
	name := base + ".db"
	for n := 2; fileExists(filepath.Join(dir, name)); n++ {
		name = fmt.Sprintf("%s-%d.db", base, n)
	}
	final := filepath.Join(dir, name)
	tmp := final + ".tmp"
	_ = os.Remove(tmp) // leftover from an interrupted backup
	if _, err := d.writer.ExecContext(ctx, `VACUUM INTO ?`, tmp); err != nil {
		_ = os.Remove(tmp)
		return BackupInfo{}, fmt.Errorf("backup: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return BackupInfo{}, fmt.Errorf("backup: %w", err)
	}
	if err := prune(dir); err != nil {
		return BackupInfo{}, err
	}
	fi, err := os.Stat(final)
	if err != nil {
		return BackupInfo{}, err
	}
	return BackupInfo{Name: name, TakenAt: taken, SizeBytes: fi.Size()}, nil
}

// prune keeps the newest keepDaily daily backups and the newest
// keepPremigrate pre-migration copies, and deletes the rest.
func prune(dir string) error {
	all, err := scanBackupDir(dir) // newest first
	if err != nil {
		return err
	}
	kept := map[string]int{}
	var errs []error
	for _, b := range all {
		reason := reasonOf(b.Name)
		limit := keepDaily
		if reason != "" {
			limit = keepPremigrate
		}
		kept[reason]++
		if kept[reason] <= limit {
			continue
		}
		if err := os.Remove(filepath.Join(dir, b.Name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ListBackups returns the backups in dataDir that pass an integrity check,
// newest first. Broken backups are never offered for restore.
func ListBackups(ctx context.Context, dataDir string) ([]BackupInfo, error) {
	all, err := scanBackups(dataDir)
	if err != nil {
		return nil, err
	}
	valid := []BackupInfo{}
	for _, b := range all {
		ok, err := backupHealthy(ctx, filepath.Join(dataDir, BackupDir, b.Name))
		if err != nil {
			return nil, err
		}
		if ok {
			valid = append(valid, b)
		}
	}
	return valid, nil
}

func backupHealthy(ctx context.Context, path string) (bool, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=query_only(1)")
	if err != nil {
		return false, err
	}
	defer db.Close()
	ok, err := integrityCheck(ctx, db)
	if err != nil {
		// Unreadable for a non-corruption reason: don't offer it either.
		return false, nil
	}
	return ok, nil
}

func scanBackups(dataDir string) ([]BackupInfo, error) {
	return scanBackupDir(filepath.Join(dataDir, BackupDir))
}

// scanBackupDir lists backup files by name pattern, newest first.
func scanBackupDir(dir string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []BackupInfo
	for _, e := range entries {
		m := backupName.FindStringSubmatch(e.Name())
		if e.IsDir() || m == nil {
			continue
		}
		taken, err := time.Parse(stampLayout, m[1])
		if err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, BackupInfo{Name: e.Name(), TakenAt: taken, SizeBytes: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].TakenAt.Equal(out[j].TakenAt) {
			return out[i].TakenAt.After(out[j].TakenAt)
		}
		return out[i].Name > out[j].Name
	})
	return out, nil
}

// Restore replaces legaj.db with the named backup. The database must be
// closed. name comes from the UI, so it must be a plain backup file name
// from the backups folder. Any current legaj.db is kept, renamed to
// legaj.db.replaced-<ts>, never deleted.
func Restore(dataDir, name string, now func() time.Time) error {
	if !backupName.MatchString(name) {
		return errors.New("not a backup name")
	}
	src := filepath.Join(dataDir, BackupDir, name)
	dst := filepath.Join(dataDir, FileName)
	tmp := dst + ".restore.tmp"
	if err := copyFile(src, tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("restore: %w", err)
	}
	if fileExists(dst) {
		replaced := FileName + ".replaced-" + now().UTC().Format(stampLayout)
		if err := os.Rename(dst, filepath.Join(dataDir, replaced)); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("restore: keep current database: %w", err)
		}
		if err := setAsideSidecars(dataDir, replaced); err != nil {
			_ = os.Remove(tmp)
			return err
		}
	} else if err := setAsideSidecars(dataDir, FileName+".orphan-"+now().UTC().Format(stampLayout)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("restore: %w", err)
	}
	return nil
}

// copyFile copies src to dst and flushes it to disk before returning.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
