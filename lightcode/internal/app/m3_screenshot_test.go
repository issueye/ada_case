package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestM3Screenshots renders the SCM panel and the diff page for the docs.
func TestM3Screenshots(t *testing.T) {
	a, _ := testGitWorkspace(t)
	a.ShowSidebar(2)
	tt := ui.NewTester(a.View, 1280, 820)
	saveShot(t, tt, "lightcode-ui-scm.png")

	a.openDiff("main.go", false)
	tt.Frame()
	saveShot(t, tt, "lightcode-ui-diff.png")
}
