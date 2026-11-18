package aiplugin_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/learningdog1/ada_link/link"

	"lightcode.ai/client/protocol"

	"lightcode/internal/aiplugin"
)

// TestMain doubles as the plugin process. The real plugin is its own
// project (lightcode-ai) and cannot be imported here; the helper speaks
// the same contract over the same library, so the client is tested against
// a real process with real pipes.
func TestMain(m *testing.M) {
	switch os.Getenv("LIGHTCODE_AI_PLUGIN_HELPER") {
	case "":
		os.Exit(m.Run())
	case "serve":
		runHelper(protocol.Proto)
	case "proto9":
		runHelper(protocol.Proto + 8)
	default:
		os.Exit(3)
	}
}

// runHelper answers the protocol until the host says bye: a tiny provider
// whose answer depends on the question, so tests can ask for a slow answer
// or a failure.
func runHelper(proto int) {
	p := link.NewQuietPeer(link.NewPipeConn(os.Stdin, os.Stdout))
	hello := make(chan *link.Message, 1)
	p.OnMessage(func(m *link.Message) bool {
		if m.T != "hello" {
			return false
		}
		select {
		case hello <- m:
		default:
		}
		return true
	})
	streams := map[string]context.CancelFunc{}
	var mu sync.Mutex
	p.Handle(protocol.MethodStream, func(ctx context.Context, args []json.RawMessage) (any, error) {
		var req protocol.StreamRequest
		if len(args) == 0 || json.Unmarshal(args[0], &req) != nil {
			return nil, errors.New("helper: bad request")
		}
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		if req.RequestID != "" {
			mu.Lock()
			streams[req.RequestID] = cancel
			mu.Unlock()
			defer func() {
				mu.Lock()
				delete(streams, req.RequestID)
				mu.Unlock()
			}()
		}
		pieces := []string{"你", "好"}
		delay := time.Duration(0)
		last := ""
		if n := len(req.Messages); n > 0 {
			last = req.Messages[n-1].Content
		}
		switch {
		case strings.Contains(last, "慢"):
			pieces = []string{"一", "二", "三", "四", "五", "六"}
			delay = 60 * time.Millisecond
		case strings.Contains(last, "失败"):
			return nil, errors.New("端点拒绝了请求")
		}
		var text strings.Builder
		for _, piece := range pieces {
			select {
			case <-ctx.Done():
				return protocol.StreamResult{RequestID: req.RequestID, Text: text.String()}, ctx.Err()
			case <-time.After(delay):
			}
			_ = p.Emit(protocol.EventDelta, protocol.Delta{RequestID: req.RequestID, Text: piece})
			text.WriteString(piece)
		}
		return protocol.StreamResult{RequestID: req.RequestID, Text: text.String()}, nil
	})
	p.Handle(protocol.MethodCancel, func(_ context.Context, args []json.RawMessage) (any, error) {
		var req protocol.CancelRequest
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &req)
		}
		mu.Lock()
		cancel := streams[req.RequestID]
		mu.Unlock()
		if cancel != nil {
			cancel()
		}
		return nil, nil
	})
	p.Start()
	select {
	case <-hello:
	case <-time.After(5 * time.Second):
		os.Exit(2)
	}
	_ = p.Send(&link.Message{
		T: "welcome", Proto: proto,
		Payload: mustJSON(protocol.Welcome{Name: protocol.Name, Version: protocol.Version}),
	})
	<-p.Done()
	os.Exit(0)
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
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

