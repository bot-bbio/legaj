package main

import (
	"encoding/json"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// TestMigrateClippedJobsToTracker covers the bulk "Add Selected to Tracker"
// data-layer helper: it must add new clips, skip clips already tracked (by link
// or company+role), and persist the additions to job-tracker.json.
func TestMigrateClippedJobsToTracker(t *testing.T) {
	trackerPath := "references/job-tracker.json"

	var backupBytes []byte
	backupExists := false
	if _, err := os.Stat(trackerPath); err == nil {
		backupBytes, _ = os.ReadFile(trackerPath)
		backupExists = true
	}
	_ = os.MkdirAll("references", 0755)

	oldApps := state.Applications
	defer func() {
		state.Applications = oldApps
		if backupExists {
			_ = os.WriteFile(trackerPath, backupBytes, 0644)
		} else {
			_ = os.Remove(trackerPath)
		}
	}()

	state.Applications = []JobApplication{}

	// Pre-seed one tracked application to exercise the dedupe path.
	if err := addApplicationGo("Existing Co", "Existing Role", "NY", "https://example.com/existing", "Applied", "r.pdf", "c.pdf", "seed"); err != nil {
		t.Fatalf("seed addApplicationGo failed: %v", err)
	}

	jobs := []ClippedJob{
		{Company: "Existing Co", Role: "Existing Role", Location: "NY", Link: "https://example.com/existing"}, // dup by link
		{Company: "Existing Co", Role: "Existing Role", Location: "NY", Link: "https://example.com/other"},    // dup by company+role
		{Company: "New Co", Role: "New Role", Location: "SF", Link: "https://example.com/new"},                 // added
	}

	added, skipped := migrateClippedJobsToTracker(jobs)
	if added != 1 {
		t.Errorf("expected added=1, got %d", added)
	}
	if skipped != 2 {
		t.Errorf("expected skipped=2, got %d", skipped)
	}
	if len(state.Applications) != 2 {
		t.Fatalf("expected 2 applications in memory, got %d", len(state.Applications))
	}

	// The migrated row must persist with the agreed status/notes.
	data, err := os.ReadFile(trackerPath)
	if err != nil {
		t.Fatalf("read tracker file: %v", err)
	}
	var fileApps []JobApplication
	if err := json.Unmarshal(data, &fileApps); err != nil {
		t.Fatalf("unmarshal tracker file: %v", err)
	}
	var migrated *JobApplication
	for i := range fileApps {
		if fileApps[i].Company == "New Co" {
			migrated = &fileApps[i]
		}
	}
	if migrated == nil {
		t.Fatalf("migrated 'New Co' not found in persisted file: %+v", fileApps)
	}
	if migrated.Status != "Applied" || migrated.Notes != "Clipped from browser." {
		t.Errorf("unexpected migrated row: status=%q notes=%q", migrated.Status, migrated.Notes)
	}
}

// TestClipInboxPlaceholderCleared reproduces the "Inbox cleared" ghost-row bug:
// the placeholder label must be removed when the first real clip is added, and
// it must never coexist with data rows.
func TestClipInboxPlaceholderCleared(t *testing.T) {
	oldBox := state.ClipInboxBox
	oldCount := clipInboxJobCount
	oldSel := clipSelectedRows
	oldApps := state.Applications
	defer func() {
		state.ClipInboxBox = oldBox
		clipInboxJobCount = oldCount
		clipSelectedRows = oldSel
		state.Applications = oldApps
	}()

	state.Applications = []JobApplication{} // no clip should count as already tracked
	clipSelectedRows = make(map[int]bool)
	state.ClipInboxBox = container.NewVBox()

	// Empty state: header + separator + placeholder label (3 objects, 0 jobs).
	showClipInboxPlaceholder("Inbox cleared.")
	if clipInboxJobCount != 0 {
		t.Fatalf("expected job count 0 after placeholder, got %d", clipInboxJobCount)
	}
	if got := len(state.ClipInboxBox.Objects); got != 3 {
		t.Fatalf("expected 3 objects in empty inbox, got %d", got)
	}
	if _, ok := state.ClipInboxBox.Objects[2].(*widget.Label); !ok {
		t.Fatalf("expected object[2] to be the placeholder *widget.Label, got %T", state.ClipInboxBox.Objects[2])
	}

	// Adding the first clip must strip the placeholder, not append after it.
	addClippedJobToInboxUI(ClippedJob{Company: "Acme", Role: "PM", Location: "NY", Link: "https://acme.example/1"})
	if clipInboxJobCount != 1 {
		t.Fatalf("expected job count 1 after first clip, got %d", clipInboxJobCount)
	}
	if got := len(state.ClipInboxBox.Objects); got != 3 {
		t.Fatalf("expected 3 objects (header, sep, row) after first clip, got %d", got)
	}
	for i, o := range state.ClipInboxBox.Objects {
		if _, isLabel := o.(*widget.Label); isLabel {
			t.Fatalf("placeholder label still present at index %d after adding a clip", i)
		}
	}
	if _, ok := state.ClipInboxBox.Objects[2].(*fyne.Container); !ok {
		t.Fatalf("expected object[2] to be the row *fyne.Container, got %T", state.ClipInboxBox.Objects[2])
	}

	// A second clip appends a row without reintroducing a placeholder.
	addClippedJobToInboxUI(ClippedJob{Company: "Globex", Role: "Eng", Location: "SF", Link: "https://globex.example/2"})
	if clipInboxJobCount != 2 {
		t.Fatalf("expected job count 2, got %d", clipInboxJobCount)
	}
	if got := len(state.ClipInboxBox.Objects); got != 4 {
		t.Fatalf("expected 4 objects after second clip, got %d", got)
	}
}

// TestLoadProfileReplacesPersonalInfo locks in the data-flow contract behind the
// "personal info doesn't change on a new résumé" fix: after a new profile JSON is
// written and loadProfileData() runs, state.Profile.PersonalInfo must fully
// reflect the new file (no stale fields carried over). The UI-thread half of the
// fix — running fillProfileForm() via fyne.Do — is verified manually since it
// depends on the Fyne main loop.
func TestLoadProfileReplacesPersonalInfo(t *testing.T) {
	profilePath := "references/user-profile.json"

	var backupBytes []byte
	backupExists := false
	if _, err := os.Stat(profilePath); err == nil {
		backupBytes, _ = os.ReadFile(profilePath)
		backupExists = true
	}
	_ = os.MkdirAll("references", 0755)

	oldProfile := state.Profile
	defer func() {
		state.Profile = oldProfile
		if backupExists {
			_ = os.WriteFile(profilePath, backupBytes, 0644)
		} else {
			_ = os.Remove(profilePath)
		}
	}()

	// Seed an "old" in-memory profile to prove it gets fully replaced.
	state.Profile = &Profile{PersonalInfo: PersonalInfo{
		Name: "Old Name", Email: "old@example.com", Phone: "111", Location: "Old City",
	}}

	newJSON := `{
	  "personal_info": {
	    "name": "New Name",
	    "email": "new@example.com",
	    "phone": "222-333-4444",
	    "location": "New City, CA",
	    "linkedin": "https://linkedin.com/in/new",
	    "website": "https://new.example"
	  },
	  "target_roles": ["PM"],
	  "skills": {"technical": ["Go"], "product_management": []}
	}`
	if err := os.WriteFile(profilePath, []byte(newJSON), 0644); err != nil {
		t.Fatalf("write new profile: %v", err)
	}

	loadProfileData()

	pi := state.Profile.PersonalInfo
	if pi.Name != "New Name" || pi.Email != "new@example.com" || pi.Phone != "222-333-4444" || pi.Location != "New City, CA" {
		t.Errorf("personal info not fully replaced after load: %+v", pi)
	}
	if pi.Linkedin != "https://linkedin.com/in/new" || pi.Website != "https://new.example" {
		t.Errorf("links not replaced after load: %+v", pi)
	}
}

// TestClearTrackerSelection covers the fix for the "allows selecting everything"
// bug: selection state must be fully reset so stale checkbox indices cannot
// survive a reload and map onto unrelated rows.
func TestClearTrackerSelection(t *testing.T) {
	oldSel := trackerSelectedRows
	oldIdx := state.SelectedAppIdx
	oldApps := state.Applications
	defer func() {
		trackerSelectedRows = oldSel
		state.SelectedAppIdx = oldIdx
		state.Applications = oldApps
	}()

	state.Applications = []JobApplication{{Company: "A"}, {Company: "B"}}
	trackerSelectedRows = map[int]bool{0: true, 1: true}
	state.SelectedAppIdx = 1

	clearTrackerSelection()

	if len(trackerSelectedRows) != 0 {
		t.Errorf("expected trackerSelectedRows cleared, got %d entries", len(trackerSelectedRows))
	}
	if state.SelectedAppIdx != -1 {
		t.Errorf("expected SelectedAppIdx reset to -1, got %d", state.SelectedAppIdx)
	}
	if got := len(getSelectedTrackerIndices()); got != 0 {
		t.Errorf("expected no selected indices after clear, got %d", got)
	}
}
