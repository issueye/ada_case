package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestSearchFindsAndOpens searches the workspace and opens a match at its
// line.
func TestSearchFindsAndOpens(t *testing.T) {
	a, _ := testWorkspace(t)
	a.resetColumns()
	a.Search.Query = "Add"
	a.RunSearchNow()
	if len(a.Search.Results) != 1 {
		t.Fatalf("results = %+v", a.Search.Results)
	}
	m := a.Search.Results[0]
	if m.Path != "pkg/util.go" || m.Line != 3 || m.Text != "func Add(a, b int) int {" {
		t.Fatalf("match = %+v", m)
	}
	a.openMatch(m)
	e := a.Col().Current()
	if e == nil || e.Doc.Name() != "util.go" {
		t.Fatalf("opened %v", e)
	}
	if pos := e.State.CursorPos(); pos.Line != m.Line-1 || pos.Col != m.Col-1 {
		t.Errorf("cursor = %+v, want line %d col %d", pos, m.Line, m.Col)
	}

}

// TestSearchPanelTypes searches as the panel is typed into and shows the
// result summary.
func TestSearchPanelTypes(t *testing.T) {
	a, _ := testWorkspace(t)
	a.ShowSidebar(1)
	tt := ui.NewTester(a.View, 1280, 820)
	if !tt.HasText("搜索工作区") {
		t.Fatalf("the search hint is missing; texts %q", tt.Texts())
	}
	tt.Type("Add")
	tt.Key(0, ui.KeyEnter)
	if !tt.HasText("1 个结果") {
		t.Fatalf("no result summary; texts %q", tt.Texts())
	}
}
