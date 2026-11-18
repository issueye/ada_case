package app

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"lightcode/internal/aiplugin"
)

// TestM4Screenshots renders the assistant with a streamed answer and its
// diff card, docked and floating.
func TestM4Screenshots(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	srv := aiServer(t, aiDiffReply)
	defer srv.Close()
	a.AI.Config = aiplugin.Config{BaseURL: srv.URL, Model: "test", APIKey: "k"}
	a.AI.Draft = "给 main.go 加一行注释"
	a.SendAI()
	if len(a.AI.Messages) > 0 {
		if answer := a.AI.Messages[len(a.AI.Messages)-1]; len(answer.Diff) > 0 {
			answer.Diff[0].Open = true
		}
	}
	a.SetAIMode(AIDock)
	tt := ui.NewTester(a.View, 1280, 820)
	saveShot(t, tt, "lightcode-ui-ai.png")

	a.SetAIMode(AIFloat)
	tt.Frame()
	saveShot(t, tt, "lightcode-ui-ai-float.png")

	a.SetAIMode(AIRail)
	tt.Frame()
	tt.Move(1242, 418)
	tt.Frame()
	saveShot(t, tt, "lightcode-ui-ai-peek.png")

	// Drag the rail card over the code.
	tt.Press(a.Prefs.AIRailX+24, a.Prefs.AIRailY+60)
	tt.Move(720, 260)
	tt.Release(720, 260)
	tt.Frame()
	saveShot(t, tt, "lightcode-ui-ai-drag.png")
}
