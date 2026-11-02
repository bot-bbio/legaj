//go:build windows

package opener

import "golang.org/x/sys/windows"

// ShellExecute opens a target with its default handler (browser, mail app,
// PDF viewer). No shell is involved, so nothing in the target is parsed as
// a command.
type ShellExecute struct{}

// Launch opens target.
func (ShellExecute) Launch(target string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}