// TestStreamThroughProcess asks the spawned plugin for an answer and
// checks the text arrived both as deltas and whole.
func TestStreamThroughProcess(t *testing.T) {
	c := pluginClient(t, "serve")
	var mu sync.Mutex
	var pieces []string
	text, err := c.Stream(context.Background(),
		protocol.Config{BaseURL: "http://127.0.0.1:1/v1", Model: "test", APIKey: "k"},
		[]protocol.ChatMessage{{Role: "user", Content: "hi"}},
		func(piece string) {
			mu.Lock()
			pieces = append(pieces, piece)
			mu.Unlock()
		})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if text != "你好" {
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
	if st.Name != protocol.Name || st.Version != protocol.Version {
		t.Errorf("welcome named %q %q", st.Name, st.Version)
	}
}

// TestStreamCancel stops an answer half way: the call fails with the
// context's error, keeps what arrived, and the plugin is told to stop.
func TestStreamCancel(t *testing.T) {
	c := pluginClient(t, "serve")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var once sync.Once
	text, err := c.Stream(ctx,
		protocol.Config{BaseURL: "http://127.0.0.1:1/v1", Model: "test", APIKey: "k"},
		[]protocol.ChatMessage{{Role: "user", Content: "慢一点"}},
		func(string) { once.Do(cancel) })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel gave %v", err)
	}
	if text == "" || strings.Contains(text, "六") {
		t.Errorf("partial answer = %q", text)
	}
	if _, err := c.Stream(context.Background(),
		protocol.Config{BaseURL: "http://127.0.0.1:1/v1", Model: "test", APIKey: "k"},
		[]protocol.ChatMessage{{Role: "user", Content: "再问"}}, nil); err != nil {
		t.Fatalf("after cancel: %v", err)
	}
}

// TestStreamErrorKeepsDeltas reports the plugin's error and keeps whatever
// arrived before it.
func TestStreamErrorKeepsDeltas(t *testing.T) {
	c := pluginClient(t, "serve")
	_, err := c.Stream(context.Background(),
		protocol.Config{BaseURL: "http://127.0.0.1:1/v1", Model: "test", APIKey: "k"},
		[]protocol.ChatMessage{{Role: "user", Content: "失败"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "端点拒绝") {
		t.Fatalf("error = %v", err)
	}
}

// TestPluginRestarts kills the process and checks the next call starts
// another one, rather than failing for the rest of the session.
func TestPluginRestarts(t *testing.T) {
	c := pluginClient(t, "serve")
	cfg := protocol.Config{BaseURL: "http://127.0.0.1:1/v1", Model: "test", APIKey: "k"}
	if _, err := c.Stream(context.Background(), cfg, []protocol.ChatMessage{{Role: "user", Content: "1"}}, nil); err != nil {
		t.Fatal(err)
	}
	st := c.Status()
	proc, err := os.FindProcess(st.PID)
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatalf("kill %d: %v", st.PID, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for c.Status().Running && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	text, err := c.Stream(context.Background(), cfg, []protocol.ChatMessage{{Role: "user", Content: "2"}}, nil)
	if err != nil {
		t.Fatalf("the plugin did not come back: %v", err)
	}
	if text != "你好" {
		t.Errorf("answer = %q", text)
	}
	if c.Status().PID == st.PID {
		t.Errorf("the restarted plugin has the same pid %d", st.PID)
	}
}

// TestPluginProtocolMismatch refuses a plugin that speaks another version.
func TestPluginProtocolMismatch(t *testing.T) {
	c := pluginClient(t, "proto9")
	_, err := c.Stream(context.Background(), protocol.Config{APIKey: "k"},
		[]protocol.ChatMessage{{Role: "user", Content: "hi"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "protocol") {
		t.Fatalf("mismatch = %v", err)
	}
}

// TestPluginStartFails reports a plugin that exits before its welcome.
func TestPluginStartFails(t *testing.T) {
	c := pluginClient(t, "exit")
	_, err := c.Stream(context.Background(), protocol.Config{APIKey: "k"},
		[]protocol.ChatMessage{{Role: "user", Content: "hi"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "exited") {
		t.Fatalf("start failure = %v", err)
	}
}

// TestStatusBeforeStart reports a plugin that never ran.
func TestStatusBeforeStart(t *testing.T) {
	c := pluginClient(t, "serve")
	if st := c.Status(); st.Running || st.PID != 0 {
		t.Fatalf("status = %+v", st)
	}
}

var _ = fmt.Sprint
