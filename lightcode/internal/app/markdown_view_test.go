package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestMarkdownDocumentView toggles a Markdown tab between its source and
// the rendered document, by the breadcrumb button and the command.
func TestMarkdownDocumentView(t *testing.T) {
	a, dir := testWorkspace(t)
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("# 标题\n\n正文。\n\n**粗体**\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.OpenFile(path)
	e := a.Col().Current()
	if e == nil || e.MarkdownView {
		t.Fatalf("Markdown opened in view mode: %+v", e)
	}
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Frame()
	if tt.HasText("粗体") {
		t.Errorf("the document is rendered before it was toggled")
	}
	if err := tt.Click("预览"); err != nil {
		t.Fatal(err)
	}
	if !e.MarkdownView {
		t.Fatalf("the breadcrumb button did not toggle the view")
	}
	tt.Frame()
	if !tt.HasText("粗体") {
		t.Errorf("the rendered document misses its text; texts %q", tt.Texts())
	}
	if !tt.HasText("源码") {
		t.Errorf("the breadcrumb still offers 预览 in view mode")
	}

	a.ToggleMarkdownPreview()
	tt.Frame()
	if e.MarkdownView {
		t.Errorf("the command did not toggle back to source")
	}
	if tt.HasText("粗体") {
		t.Errorf("the rendered text stayed after toggling to source")
	}

	// The shortcut does the same as the command.
	tt.Key(ui.Cmd|ui.Shift, ui.KeyV)
	tt.Frame()
	if !e.MarkdownView {
		t.Errorf("Ctrl+Shift+V did not open the document view")
	}

	a.OpenFile(filepath.Join(dir, "main.go"))
	a.ToggleMarkdownPreview()
	if !strings.Contains(a.Message, "不是 Markdown") {
		t.Errorf("a Go file said %q, want a not-Markdown refusal", a.Message)
	}
}
