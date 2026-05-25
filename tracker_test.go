package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestTrackerOperationsGo(t *testing.T) {
	// Backup references/job-tracker.json if it exists
	var backupBytes []byte
	backupExists := false
	trackerPath := "references/job-tracker.json"
	if _, err := os.Stat(trackerPath); err == nil {
		backupBytes, _ = os.ReadFile(trackerPath)
		backupExists = true
	}

	// Make sure directory exists
	_ = os.MkdirAll("references", 0755)

	defer func() {
		if backupExists {
			_ = os.WriteFile(trackerPath, backupBytes, 0644)
		} else {
			_ = os.Remove(trackerPath)
		}
	}()

	// Save original applications and restore at the end
	oldApps := state.Applications
	defer func() {
		state.Applications = oldApps
	}()

	// Reset state
	state.Applications = []JobApplication{}

	// Test 1: Add Application
	err := addApplicationGo("Test Company Inc", "Senior Engineer", "Boston, MA", "https://example.com/job", "Wishlist", "resume1.pdf", "cover1.pdf", "Initial notes")
	if err != nil {
		t.Fatalf("addApplicationGo failed: %v", err)
	}

	if len(state.Applications) != 1 {
		t.Fatalf("Expected 1 application in memory, got %d", len(state.Applications))
	}

	app := state.Applications[0]
	if app.Company != "Test Company Inc" || app.Role != "Senior Engineer" || app.Status != "Wishlist" || app.Notes != "Initial notes" {
		t.Errorf("Unexpected application details: %+v", app)
	}

	// Check if file was written
	fileData, err := os.ReadFile(trackerPath)
	if err != nil {
		t.Fatalf("Failed to read job-tracker.json: %v", err)
	}

	var fileApps []JobApplication
	err = json.Unmarshal(fileData, &fileApps)
	if err != nil {
		t.Fatalf("Failed to unmarshal file data: %v", err)
	}

	if len(fileApps) != 1 || fileApps[0].Company != "Test Company Inc" {
		t.Errorf("File contents mismatch: %+v", fileApps)
	}

	// Test 2: Update Application
	err = updateApplicationGo("Test Company Inc", "Senior Engineer", "Interviewing", "Called back")
	if err != nil {
		t.Fatalf("updateApplicationGo failed: %v", err)
	}

	if state.Applications[0].Status != "Interviewing" {
		t.Errorf("Expected status 'Interviewing', got %q", state.Applications[0].Status)
	}
	if !strings.Contains(state.Applications[0].Notes, "Called back") {
		t.Errorf("Expected notes to contain 'Called back', got %q", state.Applications[0].Notes)
	}

	// Test 3: Load Applications
	state.Applications = []JobApplication{} // Clear in-memory
	loadTrackerData()
	if len(state.Applications) != 1 || state.Applications[0].Company != "Test Company Inc" {
		t.Errorf("loadTrackerData failed to reload correctly: %+v", state.Applications)
	}

	// Test 4: Delete Application
	err = deleteApplicationGo("Test Company Inc", "Senior Engineer")
	if err != nil {
		t.Fatalf("deleteApplicationGo failed: %v", err)
	}

	if len(state.Applications) != 0 {
		t.Errorf("Expected 0 applications in memory after delete, got %d", len(state.Applications))
	}

	// Verify file is empty list
	fileData, err = os.ReadFile(trackerPath)
	if err != nil {
		t.Fatalf("Failed to read file after delete: %v", err)
	}
	err = json.Unmarshal(fileData, &fileApps)
	if err != nil {
		t.Fatalf("Failed to unmarshal empty list: %v", err)
	}
	if len(fileApps) != 0 {
		t.Errorf("Expected 0 applications in file, got %d", len(fileApps))
	}
}
