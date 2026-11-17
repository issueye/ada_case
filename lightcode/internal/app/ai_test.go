package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"lightcode/internal/ai"
)

// aiServer streams reply as one delta, as an OpenAI-compatible endpoint
// does in pieces.
func aiServer(t *testing.T, reply string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		payload, _ := json.Marshal(map[string]any{
			"choices": []any{map[string]any{"delta": map[string]any{"content": reply}}},
		})
		w.Write([]byte("data: " + string(payload) + "\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
}

const aiDiffReply = "这次改动加了一行注释：\n```diff\n" +
	"diff --git a/main.go b/main.go\n" +
	"--- a/main.go\n" +
	"+++ b/main.go\n" +
	"@@ -1,2 +1,3 @@\n" +
	" package main\n" +
	"+// ai added\n" +
	" \n" +
	"```\n"

// TestAIStreamsAndAppliesDiff sends a question, reads the streamed answer
// and applies the diff it produced, only after the user's click.
func TestAIStreamsAndAppliesDiff(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	srv := aiServer(t, aiDiffReply)
	defer srv.Close()
	a.AI.Config = ai.Config{BaseURL: srv.URL, Model: "test", APIKey: "k"}
	a.AI.Draft = "加一行注释"
	a.SendAI()
	if a.AI.Streaming {
		// No window: the call runs inline and is done already.
		t.Fatalf("stream still running after a synchronous call")
	}
	if len(a.AI.Messages) < 2 {
		t.Fatalf("messages = %+v", a.AI.Messages)
	}
	answer := a.AI.Messages[len(a.AI.Messages)-1]
	if !strings.Contains(answer.Content, "这次改动") || strings.Contains(answer.Content, "请求失败") {
		t.Fatalf("answer = %q", answer.Content)
	}
	if len(answer.Diff) != 1 {
		t.Fatalf("diff cards = %d, want 1: %q", len(answer.Diff), answer.Content)
	}
	card := answer.Diff[0]
	if card.Path != "main.go" || card.Applied {
		t.Fatalf("card = %+v", card)
	}
	if !strings.Contains(a.Col().Current().Doc.Buffer.String(), "package main") {
		t.Fatalf("the file changed before the click")
	}
	a.ApplyAIDiff(card)
	if !card.Applied {
		t.Errorf("the card was not marked applied")
	}
	if !a.Col().Current().Doc.Dirty() {
		t.Errorf("applying the diff did not dirty the document")
	}
	if !strings.Contains(a.Col().Current().Doc.Buffer.String(), "// ai added") {
		t.Errorf("the change is not in the buffer: %q", a.Col().Current().Doc.Buffer.String())
	}
}

// TestAINoKeyOpensSettings asks for the key instead of failing silently.
func TestAINoKeyOpensSettings(t *testing.T) {
	a, _ := testWorkspace(t)
	a.AI.Config.APIKey = ""
	a.AI.Draft = "hi"
	a.SendAI()
	if !a.AI.SettingsOpen {
		t.Errorf("no settings opened for a missing key")
	}
	if !strings.Contains(a.AI.Err, "API Key") {
		t.Errorf("error = %q", a.AI.Err)
	}
}

// TestAIRetryResends resends the last question on a failure.
func TestAIRetryResends(t *testing.T) {
	a, _ := testWorkspace(t)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer bad.Close()
	a.AI.Config = ai.Config{BaseURL: bad.URL, Model: "m", APIKey: "k"}
	a.AI.Draft = "问一次"
	a.SendAI()
	last := a.AI.Messages[len(a.AI.Messages)-1]
	if last.Err == "" {
		t.Fatalf("no error was kept: %+v", last)
	}
	a.RetryAI()
	if len(a.AI.Messages) < 2 || a.AI.Messages[0].Content != "问一次" {
		t.Errorf("retry lost the question: %+v", a.AI.Messages)
	}
	if a.AI.Messages[len(a.AI.Messages)-1].Err == "" {
		t.Errorf("the retry did not fail too")
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

// TestAISettingsPersist saves the endpoint and model, not the key.
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

// TestAIFloatOverlay shows the floating panel above the editor.
func TestAIFloatOverlay(t *testing.T) {
	a, _ := testWorkspace(t)
	a.SetAIMode(AIFloat)
	tt := ui.NewTester(a.View, 1280, 820)
	if !tt.HasText("AI 助手") {
		t.Fatalf("no floating panel; texts %q", tt.Texts())
	}
	if !tt.HasText("Enter 发送 · 选中代码会作为上下文 · diff 需点「应用」") {
		t.Errorf("no input hints")
	}
}

// TestAITypewriterReveal shows the answer a step at a time off the view:
// the reveal is paced by time, so the panel itself never asks for a frame
// to keep typing.
func TestAITypewriterReveal(t *testing.T) {
	a, _ := testWorkspace(t)
	a.AI.Messages = []aiMessage{{Role: "assistant", Full: "你好，世界！这是打字机式的输出。"}}
	a.AI.Streaming = true
	m := &a.AI.Messages[0]
	full := []rune(m.Full)
	a.advanceAIReveal(m)
	got := []rune(m.Content)
	if len(got) >= len(full) {
		t.Fatalf("the whole answer showed at once: %q", m.Content)
	}
	for i := 0; i < 100 && len(got) < len(full); i++ {
		a.advanceAIReveal(m)
		got = []rune(m.Content)
	}
	if string(got) != string(full) {
		t.Fatalf("reveal stopped at %q", m.Content)
	}
	if a.AI.RevealPending {
		t.Errorf("the reveal still reports a backlog after showing it all")
	}
	a.AI.Streaming = false
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Frame()
	if tt.FramePending() {
		t.Errorf("the view keeps asking for frames after the reveal")
	}
}

// TestAIRailHoverPeek previews the last answer when the rail is hovered,
// and hides it when the pointer leaves.
func TestAIRailHoverPeek(t *testing.T) {
	a, _ := testWorkspace(t)
	a.AI.Mode = AIRail
	a.AI.Messages = []aiMessage{{Role: "assistant", Content: "上一轮的回答摘要", Full: "上一轮的回答摘要"}}
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Move(1242, 418)
	tt.Frame()
	if !tt.HasText("点击卡片展开") {
		t.Fatalf("no peek; texts %q", tt.Texts())
	}
	tt.Move(600, 400)
	tt.Frame()
	if tt.HasText("点击卡片展开") {
		t.Errorf("the peek stayed after the pointer left")
	}
}
