package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"lightcode/internal/document"
)

// TestOutlineOfGoFile lists a Go file's declarations and follows an edit.
func TestOutlineOfGoFile(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "pkg", "util.go"))
	e := a.Col().Current()
	syms := a.outlineOf(e)
	if len(syms) != 1 || syms[0].Name != "Add" || syms[0].Kind != "函数" || syms[0].Line != 3 {
		t.Fatalf("outline = %+v", syms)
	}
	e.Doc.Buffer.Set("package pkg\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n\nfunc Sub(a, b int) int {\n\treturn a - b\n}\n")
	syms = a.outlineOf(e)
	if len(syms) != 2 || syms[1].Name != "Sub" {
		t.Errorf("outline after edit = %+v", syms)
	}
}

// TestOutlineRendersAndJumps shows the outline in the sidebar and jumps to
// a symbol.
func TestOutlineRendersAndJumps(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "pkg", "util.go"))
	tt := ui.NewTester(a.View, 1280, 820)
	if !tt.HasText("大纲") || !tt.HasText("Add") {
		t.Fatalf("outline missing; texts %q", tt.Texts())
	}
	if _, ok := tt.Find("Add"); !ok {
		t.Fatalf("no Add row")
	}
	syms := a.outlineOf(a.Col().Current())
	if err := tt.Click("Add"); err != nil {
		t.Fatal(err)
	}
	if pos := a.Col().Current().State.CursorPos(); pos.Line != syms[0].Line-1 {
		t.Errorf("cursor line = %d, want %d", pos.Line, syms[0].Line-1)
	}
}

// bigFile writes a Go file with 40 functions, Fn00 to Fn39, for a list
// longer than the outline shows at once.
func bigFile(t *testing.T, dir string) string {
	t.Helper()
	var src strings.Builder
	src.WriteString("package big\n\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&src, "func Fn%02d() {}\n\n", i)
	}
	path := filepath.Join(dir, "big.go")
	if err := os.WriteFile(path, []byte(src.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// caretToFn39 puts the caret of the editor on Fn39, the last function.
func caretToFn39(e *document.Editor) {
	line := 2 + 2*39
	e.State.GoTo(editor.Pos{Line: line, Col: 0})
	e.State.ScrollTo(line)
}

// TestOutlineGoesToTheUnitOfATab: a tab another file's list had scrolled
// away shows again, the outline goes to the unit at its caret, rather
// than staying where the last file left it.
func TestOutlineGoesToTheUnitOfATab(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(bigFile(t, dir))
	tt := ui.NewTester(a.View, 1280, 820)
	// The list starts at the top, as the caret does.
	if r, ok := tt.Find("Fn00"); !ok || r.W == 0 {
		t.Fatalf("the outline shows no Fn00: %+v", r)
	}
	e := a.Col().Current()
	caretToFn39(e)
	a.OpenFile(filepath.Join(dir, "pkg", "util.go"))
	tt.Frame()
	tt.ClickAt(340, 64) // the first tab, big.go
	tt.Frame()
	r, ok := tt.Find("Fn39")
	if !ok || r.W == 0 {
		t.Fatalf("the outline did not go to Fn39, the unit at the caret: %+v", r)
	}
	if r.Y < 0 || r.Y+r.H > 820 {
		t.Fatalf("Fn39 is outside the window: %+v", r)
	}
	if top, ok := tt.Find("Fn00"); ok && top.W != 0 {
		t.Errorf("the list still shows Fn00: it did not scroll")
	}
}

// TestOutlineFollowsADoubleClickedTab: a second click on the tab the
// editor shows asks the outline for the unit at the caret, where it may
// have been scrolled away.
func TestOutlineFollowsADoubleClickedTab(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(bigFile(t, dir))
	tt := ui.NewTester(a.View, 1280, 820)
	e := a.Col().Current()
	caretToFn39(e)
	tt.Frame()
	if r, ok := tt.Find("Fn39"); !ok || r.W != 0 {
		t.Fatalf("the outline moved before the tab was clicked: %+v", r)
	}
	tt.ClickAt(340, 64)
	tt.ClickAt(340, 64)
	tt.Frame()
	r, ok := tt.Find("Fn39")
	if !ok || r.W == 0 {
		t.Fatalf("a double click on the tab did not bring Fn39 into view: %+v", r)
	}
}

// TestFileOperations creates, renames and deletes through the dialog's
// state machine.
func TestFileOperations(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.BeginNewFile(dir)
	a.FileOp.Name = "notes.txt"
	a.fileOpConfirm()
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatalf("new file: %v", err)
	}
	if e := a.Col().Current(); e == nil || e.Doc.Name() != "notes.txt" {
		t.Errorf("the new file did not open")
	}
	// A name with a separator is refused.
	a.BeginNewFile(dir)
	a.FileOp.Name = "bad/name.txt"
	a.fileOpConfirm()
	if a.FileOp.Error == "" {
		t.Errorf("a name with a separator was accepted")
	}
	a.FileOp = fileOp{}
	// Rename the file.
	a.BeginRename(filepath.Join(dir, "notes.txt"))
	a.FileOp.Name = "todo.txt"
	a.fileOpConfirm()
	if _, err := os.Stat(filepath.Join(dir, "todo.txt")); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err == nil {
		t.Errorf("the old name still exists")
	}
	// Delete it.
	a.BeginDelete(filepath.Join(dir, "todo.txt"))
	a.fileOpConfirm()
	if _, err := os.Stat(filepath.Join(dir, "todo.txt")); err == nil {
		t.Errorf("delete left the file")
	}
}

// TestRefreshTreePicksUpNewFiles rereads the tree after a file appears on
// disk, as the watcher does.
func TestRefreshTreePicksUpNewFiles(t *testing.T) {
	a, dir := testWorkspace(t)
	writeFileAt(dir, "later.txt", "later")
	tt := ui.NewTester(a.View, 1280, 820)
	if tt.HasText("later.txt") {
		t.Fatalf("the file showed before the tree was read")
	}
	a.refreshTree()
	tt.Frame()
	if !tt.HasText("later.txt") {
		t.Errorf("the tree did not pick up the new file")
	}
	if a.WS.FileCount() == 0 {
		t.Errorf("the index is empty")
	}
}
