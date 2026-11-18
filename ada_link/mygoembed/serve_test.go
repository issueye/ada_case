package mygoembed

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/learningdog1/ada_link/link"
)

// e2eState is the child's state, and its service's result.
type e2eState struct {
	N int `json:"n"`
}

// e2eApp is the child of the end-to-end test: a full-size button and a
// counter the host calls.
type e2eApp struct {
	n int
}

func (a *e2eApp) view(c *ui.Context) {
	if ui.PrimaryButton(c, fmt.Sprintf("count %d", a.n)).Fill().Clicked() {
		a.n++
	}
}

func (a *e2eApp) call(method string, _ []json.RawMessage) (any, error) {
	switch method {
	case "E2E.Increment":
		a.n++
	case "E2E.State":
	default:
		return nil, fmt.Errorf("no method %q", method)
	}
	return e2eState{N: a.n}, nil
}

// TestServeEndToEnd runs a host and a child of the same test binary, and
// drives the child: its first frame, its service, a click in its area, a
// resize, and its exit.
func TestServeEndToEnd(t *testing.T) {
	if os.Getenv("ADALINK_E2E_CHILD") == "1" {
		a := &e2eApp{}
		if err := Serve(ServeOptions{View: a.view, Title: "e2e", Service: a.call}); err != nil {
			log.Fatalf("child: %v", err)
		}
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := link.Spawn(link.SpawnOptions{
		Command: []string{exe, "-test.run=TestServeEndToEnd"},
		Env:     []string{"ADALINK_E2E_CHILD=1"},
		W:       200,
		H:       150,
		Scale:   2,
		Title:   "e2e",
		Stderr:  os.Stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	stream := &frameStream{ch: make(chan struct{})}
	sess.OnFrame(func(f *link.Frame) {
		img := image.NewRGBA(f.Image.Rect)
		copy(img.Pix, f.Image.Pix)
		f.Release()
		stream.put(img)
	})

	img, seq, err := stream.wait(10*time.Second, 0)
	if err != nil {
		t.Fatalf("first frame: %v", err)
	}
	if got, want := img.Rect.Dx(), 400; got != want {
		t.Errorf("frame width %d, want %d (200 DIPs at scale 2)", got, want)
	}
	if got, want := img.Rect.Dy(), 300; got != want {
		t.Errorf("frame height %d, want %d", got, want)
	}
	if s := sess.Surface(); s.W != 200 || s.H != 150 || s.Scale != 2 {
		t.Errorf("surface %+v", s)
	}

	if n := callN(t, sess, "E2E.State"); n != 0 {
		t.Fatalf("count %d at start", n)
	}

	// A click in the middle of the child's area, which the button fills.
	for _, kind := range []link.InputKind{link.InputDown, link.InputUp} {
		if err := sess.Input(link.InputEvent{Kind: kind, X: 100, Y: 75, Button: 0, Clicks: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if n := callN(t, sess, "E2E.State"); n != 1 {
		t.Fatalf("a click left count %d", n)
	}
	if _, seq, err = stream.wait(5*time.Second, seq); err != nil {
		t.Fatalf("the click's frame: %v", err)
	}

	if n := callN(t, sess, "E2E.Increment"); n != 2 {
		t.Fatalf("a call left count %d", n)
	}
	if _, seq, err = stream.wait(5*time.Second, seq); err != nil {
		t.Fatalf("the call's frame: %v", err)
	}

	// A resize: the child draws 320×200 DIPs at scale 1 into a new ring.
	if err := sess.Resize(320, 200, 1); err != nil {
		t.Fatal(err)
	}
	img, seq, err = stream.wait(10*time.Second, seq)
	if err != nil {
		t.Fatalf("the resized frame: %v", err)
	}
	if img.Rect.Dx() != 320 || img.Rect.Dy() != 200 {
		t.Errorf("resized frame %dx%d", img.Rect.Dx(), img.Rect.Dy())
	}
	if s := sess.Surface(); s.W != 320 || s.H != 200 || s.Scale != 1 {
		t.Errorf("resized surface %+v", s)
	}

	st := sess.Stats()
	if st.Frames < 3 {
		t.Errorf("stats %+v", st)
	}
	sess.Close()
	select {
	case <-sess.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the session did not end")
	}
}

func callN(t *testing.T, sess *link.Session, method string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := sess.Call(ctx, method)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	var s e2eState
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	return s.N
}

// frameStream keeps the newest frame, as a host's main-thread turn would.
type frameStream struct {
	mu  sync.Mutex
	img *image.RGBA
	seq uint64
	ch  chan struct{}
}

// BenchmarkViewFrame1280 measures what a child does for one 1280×800 frame
// at scale 1: build the view, render it, convert it to RGBA and find where
// it changed. The ring's write is measured in package link.
func BenchmarkViewFrame1280(b *testing.B) {
	app := &e2eApp{}
	tt := ui.NewTester(app.view, 1280, 800)
	var prev []byte
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tt.Frame()
		img := tt.Image()
		if prev != nil {
			link.Damage(prev, img.Pix, img.Rect.Dx(), img.Rect.Dy(), img.Stride)
		}
		prev = img.Pix
	}
}

// BenchmarkViewFrame1280Still is the same with nothing to draw anew, as an
// idle tick that only checks the frame.
func BenchmarkViewFrame1280Still(b *testing.B) {
	app := &e2eApp{}
	tt := ui.NewTester(app.view, 1280, 800)
	prev := tt.Image()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		img := tt.Image()
		link.Damage(prev.Pix, img.Pix, img.Rect.Dx(), img.Rect.Dy(), img.Stride)
	}
}

func (s *frameStream) put(img *image.RGBA) {
	s.mu.Lock()
	s.img, s.seq = img, s.seq+1
	close(s.ch)
	s.ch = make(chan struct{})
	s.mu.Unlock()
}

func (s *frameStream) wait(d time.Duration, after uint64) (*image.RGBA, uint64, error) {
	deadline := time.NewTimer(d)
	defer deadline.Stop()
	for {
		s.mu.Lock()
		img, seq, ch := s.img, s.seq, s.ch
		s.mu.Unlock()
		if seq > after {
			return img, seq, nil
		}
		select {
		case <-ch:
		case <-deadline.C:
			return nil, after, fmt.Errorf("no frame in %v", d)
		}
	}
}
