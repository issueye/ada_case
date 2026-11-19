package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestM5Screenshots renders the themed tab menu on both boards and the
// code area's menu. The light board is the one that showed a second,
// larger background behind the menu while the panel took the theme's
// Popover look; the code area's menu was the system's, white and in
// English, until it was drawn in the window too.
func TestM5Screenshots(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	a.OpenFile(filepath.Join(dir, "pkg", "util.go"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.RightClickAt(340, 64)
	tt.Frame()
	saveShot(t, tt, "lightcode-ui-tabmenu.png")

	a.RunCommand("view.toggleTheme")
	tt.Frame()
	saveShot(t, tt, "lightcode-ui-tabmenu-light.png")

	// The code area's menu, in the app's palette and language.
	tt.ClickAt(700, 200)
	tt.SetLocale("zh-Hans")
	tt.RightClickAt(700, 300)
	tt.Frame()
	saveShot(t, tt, "lightcode-ui-editor-menu.png")

	// The palette is the package's global state: leave it dark for the
	// screenshot tests that run after this one.
	a.RunCommand("view.toggleTheme")
}
