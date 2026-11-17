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
	for _, want := range []string{"打开文件夹…", "切换 AI 面板", "重置缩放"} {
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

// TestAIRailKeepsItsOwnStrip puts the collapsed assistant in a strip of
// its own, clear of the editor.
func TestAIRailKeepsItsOwnStrip(t *testing.T) {
	a, _ := testWorkspace(t)
	a.AI.Mode = AIRail
	tt := ui.NewTester(a.View, 1280, 820)
	box, ok := tt.Find("AI")
	if !ok {
		t.Fatalf("no rail label")
	}
	// The editor ends where the rail's 62 DIP strip starts, at 1280-62,
	// and the rail starts under the tab strip and breadcrumbs.
	if box.X < 1280-62 {
		t.Errorf("the rail overlaps the editor: AI at x %.0f", box.X)
	}
	if box.Y <= 100 {
		t.Errorf("the rail starts above the editor: AI at y %.0f", box.Y)
	}
}
