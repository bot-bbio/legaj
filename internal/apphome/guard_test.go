package apphome_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/apphome"
	"github.com/bot-bbio/legaj/internal/proc"
)

const envHelper = "LEGAJ_TEST_GUARD_HELPER"

// TestHelperTouchesHome is the body of the child process in the R36 tests.
// It writes into whatever data dir the app would use. If the guard works,
// TestMain aborts the child before this ever runs.
func TestHelperTouchesHome(t *testing.T) {
	if os.Getenv(envHelper) != "1" {
		t.Skip("helper process only")
	}
	dir, err := apphome.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := apphome.Ensure(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "legaj.db"), []byte("test wrote here"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// childEnv is the current environment minus LEGAJ_HOME and LOCALAPPDATA,
// plus the given overrides. LOCALAPPDATA always points at a temp folder, so
// even a broken guard can never reach the user's real data.
func childEnv(overrides ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		key := strings.ToUpper(strings.SplitN(kv, "=", 2)[0])
		if key == apphome.EnvVar || key == "LOCALAPPDATA" {
			continue
		}
		env = append(env, kv)
	}
	return append(env, overrides...)
}

func runHelper(t *testing.T, env []string) (exitCode int, output string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := proc.Command(ctx, os.Args[0], "-test.run=^TestHelperTouchesHome$", "-test.v")
	cmd.Env = append(env, envHelper+"=1")
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		return exitErr.ExitCode(), string(out)
	case err != nil:
		t.Fatalf("could not run helper: %v", err)
	}
	return 0, string(out)
}

func TestR36_GuardAbortsOnRealHome(t *testing.T) {
	fakeLocal := t.TempDir()
	realHome := filepath.Join(fakeLocal, "LeGaJ")

	// No LEGAJ_HOME: the data dir resolves to %LOCALAPPDATA%\LeGaJ.
	code, out := runHelper(t, childEnv("LOCALAPPDATA="+fakeLocal))

	if code == 0 {
		t.Fatalf("child exited 0; the guard let a test run against the real home\n%s", out)
	}
	if strings.Contains(out, "TestHelperTouchesHome") {
		t.Fatalf("a test ran before the guard aborted\n%s", out)
	}
	if _, err := os.Stat(realHome); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the real home was touched before the abort (stat err = %v)", err)
	}
}

func TestR36_GuardAbortsWhenHomeIsInsideRealHome(t *testing.T) {
	fakeLocal := t.TempDir()
	nested := filepath.Join(fakeLocal, "LeGaJ", "sub")

	code, out := runHelper(t, childEnv("LOCALAPPDATA="+fakeLocal, apphome.EnvVar+"="+nested))

	if code == 0 {
		t.Fatalf("child exited 0 with LEGAJ_HOME inside the real home\n%s", out)
	}
	if _, err := os.Stat(nested); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nested home was created before the abort (stat err = %v)", err)
	}
}

func TestR36_GuardAllowsIsolatedHome(t *testing.T) {
	fakeLocal := t.TempDir()
	isolated := t.TempDir()

	code, out := runHelper(t, childEnv("LOCALAPPDATA="+fakeLocal, apphome.EnvVar+"="+isolated))

	if code != 0 {
		t.Fatalf("child exited %d with an isolated home\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(isolated, "legaj.db")); err != nil {
		t.Fatalf("helper did not run against the isolated home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fakeLocal, "LeGaJ")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the real home was created even though LEGAJ_HOME was isolated")
	}
}
