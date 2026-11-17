package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
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
