package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSplitSharesDocument splits the editor and checks both columns show
// one Document, so an edit and its undo are one history.
func TestSplitSharesDocument(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	a.RunCommand("view.split")
	if len(a.Cols) != 2 {
		t.Fatalf("columns = %d, want 2", len(a.Cols))
	}
	d := a.Cols[0].Current().Doc
	if a.Cols[1].Current().Doc != d {
		t.Fatalf("the second column shows another document")
	}
	a.ActiveCol = 1
	d.Buffer.Set("package main\n\n// split edit\n")
	if !a.Cols[0].Current().Doc.Dirty() {
		t.Errorf("the edit is not seen from the other column")
	}
}

// TestSplitByKey splits with Ctrl+\ and closes with it again, keeping the
// tabs of both columns.
func TestSplitByKey(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Key(ui.Cmd, ui.KeyBackslash)
	if len(a.Cols) != 2 {
		t.Fatalf("Ctrl+\\ did not split, columns = %d", len(a.Cols))
	}
	a.ActiveCol = 1
	a.OpenFile(filepath.Join(dir, "pkg", "util.go"))
	tt.Frame()
	// The split draws two tab strips, so util.go shows.
	if !tt.HasText("util.go") {
		t.Errorf("the second column's tab is missing")
	}
	tt.Key(ui.Cmd, ui.KeyBackslash)
	if len(a.Cols) != 1 {
		t.Fatalf("Ctrl+\\ did not close the split, columns = %d", len(a.Cols))
	}
	if len(a.Cols[0].Editors) != 2 {
		t.Errorf("closing the split lost tabs: %d", len(a.Cols[0].Editors))
	}
}

// TestSplitRestores restores a split session with its two columns and the
// chosen one.
func TestSplitRestores(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	a.RunCommand("view.split")
	a.ActiveCol = 1
	a.OpenFile(filepath.Join(dir, "pkg", "util.go"))
	a.SplitPercent = 40
	a.SaveState()

	a2 := New(dir, "")
	if len(a2.Cols) != 2 {
		t.Fatalf("restored %d columns, want 2", len(a2.Cols))
	}
	if a2.ActiveCol != 1 {
		t.Errorf("restored active column = %d", a2.ActiveCol)
	}
	if len(a2.Cols[1].Editors) != 2 {
		t.Errorf("second column has %d tabs, want 2", len(a2.Cols[1].Editors))
	}
	if a2.SplitPercent != 40 {
		t.Errorf("split percent = %v", a2.SplitPercent)
	}
}

// TestClosingLastTabOfSplitCollapses removes the split when its last tab
// goes, so the window does not keep an empty pane.
func TestClosingLastTabOfSplitCollapses(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	a.RunCommand("view.split")
	a.ActiveCol = 1
	e := a.Cols[1].Current()
	a.Cols[1].Remove(0)
	a.collapseEmptyColumns()
	if len(a.Cols) != 1 || a.ActiveCol != 0 {
		t.Errorf("split did not collapse: %d columns, active %d", len(a.Cols), a.ActiveCol)
	}
	_ = e
}
