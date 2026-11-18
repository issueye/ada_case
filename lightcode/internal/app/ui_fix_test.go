package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestPaletteScrollsAllCommands keeps every command reachable: the list
// scrolls, so 打开文件夹… and the last commands are built too, not only
// the first eight.
func TestPaletteScrollsAllCommands(t *testing.T) {
	a, _ := testWorkspace(t)
	a.OpenPalette(PaletteCommands)
	tt := ui.NewTester(a.View, 1280, 820)
	for _, want := range []string{"打开文件夹…", "键盘快捷键", "重置缩放"} {
		if !tt.HasText(want) {
			t.Errorf("palette does not list %q", want)
		}
	}
	// Moving the choice keeps a row in view: the arrows can reach the last
	// commands, which the first eight rows could not.
	a.PaletteSel = 20
	tt.Frame()
	if r, ok := tt.Find("重置缩放"); !ok || r.Y <= 0 {
		t.Errorf("the chosen row is not laid out: %+v", r)
	}
}
