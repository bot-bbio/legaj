// Package archtest holds repository-wide architecture tests: rules that apply
// to the whole source tree rather than to one package's behaviour.
package archtest

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

// skipDirs are never scanned: VCS data, local-only harness docs, dependencies,
// and V1 runtime/build output that is gitignored and not part of the source tree.
var skipDirs = map[string]bool{
	".git":          true,
	".agent":        true,
	".claude":       true,
	".workflow":     true,
	".pytest_cache": true,
	"node_modules":  true,
	"scratch":       true,
	"outputs":       true,
	"references":    true,
	"dist":          true,
	"wailsjs":       true, // generated bindings, regenerated from Go on every build
}

// repoRoot walks up from the test's working directory to the go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above test directory")
		}
		dir = parent
	}
}

// walkFiles calls visit for every regular file under root whose path is not
// inside a skipped directory. Paths passed to visit are slash-separated and
// relative to root.
func walkFiles(t *testing.T, root string, visit func(rel, abs string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		visit(filepath.ToSlash(rel), path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// R6: no process spawning outside internal/proc
// ---------------------------------------------------------------------------

// spawnAPIs lists, per import path, the identifiers that create a process.
// exec.Cmd is included so a hand-built &exec.Cmd{} cannot dodge the rule.
var spawnAPIs = map[string]map[string]bool{
	"os/exec":                  {"Command": true, "CommandContext": true, "Cmd": true},
	"os":                       {"StartProcess": true},
	"syscall":                  {"StartProcess": true, "ForkExec": true, "CreateProcess": true, "CreateProcessAsUser": true},
	"golang.org/x/sys/windows": {"CreateProcess": true, "CreateProcessAsUser": true},
}

// findSpawnCalls returns "pkg.Name" for every process-spawning API referenced
// in the given Go source.
func findSpawnCalls(filename string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	// Map each local import name to its path, honouring aliases.
	local := map[string]string{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, err
		}
		if _, watched := spawnAPIs[path]; !watched {
			continue
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "." {
			return nil, errors.New("dot-import of " + path + " hides process-spawning calls; not allowed")
		}
		local[name] = path
	}
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		path, ok := local[ident.Name]
		if ok && spawnAPIs[path][sel.Sel.Name] {
			found = append(found, fset.Position(sel.Pos()).String()+" "+ident.Name+"."+sel.Sel.Name)
		}
		return true
	})
	return found, nil
}

func TestR6_DetectorCatchesEachSpawnForm(t *testing.T) {
	cases := map[string]string{
		"exec.Command":        `package p; import "os/exec"; var _ = exec.Command("x")`,
		"exec.CommandContext": `package p; import "os/exec"; var _ = exec.CommandContext(nil, "x")`,
		"exec.Cmd literal":    `package p; import "os/exec"; var _ = &exec.Cmd{Path: "x"}`,
		"aliased exec":        `package p; import run "os/exec"; var _ = run.Command("x")`,
		"os.StartProcess":     `package p; import "os"; var _, _ = os.StartProcess("x", nil, nil)`,
		"syscall.StartProcess": `package p; import "syscall"
var _, _, _ = syscall.StartProcess("x", nil, nil)`,
		"windows.CreateProcess": `package p; import "golang.org/x/sys/windows"; var _ = windows.CreateProcess`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			found, err := findSpawnCalls("case.go", []byte(src))
			if err != nil {
				t.Fatal(err)
			}
			if len(found) == 0 {
				t.Fatalf("detector missed %s", name)
			}
		})
	}
	// A local variable that merely shares a name with a package is not flagged
	// unless that package is imported.
	clean := `package p; type T struct{}; func (T) Command() {}; var exec T; func f() { exec.Command() }`
	found, err := findSpawnCalls("clean.go", []byte(clean))
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("false positive: %v", found)
	}
}

func TestR6_NoProcessSpawnOutsideProc(t *testing.T) {
	root := repoRoot(t)
	var violations []string
	walkFiles(t, root, func(rel, abs string) {
		if !strings.HasSuffix(rel, ".go") || strings.HasPrefix(rel, "internal/proc/") {
			return
		}
		src, err := os.ReadFile(abs)
		if err != nil {
			t.Fatal(err)
		}
		found, err := findSpawnCalls(abs, src)
		if err != nil {
			violations = append(violations, rel+": "+err.Error())
			return
		}
		violations = append(violations, found...)
	})
	if len(violations) > 0 {
		t.Fatalf("process-spawning calls outside internal/proc (use proc.Command):\n%s", strings.Join(violations, "\n"))
	}
}

// ---------------------------------------------------------------------------
// R36: every test binary runs behind the real-data guard
// ---------------------------------------------------------------------------

const testenvPath = "github.com/bot-bbio/legaj/internal/testenv"

