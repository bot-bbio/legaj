//go:build windows

package instance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/proc"
)

// Environment variables that turn this test binary into a "second launch" of
// the app for TestHelperProcessGate.
const (
	envGateName   = "LEGAJ_TEST_GATE_NAME"
	envGateMarker = "LEGAJ_TEST_GATE_MARKER"
)

func uniqueName(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return `Local\LeGaJ-test-` + hex.EncodeToString(b)
}

// runChild re-executes this test binary as a separate process that calls Gate
// with the given lock name. start() in the child writes the marker file,
// standing in for "open the DB, bind the port, start the queue".
func runChild(t *testing.T, name, marker string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := proc.Command(ctx, os.Args[0], "-test.run=^TestHelperProcessGate$")
	cmd.Env = append(os.Environ(), envGateName+"="+name, envGateMarker+"="+marker)
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		t.Fatalf("child exited with %d, want 0; output:\n%s", exitErr.ExitCode(), out)
	}
	if err != nil {
		t.Fatalf("child failed to run: %v\n%s", err, out)
	}
}

// TestHelperProcessGate is not a real test: it is the body of the child
// process launched by the R42 tests, and skips when run normally.
func TestHelperProcessGate(t *testing.T) {
	name, marker := os.Getenv(envGateName), os.Getenv(envGateMarker)
	if name == "" || marker == "" {
		t.Skip("helper process only")
	}
	first, err := Gate(name, func() {}, func() error {
		return os.WriteFile(marker, []byte("started"), 0o600)
	})
	if err != nil {
		t.Fatalf("Gate: %v", err)
	}
	t.Logf("first=%v", first)
}

func TestR42_SecondInstanceExits(t *testing.T) {
	name := uniqueName(t)
	lock, acquired, err := Acquire(name)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("parent could not take a fresh lock")
	}
	defer func() { _ = lock.Release() }()

	marker := filepath.Join(t.TempDir(), "started")
	runChild(t, name, marker)

	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("second instance ran start() (DB/port/queue would be touched); stat err = %v", err)
	}
}

func TestR42_FirstInstanceStarts(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "started")
	runChild(t, uniqueName(t), marker)

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("first instance did not run start(): %v", err)
	}
}

func TestR42_SecondInstanceActivatesFirst(t *testing.T) {
	name := uniqueName(t)
	lock, _, err := Acquire(name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Release() }()

	activated, started := false, false
	first, err := Gate(name, func() { activated = true }, func() error { started = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if first || started || !activated {
		t.Fatalf("first=%v started=%v activated=%v; want false,false,true", first, started, activated)
	}
}

func TestR42_ReleaseFreesLock(t *testing.T) {
	name := uniqueName(t)
	lock, acquired, err := Acquire(name)
	if err != nil || !acquired {
		t.Fatalf("acquire: %v %v", acquired, err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	again, acquired, err := Acquire(name)
	if err != nil || !acquired {
		t.Fatalf("re-acquire after release: %v %v", acquired, err)
	}
	_ = again.Release()
}

func TestR42_WailsWindowNames(t *testing.T) {
	class, window := wailsEventWindow("abc")
	if class != "wails-app-abc-sic" || window != "wails-app-abc-siw" {
		t.Fatalf("got %q %q; must match Wails v2 single_instance.go", class, window)
	}
}
