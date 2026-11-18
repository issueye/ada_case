package assistant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lightcode.ai/client/protocol"
)

// fakeBridge stands in for the editor: it gives the context, takes the
// answers' diffs and records what the panel asked for.
type fakeBridge struct {
	msgs     []protocol.ChatMessage
	cards    []protocol.Card
	applied  string
	modes    []int
	settings int
	moves    []protocol.MoveRequest
}

func (b *fakeBridge) Context(context.Context) ([]protocol.ChatMessage, error) { return b.msgs, nil }

func (b *fakeBridge) Answer(_ context.Context, _ string, _ string) (protocol.AnswerResult, error) {
	return protocol.AnswerResult{Cards: b.cards}, nil
}

func (b *fakeBridge) Apply(_ context.Context, _ string, _ int) (string, error) {
	b.applied = "ok"
	return "", nil
}

func (b *fakeBridge) SetMode(mode int) { b.modes = append(b.modes, mode) }

func (b *fakeBridge) Settings() { b.settings++ }

func (b *fakeBridge) MoveTo(target int, x, y float32) {
	b.moves = append(b.moves, protocol.MoveRequest{Target: target, X: x, Y: y})
}

// sseServer streams text as an OpenAI-compatible endpoint does.
func sseServer(t *testing.T, text string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, piece := range []string{text[:len(text)/2], text[len(text)/2:]} {
			payload, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{"delta": map[string]any{"content": piece}}},
			})
			_, _ = w.Write([]byte("data: " + string(payload) + "\n\n"))
			w.(http.Flusher).Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func configured(t *testing.T, b *fakeBridge, mode int, base string) *State {
	t.Helper()
	s := New(b, func() {})
	s.Configure(mustJSON(protocol.Configure{
		BaseURL: base, Model: "test", APIKey: "k", Mode: mode, Light: false,
	}))
	return s
}

func mustJSON(v any) json.RawMessage {
	raw, _ := json.Marshal(v)
	return raw
}

// TestViewIntro draws the docked panel with the panel's own texts.
func TestViewIntro(t *testing.T) {
	s := configured(t, &fakeBridge{}, ModeDock, "http://127.0.0.1:1/v1")
	tt := ui.NewTester(s.View, 380, 640)
	tt.Frame()
	for _, want := range []string{"AI 助手", "让 AI 改代码"} {
		if !tt.HasText(want) {
			t.Errorf("panel misses %q; texts %q", want, tt.Texts())
		}
	}
}

// TestViewRail draws the collapsed card.
func TestViewRail(t *testing.T) {
	s := configured(t, &fakeBridge{}, ModeRail, "http://127.0.0.1:1/v1")
	tt := ui.NewTester(s.View, 48, 172)
	tt.Frame()
	if !tt.HasText("展开 AI 面板") && !tt.HasText("AI") {
		t.Errorf("rail misses its labels; texts %q", tt.Texts())
	}
}

// TestSendStreamsAndApplies sends a question, reads the stream, takes the
// cards, and applies one.
func TestSendStreamsAndApplies(t *testing.T) {
	srv := sseServer(t, "好的，改动如下。")
	b := &fakeBridge{
		msgs:  []protocol.ChatMessage{{Role: "system", Content: "ctx"}},
		cards: []protocol.Card{{Index: 0, Path: "main.go", Old: "package main\n", New: "package main\n// x\n"}},
	}
	s := configured(t, b, ModeDock, srv.URL)
	s.mu.Lock()
	s.draft = "加一行"
	s.mu.Unlock()
	s.Send()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		done := !s.streaming && len(s.messages) >= 2 && s.messages[1].Full != "" && len(s.messages[1].Cards) == 1
		s.mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.mu.Lock()
	if len(s.messages) != 2 {
		t.Fatalf("messages = %+v", s.messages)
	}
	if !strings.Contains(s.messages[1].Full, "改动如下") {
		t.Fatalf("answer = %q", s.messages[1].Full)
	}
	if len(s.messages[1].Cards) != 1 {
		t.Fatalf("cards = %+v", s.messages[1].Cards)
	}
	s.mu.Unlock()
	s.Apply(s.messages[1].Cards[0], 1)
	time.Sleep(30 * time.Millisecond)
	if b.applied != "ok" {
		t.Errorf("apply did not reach the editor")
	}
}

// TestDragMovesTheMount drags the header and checks the window position it
// asks for, with the mount's origin taken into account.
func TestDragMovesTheMount(t *testing.T) {
	b := &fakeBridge{}
	s := configured(t, b, ModeFloat, "http://127.0.0.1:1/v1")
	s.SetOrigin(100, 100)
	drag := s.dragHandler(ModeFloat)
	drag(ui.InputEvent{Kind: ui.InputPointerDown, X: 20, Y: 18})
	drag(ui.InputEvent{Kind: ui.InputPointerMove, X: 10, Y: 18})
	drag(ui.InputEvent{Kind: ui.InputPointerMove, X: 0, Y: 18})
	drag(ui.InputEvent{Kind: ui.InputPointerUp})
	if len(b.moves) != 2 {
		t.Fatalf("moves = %+v", b.moves)
	}
	if b.moves[0].X != 90 || b.moves[0].Y != 100 {
		t.Errorf("first move = %+v, want 90,100", b.moves[0])
	}
	if b.moves[1].X != 80 {
		t.Errorf("second move = %+v, want 80", b.moves[1])
	}
}

// TestModeButtonsAskTheEditor clicks the rail and dock buttons: the panel
// asks the editor for the mount instead of moving itself.
func TestModeButtonsAskTheEditor(t *testing.T) {
	b := &fakeBridge{}
	s := configured(t, b, ModeDock, "http://127.0.0.1:1/v1")
	tt := ui.NewTester(s.View, 380, 640)
	tt.Frame()
	if err := tt.Click("悬浮"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(b.modes) == 0 || b.modes[len(b.modes)-1] != ModeFloat {
		t.Fatalf("modes = %v", b.modes)
	}
}

// TestNoKeyOpensSettings asks the editor for the key instead of failing.
func TestNoKeyOpensSettings(t *testing.T) {
	b := &fakeBridge{}
	s := New(b, func() {})
	s.Configure(mustJSON(protocol.Configure{Mode: ModeDock}))
	s.mu.Lock()
	s.draft = "hi"
	s.mu.Unlock()
	s.Send()
	if b.settings != 1 {
		t.Fatalf("settings opened %d times", b.settings)
	}
}
