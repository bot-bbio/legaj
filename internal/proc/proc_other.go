//go:build !windows

package proc

import "os/exec"

// hideWindow is a no-op off Windows: there is no console subsystem to suppress.
func hideWindow(*exec.Cmd) {}
