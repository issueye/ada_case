// Command linkdemo runs the whole link without a window: a host spawns a
// child of its own binary, shows the child's frames as PNGs, clicks in it,
// calls its service, and checks what it shows.
//
//	go run ./cmd/linkdemo            # host; starts the child itself
//	go run ./cmd/linkdemo -child     # the child, as link.Spawn starts it
//
// The frames land in artifacts/.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/learningdog1/ada_link/link"
	"github.com/learningdog1/ada_link/mygoembed"
)

func main() {
	child := flag.Bool("child", false, "run as the child process")
	out := flag.String("out", "artifacts", "where the host writes its frames")
	w := flag.Int("w", 480, "the child's width, in DIPs")
	h := flag.Int("h", 320, "the child's height, in DIPs")
	flag.Parse()
	if *child {
		runChild()
		return
	}
	if err := runHost(*out, *w, *h); err != nil {
		log.Fatalf("linkdemo: %v", err)
	}
}

// demo is the child's app: a full-window button whose label counts the
// clicks, so that any click in the area hits it, and a service the host
// calls.
type demo struct {
	n int
}

type state struct {
	N int `json:"n"`
}

func (d *demo) view(c *ui.Context) {
	if ui.PrimaryButton(c, fmt.Sprintf("process 2 · count %d · click me", d.n)).Fill().FontSize(24).Clicked() {
		d.n++
	}
}

func (d *demo) call(method string, _ []json.RawMessage) (any, error) {
	switch method {
	case "Demo.Increment":
		d.n++
	case "Demo.State":
	default:
		return nil, fmt.Errorf("no method %q", method)
	}
	return state{N: d.n}, nil
}

func runChild() {
	d := &demo{}
	err := mygoembed.Serve(mygoembed.ServeOptions{
		View:    d.view,
		Title:   "linkdemo child",
		Service: d.call,
		OnReady: func(c *mygoembed.Child) {
			_ = c.Emit("demo.ready", state{N: d.n})
		},
	})
	if err != nil {
		log.Fatalf("child: %v", err)
	}
}

func runHost(out string, w, h int) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	sess, err := link.Spawn(link.SpawnOptions{
		Command: []string{exe, "-child"},
		W:       w,
		H:       h,
		Scale:   1,
		Title:   "linkdemo",
		Stderr:  os.Stderr,
	})
	if err != nil {
		return err
	}
	defer sess.Close()

	ready := make(chan state, 1)
	sess.OnEvent("demo.ready", func(p json.RawMessage) {
		var s state
		_ = json.Unmarshal(p, &s)
		select {
		case ready <- s:
		default:
		}
	})
	frames := &sink{ch: make(chan struct{})}
	sess.OnFrame(func(f *link.Frame) {
		img := image.NewRGBA(f.Image.Rect)
		copy(img.Pix, f.Image.Pix)
		f.Release()
		frames.put(img)
	})

	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		return fmt.Errorf("no demo.ready event")
	}
	first, seq1, err := frames.wait(10*time.Second, 0)
	if err != nil {
		return err
	}
	if err := savePNG(filepath.Join(out, "frame-1.png"), first); err != nil {
		return err
	}
	fmt.Printf("frame 1: %dx%d\n", first.Rect.Dx(), first.Rect.Dy())

	got, err := callState(sess, "Demo.State", 5*time.Second)
	if err != nil {
		return err
	}
	fmt.Printf("state: %+v\n", got)
	if got.N != 0 {
		return fmt.Errorf("count is %d at start", got.N)
	}

	// Input: a click in the middle of the area, where the button fills.
	start := time.Now()
	x, y := float32(w/2), float32(h/2)
	for _, kind := range []link.InputKind{link.InputDown, link.InputUp} {
		if err := sess.Input(link.InputEvent{Kind: kind, X: x, Y: y, Button: 0, Clicks: 1}); err != nil {
			return err
		}
	}
	got, err = callState(sess, "Demo.State", 5*time.Second)
	if err != nil {
		return err
	}
	if got.N != 1 {
		return fmt.Errorf("a click left count %d", got.N)
	}
	second, seq2, err := frames.wait(5*time.Second, seq1)
	if err != nil {
		return err
	}
	fmt.Printf("click: count 1 after %v\n", time.Since(start).Round(time.Millisecond))
	if err := savePNG(filepath.Join(out, "frame-2.png"), second); err != nil {
		return err
	}

	// Call the child's service.
	start = time.Now()
	got, err = callState(sess, "Demo.Increment", 5*time.Second)
	if err != nil {
		return err
	}
	if got.N != 2 {
		return fmt.Errorf("a call left count %d", got.N)
	}
	third, _, err := frames.wait(5*time.Second, seq2)
	if err != nil {
		fmt.Printf("stats after the call: %+v, err %v\n", sess.Stats(), sess.Err())
		if _, cerr := callState(sess, "Demo.State", 2*time.Second); cerr != nil {
			fmt.Printf("child state call: %v\n", cerr)
		} else {
			fmt.Printf("child still answers\n")
		}
		return err
	}
	fmt.Printf("call: count 2 after %v\n", time.Since(start).Round(time.Millisecond))
	if err := savePNG(filepath.Join(out, "frame-3.png"), third); err != nil {
		return err
	}

	st := sess.Stats()
	fmt.Printf("stats: %d frames, %d dropped, surface %+v\n", st.Frames, st.Dropped, sess.Surface())
	sess.Close()
	select {
	case <-sess.Done():
	case <-time.After(5 * time.Second):
		return fmt.Errorf("the child did not exit")
	}
	fmt.Printf("session ended: %v\n", sess.Err())
	return nil
}

// sink keeps the newest frame, as a host's main-thread turn would: a frame
// that comes while the host is busy replaces the one before.
type sink struct {
	mu  sync.Mutex
	img *image.RGBA
	seq uint64
	ch  chan struct{}
}

func (s *sink) put(img *image.RGBA) {
	s.mu.Lock()
	s.img, s.seq = img, s.seq+1
	close(s.ch)
	s.ch = make(chan struct{})
	s.mu.Unlock()
}

func (s *sink) wait(d time.Duration, after uint64) (*image.RGBA, uint64, error) {
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

func callState(sess *link.Session, method string, d time.Duration) (state, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	raw, err := sess.Call(ctx, method)
	if err != nil {
		return state{}, err
	}
	var s state
	if err := json.Unmarshal(raw, &s); err != nil {
		return state{}, err
	}
	return s, nil
}

func waitFrame(frames <-chan *image.RGBA, d time.Duration) (*image.RGBA, error) {
	select {
	case img := <-frames:
		return img, nil
	case <-time.After(d):
		return nil, fmt.Errorf("no frame in %v", d)
	}
}

func savePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", path)
	return nil
}
