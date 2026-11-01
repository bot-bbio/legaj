//go:build windows

// Package instance enforces R42: at most one LeGaJ process per Windows session.
//
// Wails v2 has its own SingleInstanceLock, but when its mutex exists and the
// first instance's message window isn't found yet, it starts a second full
// instance — which would open the DB, bind the clip port and run the job
// queue twice. Gate closes that gap: it takes a named mutex before anything
// else runs, and a second process exits without calling start.
package instance

import (
	"errors"

	"golang.org/x/sys/windows"
)

// AppID identifies LeGaJ to both this gate and Wails' SingleInstanceLock.
// Changing it breaks single-instance detection across versions, so don't.
const AppID = "3b9c6f1e-legaj-v2"

// MutexName is the session-local mutex held for the life of the first process.
const MutexName = `Local\LeGaJ-single-instance-` + AppID

// Lock is a held named mutex. The OS releases it when the process exits.
type Lock struct {
	handle windows.Handle
}

// Acquire creates or opens the named mutex. acquired is false when another
// process already holds it; the returned Lock is then nil.
func Acquire(name string) (lock *Lock, acquired bool, err error) {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, false, err
	}
	h, err := windows.CreateMutex(nil, false, ptr)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		// CreateMutex returned a handle to the existing mutex; we don't own it.
		_ = windows.CloseHandle(h)
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &Lock{handle: h}, true, nil
}

// Release closes the mutex handle so another process may acquire it.
func (l *Lock) Release() error {
	if l == nil || l.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(l.handle)
	l.handle = 0
	return err
}

// Gate runs start only in the first instance. In a later instance it calls
// activate (best effort: focus the running window) and returns first=false
// without calling start. The first instance's lock is held until exit.
func Gate(name string, activate func(), start func() error) (first bool, err error) {
	lock, acquired, err := Acquire(name)
	if err != nil {
		return false, err
	}
	if !acquired {
		activate()
		return false, nil
	}
	// Keep the handle open for the process lifetime; the OS releases it on exit.
	_ = lock
	return true, start()
}
