//go:build !windows

package proc

import "os/exec"

// Hide exists on every platform; elsewhere a child has no console window to
// hide.
func Hide(cmd *exec.Cmd) {}
