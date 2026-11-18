package serve_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/learningdog1/ada_link/link"

	"lightcode.ai/client/protocol"
	"lightcode.ai/client/serve"
)

// streamServer answers like an OpenAI-compatible endpoint: one delta per
// piece, then [DONE].
func streamServer(t *testing.T, pieces []string, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flush, _ := w.(http.Flusher)
		for _, p := range pieces {
			payload, _ := json.Marshal(map[string]any{
				"choices": []any{map[string]any{"delta": map[string]any{"content": p}}},
			})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			if flush != nil {
				flush.Flush()
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(delay):
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	return srv
}

// start runs the plugin over two in-memory pipes and returns the editor's
// end, already handshaken.
func start(t *testing.T) *link.Peer {
	t.Helper()
	var peer *link.Peer
	// pluginIn ← the editor writes; pluginOut → the editor reads.
	pluginIn, editorOut := io.Pipe()
	editorIn, pluginOut := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- serve.Run(serve.Options{Stdin: pluginIn, Stdout: pluginOut, Timeout: 3 * time.Second})
	}()
	t.Cleanup(func() {
		peer.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})
	peer = link.NewQuietPeer(link.NewPipeConn(editorIn, editorOut))
	welcome := make(chan *link.Message, 1)
	peer.OnMessage(func(m *link.Message) bool {
		if m.T != "welcome" {
			return false
		}
		select {
		case welcome <- m:
		default:
		}
		return true
	})
	peer.Start()
	if err := peer.Send(&link.Message{T: "hello", Proto: protocol.Proto, App: "lightcode"}); err != nil {
		t.Fatal(err)
	}
	select {
	case w := <-welcome:
		if w.Proto != protocol.Proto {
			t.Fatalf("welcome speaks %d", w.Proto)
		}
		var who protocol.Welcome
		if err := json.Unmarshal(w.Payload, &who); err != nil {
			t.Fatalf("welcome payload: %v", err)
		}
		if who.Name != protocol.Name || len(who.Methods) != 2 {
			t.Fatalf("welcome = %+v", who)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no welcome")
	}
	return peer
}

// TestStream answers a request and checks the deltas add up to the reply.
func TestStream(t *testing.T) {
	srv := streamServer(t, []string{"你", "好", "，", "世界"}, 0)
	peer := start(t)
	var mu sync.Mutex
	var pieces []string
	off := peer.OnEvent(protocol.EventDelta, func(raw json.RawMessage) {
		var d protocol.Delta
		if err := json.Unmarshal(raw, &d); err != nil {
			return
		}
		mu.Lock()
		pieces = append(pieces, d.Text)
		mu.Unlock()
	})
	defer off()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := peer.Call(ctx, protocol.MethodStream, protocol.StreamRequest{
		RequestID: "r1",
		Config:    protocol.Config{BaseURL: srv.URL, Model: "test", APIKey: "k"},
		Messages:  []protocol.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	var res protocol.StreamResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}
	if res.Text != "你好，世界" {
		t.Errorf("answer = %q", res.Text)
	}
	mu.Lock()
	joined := strings.Join(pieces, "")
	mu.Unlock()
	if joined != res.Text {
		t.Errorf("deltas = %q, answer = %q", joined, res.Text)
	}
}

// TestCancel stops an answer half way through: the reply carries the
// context's error, and the plugin keeps serving.
func TestCancel(t *testing.T) {
	srv := streamServer(t, []string{"一", "二", "三", "四", "五", "六"}, 60*time.Millisecond)
	peer := start(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var once sync.Once
	off := peer.OnEvent(protocol.EventDelta, func(json.RawMessage) { once.Do(cancel) })
	defer off()
	_, err := peer.Call(ctx, protocol.MethodStream, protocol.StreamRequest{
		RequestID: "r2",
		Config:    protocol.Config{BaseURL: srv.URL, Model: "test", APIKey: "k"},
		Messages:  []protocol.ChatMessage{{Role: "user", Content: "long"}},
	})
	if !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("cancel gave %v", err)
	}
	// The plugin keeps serving after a canceled answer.
	raw, err := peer.Call(context.Background(), protocol.MethodStream, protocol.StreamRequest{
		RequestID: "r3",
		Config:    protocol.Config{BaseURL: srv.URL, Model: "test", APIKey: "k"},
		Messages:  []protocol.ChatMessage{{Role: "user", Content: "again"}},
	})
	if err != nil {
		t.Fatalf("after cancel: %v", err)
	}
	var res protocol.StreamResult
	if err := json.Unmarshal(raw, &res); err != nil || res.Text == "" {
		t.Fatalf("second answer = %q (%v)", res.Text, err)
	}
}

// TestNoKey answers an explicit error instead of calling anything.
func TestNoKey(t *testing.T) {
	peer := start(t)
	_, err := peer.Call(context.Background(), protocol.MethodStream, protocol.StreamRequest{
		RequestID: "r4",
		Config:    protocol.Config{},
		Messages:  []protocol.ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err == nil || !strings.Contains(err.Error(), "API Key") {
		t.Fatalf("no key gave %v", err)
	}
}

func TestVersionFlag(t *testing.T) {
	if protocol.Name == "" || protocol.Version == "" {
		t.Fatal("the plugin has no name or version")
	}
}
