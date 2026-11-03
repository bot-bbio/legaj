package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/bot-bbio/legaj/internal/update"
)

func TestR40_CheckForUpdateFollowsTheSettingsToggle(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"tag_name":"v9.0.0","html_url":"https://github.com/bot-bbio/legaj/releases/tag/v9.0.0"}`))
	}))
	defer srv.Close()

	app, _, _ := newTestAppWithLaunches(t)
	app.updates = update.New(update.Config{APIBase: srv.URL, Current: "2.0.0", Enabled: app.updateEnabled})
	_, svc, err := app.services()
	if err != nil {
		t.Fatal(err)
	}
	st, err := svc.Settings.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	st.UpdateCheck = false
	if _, err := svc.Settings.Save(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	if res, err := app.CheckForUpdate(); err != nil || res.Available || hits.Load() != 0 {
		t.Fatalf("toggle off: %+v %v, %d requests", res, err, hits.Load())
	}

	st.UpdateCheck = true
	if _, err := svc.Settings.Save(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	res, err := app.CheckForUpdate()
	if err != nil || !res.Available || res.Latest != "9.0.0" {
		t.Fatalf("toggle on: %+v %v", res, err)
	}
}

func TestR40_UpdateCheckFailuresStayQuiet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer srv.Close()
	app, _, _ := newTestAppWithLaunches(t)
	app.updates = update.New(update.Config{APIBase: srv.URL, Current: "2.0.0", Enabled: func(context.Context) bool { return true }})
	if res, err := app.CheckForUpdate(); err != nil || res.Available {
		t.Fatalf("a failed check must look like no update: %+v %v", res, err)
	}
}
