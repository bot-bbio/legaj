package main

import (
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/secrets"
	"github.com/bot-bbio/legaj/internal/settings"
)

// SettingsAPI is the Settings screen's binding (R37, R4).
type SettingsAPI struct {
	app *App
}

// SettingsView is everything the Settings screen shows. The key appears
// only as a saved flag and a mask (R4).
type SettingsView struct {
	Settings settings.Settings `json:"settings"`
	Key      secrets.Status    `json:"key"`
}

// GetSettings returns the settings and the key's masked status.
func (s *SettingsAPI) GetSettings() (SettingsView, error) {
	c, svc, err := s.app.services()
	if err != nil {
		return SettingsView{}, err
	}
	st, err := svc.Settings.Get(s.app.ctx)
	if err != nil {
		return SettingsView{}, apperr.Public(err)
	}
	key, err := c.Secrets.Status()
	if err != nil {
		return SettingsView{}, apperr.Public(err)
	}
	return SettingsView{Settings: st, Key: key}, nil
}

// SaveSettings validates and stores the settings.
func (s *SettingsAPI) SaveSettings(in settings.Settings) (SettingsView, error) {
	_, svc, err := s.app.services()
	if err != nil {
		return SettingsView{}, err
	}
	if _, err := svc.Settings.Save(s.app.ctx, in); err != nil {
		return SettingsView{}, apperr.Public(err)
	}
	return s.GetSettings()
}

// SaveAPIKey stores a new key (first time or Replace). It returns nothing
// about the key; the screen refreshes from GetSettings.
func (s *SettingsAPI) SaveAPIKey(key string) error {
	c, err := s.app.coreOnly()
	if err != nil {
		return err
	}
	if err := c.Secrets.Save(key); err != nil {
		return apperr.Public(err)
	}
	c.Log.Info("api key saved")
	c.Bus.Emit(events.Change(events.EntitySettings))
	return nil
}

// RemoveAPIKey deletes the key from Credential Manager.
func (s *SettingsAPI) RemoveAPIKey() error {
	c, err := s.app.coreOnly()
	if err != nil {
		return err
	}
	if err := c.Secrets.Remove(); err != nil {
		return apperr.Public(err)
	}
	c.Log.Info("api key removed")
	c.Bus.Emit(events.Change(events.EntitySettings))
	return nil
}
