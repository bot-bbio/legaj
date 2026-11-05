package main

import (
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/clip"
	"github.com/bot-bbio/legaj/internal/events"
)

// ClipStatusView tells the frontend where the clipper listens (R18), and
// whether the installed bookmarklet points at an old port, in which case
// the app shows a banner and a regenerated bookmarklet.
type ClipStatusView struct {
	Running        bool   `json:"running"`
	Port           int    `json:"port"`
	NeedsReinstall bool   `json:"needsReinstall"`
	Message        string `json:"message"` // why it isn't running; "" when it is
}

// ClipStatus reports the clip server's state. The installed port is read
// fresh, so a reinstall clears the banner at once, without a restart.
func (a *App) ClipStatus() (ClipStatusView, error) {
	_, svc, err := a.services()
	if err != nil {
		return ClipStatusView{}, a.public("App.ClipStatus", err)
	}
	st := svc.ClipPort
	if st.Bookmarklet, err = clip.InstalledPort(a.ctx, svc.DB); err != nil {
		return ClipStatusView{}, a.public("App.ClipStatus", err)
	}
	return ClipStatusView{
		Running:        svc.ClipServer != nil,
		Port:           st.Port,
		NeedsReinstall: st.NeedsReinstall(),
		Message:        svc.ClipProblem,
	}, nil
}

// MarkBookmarkletInstalled records that the user took the bookmarklet for
// the running server's port. The install page calls it when the user drags
// or copies the bookmarklet, not when the page opens: a page merely seen
// leaves the old bookmark in place, so the banner has to stay.
func (a *App) MarkBookmarkletInstalled() error {
	c, svc, err := a.services()
	if err != nil {
		return a.public("App.MarkBookmarkletInstalled", err)
	}
	if svc.ClipServer == nil {
		return a.public("App.MarkBookmarkletInstalled",
			apperr.New(apperr.CodeConflict, "LeGaJ's clipper isn't running. Restart LeGaJ and try again."))
	}
	if err := clip.MarkInstalled(a.ctx, svc.DB, svc.ClipPort.Port); err != nil {
		return a.public("App.MarkBookmarkletInstalled", err)
	}
	c.Bus.Emit(events.Change(events.EntitySettings))
	return nil
}
