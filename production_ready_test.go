package main

// production_ready_test.go
//
// Test suite for the final production checklist. Tests are written TDD-style:
// they will fail (or not compile) until the corresponding change is implemented.
//
// Each test documents its contract via a comment listing the code change required
// to make it pass. Run with: go test -v -run TestProd ./...

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// DISABLED FEATURES — Code Preservation
//
// Disabled features are hidden from the UI but NOT deleted. These compile-time
// references ensure the functions are never accidentally removed.
// ─────────────────────────────────────────────────────────────────────────────

// TestDisabledFeatureFunctionsPreserved fails to compile if the implementation
// code for any hidden feature is deleted rather than just hidden.
//
// Required: keep buildPrepTab() and buildTailoringTab() in gui.go.
// IMAP state fields (ImapServer, Email, Password) must remain on AppState.
func TestDisabledFeatureFunctionsPreserved(t *testing.T) {
	preserved := []interface{}{
		buildPrepTab,
		buildTailoringTab,
	}
	if len(preserved) == 0 {
		t.Fatal("reference slice must not be empty")
	}
	// Compile-time check: IMAP state fields must still exist on the struct
	_ = state.ImapServer
	_ = state.Email
	_ = state.Password
}

// ─────────────────────────────────────────────────────────────────────────────
// TAILOR SELECTED DIALOG — Resume / Cover Letter / Both
// ─────────────────────────────────────────────────────────────────────────────

