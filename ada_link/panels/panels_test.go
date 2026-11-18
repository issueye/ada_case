package panels

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/learningdog1/ada_link/link"
	"github.com/learningdog1/ada_link/mygoembed"
)

func TestTopic(t *testing.T) {
	topic, err := Topic("orders", "changed")
	if err != nil {
		t.Fatal(err)
	}
	if topic != "panel/orders/changed" {
		t.Fatalf("topic %q", topic)
	}
	id, name, ok := SplitTopic(topic)
	if !ok || id != "orders" || name != "changed" {
		t.Fatalf("split: %q %q %v", id, name, ok)
	}
	for _, bad := range []string{"", "panel/", "panel/a", "panel/a/", "host/theme", "panel//x"} {
		if _, _, ok := SplitTopic(bad); ok {
			t.Errorf("%q split", bad)
		}
	}
	if _, err := Topic("a/b", "x"); err == nil {
		t.Error("an ID with a slash accepted")
	}
	if _, err := Topic("a b", "x"); err == nil {
		t.Error("an ID with a space accepted")
	}
	if _, err := Topic("a", ""); err == nil {
		t.Error("an empty event name accepted")
	}
	if _, err := Topic("a", "x/y"); err == nil {
		t.Error("an event name with a slash accepted")
	}
	host, err := HostTopic("theme")
	if err != nil || host != "host/theme" {
		t.Fatalf("host topic %q, %v", host, err)
	}
}

// childApp is the child of the end-to-end test: a counter and two events.
type childApp struct {
	c *mygoembed.Child
	n int
}

func (a *childApp) view(c *ui.Context) {
	ui.Column(c).Fill().Center().Children(func() {
		ui.Text(c, fmt.Sprintf("count %d", a.n)).FontSize(14)
	})
}

func (a *childApp) call(method string, _ []json.RawMessage) (any, error) {
	switch method {
	case "Panel.Echo":
		a.n++
		p := state{N: a.n}
		if a.c != nil {
			_ = a.c.Emit("echoed", p)
		}
		return p, nil
	case "Panel.State":
	default:
		return nil, fmt.Errorf("no method %q", method)
	}
	return state{N: a.n}, nil
}

type state struct {
	N int `json:"n"`
}

func runPanelsChild() {
	a := &childApp{}
	err := mygoembed.Serve(mygoembed.ServeOptions{
		View:    a.view,
		Title:   "panels-child",
		Service: a.call,
		OnReady: func(c *mygoembed.Child) {
			a.c = c
			c.OnEvent("theme", func(p json.RawMessage) {
				_ = c.Emit("theme-ack", p)
			})
		},
	})
	if err != nil {
		panic(err)
	}
}

// TestRegistryE2E runs two children of the same test binary, and drives the
// registry: namespaced events, broadcasts, changes, removal.
func TestRegistryE2E(t *testing.T) {
	if os.Getenv("PANELS_CHILD") == "1" {
		runPanelsChild()
		return
	}
	spawn := func() *link.Session {
		t.Helper()
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		sess, err := link.Spawn(link.SpawnOptions{
			Command: []string{exe, "-test.run=TestRegistryE2E"},
			Env:     []string{"PANELS_CHILD=1"},
			W:       200,
			H:       120,
			Scale:   1,
			Title:   "panels-child",
			Stderr:  os.Stderr,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(sess.Close)
		return sess
	}
	sessA, sessB := spawn(), spawn()

	reg := New()
	defer reg.Close()

	changes := make(chan Change, 32)
	reg.OnChange(func(c Change) { changes <- c })
	events := make(chan Event, 32)
	reg.OnEvent("echoed", func(e Event) { events <- e })
	reg.OnEvent("theme-ack", func(e Event) { events <- e })

	if _, err := reg.Add("a", "Panel A", sessA); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Add("b", "Panel B", sessB); err != nil {
		t.Fatal(err)
	}
	if len(reg.IDs()) != 2 {
		t.Fatalf("IDs %v", reg.IDs())
	}
	if _, err := reg.Add("a", "again", sessA); err == nil {
		t.Fatal("duplicate ID accepted")
	}
	if _, ok := reg.Get("a"); !ok {
		t.Fatal("panel a missing")
	}

	call := func(sess *link.Session) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := sess.Call(ctx, "Panel.Echo"); err != nil {
			t.Fatal(err)
		}
	}
	call(sessA)
	ev := waitEvent(t, events, "echoed")
	if ev.Panel != "a" || ev.Name != "echoed" || ev.Topic != "panel/a/echoed" {
		t.Fatalf("event %+v", ev)
	}
	var s state
	if err := json.Unmarshal(ev.Payload, &s); err != nil || s.N != 1 {
		t.Fatalf("payload %s: %v", ev.Payload, err)
	}
	call(sessB)
	if ev := waitEvent(t, events, "echoed"); ev.Panel != "b" {
		t.Fatalf("second event from %q", ev.Panel)
	}

	res := reg.Broadcast("theme", map[string]string{"dark": "true"})
	if len(res.Sent) != 2 || res.Err() != nil {
		t.Fatalf("broadcast %+v", res)
	}
	acked := map[string]bool{}
	for len(acked) < 2 {
		ev := waitEvent(t, events, "theme-ack")
		acked[ev.Panel] = true
		if string(ev.Payload) != `{"dark":"true"}` {
			t.Errorf("ack payload %s", ev.Payload)
		}
	}
	if !acked["a"] || !acked["b"] {
		t.Fatalf("acks %v", acked)
	}

	// An update arrives when the child ends.
	if err := reg.Remove("a"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sessA.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("removing a did not close its session")
	}
	if _, ok := reg.Get("a"); ok {
		t.Fatal("panel a still registered")
	}
	if got := len(reg.List()); got != 1 {
		t.Fatalf("%d panels after removing a", got)
	}
	var removed bool
	for !removed {
		select {
		case c := <-changes:
			if c.Kind == Removed && c.Panel.ID == "a" {
				removed = true
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no removed change for a")
		}
	}

	// Broadcast excludes a removed panel, and reaches b.
	reg.Close()
	if !sessB.IsClosed() {
		t.Fatal("closing the registry left b open")
	}
	if got := len(reg.List()); got != 0 {
		t.Fatalf("%d panels after close", got)
	}
}

func waitEvent(t *testing.T, ch <-chan Event, name string) Event {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case e := <-ch:
			if e.Name == name {
				return e
			}
		case <-deadline:
			t.Fatalf("no %q event", name)
		}
	}
}
