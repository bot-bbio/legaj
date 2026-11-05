package main

import (
	"os"
	"strings"
	"testing"
)

// TestR41_PublicDocs keeps the public docs in place: the MIT license with
// the author's copyright, a product README with its four sections and the
// credit, the privacy notice and the install guide.
func TestR41_PublicDocs(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("%s is missing: %v", name, err)
		}
		return string(b)
	}
	want := map[string][]string{
		"LICENSE":    {"MIT License", "Copyright (c) 2026 Roberto Montero", "WITHOUT WARRANTY OF ANY KIND"},
		"README.md":  {"## Mission", "## Setup", "## Usage", "## Architecture", "Roberto Montero", "linkedin.com/in/alvvays", "PRIVACY.md", "INSTALL.md"},
		"PRIVACY.md": {"Gemini", "Free tier", "Paid tier", "Credential Manager", "GitHub", "update check", "Clipping a job can also reach Gemini", "your name, if the page shows it"},
		"INSTALL.md": {"More info", "Run anyway", "SHA-256", "LOCALAPPDATA", "(docs/img/smartscreen-1.png)", "(docs/img/smartscreen-2.png)"},
	}
	// The install guide's SmartScreen screenshots exist (R41).
	for _, img := range []string{"docs/img/smartscreen-1.png", "docs/img/smartscreen-2.png"} {
		read(img)
	}
	for name, phrases := range want {
		text := read(name)
		for _, p := range phrases {
			if !strings.Contains(text, p) {
				t.Errorf("%s doesn't contain %q", name, p)
			}
		}
	}
	// The README describes V2 only: no leftovers from the Python/Fyne app.
	readme := read("README.md")
	for _, stale := range []string{"Fyne", "Python", "reportlab", "install_mac.sh"} {
		if strings.Contains(readme, stale) {
			t.Errorf("README.md still mentions %q (V1)", stale)
		}
	}
}
