package clip_test

import (
	"fmt"
	"net"
	"strconv"
	"testing"

	"github.com/bot-bbio/legaj/internal/clip"
	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func openDB(t *testing.T) *store.DB {
	t.Helper()
	db, st, err := store.Open(ctx, store.Options{Dir: testenv.Isolate(t)})
	if err != nil || st.Kind != store.StartupReady {
		t.Fatalf("open: %v %+v", err, st)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func storedPort(t *testing.T, db *store.DB) int {
	t.Helper()
	v, ok, err := db.GetMeta(ctx, clip.PortKey)
	if err != nil || !ok {
		t.Fatalf("no stored port: %v %v", ok, err)
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestR18_BindsLoopbackOnly(t *testing.T) {
	db := openDB(t)
	ln, st, err := clip.Listen(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	addr := ln.Addr().(*net.TCPAddr)
	if !addr.IP.IsLoopback() || addr.IP.String() != "127.0.0.1" {
		t.Fatalf("listening on %s, want 127.0.0.1 only", addr)
	}
	if addr.Port != st.Port || st.Port < clip.PortLow || st.Port > clip.PortHigh {
		t.Fatalf("port %d, status %+v", addr.Port, st)
	}
}

func TestR18_PortPersistedAndReused(t *testing.T) {
	db := openDB(t)
	ln, first, err := clip.Listen(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if storedPort(t, db) != first.Port {
		t.Fatalf("stored %d, listening on %d", storedPort(t, db), first.Port)
	}
	ln.Close()
	ln, again, err := clip.Listen(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if again.Port != first.Port || again.Previous != 0 {
		t.Fatalf("second start %+v, want port %d unchanged", again, first.Port)
	}
}

func TestR18_PortTakenRaisesBanner(t *testing.T) {
	db := openDB(t)
	ln, first, err := clip.Listen(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()
	// The user installed a bookmarklet for that port; then another program
	// takes it before LeGaJ starts again.
	if err := db.SetMeta(ctx, clip.BookmarkletPortKey, strconv.Itoa(first.Port)); err != nil {
		t.Fatal(err)
	}
	blocker, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", first.Port))
	if err != nil {
		t.Skipf("couldn't take port %d for the test: %v", first.Port, err)
	}
	defer blocker.Close()

	ln, st, err := clip.Listen(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if st.Port == first.Port || st.Previous != first.Port || storedPort(t, db) != st.Port {
		t.Fatalf("status %+v, stored %d, old port %d", st, storedPort(t, db), first.Port)
	}
	if !st.NeedsReinstall() {
		t.Fatalf("status %+v: the installed bookmarklet points at the old port", st)
	}
}

func TestR18_FreshInstallNeedsNoReinstall(t *testing.T) {
	db := openDB(t)
	ln, st, err := clip.Listen(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if st.NeedsReinstall() {
		t.Fatalf("%+v: no bookmarklet was installed yet", st)
	}
	if err := db.SetMeta(ctx, clip.BookmarkletPortKey, strconv.Itoa(st.Port)); err != nil {
		t.Fatal(err)
	}
	ln.Close()
	ln2, st, err := clip.Listen(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer ln2.Close()
	if st.NeedsReinstall() {
		t.Fatalf("%+v: the bookmarklet matches the port", st)
	}
}

// The banner offers a new bookmarklet, which only exists when a server does:
// with no port there's nothing to reinstall, and ClipStatus reports why.
func TestR18_NoServerNeedsNoReinstall(t *testing.T) {
	st := clip.PortStatus{Port: 0, Bookmarklet: clip.PortLow}
	if st.NeedsReinstall() {
		t.Fatalf("%+v: there's no port to reinstall for", st)
	}
}

func TestR18_InstalledPortIsWhatWasMarked(t *testing.T) {
	db := openDB(t)
	if port, err := clip.InstalledPort(ctx, db); err != nil || port != 0 {
		t.Fatalf("before any install: %d %v", port, err)
	}
	if err := clip.MarkInstalled(ctx, db, clip.PortLow); err != nil {
		t.Fatal(err)
	}
	if port, err := clip.InstalledPort(ctx, db); err != nil || port != clip.PortLow {
		t.Fatalf("after the install: %d %v", port, err)
	}
	// Listen reads the same value, so the banner survives a restart.
	ln, st, err := clip.Listen(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if st.Bookmarklet != clip.PortLow {
		t.Fatalf("status %+v", st)
	}
}

// A damaged value reads as "nothing installed", like a fresh install: the
// clipper keeps working and the user can install again.
func TestR18_DamagedInstalledPortReadsAsNone(t *testing.T) {
	db := openDB(t)
	if err := db.SetMeta(ctx, clip.BookmarkletPortKey, "not a port"); err != nil {
		t.Fatal(err)
	}
	if port, err := clip.InstalledPort(ctx, db); err != nil || port != 0 {
		t.Fatalf("%d %v", port, err)
	}
}
