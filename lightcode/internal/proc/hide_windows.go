//go:build windows

package proc

import (
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW: the child gets no console at all, so
// none can show, even for the moment before HideWindow has it.
const createNoWindow = 0x08000000

// Hide keeps cmd, a console program, from opening a console window.
func Hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
