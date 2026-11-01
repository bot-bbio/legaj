package testenv_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bot-bbio/legaj/internal/apphome"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

func TestR36_GuardRejectsUnsetHome(t *testing.T) {
	t.Setenv(apphome.EnvVar, "")
	if testenv.Guard() == nil {
		t.Fatal("Guard passed with LEGAJ_HOME unset")
	}
}

func TestR36_GuardRejectsRealHomeAnyCase(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv(apphome.EnvVar, filepath.Join(local, "legaj")) // lower-case name
	if testenv.Guard() == nil {
		t.Fatal("Guard passed with LEGAJ_HOME equal to the real home (case differs)")
	}
}

func TestR36_GuardAcceptsSiblingFolder(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv(apphome.EnvVar, filepath.Join(local, "LeGaJ-test")) // shares a prefix, not inside
	if err := testenv.Guard(); err != nil {
		t.Fatalf("Guard rejected a sibling folder: %v", err)
	}
}

func TestR36_IsolateSetsTempHome(t *testing.T) {
	dir := testenv.Isolate(t)
	if got := os.Getenv(apphome.EnvVar); got != dir {
		t.Fatalf("LEGAJ_HOME = %q, want %q", got, dir)
	}
	if err := testenv.Guard(); err != nil {
		t.Fatalf("Guard rejected an isolated home: %v", err)
	}
}
