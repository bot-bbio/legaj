package store_test

import (
	"testing"

	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestR18_AppMetaRoundTrip(t *testing.T) {
	db, st, err := store.Open(ctx, store.Options{Dir: testenv.Isolate(t)})
	if err != nil || st.Kind != store.StartupReady {
		t.Fatalf("open: %v %+v", err, st)
	}
	t.Cleanup(func() { _ = db.Close() })

	if v, ok, err := db.GetMeta(ctx, "clip_port"); err != nil || ok || v != "" {
		t.Fatalf("missing key: %q %v %v", v, ok, err)
	}
	for _, want := range []string{"47321", "47522"} {
		if err := db.SetMeta(ctx, "clip_port", want); err != nil {
			t.Fatal(err)
		}
		if v, ok, err := db.GetMeta(ctx, "clip_port"); err != nil || !ok || v != want {
			t.Fatalf("got %q %v %v, want %q", v, ok, err, want)
		}
	}
	// The row written by migration 0001 is still there.
	if v, ok, _ := db.GetMeta(ctx, "created_by"); !ok || v != "LeGaJ 2" {
		t.Fatalf("created_by %q %v", v, ok)
	}
}
