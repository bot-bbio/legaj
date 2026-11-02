package opener_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/opener"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

// recorder stands in for ShellExecute: it records instead of launching.
type recorder struct{ launched []string }

func (r *recorder) Launch(target string) error {
	r.launched = append(r.launched, target)
	return nil
}

func newOpener(t *testing.T) (*opener.Opener, *recorder, string) {
	t.Helper()
	dir := testenv.Isolate(t)
	rec := &recorder{}
	return opener.New(dir, rec), rec, dir
}

func TestR33_RejectsFileAmpersandAndJavascript(t *testing.T) {
	o, rec, _ := newOpener(t)
	payloads := []string{
		`file:///x&cd>pwned.txt`, // A8
		`file:///C:/Windows/System32/calc.exe`,
		`javascript:alert(1)`,
		`JavaScript:alert(1)`,
		`vbscript:msgbox(1)`,
		`data:text/html,<script>alert(1)</script>`,
		`ms-settings:privacy`,
		`search-ms:query=x`,
		`\\attacker\share\run.exe`,
		`C:\Windows\System32\calc.exe`,
		`calc.exe`,
		`https://user:pass@example.com/`,
		"https://example.com/\x00evil",
		"https://example.com/a\nb",
		`https://`,
		``,
	}
	for _, p := range payloads {
		err := o.OpenURL(p)
		if apperr.CodeOf(err) != apperr.CodeInvalid {
			t.Errorf("OpenURL(%q) err = %v, want invalid", p, err)
		}
	}
	if len(rec.launched) != 0 {
		t.Fatalf("launched %q; nothing may execute", rec.launched)
	}
}

func TestR33_AllowsHTTPSAndMailto(t *testing.T) {
	o, rec, _ := newOpener(t)
	for _, u := range []string{
		"https://jobs.lever.co/acme/abc?x=1&y=2",
		"http://example.com/jobs",
		"mailto:hr@example.com?subject=Follow%20up&body=Hi%20there",
	} {
		if err := o.OpenURL(u); err != nil {
			t.Fatalf("OpenURL(%q): %v", u, err)
		}
	}
	want := []string{
		"https://jobs.lever.co/acme/abc?x=1&y=2",
		"http://example.com/jobs",
		"mailto:hr@example.com?subject=Follow%20up&body=Hi%20there",
	}
	for i := range want {
		if rec.launched[i] != want[i] {
			t.Fatalf("launched %q, want %q", rec.launched[i], want[i])
		}
	}
}

func TestR33_AllowsDataDirFiles(t *testing.T) {
	o, rec, dir := newOpener(t)
	pdf := filepath.Join(dir, "documents", "Acme & Co_v1.pdf")
	if err := os.MkdirAll(filepath.Dir(pdf), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pdf, []byte("%PDF-1.7"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := o.OpenFile(pdf); err != nil {
		t.Fatal(err)
	}
	if len(rec.launched) != 1 || rec.launched[0] != pdf {
		t.Fatalf("launched %q, want the absolute path (an & in a name is just a character)", rec.launched)
	}
}

func TestR33_RejectsFilesOutsideDataDirOrOfUnsafeType(t *testing.T) {
	o, rec, dir := newOpener(t)
	exe := filepath.Join(dir, "evil.exe")
	if err := os.WriteFile(exe, []byte("MZ"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.pdf")
	if err := os.WriteFile(outside, []byte("%PDF"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		exe,                               // inside, but executable
		outside,                           // a real PDF outside the data folder
		filepath.Join(dir, "..", "x.pdf"), // traversal
		filepath.Join(dir, "missing.pdf"), // doesn't exist
		dir,                               // a folder
		`C:\Windows\System32\calc.exe`,
		`relative.pdf`,
	} {
		if err := o.OpenFile(p); err == nil {
			t.Errorf("OpenFile(%q) allowed", p)
		}
	}
	if len(rec.launched) != 0 {
		t.Fatalf("launched %q", rec.launched)
	}
}
