package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestAppHomeTargetOverride verifies an explicit override always wins, even when
// a source checkout is detected.
func TestAppHomeTargetOverride(t *testing.T) {
	dir, change := appHomeTarget("/custom/legaj/home", true)
	if !change {
		t.Fatal("override should force a working-directory change")
	}
	if dir != "/custom/legaj/home" {
		t.Errorf("override dir: got %q, want %q", dir, "/custom/legaj/home")
	}
}

// TestAppHomeTargetSourceCheckout verifies that in development (go.mod present,
// no override) the working directory is left unchanged — protecting the original
// build's behavior.
func TestAppHomeTargetSourceCheckout(t *testing.T) {
	_, change := appHomeTarget("", true)
	if change {
		t.Error("source checkout should keep the current working directory (change=false)")
	}
}

// TestAppHomeTargetPackaged verifies that a packaged build (no go.mod, no
// override) switches into the per-user data directory.
func TestAppHomeTargetPackaged(t *testing.T) {
	dir, change := appHomeTarget("", false)
	if !change {
		t.Fatal("packaged build should switch to the data directory")
	}
	if dir == "" {
		t.Fatal("packaged data directory must not be empty")
	}
	if !strings.Contains(dir, "LeGaJ") {
		t.Errorf("packaged data directory %q should be namespaced under LeGaJ", dir)
	}
}

// TestLegajDataDirIsAbsolute verifies the resolved data directory is absolute so
// it does not depend on the process working directory.
func TestLegajDataDirIsAbsolute(t *testing.T) {
	dir := legajDataDir()
	if dir == "." {
		t.Skip("no home/config/executable dir resolvable in this environment")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("legajDataDir() = %q, expected an absolute path on %s", dir, runtime.GOOS)
	}
}
