package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestScreenshots renders the window with the headless tester and saves
// the images the docs show, as the screenshot regression of
// docs/dev-plan.md §6 seeds.
func TestScreenshots(t *testing.T) {
	a, dir := testWorkspace(t)
	_ = dir
	a.OpenFile(filepath.Join(dir, "main.go"))
	tt := ui.NewTester(a.View, 1280, 820)
	saveShot(t, tt, "lightcode-ui-editor.png")

	a.AIDocked = false
	tt.Frame()
	saveShot(t, tt, "lightcode-ui-rail.png")

	a.AIDocked = true
	a.OpenPalette(PaletteCommands)
	tt.Frame()
	saveShot(t, tt, "lightcode-ui-palette.png")
	tt.Key(0, ui.KeyEscape)
	a.ClosePrompt = false

	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	a.RunCommand("view.split")
	a.ActiveCol = 1
	a.OpenFile(filepath.Join(dir, "pkg", "util.go"))
	tt.Frame()
	saveShot(t, tt, "lightcode-ui-split.png")
}

func saveShot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	img := tt.Image()
	out := filepath.Join("..", "..", "..", "_shots", name)
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%dx%d)", out, img.Bounds().Dx(), img.Bounds().Dy())
}
