package aiplugin_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"lightcode/internal/aiplugin"
	"lightcode/plugins/aiprovider"
)

// TestMain doubles as the plugin process: the client spawns this test
// binary, which finds LIGHTCODE_AI_PLUGIN_HELPER and serves the protocol
// instead of running the tests. That keeps the end-to-end test on the real
// plugin, with a real process and real pipes, rather than a stand-in.
func TestMain(m *testing.M) {
	switch os.Getenv("LIGHTCODE_AI_PLUGIN_HELPER") {
	case "":
		os.Exit(m.Run())
	case "serve":
		if err := aiprovider.Serve(aiprovider.Options{}); err != nil {
			fmt.Fprintln(os.Stderr, "helper:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "proto9":
		hello, err := aiplugin.ReadMessage(os.Stdin)
		if err != nil {
			os.Exit(1)
		}
		_ = hello
		_ = aiplugin.WriteMessage(os.Stdout, &aiplugin.Message{T: "welcome", Proto: aiplugin.Proto + 8})
		os.Exit(0)
	default:
		os.Exit(3)
	}
}

// pluginClient makes a client that spawns this test binary as the plugin.
func pluginClient(t *testing.T, helper string) *aiplugin.Client {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := aiplugin.New(aiplugin.Options{
		Command: []string{exe},
		Env:     []string{"LIGHTCODE_AI_PLUGIN_HELPER=" + helper},
		Timeout: 10 * time.Second,
		Stderr:  os.Stderr,
	})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

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
		if flush != nil {
			flush.Flush()
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestStreamThroughProcess asks the spawned plugin for an answer and
// checks the text arrived both as deltas and whole.
func TestStreamThroughProcess(t *testing.T) {
	srv := streamServer(t, []string{"你", "好", "，", "世界"}, 0)
	c := pluginClient(t, "serve")
	var mu sync.Mutex
	var pieces []string
	text, err := c.Stream(context.Background(),
		aiplugin.Config{BaseURL: srv.URL, Model: "test", APIKey: "k"},
		[]aiplugin.ChatMessage{{Role: "user", Content: "问候一下"}},
		func(piece string) {
			mu.Lock()
			pieces = append(pieces, piece)
			mu.Unlock()
		})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if text != "你好，世界" {
		t.Errorf("answer = %q", text)
	}
	mu.Lock()
	joined := strings.Join(pieces, "")
	mu.Unlock()
	if joined != text {
		t.Errorf("deltas = %q, whole = %q", joined, text)
	}
	st := c.Status()
	if !st.Running || st.PID == 0 {
		t.Fatalf("status = %+v", st)
	}
	if st.PID == os.Getpid() {
		t.Fatalf("the answer was read in the host process (pid %d): the plugin must be its own process", st.PID)
	}
	if st.Name != aiplugin.Name || st.Version != aiplugin.Version {
		t.Errorf("welcome named %q %q", st.Name, st.Version)
	}
}

// TestStreamCancel stops an answer half way: the call fails with the
// context's error, keeps what arrived, and the plugin is told to stop.
func TestStreamCancel(t *testing.T) {
	srv := streamServer(t, []string{"一", "二", "三", "四", "五", "六", "七", "八"}, 60*time.Millisecond)
	c := pluginClient(t, "serve")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var once sync.Once
	text, err := c.Stream(ctx,
		aiplugin.Config{BaseURL: srv.URL, Model: "test", APIKey: "k"},
		[]aiplugin.ChatMessage{{Role: "user", Content: "长一点"}},
		func(string) { once.Do(cancel) })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel gave %v", err)
	}
	if text == "" || strings.Contains(text, "八") {
		t.Errorf("partial answer = %q", text)
	}
	// The process keeps serving after a cancel.
	text2, err := c.Stream(context.Background(),
		aiplugin.Config{BaseURL: srv.URL, Model: "test", APIKey: "k"},
		[]aiplugin.ChatMessage{{Role: "user", Content: "再问"}}, nil)
	if err != nil || text2 != "一二三四五六七八" {
		t.Fatalf("after cancel: %q %v", text2, err)
	}
}

// TestPluginRestarts kills the process and checks the next call starts
// another one, rather than failing for the rest of the session.
func TestPluginRestarts(t *testing.T) {
	srv := streamServer(t, []string{"ok"}, 0)
	c := pluginClient(t, "serve")
	cfg := aiplugin.Config{BaseURL: srv.URL, Model: "test", APIKey: "k"}
	if _, err := c.Stream(context.Background(), cfg, []aiplugin.ChatMessage{{Role: "user", Content: "1"}}, nil); err != nil {
		t.Fatal(err)
	}
	st := c.Status()
	if st.PID == 0 {
		t.Fatalf("no pid: %+v", st)
	}
	proc, err := os.FindProcess(st.PID)
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatalf("kill %d: %v", st.PID, err)
	}
	// Wait until the host notices, as it does before the next call.
	deadline := time.Now().Add(5 * time.Second)
	for c.Status().Running && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	text, err := c.Stream(context.Background(), cfg, []aiplugin.ChatMessage{{Role: "user", Content: "2"}}, nil)
	if err != nil {
		t.Fatalf("the plugin did not come back: %v", err)
	}
	if text != "ok" {
		t.Errorf("answer = %q", text)
	}
	if c.Status().PID == st.PID {
		t.Errorf("the restarted plugin has the same pid %d", st.PID)
	}
}

// TestPluginProtocolMismatch refuses a plugin that speaks another version.
func TestPluginProtocolMismatch(t *testing.T) {
	c := pluginClient(t, "proto9")
	_, err := c.Stream(context.Background(), aiplugin.Config{APIKey: "k"},
		[]aiplugin.ChatMessage{{Role: "user", Content: "hi"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("mismatch = %v", err)
	}
}

// TestPluginStartFails reports a plugin that exits before its welcome.
func TestPluginStartFails(t *testing.T) {
	c := pluginClient(t, "exit")
	_, err := c.Stream(context.Background(), aiplugin.Config{APIKey: "k"},
		[]aiplugin.ChatMessage{{Role: "user", Content: "hi"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "exited") {
		t.Fatalf("start failure = %v", err)
	}
}

// TestStreamNeedsKey keeps the key requirement in the provider rather than
// only in the window.
func TestStreamNeedsKey(t *testing.T) {
	_, err := aiprovider.Stream(context.Background(), aiplugin.Config{},
		[]aiplugin.ChatMessage{{Role: "user", Content: "hi"}}, nil)
	if !errors.Is(err, aiprovider.ErrNoKey) {
		t.Fatalf("no key gave %v", err)
	}
}
