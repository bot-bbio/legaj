package core_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/ai/aitest"
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

func (noAutostart) Enable(string) error            { return nil }
func (noAutostart) Disable() error                 { return nil }
func (noAutostart) Current() (string, bool, error) { return "", false, nil }

// noLaunch never opens anything: tests must not pop up a browser.
type noLaunch struct{}

func (*noLaunch) Launch(string) error { return nil }

// opts never reaches real user state: memory secrets, no registry.
func opts(dir string, now time.Time) core.Options {
	return core.Options{
		Dir: dir, Now: func() time.Time { return now }, Emitter: &events.Recorder{},
		Secrets: secrets.NewMemory(), Autostart: noAutostart{}, AI: &aitest.Fake{}, KeyTester: func(context.Context, string) error { return nil }, Launcher: &noLaunch{},
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

func TestR2_ConcurrentRestoresAreSerialized(t *testing.T) {
	dir := testenv.Isolate(t)
	day1 := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	c, _, err := core.Boot(ctx, opts(dir, day1))
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Close()
	if err := os.WriteFile(filepath.Join(dir, store.FileName), []byte(strings.Repeat("garbage", 1000)), 0o600); err != nil {
		t.Fatal(err)
	}
	c, st, err := core.Boot(ctx, opts(dir, day1.Add(time.Hour)))
	if err != nil || st.Kind != store.StartupCorrupt {
		t.Fatalf("boot: %+v %v", st, err)
	}
	defer c.Close()

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = c.Restore(ctx, st.Backups[0].Name)
		}()
	}
	wg.Wait()
	ok := 0
	for _, e := range errs {
		if e == nil {
			ok++
		}
	}
	if ok != 1 || c.DB() == nil {
		t.Fatalf("%d restores succeeded (errs %v), want exactly 1 and an open DB", ok, errs)
	}
}

// movingClock is a clock tests can advance while the backend runs.
type movingClock struct {
	mu sync.Mutex
	t  time.Time
}

func (m *movingClock) now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.t
}

func (m *movingClock) advance(d time.Duration) {
	m.mu.Lock()
	m.t = m.t.Add(d)
	m.mu.Unlock()
}

func TestR2_DailyBackupWhileRunning(t *testing.T) {
	dir := testenv.Isolate(t)
	clk := &movingClock{t: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)}
	o := opts(dir, clk.t)
	o.Now = clk.now
	o.BackupEvery = 5 * time.Millisecond
	c, _, err := core.Boot(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	clk.advance(25 * time.Hour) // LeGaJ has been left running overnight
	deadline := time.Now().Add(5 * time.Second)
	for {
		backups, err := store.ListBackups(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(backups) == 2 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d backups after a day of uptime, want 2 (one per day)", len(backups))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// recordingAutostart remembers the last registry action.
type recordingAutostart struct {
	mu      sync.Mutex
	enabled bool
	calls   int
}

func (r *recordingAutostart) Enable(string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled, r.calls = true, r.calls+1
	return nil
}

func (r *recordingAutostart) Disable() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled, r.calls = false, r.calls+1
	return nil
}

func (r *recordingAutostart) Current() (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return "", r.enabled, nil
}

func TestR37_BootReconcilesStartWithWindows(t *testing.T) {
	dir := testenv.Isolate(t)
	o := opts(dir, time.Now())
	c, _, err := core.Boot(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	svc, _ := c.Ready()
	s, _ := svc.Settings.Get(ctx)
	s.StartWithWindows = true
	if _, err := svc.Settings.Save(ctx, s); err != nil {
		t.Fatal(err)
	}
	_ = c.Close()

	// The Run entry went missing (or a save half-failed): the next start fixes it.
	reg := &recordingAutostart{}
	o.Autostart = reg
	c, _, err = core.Boot(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if !reg.enabled {
		t.Fatal("boot didn't bring the Run entry in line with the saved setting")
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

func TestR18_CoreRunsTheClipServer(t *testing.T) {
	o := opts(testenv.Isolate(t), time.Now())
	o.StartClipServer = true
	c, st, err := core.Boot(ctx, o)
	if err != nil || st.Kind != store.StartupReady {
		t.Fatalf("boot: %v %+v", err, st)
	}
	svc, err := c.Ready()
	if err != nil {
		t.Fatal(err)
	}
	port := svc.ClipPort.Port
	if svc.ClipServer == nil || port == 0 || svc.ClipProblem != "" {
		t.Fatalf("clip server not running: %+v %q", svc.ClipPort, svc.ClipProblem)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/clip", port)
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("popup: %d", res.StatusCode)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	// Closed: the port is free again.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("port %d still held after Close: %v", port, err)
	}
	ln.Close()
}

func TestR18_TestsStartNoClipServer(t *testing.T) {
	c, _, err := core.Boot(ctx, opts(testenv.Isolate(t), time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	svc, err := c.Ready()
	if err != nil {
		t.Fatal(err)
	}
	if svc.ClipServer != nil {
		t.Fatal("a clip server started without StartClipServer")
	}
}
