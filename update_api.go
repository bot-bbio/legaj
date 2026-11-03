package main

import (
	"context"

	"github.com/bot-bbio/legaj/internal/update"
)

type updateChecker interface {
	Check(ctx context.Context) (update.Result, error)
}

// updateEnabled reports the Settings toggle (R40). Before boot, or if
// settings can't be read, the answer is no: no request is made.
func (a *App) updateEnabled(ctx context.Context) bool {
	_, svc, err := a.services()
	if err != nil {
		return false
	}
	st, err := svc.Settings.Get(ctx)
	return err == nil && st.UpdateCheck
}

// CheckForUpdate tells the banner whether a newer release exists (R40).
// It only notifies; the Download button opens the release page. A failed
// check is logged and looks like "no update".
func (a *App) CheckForUpdate() (update.Result, error) {
	res, err := a.updates.Check(a.ctx)
	if err != nil {
		a.log().Info("update check failed", "err", err)
		return update.Result{}, nil
	}
	return res, nil
}
