//go:build windows

package instance

import (
	"encoding/json"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Wails v2 (desktop/windows/single_instance.go) listens on a message-only
// window for WM_COPYDATA with this dwData tag, then fires the first
// instance's OnSecondInstanceLaunch callback, which restores and focuses it.
const (
	wmCopyData             = 0x004A
	wailsSingleInstanceTag = 1542
)

// hwndMessage is HWND_MESSAGE: the parent of all message-only windows.
// FindWindowW does not search message-only windows, so FindWindowExW with
// this parent is required to locate Wails' event target.
const hwndMessage = ^uintptr(2) // (HWND)-3

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW = user32.NewProc("FindWindowExW")
	procSendMessage = user32.NewProc("SendMessageW")
)

type copyDataStruct struct {
	dwData uintptr
	cbData uint32
	lpData uintptr
}

// wailsEventWindow returns the class and window names Wails v2 registers for
// a SingleInstanceLock with the given unique ID.
func wailsEventWindow(uniqueID string) (class, window string) {
	id := "wails-app-" + uniqueID
	return id + "-sic", id + "-siw"
}

// ActivateFirst asks the running instance to restore and focus its window.
// It is best effort: if the first instance is still starting and its window
// doesn't exist yet, nothing happens and this process still exits.
func ActivateFirst(uniqueID string) {
	class, window := wailsEventWindow(uniqueID)
	classPtr, err := windows.UTF16PtrFromString(class)
	if err != nil {
		return
	}
	windowPtr, err := windows.UTF16PtrFromString(window)
	if err != nil {
		return
	}
	hwnd, _, _ := procFindWindowW.Call(hwndMessage, 0,
		uintptr(unsafe.Pointer(classPtr)), uintptr(unsafe.Pointer(windowPtr)))
	if hwnd == 0 {
		return
	}
	payload, err := json.Marshal(struct {
		Args             []string `json:"Args"`
		WorkingDirectory string   `json:"WorkingDirectory"`
	}{Args: os.Args[1:]})
	if err != nil {
		return
	}
	text, err := windows.UTF16FromString(string(payload))
	if err != nil {
		return
	}
	cds := copyDataStruct{
		dwData: wailsSingleInstanceTag,
		cbData: uint32(len(text) * 2),
		lpData: uintptr(unsafe.Pointer(&text[0])),
	}
	_, _, _ = procSendMessage.Call(hwnd, wmCopyData, 0, uintptr(unsafe.Pointer(&cds)))
	// SendMessage is synchronous; keep the buffer alive until it returns.
	runtime.KeepAlive(text)
}
