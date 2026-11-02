//go:build windows

package settings

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

// runKey is the per-user list of programs Windows starts at login. Writing
// here needs no admin rights.
const (
	runKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue = "LeGaJ"
)

// RegistryAutostart manages LeGaJ's entry under HKCU\...\Run.
type RegistryAutostart struct{}

// Enable sets the Run entry to command (a quoted exe path).
func (RegistryAutostart) Enable(command string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(runValue, command)
}

// Disable removes the Run entry. A missing entry is fine.
func (RegistryAutostart) Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(runValue); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// Current reports the Run entry's command and whether it exists.
func (RegistryAutostart) Current() (string, bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(runValue)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}
