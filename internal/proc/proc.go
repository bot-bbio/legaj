// Package proc is the only place in LeGaJ allowed to start a child process.
//
// LeGaJ is a GUI-subsystem app. Any console-subsystem child it starts (a
// shell, a CLI tool) would get its own console window unless it is created
// with CREATE_NO_WINDOW — that flashing window was V1 bug U10. Routing every
// spawn through Command makes the flag impossible to forget; the architecture
// test in internal/archtest enforces that nothing else calls os/exec.
package proc

import (
	"context"
	"os/exec"
)

// Command is exec.CommandContext with the child's window suppressed.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	return cmd
}
