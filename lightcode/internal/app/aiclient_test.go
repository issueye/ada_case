package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"lightcode.ai/client/protocol"
)

const aiDiffAnswer = "这次改动加了一行注释：\n```diff\n" +
	"diff --git a/main.go b/main.go\n" +
	"--- a/main.go\n" +
	"+++ b/main.go\n" +
	"@@ -1,2 +1,3 @@\n" +
	" package main\n" +
	"+// ai added\n" +
	" \n" +
	"```\n"

// TestAIPluginCommandFromEnv points the editor at the panel's own build.
func TestAIPluginCommandFromEnv(t *testing.T) {
	t.Setenv("LIGHTCODE_AI_PLUGIN", `C:\tools\lightcode-ai.exe`)
	cmd, err := aiPluginCommand()
	if err != nil {
		t.Fatal(err)
	}
	if len(cmd) != 1 || cmd[0] != `C:\tools\lightcode-ai.exe` {
		t.Fatalf("command = %v", cmd)
	}
}

// TestAIPluginCommandMissing says which binary is missing and how to build
// it: the panel is a separate project, so nothing falls back silently.
func TestAIPluginCommandMissing(t *testing.T) {
	t.Setenv("LIGHTCODE_AI_PLUGIN", "")
	t.Setenv("PATH", "")
	if _, err := aiPluginCommand(); err == nil {
		t.Fatalf("a missing panel was found")
	} else if !strings.Contains(err.Error(), "lightcode-ai") {
		t.Fatalf("error = %v", err)
	}
}

// TestAIPluginName names the panel per platform.
func TestAIPluginName(t *testing.T) {
	if !strings.HasPrefix(aiPluginName(), "lightcode-ai") {
		t.Fatalf("name = %q", aiPluginName())
	}
}

// TestAIPanelText says what the panel is doing, including why it last
// stopped: the capabilities view shows this line.
func TestAIPanelText(t *testing.T) {
	for _, tc := range []struct {
		st   aiPanelStatus
		want string
	}{
		{aiPanelStatus{Running: true}, "运行中 · " + protocol.Name + " v" + protocol.Version},
		{aiPanelStatus{Err: "找不到 AI 面板"}, "未启动：找不到 AI 面板"},
		{aiPanelStatus{}, "按需启动（首次显示面板时拉起）"},
	} {
		if got := aiPanelText(tc.st); got != tc.want {
			t.Errorf("text = %q, want %q", got, tc.want)
		}
	}
}

// TestAIPanelRow shows the panel's row in the capabilities view before
// anything ran.
func TestAIPanelRow(t *testing.T) {
	a, _ := testWorkspace(t)
	a.Prefs.AIEnabled = false
	a.ShowSidebar(4)
	tt := ui.NewTester(a.View, 1280, 820)
	if !tt.HasText("AI 进程插件") {
		t.Fatalf("no panel row; texts %q", tt.Texts())
	}
	if !tt.HasText("按需启动（首次显示面板时拉起）") {
		t.Errorf("idle panel text missing; texts %q", tt.Texts())
	}
}

// TestAIMountNeedsAWindow draws the mount of a headless app: it says why
// there is no panel, instead of drawing a blank.
func TestAIMountNeedsAWindow(t *testing.T) {
	t.Setenv("LIGHTCODE_AI_PLUGIN", "")
	t.Setenv("PATH", "")
	a, _ := testWorkspace(t)
	a.AI.Mode = AIDock
	tt := ui.NewTester(a.View, 1280, 820)
	if !tt.HasText("AI 面板需要窗口") {
		t.Fatalf("no hint; texts %q", tt.Texts())
	}
	if !tt.HasText("重试") {
		t.Errorf("no retry button; texts %q", tt.Texts())
	}
}

// TestAIContextCarriesFile builds the next question's context from the open
// file.
func TestAIContextCarriesFile(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	msgs := a.aiContextMessages()
	if len(msgs) < 2 || msgs[0].Role != "system" {
		t.Fatalf("context = %+v", msgs)
	}
	last := msgs[len(msgs)-1]
	if !strings.Contains(last.Content, "main.go") || !strings.Contains(last.Content, "package main") {
		t.Fatalf("file context = %q", last.Content)
	}
}

// TestAICardsAndApply turns an answer's diff into a card, then applies it
// to the buffer only when asked.
func TestAICardsAndApply(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	cards := a.aiCards(protocol.AnswerRequest{RequestID: "r1", Text: aiDiffAnswer})
	if len(cards) != 1 {
		t.Fatalf("cards = %+v", cards)
	}
	if cards[0].Path != "main.go" || cards[0].Err != "" {
		t.Fatalf("card = %+v", cards[0])
	}
	if !strings.Contains(cards[0].New, "// ai added") || strings.Contains(cards[0].Old, "// ai added") {
		t.Fatalf("card old/new = %q / %q", cards[0].Old, cards[0].New)
	}
	if strings.Contains(a.Col().Current().Doc.Buffer.String(), "// ai added") {
		t.Fatalf("the buffer changed before apply")
	}
	if err := a.aiApply(protocol.ApplyRequest{RequestID: "r1", Index: 0}); err != "" {
		t.Fatalf("apply: %s", err)
	}
	if !a.Col().Current().Doc.Dirty() {
		t.Errorf("applying did not dirty the document")
	}
	if !strings.Contains(a.Col().Current().Doc.Buffer.String(), "// ai added") {
		t.Errorf("the change is not in the buffer")
	}
	if err := a.aiApply(protocol.ApplyRequest{RequestID: "r1", Index: 9}); err == "" {
		t.Errorf("an out-of-range card applied")
	}
}

// TestAIModePersists restores how the panel showed.
func TestAIModePersists(t *testing.T) {
	a, dir := testWorkspace(t)
	a.SetAIMode(AIFloat)
	a2 := New(dir, "")
	if a2.AI.Mode != AIFloat {
		t.Errorf("restored AI mode = %d", a2.AI.Mode)
	}
}

// TestAISettingsPersist saves the endpoint and model in the session.
func TestAISettingsPersist(t *testing.T) {
	a, dir := testWorkspace(t)
	a.OpenAISettings()
	a.AI.SetBase = "http://127.0.0.1:9999/v1"
	a.AI.SetModel = "local-model"
	a.SaveAISettings()
	a2 := New(dir, "")
	if a2.AI.Config.BaseURL != "http://127.0.0.1:9999/v1" || a2.AI.Config.Model != "local-model" {
		t.Errorf("restored config = %+v", a2.AI.Config)
	}
}

// TestAISecret keeps the key out of the session file and in the store.
func TestAISecret(t *testing.T) {
	a, _ := testWorkspace(t)
	a.OpenAISettings()
	a.AI.SetKey = "sk-test"
	a.SaveAISettings()
	if a.AI.Config.APIKey != "sk-test" {
		t.Fatalf("key = %q", a.AI.Config.APIKey)
	}
	raw, err := os.ReadFile(a.StatePath)
	if err == nil && strings.Contains(string(raw), "sk-test") {
		t.Errorf("the session file holds the key")
	}
}
