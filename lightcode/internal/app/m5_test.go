package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"lightcode/internal/theme"
)

// TestSettingsPageShowsAndChanges opens the settings page and changes the
// editor's font size, which settings.json keeps.
func TestSettingsPageShowsAndChanges(t *testing.T) {
	a, dir := testWorkspace(t)
	a.RunCommand("view.settings")
	tt := ui.NewTester(a.View, 1280, 820)
	for _, want := range []string{"设置", "编辑器", "软换行", "终端"} {
		if !tt.HasText(want) {
			t.Fatalf("settings misses %q; texts %q", want, tt.Texts())
		}
	}
	before := a.Prefs.EditorFontSize
	if err := tt.Click("+"); err != nil {
		t.Fatal(err)
	}
	if a.Prefs.EditorFontSize <= before {
		t.Errorf("font size did not grow: %v", a.Prefs.EditorFontSize)
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("settings.json was not written")
		}
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Errorf("settings.json is empty")
	}
	a2 := New(dir, "")
	if a2.Prefs.EditorFontSize != a.Prefs.EditorFontSize {
		t.Errorf("restored font size = %v, want %v", a2.Prefs.EditorFontSize, a.Prefs.EditorFontSize)
	}
}

// TestCapabilitiesPanel lists the providers and their toggles.
func TestCapabilitiesPanel(t *testing.T) {
	a, _ := testWorkspace(t)
	a.ShowSidebar(4)
	tt := ui.NewTester(a.View, 1280, 820)
	for _, want := range []string{"能力", "源代码管理", "终端", "ui/markdown"} {
		if !tt.HasText(want) {
			t.Fatalf("capabilities misses %q; texts %q", want, tt.Texts())
		}
	}
	if err := tt.Click("启用终端"); err != nil {
		t.Fatal(err)
	}
	if a.Prefs.TerminalEnabled {
		t.Errorf("the terminal switch did not turn off")
	}
}

// TestTerminalPaneFailure shows why a terminal cannot start instead of
// crashing the window.
func TestTerminalPaneFailure(t *testing.T) {
	a, _ := testWorkspace(t)
	a.Prefs.TerminalEnabled = true
	a.TermOpen = true
	a.TermErr = "libghostty-vt: not found"
	tt := ui.NewTester(a.View, 1280, 820)
	if !tt.HasText("终端无法启动") || !tt.HasText("libghostty-vt 由 mygo build") {
		t.Fatalf("terminal error pane is missing; texts %q", tt.Texts())
	}
}

// TestTerminalDisabled toggle does nothing while the capability is off.
func TestTerminalDisabled(t *testing.T) {
	a, _ := testWorkspace(t)
	a.Prefs.TerminalEnabled = false
	a.toggleTerminal()
	if a.TermOpen {
		t.Errorf("the terminal opened while disabled")
	}
}

// TestTerminalLinkOpensATLine opens a path:line:col the terminal prints.
func TestTerminalLinkOpensATLine(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	link := filepath.ToSlash(filepath.Join(dir, "main.go")) + ":3:2"
	if !a.openTerminalLink(link, dir) {
		t.Fatalf("the link did not open")
	}
	e := a.Col().Current()
	if e == nil || e.Doc.Name() != "main.go" {
		t.Fatalf("opened %v", e)
	}
	if pos := e.State.CursorPos(); pos.Line != 2 || pos.Col != 1 {
		t.Errorf("cursor = %+v, want line 3 col 2", pos)
	}
	// A path outside the workspace is refused.
	if a.openTerminalLink(filepath.ToSlash(filepath.Join(os.TempDir(), "outside.go")), dir) {
		t.Errorf("a path outside the workspace opened")
	}
}

// TestThemeToggle switches to the light board, keeps it in settings.json
// and restores it.
func TestThemeToggle(t *testing.T) {
	a, dir := testWorkspace(t)
	if theme.IsLight() {
		t.Fatalf("a new app is not on the dark board")
	}
	a.RunCommand("view.toggleTheme")
	if !theme.IsLight() || a.Prefs.Theme != "light" {
		t.Fatalf("light = %v, pref = %q", theme.IsLight(), a.Prefs.Theme)
	}
	a2 := New(dir, "")
	if !theme.IsLight() || a2.Prefs.Theme != "light" {
		t.Errorf("the light board did not survive a restart")
	}
	a2.RunCommand("view.toggleTheme")
	if theme.IsLight() {
		t.Errorf("the toggle did not go back to dark")
	}
}

// TestCloseOthersAndSaved covers the tab menu's bulk closes.
func TestCloseOthersAndSaved(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	a.OpenFile(filepath.Join(dir, "pkg", "util.go"))
	a.OpenFile(filepath.Join(dir, "README.md"))
	util := a.Cols[0].Current().Doc
	util.Buffer.Set("dirty\n")
	mainDoc := a.Cols[0].Editors[0].Doc
	a.closeOthersIn(0, mainDoc)
	if len(a.Cols[0].Editors) != 2 {
		t.Fatalf("tabs = %d, want main and the dirty util", len(a.Cols[0].Editors))
	}
	a.closeSavedIn(0)
	if len(a.Cols[0].Editors) != 1 || a.Cols[0].Editors[0].Doc != util {
		t.Errorf("closeSaved left %d tabs", len(a.Cols[0].Editors))
	}
}

// TestTabContextMenu right-clicks a tab and finds its themed menu.
func TestTabContextMenu(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	a.OpenFile(filepath.Join(dir, "pkg", "util.go"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.RightClickAt(340, 64)
	tt.Frame()
	for _, want := range []string{"关闭", "关闭其他", "关闭已保存", "复制路径", "在文件管理器中显示"} {
		if !tt.HasText(want) {
			t.Errorf("themed tab menu misses %q; texts %q", want, tt.Texts())
		}
	}
}
