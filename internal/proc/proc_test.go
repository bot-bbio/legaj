//go:build windows

package proc

import (
	"context"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

func TestR6_CommandSetsCreateNoWindow(t *testing.T) {
	cmd := Command(context.Background(), "cmd.exe", "/c", "exit", "0")
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr not set: the child would get a console window")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow = false, want true")
	}
}