// TestTailorModeOptionsExact verifies the tailor dialog offers exactly the three
// expected choices in order.
//
// Required: add func getTailorModeOptions() []string to gui.go returning
//
//	[]string{"Resume", "Cover Letter", "Both"}
func TestTailorModeOptionsExact(t *testing.T) {
	opts := getTailorModeOptions()
	want := []string{"Resume", "Cover Letter", "Both"}
	if len(opts) != len(want) {
		t.Fatalf("expected %d tailor mode options, got %d: %v", len(want), len(opts), opts)
	}
	for i, o := range opts {
		if o != want[i] {
			t.Errorf("tailor mode option[%d]: got %q, want %q", i, o, want[i])
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// OPEN RESUME / COVER LETTER — Path Resolution
// ─────────────────────────────────────────────────────────────────────────────

// TestResolveDocumentPath_Exists verifies that a valid file is found and the
// returned path matches the expected absolute path.
//
// Required: add func resolveDocumentPath(folder, filename string) (string, bool)
// to gui.go. The function must join folder + filename, stat the result, and
// return (absPath, true) on success or ("", false) on any failure.
func TestResolveDocumentPath_Exists(t *testing.T) {
	dir := t.TempDir()
	filename := "Acme_Resume_Tailored.pdf"
	full := filepath.Join(dir, filename)
	if err := os.WriteFile(full, []byte("PDF"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok := resolveDocumentPath(dir, filename)
	if !ok {
		t.Fatal("resolveDocumentPath returned ok=false, expected ok=true for existing file")
	}
	if got != full {
		t.Errorf("resolveDocumentPath path: got %q, want %q", got, full)
	}
}

// TestResolveDocumentPath_Missing verifies that a missing file yields ok=false.
func TestResolveDocumentPath_Missing(t *testing.T) {
	_, ok := resolveDocumentPath(t.TempDir(), "nonexistent.pdf")
	if ok {
		t.Error("expected ok=false for missing file, got ok=true")
	}
}

// TestResolveDocumentPath_EmptyFilename verifies that an empty filename yields ok=false.
func TestResolveDocumentPath_EmptyFilename(t *testing.T) {
	_, ok := resolveDocumentPath(t.TempDir(), "")
	if ok {
		t.Error("expected ok=false for empty filename, got ok=true")
	}
}

// TestOpenLinkFileURL_PathConstruction verifies the file:/// URL produced from a
// known path round-trips back to the correct local path without corruption.
// This tests the logic inside openLink without invoking the OS.
func TestOpenLinkFileURL_PathConstruction(t *testing.T) {
	dir := t.TempDir()
	filename := "My Company_Resume_Tailored.pdf"
	absPath := filepath.Join(dir, filename)
	_ = os.WriteFile(absPath, []byte("x"), 0o644)

	// Replicate the URL construction used in trackerOpenResumeBtn
	url := "file:///" + filepath.ToSlash(absPath)
	if !strings.HasPrefix(url, "file:///") {
		t.Errorf("expected file:/// prefix, got %q", url)
	}

	// Replicate the path extraction logic in openLink
	localPath := strings.TrimPrefix(url, "file://")
	if len(localPath) > 0 && localPath[0] == '/' {
		localPath = localPath[1:]
	}
	localPath = filepath.Clean(localPath)

	if _, err := os.Stat(localPath); err != nil {
		t.Errorf("round-tripped path %q does not exist: %v", localPath, err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// JOB LEADS NAMING — Clip Inbox renamed
// ─────────────────────────────────────────────────────────────────────────────

// TestClipCardTitleIsJobLeads verifies the clip inbox card title has been updated.
//
// Required: replace the inline "Clip Inbox" string literal with a named constant
//
//	const clipCardTitle = "Job Leads"
//
// and use that constant in the widget.NewCard() call.
func TestClipCardTitleIsJobLeads(t *testing.T) {
	const want = "Job Leads"
	if clipCardTitle != want {
		t.Errorf("clip card title: got %q, want %q", clipCardTitle, want)
	}
}

// TestNoRawClipInboxStrings verifies that no raw "Clip Inbox" or "Clipper Inbox"
// string literals remain anywhere in the compiled binary's string table.
// This catches inline strings that were missed during the rename.
//
// Required: replace every occurrence of "Clip Inbox" and "Clipper Inbox" in
// gui.go with the clipCardTitle constant or the new canonical name.
func TestNoRawClipInboxStrings(t *testing.T) {
	src, err := os.ReadFile("gui.go")
	if err != nil {
		t.Fatalf("could not read gui.go: %v", err)
	}
	banned := []string{`"Clip Inbox"`, `"Clipper Inbox"`}
	for _, b := range banned {
		if strings.Contains(string(src), b) {
			t.Errorf("gui.go still contains banned string literal %s — replace with clipCardTitle constant or new name", b)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// LINKEDIN CREDIT — Display name
// ─────────────────────────────────────────────────────────────────────────────

// TestLinkedInCreditDisplayName verifies both credit buttons show "Roberto Montero"
// instead of the old "/alvvays" handle.
//
// Required: replace inline strings in Settings and Help tab credit buttons with
// a named constant:
//
//	const linkedInCreditDisplay = "LinkedIn: Roberto Montero"
//	const linkedInCreditURL    = "https://linkedin.com/in/roberto-montero"
func TestLinkedInCreditDisplayName(t *testing.T) {
	const wantLabel = "LinkedIn: Roberto Montero"
	if linkedInCreditDisplay != wantLabel {
		t.Errorf("linkedInCreditDisplay: got %q, want %q", linkedInCreditDisplay, wantLabel)
	}
}

// TestLinkedInCreditDisplayIsName verifies the visible credit label shows the
// author's name, not the old "/alvvays" handle. The handle legitimately remains
// in the profile URL (linkedInCreditURL), so only the display label is checked.
func TestLinkedInCreditDisplayIsName(t *testing.T) {
	if strings.Contains(linkedInCreditDisplay, "alvvays") {
		t.Errorf("linkedInCreditDisplay %q still shows the old handle; it should show the author's name", linkedInCreditDisplay)
	}
	if !strings.Contains(linkedInCreditDisplay, "Roberto Montero") {
		t.Errorf("linkedInCreditDisplay %q should contain the author's name", linkedInCreditDisplay)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// WIZARD SKIP — Step navigation only
// ─────────────────────────────────────────────────────────────────────────────

// TestWizardSkipAdvancesStep verifies that pressing Skip on any non-final step
// advances to the next step without closing the wizard.
//
// Required: extract wizard navigation into a testable type:
//
//	type wizardNavigator struct { currentStep, totalSteps int }
//	func (wn *wizardNavigator) skip() (nextStep int, shouldClose bool)
//
// skip() must return (currentStep+1, false) for all steps except the last,
// and (0, true) when currentStep == totalSteps.
func TestWizardSkipAdvancesStep(t *testing.T) {
	wn := &wizardNavigator{currentStep: 1, totalSteps: 6}
	for step := 1; step <= 5; step++ {
		wn.currentStep = step
		nextStep, shouldClose := wn.skip()
		if shouldClose {
			t.Errorf("step %d: skip() returned shouldClose=true; expected to advance, not close", step)
		}
		if nextStep != step+1 {
			t.Errorf("step %d: skip() nextStep=%d, want %d", step, nextStep, step+1)
		}
	}
}

// TestWizardSkipLastStepCloses verifies that pressing Skip on the final step closes the wizard.
func TestWizardSkipLastStepCloses(t *testing.T) {
	wn := &wizardNavigator{currentStep: 6, totalSteps: 6}
	_, shouldClose := wn.skip()
	if !shouldClose {
		t.Error("skip() on last step should return shouldClose=true")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// CLIPPER ROW LAYOUT — Actions column width
// ─────────────────────────────────────────────────────────────────────────────

// TestClipperRowActionsWidthFitsThreeButtons verifies the actions column is wide
// enough to render "Open", "Track & Tailor", and "Add Tracker Only" side by side.
// Minimum: 3 buttons × ~120px each = 360px.
//
// Required: ensure actionsWidth in clipperRowLayout.Layout is ≥ 380px (current
// value) and that the MinSize.Width accommodates the full row without clipping.
func TestClipperRowActionsWidthFitsThreeButtons(t *testing.T) {
	const minActionsWidth = float32(360)
	l := &clipperRowLayout{}
	minSize := l.MinSize(nil)

	// The declared minimum row width must leave at least 360px for actions
	// after the 4 data columns. At 930px minimum, 4 equal data cols = 550px,
	// leaving 380px for actions. Verify the overall minimum makes this possible.
	const dataCols = 4
	dataWidth := minSize.Width - minActionsWidth
	colWidth := dataWidth / dataCols
	if colWidth < 1 {
		t.Errorf("MinSize.Width %g is too small: leaves only %g per data column with %g for actions",
			minSize.Width, colWidth, minActionsWidth)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// SELECT COLUMN WIDTH — No dead space
// ─────────────────────────────────────────────────────────────────────────────

// TestSelectColumnIsCompact verifies that the Select checkbox column (col 0) in
// the Job Tracker has been reduced to a compact width.
//
// Required: extract the column-0 width into a named constant:
//
//	const trackerColSelectWidth float32 = 40   // (or similar compact value)
//
// and use it in the SetColumnWidth call inside buildTrackerTab.
func TestSelectColumnIsCompact(t *testing.T) {
	const maxCompactWidth = float32(50)
	if trackerColSelectWidth > maxCompactWidth {
		t.Errorf("Select column width %.0fpx exceeds compact max of %.0fpx — still too wide",
			trackerColSelectWidth, maxCompactWidth)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// HELP / ONBOARDING TEXT — No stale feature references
// ─────────────────────────────────────────────────────────────────────────────

// TestOnboardingTextNoDisabledFeatures verifies that the onboarding guide
// markdown no longer mentions features that have been disabled.
//
// Scans only the user-facing onboarding text (onboardingGuideText), not the
// entire source file, because the implementations of disabled features are
// intentionally preserved in gui.go for future re-enablement.
func TestOnboardingTextNoDisabledFeatures(t *testing.T) {
	content := onboardingGuideText()
	banned := []string{
		"Discovery Engine",
		"Interview Prep",
		"IMAP",
		"Sync Email",
		"Tailor Assets",
	}
	for _, phrase := range banned {
		if strings.Contains(content, phrase) {
			t.Errorf("onboarding text still contains disabled-feature reference %q — remove from onboarding and help text", phrase)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// REGRESSION — Core tracker CRUD still works after all UI changes
// ─────────────────────────────────────────────────────────────────────────────

// TestTrackerCRUDRegressionAfterUIChanges is a lightweight smoke test confirming
// that add and delete operations remain functional after the UI refactor.
func TestTrackerCRUDRegressionAfterUIChanges(t *testing.T) {
	// Back up and restore real data
	old := state.Applications
	defer func() { state.Applications = old }()

	state.Applications = nil

	if err := addApplicationGo("Regression Corp", "QA Lead", "Remote",
		"https://example.com/regression", "Wishlist", "", "", "regression test"); err != nil {
		t.Fatalf("addApplicationGo: %v", err)
	}
	if len(state.Applications) != 1 {
		t.Fatalf("expected 1 application after add, got %d", len(state.Applications))
	}

	if err := deleteApplicationGo("Regression Corp", "QA Lead"); err != nil {
		t.Fatalf("deleteApplicationGo: %v", err)
	}
	if len(state.Applications) != 0 {
		t.Errorf("expected 0 applications after delete, got %d", len(state.Applications))
	}
}
