package core_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/core"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/logx"
	"github.com/bot-bbio/legaj/internal/secrets"
	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

var ctx = context.Background()

// noAutostart stands in for the registry; tests never touch HKCU.
type noAutostart struct{}

func (noAutostart) Enable(string) error { return nil }
func (noAutostart) Disable() error      { return nil }

// opts never reaches real user state: memory secrets, no registry.
func opts(dir string, now time.Time) core.Options {
	return core.Options{
		Dir: dir, Now: func() time.Time { return now }, Emitter: &events.Recorder{},
		Secrets: secrets.NewMemory(), Autostart: noAutostart{},
		Exe: func() (string, error) { return `C:\LeGaJ\legaj.exe`, nil },
	}
}

func TestR4_SavedKeyRegisteredWithLogRedactorAtBoot(t *testing.T) {
	const key = "AIzaSyD-SENTINEL-KEY-0123456789abcdWXYZ"
	o := opts(testenv.Isolate(t), time.Now())
	if err := o.Secrets.Set(key); err != nil {
		t.Fatal(err)
	}
	c, _, err := core.Boot(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Log.Info("leak " + key)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(c.Dir, logx.LogDir, "legaj.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), key) {
		t.Fatal("the saved key reached the log file")
	}
}

func TestR37_SettingsServiceAvailableWhenReady(t *testing.T) {
	c, _, err := core.Boot(ctx, opts(testenv.Isolate(t), time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	svc, err := c.Ready()
	if err != nil || svc.Settings == nil {
		t.Fatalf("Ready = %v, %v", svc, err)
	}
}

func TestR2_BootFreshFolderIsReadyAndBacksUp(t *testing.T) {
	dir := testenv.Isolate(t)
	c, st, err := core.Boot(ctx, opts(dir, time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if st.Kind != store.StartupReady || c.DB() == nil {
		t.Fatalf("startup = %+v, db = %v", st, c.DB())
	}
	if _, err := os.Stat(filepath.Join(dir, logx.LogDir)); err != nil {
		t.Fatalf("log folder not created: %v", err)
	}
	backups, err := store.ListBackups(ctx, dir)
	if err != nil || len(backups) != 1 {
		t.Fatalf("daily backup on boot: %+v, %v", backups, err)
	}
}

func TestR2_BootCorruptOffersRestoreThenRecovers(t *testing.T) {
	dir := testenv.Isolate(t)
	day1 := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	c, _, err := core.Boot(ctx, opts(dir, day1))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, store.FileName), []byte(strings.Repeat("garbage", 1000)), 0o600); err != nil {
		t.Fatal(err)
	}

	c, st, err := core.Boot(ctx, opts(dir, day1.Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if st.Kind != store.StartupCorrupt || c.DB() != nil || len(st.Backups) != 1 {
		t.Fatalf("startup = %+v, db = %v", st, c.DB())
	}

	st, err = c.Restore(ctx, st.Backups[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	if st.Kind != store.StartupReady || c.DB() == nil {
		t.Fatalf("after restore: %+v", st)
	}
}

func TestR2_RestoreRefusedWhenReady(t *testing.T) {
	dir := testenv.Isolate(t)
	c, _, err := core.Boot(ctx, opts(dir, time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Restore(ctx, "legaj-20261101T090000Z.db"); err == nil {
		t.Fatal("restore over a healthy open database was allowed")
	}
}