// hasGuardedTestMain reports whether the Go source declares
// func TestMain(m *testing.M) and calls testenv.Main inside it.
func hasGuardedTestMain(filename string, src []byte) (bool, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return false, err
	}
	local := ""
	for _, imp := range file.Imports {
		if path, _ := strconv.Unquote(imp.Path.Value); path == testenvPath {
			local = "testenv"
			if imp.Name != nil {
				local = imp.Name.Name
			}
		}
	}
	if local == "" {
		return false, nil
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != "TestMain" || fn.Body == nil {
			continue
		}
		found := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == local && sel.Sel.Name == "Main" {
					found = true
				}
			}
			return !found
		})
		return found, nil
	}
	return false, nil
}

func TestR36_GuardDetector(t *testing.T) {
	good := `package p
import ("testing"; "github.com/bot-bbio/legaj/internal/testenv")
func TestMain(m *testing.M) { testenv.Main(m) }`
	bad := `package p
import "testing"
func TestMain(m *testing.M) { m.Run() }`
	if ok, err := hasGuardedTestMain("good_test.go", []byte(good)); err != nil || !ok {
		t.Fatalf("guarded TestMain not recognised: %v %v", ok, err)
	}
	if ok, err := hasGuardedTestMain("bad_test.go", []byte(bad)); err != nil || ok {
		t.Fatalf("unguarded TestMain accepted: %v %v", ok, err)
	}
}

func TestR36_EveryTestPackageUsesGuard(t *testing.T) {
	root := repoRoot(t)
	guarded := map[string]bool{} // dir -> has guarded TestMain
	walkFiles(t, root, func(rel, abs string) {
		if !strings.HasSuffix(rel, "_test.go") {
			return
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		if _, seen := guarded[dir]; !seen {
			guarded[dir] = false
		}
		src, err := os.ReadFile(abs)
		if err != nil {
			t.Fatal(err)
		}
		ok, err := hasGuardedTestMain(abs, src)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			guarded[dir] = true
		}
	})
	var missing []string
	for dir, ok := range guarded {
		if !ok {
			missing = append(missing, dir)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("A1: these test packages lack `func TestMain(m *testing.M) { testenv.Main(m) }`:\n%s", strings.Join(missing, "\n"))
	}
}

// ---------------------------------------------------------------------------
// R10: no tailoring surfaces
// ---------------------------------------------------------------------------

// frontendSourceExt are the file types under frontend/src that ship to users.
var frontendSourceExt = map[string]bool{".ts": true, ".svelte": true, ".js": true, ".html": true, ".css": true, ".json": true}

// promptTemplateExt are the file types that hold AI prompt text in Go packages.
var promptTemplateExt = map[string]bool{".tmpl": true, ".txt": true, ".md": true, ".prompt": true}

// isTailorScanTarget reports whether rel is in the R10 search scope: Go
// sources, frontend sources, and prompt templates under internal/ —
// excluding tests, which must be able to name the forbidden word.
func isTailorScanTarget(rel string) bool {
	base := filepath.Base(rel)
	ext := filepath.Ext(rel)
	switch {
	case strings.HasSuffix(base, "_test.go"),
		strings.HasSuffix(base, ".test.ts"),
		strings.HasSuffix(base, ".spec.ts"):
		return false
	case ext == ".go":
		return true
	case strings.HasPrefix(rel, "frontend/src/"):
		return frontendSourceExt[ext]
	case strings.HasPrefix(rel, "internal/"):
		return promptTemplateExt[ext]
	}
	return false
}

func TestR10_ScopeIncludesSourcesExcludesTests(t *testing.T) {
	in := []string{"main.go", "internal/letters/prompt.go", "frontend/src/App.svelte", "frontend/src/lib/nav.ts", "internal/letters/prompts/letter.tmpl"}
	out := []string{"internal/letters/prompt_test.go", "frontend/src/lib/nav.test.ts", "README.md", ".agent/PRD.md", "frontend/package.json"}
	for _, p := range in {
		if !isTailorScanTarget(p) {
			t.Errorf("%s should be scanned", p)
		}
	}
	for _, p := range out {
		if isTailorScanTarget(p) {
			t.Errorf("%s should not be scanned", p)
		}
	}
}

func TestR10_NoTailorReferences(t *testing.T) {
	root := repoRoot(t)
	needle := []byte("tailor")
	var hits []string
	walkFiles(t, root, func(rel, abs string) {
		if !isTailorScanTarget(rel) {
			return
		}
		src, err := os.ReadFile(abs)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range bytes.Split(src, []byte("\n")) {
			if bytes.Contains(bytes.ToLower(line), needle) {
				hits = append(hits, rel+":"+strconv.Itoa(i+1))
			}
		}
	})
	if len(hits) > 0 {
		t.Fatalf("D12: tailoring is out of scope forever, but %d line(s) mention it:\n%s", len(hits), strings.Join(hits, "\n"))
	}
}
