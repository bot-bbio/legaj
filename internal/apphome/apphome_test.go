package apphome_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bot-bbio/legaj/internal/apphome"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

func TestR2_HomeFromEnv(t *testing.T) {
	want := t.TempDir()
	t.Setenv(apphome.EnvVar, want)
	got, err := apphome.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}
}

func TestR2_HomeFromEnvIsMadeAbsolute(t *testing.T) {
	t.Setenv(apphome.EnvVar, "relative-home")
	got, err := apphome.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("Dir() = %q, want an absolute path", got)
	}
}

func TestR2_DefaultHomeIsLocalAppData(t *testing.T) {
	local := t.TempDir()
	t.Setenv(apphome.EnvVar, "")
	t.Setenv("LOCALAPPDATA", local)
	got, err := apphome.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(local, "LeGaJ"); got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}
}

func TestR2_EnsureCreatesDir(t *testing.T) {
	dir := filepath.Join(testenv.Isolate(t), "nested", "home")
	if err := apphome.Ensure(dir); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("Ensure did not create %q: %v", dir, err)
	}
}
