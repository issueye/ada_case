package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestM2Screenshots renders the search panel and the outline for the docs.
func TestM2Screenshots(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "pkg", "util.go"))
	tt := ui.NewTester(a.View, 1280, 820)
	saveShot(t, tt, "lightcode-ui-outline.png")

	a.ShowSidebar(1)
	a.Search.Query = "Add"
	a.RunSearchNow()
	tt.Frame()
	saveShot(t, tt, "lightcode-ui-search.png")
}
