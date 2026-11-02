package main

import (
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/profile"
)

// ProfileAPI is the Profile screen's binding (R9).
type ProfileAPI struct {
	app *App
}

// GetProfile returns the saved profile (empty lists, never null).
func (p *ProfileAPI) GetProfile() (profile.Profile, error) {
	_, svc, err := p.app.services()
	if err != nil {
		return profile.Profile{}, err
	}
	out, err := svc.Profile.Get(p.app.ctx)
	if err != nil {
		return profile.Profile{}, apperr.Public(err)
	}
	return out, nil
}

// SaveProfile validates and stores the whole profile, returning what was saved.
func (p *ProfileAPI) SaveProfile(in profile.Profile) (profile.Profile, error) {
	_, svc, err := p.app.services()
	if err != nil {
		return profile.Profile{}, err
	}
	out, err := svc.Profile.Save(p.app.ctx, in)
	if err != nil {
		return profile.Profile{}, apperr.Public(err)
	}
	return out, nil
}
