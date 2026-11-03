package main

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/bot-bbio/legaj/internal/ai"
	"github.com/bot-bbio/legaj/internal/ai/aitest"
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/core"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/pipeline"
	"github.com/bot-bbio/legaj/internal/profile"
	"github.com/bot-bbio/legaj/internal/secrets"
	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

// Sentinels for the end-to-end scan (R4, R34). The key reuses the binding
// test's sentinel; the rest are made up and must never reach a log.
const (
	sentinelEmail  = "sentinel.person@example.org"
	sentinelPhone  = "+48 601 234 567"
	sentinelResume = "SENTINEL-RESUME-LINE built the Analytical Engine compiler"
)

func scanDOCX(t *testing.T, line string) string {
	t.Helper()
	doc := `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:r><w:t>Ada Lovelace</w:t></w:r></w:p><w:p><w:r><w:t>` + sentinelEmail + ` ` + sentinelPhone + `</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>` + line + `</w:t></w:r></w:p></w:body></w:document>`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`,
		"word/document.xml":   doc,
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "resume.docx")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// scanModel plays Gemini: letters and answers succeed; the resume parse
// fails with a 400 exactly as the real client reports one. The client never
// puts a request or response body in an error (TestR5_401MapsToKeyRejected
// pins that contract), so the resume text sent in the request may only
// reach the log if a request body were logged, which is what this checks.
func scanModel(req ai.Request) (string, error) {
	switch {
	case strings.HasPrefix(req.System, "You write cover letters"):
		b, _ := json.Marshal(map[string]any{
			"salutation": "Dear Hiring Team,",
			"opening":    "I'm applying for the Engineer role at Acme.",
			"whyOptions": []string{"Acme builds engines.", "At Acme the work is the product.", "I've followed Acme for years."},
			"body":       []string{"At Babbage & Co I wrote the first algorithm."},
			"closing":    "I'd welcome the chance to talk.",
			"signOff":    "Sincerely,",
		})
		return string(b), nil
	case strings.HasPrefix(req.System, "You write answers"):
		a := "Acme " + strings.Repeat("engines ", 99)
		b, _ := json.Marshal(map[string]any{"answers": []string{a, a, a}})
		return string(b), nil
	}
	return "", apperr.Wrap(errors.New("gemini: HTTP 400 INVALID_ARGUMENT"), apperr.CodeInternal, "Gemini couldn't process this request.")
}

func TestR4_R34_SentinelScan(t *testing.T) {
	dir := testenv.Isolate(t)
	keys := secrets.NewMemory()
	c, st, err := core.Boot(context.Background(), core.Options{
		Dir: dir, Now: time.Now, Emitter: &events.Recorder{},
		Secrets: keys, Autostart: noAutostart{}, AI: &aitest.Fake{Respond: scanModel},
		KeyTester: func(context.Context, string) error { return nil },
		Exe:       func() (string, error) { return `C:\LeGaJ\legaj.exe`, nil },
		Launcher:  &launches{},
	})
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	app.ctx = context.Background()
	app.updates = noUpdates{}
	docx := scanDOCX(t, sentinelResume)
	app.pickFile = func(context.Context) (string, error) { return docx, nil }
	app.setStartup(c, toStartupView(st))
	close(app.booted)

	// Onboarding: key, resume (its parse fails with the hostile 400), profile.
	onb := &OnboardingAPI{app: app}
	if _, err := onb.SaveKey(sentinelKey); err != nil {
		t.Fatal(err)
	}
	_, _ = onb.UploadResume() // stores the file; the parse fails (and is logged)
	if _, err := (&ProfileAPI{app: app}).SaveProfile(profile.Profile{
		FullName: "Ada Lovelace", Email: sentinelEmail, Phone: sentinelPhone, Summary: sentinelResume,
		Experiences: []profile.Experience{{Company: "Babbage & Co", Title: "Analyst", Bullets: []string{sentinelResume}}},
	}); err != nil {
		t.Fatal(err)
	}
	// Failed calls whose inputs carry the key, email and phone: the
	// redactor must catch them in the logged errors.
	_ = (&SettingsAPI{app: app}).SaveAPIKey("not a key " + sentinelKey + " " + sentinelEmail + " " + sentinelPhone)
	_, _ = (&PipelineAPI{app: app}).SetStage([]int64{1}, sentinelEmail+" "+sentinelPhone)

	// A job, a generated and approved letter, and form answers.
	pasted, err := (&PipelineAPI{app: app}).PasteJob(pipeline.PasteInput{Company: "Acme", Role: "Engineer", Description: "Acme builds engines."})
	if err != nil {
		t.Fatal(err)
	}
	letters := &LettersAPI{app: app}
	if _, err := letters.Generate([]int64{pasted.ID}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		items, err := letters.ReviewQueue()
		if err != nil {
			t.Fatal(err)
		}
		if len(items) == 1 && items[0].Status == "draft" {
			break
		}
		if len(items) == 1 && items[0].Status == "failed" {
			t.Fatalf("generation failed: %s", items[0].Reason)
		}
		if time.Now().After(deadline) {
			t.Fatalf("no draft in time: %+v", items)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := letters.ApproveLetter(pasted.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&AnswersAPI{app: app}).Generate(pasted.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	// R4: the key appears in no file at all, in UTF-8 or UTF-16.
	utf16le := func(s string) []byte {
		var b []byte
		for _, u := range utf16.Encode([]rune(s)) {
			b = append(b, byte(u), byte(u>>8))
		}
		return b
	}
	files := 0
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		files++
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(sentinelKey)) || bytes.Contains(data, utf16le(sentinelKey)) {
			t.Errorf("the API key is in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 4 {
		t.Fatalf("scanned only %d files; the run didn't write what it should have", files)
	}

	// R34: no email, phone or resume line in the logs.
	logs, err := os.ReadFile(filepath.Join(dir, "logs", "legaj.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"op":"SettingsAPI.SaveAPIKey"`, `"op":"PipelineAPI.SetStage"`, "resume parse failed"} {
		if !strings.Contains(string(logs), want) {
			t.Fatalf("the log lacks %s, so the scan proves nothing", want)
		}
	}
	digits := strings.NewReplacer(" ", "", "+", "").Replace(sentinelPhone)
	for _, s := range []string{sentinelEmail, sentinelPhone, digits, sentinelResume, "SENTINEL-RESUME-LINE"} {
		if strings.Contains(string(logs), s) {
			t.Errorf("the log contains %q", s)
		}
	}

	// The user's details stay in the tables meant for them. Approved
	// letters keep the contact header they were printed with.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, store.FileName)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"applications", "stage_events", "answers", "rounds", "followups", "settings"} {
		if n := rowsContaining(t, db, table, sentinelEmail, sentinelPhone, sentinelResume); n > 0 {
			t.Errorf("table %s holds the user's details in %d rows", table, n)
		}
	}
}

// rowsContaining counts rows of a table with any of the needles in any column.
func rowsContaining(t *testing.T, db *sql.DB, table string, needles ...string) int {
	t.Helper()
	rows, err := db.Query("SELECT * FROM " + table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		line := fmt.Sprint(vals...)
		for _, needle := range needles {
			if strings.Contains(line, needle) {
				n++
				break
			}
		}
	}
	return n
}
