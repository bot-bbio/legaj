// Package testenv keeps tests away from the user's real data (R36, V1 bug A1:
// the V1 test suite overwrote the real job tracker).
//
// Every test package must declare
//
//	func TestMain(m *testing.M) { testenv.Main(m) }
//
// (an architecture test enforces this). Main refuses to run any test unless
// LEGAJ_HOME points somewhere other than the real %LOCALAPPDATA%\LeGaJ.
// scripts/verify.ps1 sets LEGAJ_HOME to a scratch folder; to run `go test`
// by hand, set it yourself first.
package testenv

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/bot-bbio/legaj/internal/apphome"
)

// ExitCode is the process exit status when the guard refuses to run.
const ExitCode = 3

// Main runs the package's tests only if Guard passes. The check does no
// file I/O beyond reading the environment and resolving links.
func Main(m *testing.M) {
	if err := Guard(); err != nil {
		fmt.Fprintln(os.Stderr, "legaj test guard: refusing to run tests:", err)
		os.Exit(ExitCode)
	}
	os.Exit(m.Run())
}

// Guard returns an error when the data folder tests would use is the real
// one, or lies inside it. It fails closed: an unset LEGAJ_HOME is an error.
func Guard() error {
	if strings.TrimSpace(os.Getenv(apphome.EnvVar)) == "" {
		return errors.New(apphome.EnvVar + " is not set, so tests would use the real data folder; point it at a scratch folder (scripts/verify.ps1 does this)")
	}
	home, err := apphome.Dir()
	if err != nil {
		return err
	}
	realHome, err := apphome.DefaultDir()
	if err != nil {
		return err
	}
	if apphome.Within(home, realHome) {
		return fmt.Errorf("%s resolves to the real data folder or a folder inside it", apphome.EnvVar)
	}
	return nil
}

// Isolate points LEGAJ_HOME at a fresh temp folder for the duration of t and
// returns it. Tests that touch data call this first.
func Isolate(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(apphome.EnvVar, dir)
	return dir
}
