package main

// ClipStatusView tells the frontend where the clipper listens (R18), and
// whether the installed bookmarklet points at an old port, in which case
// the app shows a banner and a regenerated bookmarklet.
type ClipStatusView struct {
	Running        bool   `json:"running"`
	Port           int    `json:"port"`
	NeedsReinstall bool   `json:"needsReinstall"`
	Message        string `json:"message"` // why it isn't running; "" when it is
}

// ClipStatus reports the clip server's state.
func (a *App) ClipStatus() (ClipStatusView, error) {
	_, svc, err := a.services()
	if err != nil {
		return ClipStatusView{}, a.public("App.ClipStatus", err)
	}
	st := svc.ClipPort
	return ClipStatusView{
		Running:        svc.ClipServer != nil,
		Port:           st.Port,
		NeedsReinstall: st.NeedsReinstall(),
		Message:        svc.ClipProblem,
	}, nil
}
