package documents_test

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/documents"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/pdf"
	"github.com/bot-bbio/legaj/internal/pipeline"
	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

var ctx = context.Background()

type fixture struct {
	svc *documents.Service
	db  *store.DB
	dir string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := testenv.Isolate(t)
	db, st, err := store.Open(ctx, store.Options{Dir: dir})
	if err != nil || st.Kind != store.StartupReady {
		t.Fatalf("open: %v %+v", err, st)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &fixture{svc: documents.NewService(db, events.NewBus(&events.Recorder{}), dir, time.Now), db: db, dir: dir}
}

func (f *fixture) app(t *testing.T, company, role string) int64 {
	t.Helper()
	var id int64
	err := f.db.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = pipeline.NewMachine(time.Now).CreateLead(ctx, tx, pipeline.NewLead{Company: company, Role: role, Source: store.SourcePaste})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func letter(p string) pdf.Letter {
	return pdf.Letter{Name: "Ada", Salutation: "Dear Hiring Team,", Paragraphs: []string{p}, Closing: "Sincerely,", Signature: "Ada"}
}

func TestR22_SecondLetterIsV2KeepsV1(t *testing.T) {
	f := newFixture(t)
	id := f.app(t, "Acme", "Go Developer")
	v1, err := f.svc.WriteLetter(ctx, id, letter("First draft."))
	if err != nil {
		t.Fatal(err)
	}
	v1Bytes, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(v1.Path)))
	if err != nil {
		t.Fatal(err)
	}
	v2, err := f.svc.WriteLetter(ctx, id, letter("Second draft."))
	if err != nil {
		t.Fatal(err)
	}
	if v1.Version != 1 || v2.Version != 2 {
		t.Fatalf("versions = %d, %d", v1.Version, v2.Version)
	}
	if !strings.HasSuffix(v2.Path, "_v2.pdf") || v1.Path == v2.Path {
		t.Fatalf("paths = %q, %q", v1.Path, v2.Path)
	}
	after, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(v1.Path)))
	if err != nil || !bytes.Equal(after, v1Bytes) {
		t.Fatal("v1 was changed or removed by writing v2 (U3)")
	}
	if filepath.Dir(v1.Path) != filepath.Dir(v2.Path) {
		t.Fatal("versions of one application belong in one folder")
	}
}

func TestR22_V1LockedStillWritesV2(t *testing.T) {
	f := newFixture(t)
	id := f.app(t, "Acme", "Dev")
	v1, err := f.svc.WriteLetter(ctx, id, letter("v1"))
	if err != nil {
		t.Fatal(err)
	}
	// A PDF viewer holding v1 open with no sharing allowed.
	p, _ := windows.UTF16PtrFromString(filepath.Join(f.dir, filepath.FromSlash(v1.Path)))
	h, err := windows.CreateFile(p, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)

	if _, err := f.svc.WriteLetter(ctx, id, letter("v2")); err != nil {
		t.Fatalf("v2 failed while v1 is open in a viewer: %v", err)
	}
}

func TestR22_FilenameSanitized(t *testing.T) {
	cases := map[string]string{
		`AT&T / R&D: "Best" <Team>?*|`: `AT&T - R&D- -Best- -Team----`,
		"CON":                          "_CON",
		"con.txt":                      "_con.txt",
		"LPT1":                         "_LPT1",
		"Acme Inc.  ":                  "Acme Inc",
		"tab\there\x00":                "tab-here-",
		"Żółć Sp. z o.o.":              "Żółć Sp. z o.o",
	}
	for in, want := range cases {
		if got := documents.Sanitize(in, "x"); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	if got := documents.Sanitize(`///`, "fallback"); got != "fallback" && strings.Trim(got, "-") != "" {
		// all separators: either the fallback or only replacement characters
		t.Errorf("Sanitize of separators = %q", got)
	}
	if got := documents.Sanitize(strings.Repeat("a", 300), "x"); len([]rune(got)) > 60 {
		t.Errorf("segment not shortened: %d runes", len([]rune(got)))
	}
	if got := documents.Sanitize("   ", "fallback"); got != "fallback" {
		t.Errorf("blank -> %q, want the fallback", got)
	}
}

func TestR22_FilesStayInsideTheApplicationFolder(t *testing.T) {
	f := newFixture(t)
	id := f.app(t, `..\..\Windows`, `..\system32`)
	v, err := f.svc.WriteLetter(ctx, id, letter("x"))
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(f.dir, filepath.FromSlash(v.Path))
	rel, err := filepath.Rel(filepath.Join(f.dir, documents.Folder), abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("path %q escapes the documents folder", v.Path)
	}
}

func TestR22_UnknownCompanyUsesAppID(t *testing.T) {
	f := newFixture(t)
	id := f.app(t, "", "")
	v, err := f.svc.WriteLetter(ctx, id, letter("x"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v.Path, "Application_") || !strings.Contains(filepath.Base(v.Path), "_v1.pdf") {
		t.Fatalf("path = %q", v.Path)
	}
	if !strings.Contains(v.Path, "/"+itoa(id)+"_") {
		t.Fatalf("path %q doesn't carry the application id", v.Path)
	}
}

func TestR22_RecordedInDBWithVersion(t *testing.T) {
	f := newFixture(t)
	id := f.app(t, "Acme", "Dev")
	v, err := f.svc.WriteLetter(ctx, id, letter("x"))
	if err != nil {
		t.Fatal(err)
	}
	var status, path string
	var version int
	if err := f.db.Reader().QueryRowContext(ctx, `SELECT status, version, file_path FROM documents WHERE id = ?`, v.DocumentID).Scan(&status, &version, &path); err != nil {
		t.Fatal(err)
	}
	if status != "approved" || version != 1 || path != v.Path {
		t.Fatalf("row = %s v%d %s", status, version, path)
	}
}

func TestR22_DeletedOrMissingApplicationWritesNoFile(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.WriteLetter(ctx, 999, letter("x")); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("err = %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(f.dir, documents.Folder)); len(entries) != 0 {
		t.Fatal("a folder or file was created for a missing application")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
