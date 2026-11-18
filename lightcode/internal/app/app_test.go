package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// testWorkspace makes a small folder and an app on it.
func testWorkspace(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"README.md":   "# Demo\n\nA test workspace.\n",
		"main.go":     "package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n",
		"pkg/util.go": "package pkg\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n",
	}
	for name, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The session file is the test's: New reads and writes it.
	t.Setenv("LIGHTCODE_STATE", filepath.Join(dir, "state.json"))
	t.Setenv("LIGHTCODE_SETTINGS", filepath.Join(dir, "settings.json"))
	a := New(dir, "")
	return a, dir
}

// writeFileAt writes a file under dir, making folders as needed.
func writeFileAt(dir, name, text string) error {
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

func TestViewChrome(t *testing.T) {
	a, _ := testWorkspace(t)
	tt := ui.NewTester(a.View, 1280, 820)
	if !tt.HasText("资源管理器") {
		t.Fatalf("no explorer title; texts: %q", tt.Texts())
	}
	for _, want := range []string{"轻码", "README.md", "main.go"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q; texts: %q", want, tt.Texts())
		}
	}
}

func TestQuickOpenOpensFile(t *testing.T) {
	a, _ := testWorkspace(t)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Key(ui.Cmd, ui.KeyP)
	tt.Type("util")
	tt.Key(0, ui.KeyEnter)
	if !tt.HasText("util.go") {
		t.Errorf("the tab of util.go did not open; texts %q", tt.Texts())
	}
	if got := a.Col().Current(); got == nil || got.Doc.Name() != "util.go" {
		t.Errorf("current doc = %v", got)
	}
}

func TestToggleSidebar(t *testing.T) {
	a, _ := testWorkspace(t)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Key(ui.Cmd, ui.KeyB)
	if tt.HasText("资源管理器") {
		t.Errorf("sidebar still shown after Ctrl+B")
	}
	tt.Key(ui.Cmd, ui.KeyB)
	if !tt.HasText("资源管理器") {
		t.Errorf("sidebar did not come back")
	}
}

func TestSaveDirtyDocument(t *testing.T) {
	a, dir := testWorkspace(t)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Key(ui.Cmd, ui.KeyP)
	tt.Type("main.go")
	tt.Key(0, ui.KeyEnter)
	tt.ClickAt(500, 300)
	// Type at the end of the first line.
	tt.Key(ui.Cmd, ui.KeyEnd)
	tt.Type(" // edited")
	if !a.Col().Current().Doc.Dirty() {
		t.Fatalf("typing did not mark the buffer dirty")
	}
	tt.Key(ui.Cmd, ui.KeyS)
	text, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "// edited") {
		t.Errorf("saved text = %q", text)
	}
}

func TestCloseDirtyAsksToSave(t *testing.T) {
	a, _ := testWorkspace(t)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Key(ui.Cmd, ui.KeyP)
	tt.Type("main.go")
	tt.Key(0, ui.KeyEnter)
	tt.ClickAt(500, 300)
	tt.Type("x")
	tt.Key(ui.Cmd, ui.KeyW)
	if !tt.HasText("是否保存对 main.go 的更改？") {
		t.Fatalf("no save prompt; texts %q", tt.Texts())
	}
	if err := tt.Click("不保存"); err != nil {
		t.Fatal(err)
	}
	if len(a.Col().Editors) != 1 || a.Col().Current().Doc.Name() != "README.md" {
		t.Errorf("tab not closed, %d editors", len(a.Col().Editors))
	}
}

// TestChineseTyping types Chinese into the editor: the text arrives, the
// caret follows its wide characters, and input methods stay active (the
// candidate window itself needs a human on a real machine).
func TestChineseTyping(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.ClickAt(500, 300)
	tt.Type("// 中文输入测试")
	got := a.Col().Current().Doc.Buffer.String()
	if !strings.Contains(got, "中文输入测试") {
		t.Fatalf("typed text missing: %q", got)
	}
	if active, noIM := tt.TextInputActive(); !active || noIM {
		t.Errorf("editor text input state = active %v, noInputMethod %v", active, noIM)
	}
}
