package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// resolveAppHome makes LeGaJ's data directory the process working directory so
// the app's relative paths (references/, extension/, outputs/) resolve to a
// writable location no matter how the program was launched.
//
// Why this exists: the GUI and helpers reference data with relative paths. That
// works when the binary is started from its own folder (Windows shortcut sets
// the working directory), but a macOS .app launched from Finder starts with the
// working directory at "/", and an app bundle is read-only under Gatekeeper — so
// those relative writes would fail. Pinning the working directory up front fixes
// every relative path at once.
//
// Resolution order:
//  1. $LEGAJ_HOME if set (explicit override).
//  2. Development mode: if a go.mod is present in the current directory we are
//     running from a source checkout, so the working directory is left unchanged
//     to preserve existing `go run` / `./legaj` behavior.
//  3. Packaged mode: an OS-appropriate per-user data directory.
//
// The frozen Python tools are located relative to the executable (see
// toolsBinaryPath), so changing the working directory does not affect them.
func resolveAppHome() {
	if dir, change := appHomeTarget(os.Getenv("LEGAJ_HOME"), fileExists("go.mod")); change {
		applyAppHome(dir)
	}
}

// appHomeTarget is the pure decision behind resolveAppHome, split out so it can
// be unit tested without changing the process working directory. It returns the
// directory to switch into and whether a switch should happen at all.
//
//   - A non-empty override (e.g. $LEGAJ_HOME) is always honored.
//   - When a source checkout is detected (go.mod present) it returns change=false
//     so development runs keep their current directory unchanged.
//   - Otherwise it returns the per-user data directory for packaged installs.
func appHomeTarget(override string, sourceCheckout bool) (dir string, change bool) {
	if override != "" {
		return override, true
	}
	if sourceCheckout {
		return "", false
	}
	return legajDataDir(), true
}

// fileExists reports whether the given path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// legajDataDir returns the per-user data directory for packaged installs.
func legajDataDir() string {
	const appName = "LeGaJ"

	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, appName)
		}
	case "darwin":
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, "Library", "Application Support", appName)
		}
	default: // linux and other unixes
		if d := os.Getenv("XDG_DATA_HOME"); d != "" {
			return filepath.Join(d, appName)
		}
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, ".local", "share", appName)
		}
	}

	// Fallbacks if the OS-specific lookup failed.
	if cfg, err := os.UserConfigDir(); err == nil {
		return filepath.Join(cfg, appName)
	}
	if self, err := os.Executable(); err == nil {
		return filepath.Dir(self)
	}
	return "."
}

// applyAppHome creates the directory if needed and switches into it. Failures are
// non-fatal: the app simply continues with its current working directory.
func applyAppHome(base string) {
	if base == "" {
		return
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return
	}
	_ = os.Chdir(base)
}
