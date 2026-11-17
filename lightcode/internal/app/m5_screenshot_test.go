package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestM5Screenshots renders the themed tab menu.
func TestM5Screenshots(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	a.OpenFile(filepath.Join(dir, "pkg", "util.go"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.RightClickAt(340, 64)
	tt.Frame()
	saveShot(t, tt, "lightcode-ui-tabmenu.png")
}
