//go:build live

package live_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/ai"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/pdf"
	"github.com/bot-bbio/legaj/internal/resume"
	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

// The synthetic resume: a made-up person, example.com addresses and a 555
// phone number, shaped like a real two-page-or-less resume (sections,
// bullets, dates, links, a GPA, a project, a certification).
const (
	resumeName    = "Alex Rivera"
	resumeContact = "alex.rivera@example.com · (555) 010-0199 · Springfield, IL · linkedin.com/in/alex-rivera-example · github.com/alexrivera-example"
)

var resumeParagraphs = []string{
	"SUMMARY: Data analyst with four years of experience turning messy operational data into dashboards and forecasts that teams actually use.",
	"EXPERIENCE",
	"Data Analyst, Northwind Logistics, Springfield, IL, March 2022 to present",
	"• Built a demand forecast in Python that cut overstock by 18% across 40 warehouses.",
	"• Automated weekly reporting with SQL and Power BI, saving the operations team 6 hours a week.",
	"Junior Analyst, Contoso Health, Chicago, IL, June 2020 to February 2022",
	"• Cleaned and merged claims data from three systems into one warehouse.",
	"• Presented monthly KPI reviews to the operations director.",
	"Analytics Intern, Fabrikam Inc., Chicago, IL, June 2019 to August 2019",
	"• Wrote SQL checks that caught a billing error worth $42,000.",
	"EDUCATION",
	"B.S. Statistics, University of Illinois Springfield, 2016 to 2020, GPA 3.6",
	"PROJECTS",
	"Transit Delay Explorer: an open-source dashboard of city bus delays (Python, Streamlit).",
	"SKILLS",
	"Languages: Python, SQL, R. Tools: Power BI, Tableau, Excel, Git.",
	"CERTIFICATIONS: Google Data Analytics Certificate (2021).",
}

func resumeText() string {
	return resumeName + "\n" + resumeContact + "\n\n" + strings.Join(resumeParagraphs, "\n")
}

// resumePDF renders the resume with LeGaJ's own PDF engine: a real PDF with
// a text layer, like one exported from Word.
func resumePDF(t *testing.T) []byte {
	t.Helper()
	data, _, err := pdf.Fit(pdf.Letter{Name: resumeName, Contact: resumeContact, Paragraphs: resumeParagraphs})
	if err != nil {
		t.Fatalf("building the synthetic PDF: %v", err)
	}
	return data
}

// resumeDOCX builds a minimal Word document, one paragraph per line.
func resumeDOCX(t *testing.T) []byte {
	t.Helper()
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	var body strings.Builder
	for _, p := range append([]string{resumeName, resumeContact}, resumeParagraphs...) {
		fmt.Fprintf(&body, `<w:p><w:r><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, esc.Replace(p))
	}
	doc := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body.String() + `</w:body></w:document>`
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
	return buf.Bytes()
}

// newResumeService is the production resume service on a scratch database,
// talking to the real Gemini API.
func newResumeService(t *testing.T, provider ai.Provider) *resume.Service {
	t.Helper()
	dir := testenv.Isolate(t)
	db, st, err := store.Open(context.Background(), store.Options{Dir: dir})
	if err != nil || st.Kind != store.StartupReady {
		t.Fatalf("open: %v %+v", err, st)
	}
	t.Cleanup(func() { _ = db.Close() })
	return resume.NewService(resume.Config{
		DB: db, Bus: events.NewBus(&events.Recorder{}), Dir: dir, AI: provider, Now: time.Now,
		Model: func(context.Context) string { return liveModel() },
	})
}

// TestLive_Fixtures needs no key: the synthetic files are what LeGaJ accepts
// as a PDF and a Word resume.
func TestLive_Fixtures(t *testing.T) {
	svc := newResumeService(t, nil)
	for name, data := range map[string][]byte{"cv.pdf": resumePDF(t), "cv.docx": resumeDOCX(t)} {
		if _, err := svc.Store(context.Background(), name, data); err != nil {
			t.Fatalf("%s refused: %v", name, err)
		}
	}
}

// TestLive_Diagnose_ResumeSchemaWithText sends the production resume schema
// with plain text and no file. A 400 here means Gemini refuses the schema
// itself (D57's hypothesis, C50).
func TestLive_Diagnose_ResumeSchemaWithText(t *testing.T) {
	zero := 0.0
	resp, err := liveClient(t).Generate(liveCtx(t), ai.Request{
		Model:       liveModel(),
		System:      "Extract the resume into the JSON schema.",
		Parts:       []ai.Part{ai.Text(resumeText())},
		Temperature: &zero,
		Schema:      resume.Schema(),
	})
	if err != nil {
		explain(t, "the resume schema with text only", err)
	}
	t.Logf("schema accepted by %s (%d bytes back)", resp.Model, len(resp.Text))
}

// TestLive_Diagnose_PDFWithoutSchema sends the PDF with no schema. A 400 here
// means Gemini refuses the file part, not the schema.
func TestLive_Diagnose_PDFWithoutSchema(t *testing.T) {
	resp, err := liveClient(t).Generate(liveCtx(t), ai.Request{
		Model: liveModel(),
		Parts: []ai.Part{ai.Text("In one sentence, whose resume is this?"), ai.Inline("application/pdf", resumePDF(t))},
	})
	if err != nil {
		explain(t, "the PDF without a schema", err)
	}
	if !strings.Contains(resp.Text, "Rivera") {
		t.Fatalf("Gemini didn't read the PDF: %q", resp.Text)
	}
}

// TestLive_ResumeRead_* run the production read end to end (R8, R62): store
// the file, send it as LeGaJ does, validate the answer into a profile draft.
func TestLive_ResumeRead_PDF(t *testing.T)  { liveResumeRead(t, "cv.pdf", resumePDF(t)) }
func TestLive_ResumeRead_DOCX(t *testing.T) { liveResumeRead(t, "cv.docx", resumeDOCX(t)) }

func liveResumeRead(t *testing.T, name string, data []byte) {
	t.Helper()
	svc := newResumeService(t, liveClient(t))
	ctx := liveCtx(t)
	info, err := svc.Store(ctx, name, data)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	draft, err := svc.Parse(ctx, info.ID)
	if err != nil {
		explain(t, "reading "+name, err)
	}
	if draft.FullName != resumeName {
		t.Errorf("name = %q, want %q", draft.FullName, resumeName)
	}
	if len(draft.Experiences) < 3 {
		t.Errorf("experiences = %d, want 3", len(draft.Experiences))
	}
	if len(draft.Education) < 1 || len(draft.Skills) < 1 {
		t.Errorf("education %d, skill groups %d: want at least one of each", len(draft.Education), len(draft.Skills))
	}
}
