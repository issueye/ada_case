//go:build windows

package proc

import (
	"os/exec"
	"testing"
)

// TestHideAttributes holds the fix for the flashing console: a child the app
// starts gets no console window.
func TestHideAttributes(t *testing.T) {
	cmd := exec.Command("git")
	Hide(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("Hide left no SysProcAttr, so Windows would show the child's console")
	}
	if !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Errorf("attributes = %+v, want HideWindow and CREATE_NO_WINDOW", cmd.SysProcAttr)
	}
}
