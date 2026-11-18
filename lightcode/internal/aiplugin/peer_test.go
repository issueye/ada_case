package aiplugin_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"lightcode/internal/aiplugin"
)

func TestMessageRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := aiplugin.WriteMessage(&buf, &aiplugin.Message{T: "call", ID: 7, Method: "m"}); err != nil {
		t.Fatal(err)
	}
	m, err := aiplugin.ReadMessage(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if m.T != "call" || m.ID != 7 || m.Method != "m" {
		t.Fatalf("read %+v", m)
	}
}

func TestMessageTooLarge(t *testing.T) {
	var buf bytes.Buffer
	head := make([]byte, 4)
	binary.LittleEndian.PutUint32(head, uint32(aiplugin.MaxMessage)+1)
	buf.Write(head)
	if _, err := aiplugin.ReadMessage(&buf); err == nil || !strings.Contains(err.Error(), "over") {
		t.Fatalf("a message over the bound was read: %v", err)
	}
}

func TestPeerCallAndReply(t *testing.T) {
	host, child := net.Pipe()
	hp := aiplugin.NewPeer(host)
	cp := aiplugin.NewPeer(child)
	defer hp.Close()
	defer cp.Close()
	cp.Handle("echo", func(_ context.Context, args []json.RawMessage) (any, error) {
		var s string
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &s)
		}
		return map[string]string{"echo": s}, nil
	})
	out, err := hp.Call(context.Background(), "echo", "hi")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(out, &got); err != nil || got["echo"] != "hi" {
		t.Fatalf("reply = %s (%v)", out, err)
	}
	if _, err := hp.Call(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), "no method") {
		t.Fatalf("an unknown method answered: %v", err)
	}
}

func TestPeerEvent(t *testing.T) {
	host, child := net.Pipe()
	hp := aiplugin.NewPeer(host)
	cp := aiplugin.NewPeer(child)
	defer hp.Close()
	defer cp.Close()
	events := make(chan string, 4)
	off := hp.OnEvent(aiplugin.EventDelta, func(raw json.RawMessage) { events <- string(raw) })
	defer off()
	if err := cp.Emit(aiplugin.EventDelta, aiplugin.Delta{RequestID: "r1", Text: "a"}); err != nil {
		t.Fatal(err)
	}
	select {
	case raw := <-events:
		var d aiplugin.Delta
		if err := json.Unmarshal([]byte(raw), &d); err != nil || d.Text != "a" || d.RequestID != "r1" {
			t.Fatalf("event = %s (%v)", raw, err)
		}
	case <-time.After(time.Second):
		t.Fatal("no event arrived")
	}
}

// TestPeerClosedCall checks that a call on an ended connection fails
// rather than waiting for a reply that cannot come.
func TestPeerClosedCall(t *testing.T) {
	host, child := net.Pipe()
	hp := aiplugin.NewPeer(host)
	cp := aiplugin.NewPeer(child)
	cp.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := hp.Call(ctx, "echo"); err == nil {
		t.Fatal("the call answered after the peer closed")
	}
}

func ExampleConfig() {
	fmt.Println(aiplugin.DefaultConfig().Normalize().BaseURL)
	// Output: https://api.openai.com/v1
}
