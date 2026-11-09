package documents_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/documents"
)

func TestR13_PurgedApplicationsLoseTheirLetterFiles(t *testing.T) {
	f := newFixture(t)
	first := f.app(t, "Acme", "Dev") // id 1
	var eleventh int64
	for range 10 {
		eleventh = f.app(t, "Globex", "Dev") // ends at id 11, whose folder starts "11_"
	}
	for _, id := range []int64{first, eleventh} {
		if _, err := f.svc.WriteLetter(ctx, id, letter("Hello.")); err != nil {
			t.Fatal(err)
		}
	}
	f.svc.RemoveFiles([]int64{first})

	entries, err := os.ReadDir(filepath.Join(f.dir, documents.Folder))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "11_Globex" {
		t.Fatalf("documents folder holds %v; want only 11_Globex", names)
	}
}

func TestR13_SoftDeletedRowGetsNoLetterFile(t *testing.T) {
	f := newFixture(t)
	id := f.app(t, "Acme", "Dev")
	if err := f.db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE applications SET deleted_at = '2026-11-02T00:00:00.000Z' WHERE id = ?`, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.WriteLetter(ctx, id, letter("Hello.")); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("letter for a deleted row: err = %v, want not found", err)
	}
	if _, err := os.Stat(filepath.Join(f.dir, documents.Folder)); err == nil {
		entries, _ := os.ReadDir(filepath.Join(f.dir, documents.Folder))
		for _, e := range entries {
			files, _ := os.ReadDir(filepath.Join(f.dir, documents.Folder, e.Name()))
			if len(files) > 0 {
				t.Fatalf("a file was written for a deleted row: %s/%s", e.Name(), files[0].Name())
			}
		}
	}
}
