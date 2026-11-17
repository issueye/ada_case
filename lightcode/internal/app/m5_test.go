package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSettingsPageShowsAndChanges opens the settings page and changes the
// editor's font size, which settings.json keeps.
func TestSettingsPageShowsAndChanges(t *testing.T) {
	a, dir := testWorkspace(t)
	a.RunCommand("view.settings")
	tt := ui.NewTester(a.View, 1280, 820)
	for _, want := range []string{"设置", "编辑器", "软换行", "终端", "打开 AI 设置"} {
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

// TestCapabilitiesPanel lists providers and toggles AI off.
func TestCapabilitiesPanel(t *testing.T) {
	a, _ := testWorkspace(t)
	a.ShowSidebar(4)
	tt := ui.NewTester(a.View, 1280, 820)
	for _, want := range []string{"能力", "源代码管理", "AI 助手", "终端", "ui/markdown"} {
		if !tt.HasText(want) {
			t.Fatalf("capabilities misses %q; texts %q", want, tt.Texts())
		}
	}
	if err := tt.Click("启用 AI 助手"); err != nil {
		t.Fatal(err)
	}
	if a.Prefs.AIEnabled {
		t.Errorf("the AI switch did not turn off")
	}
	tt.Frame()
	if tt.HasText("让 AI 改代码") {
		t.Errorf("the AI panel stayed after it was disabled")
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
