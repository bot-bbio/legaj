package main

import (
	"testing"
)

func TestR18_ClipStatusWithoutServer(t *testing.T) {
	// Test apps run no clip server (core.Options.StartClipServer is off).
	app, _ := newTestApp(t)
	st, err := app.ClipStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st.Running || st.Port != 0 || st.NeedsReinstall {
		t.Fatalf("status %+v", st)
	}
}
