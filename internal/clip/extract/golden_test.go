package extract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/bot-bbio/legaj/internal/clip/extract"
)

// requiredGolden are the fixtures that must exist (R19). See
// testdata/golden/README.md for where each came from.
var requiredGolden = []string{
	"ashby", "greenhouse", "lever", "workday",
	"indeed-search", "indeed-viewjob",
	"linkedin-guest-en", "linkedin-guest-pl", "linkedin-member", "linkedin-member-search",
}

func TestR19_Golden(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("testdata", "golden", "*", "input.json"))
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	for _, in := range dirs {
		dir := filepath.Dir(in)
		name := filepath.Base(dir)
		seen = append(seen, name)
		t.Run(name, func(t *testing.T) {
			var page extract.Page
			readJSON(t, in, &page)
			var want extract.Result
			readJSON(t, filepath.Join(dir, "expected.json"), &want)
			got := extract.Extract(page)
			if got.Company != want.Company {
				t.Errorf("company %q, want %q", got.Company, want.Company)
			}
			if got.Role != want.Role {
				t.Errorf("role %q, want %q", got.Role, want.Role)
			}
			if got.Location != want.Location {
				t.Errorf("location %q, want %q", got.Location, want.Location)
			}
			if got.Description != want.Description {
				t.Errorf("description:\n%q\nwant\n%q", got.Description, want.Description)
			}
			if got.From != want.From {
				t.Errorf("sources %+v, want %+v", got.From, want.From)
			}
		})
	}
	for _, name := range requiredGolden {
		if !slices.Contains(seen, name) {
			t.Errorf("golden fixture %q is missing", name)
		}
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}
