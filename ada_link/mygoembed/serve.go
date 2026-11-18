// Package mygoembed puts a MyGo app's native UI, drawn by package ui, in
// another MyGo app's window, in a process of its own.
//
// A child calls Serve with the view it would otherwise give a window. It
// runs that view in package ui's headless renderer (ui.NewTester), renders
// frames into a ring both processes map, forwards input to the view, and
// serves its Go services over the link. A host calls Attach with a window
// and the session that link.Spawn made, and places the frames in its view.
//
// The child renders with the CPU (ui.Tester): no window and no GPU, only a
// small loop of its own that draws what changed. That is the version 1
// tradeoff; DESIGN.md lays out the window- and GPU-backed surface that
// replaces it.
package mygoembed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"log"
	"math"
	"os"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/learningdog1/ada_link/link"
)

// ServeOptions configure Serve.
type ServeOptions struct {
	// View is the child's user interface, as MyGo's WindowOptions.Content
	// would take it: a function of the app's state, built each frame.
	View func(*ui.Context)
	// Title is the child's title; the host passes one in its environment,
	// which this overrides.
	Title string
	// Service serves the host's calls: it runs on the UI goroutine, before
	// a frame, so it may change the state View reads. A nil Service refuses
	// every call.
	Service func(method string, args []json.RawMessage) (any, error)
	// OnReady runs once the link is up, before the first frame, on the UI
	// goroutine: emit the child's first events, or call the host, here.
	OnReady func(*Child)
}

// Child is a running child: the session's peer, and what the child's app
// can do with it.
type Child struct {
	peer   *link.Peer
	dir    string
	title  string
	tester *ui.Tester

	ring  *link.FrameRing
	info  link.FrameInfo
	gen   int
	seq   uint64
	slots [link.RingSlots]uint64 // the seq each slot holds
	acked atomic.Uint64
	prev  []byte

	sizeW, sizeH int
	scale        float32

	dirty     atomic.Bool
	mustBuild atomic.Bool
	dropped   uint64
	cmds      chan func(*Child)
	service   func(string, []json.RawMessage) (any, error)
	// pendingSize is the size the host last asked for, and resizeTimer says
	// a rebuild is due: sizes that arrive while one waits coalesce, so that a
	// layout that oscillates cannot make a ring per frame.
	pendingSize *link.SizeInfo
	resizeDue   bool
}

// Peer returns the child's message layer, for events and calls of its own.
func (c *Child) Peer() *link.Peer { return c.peer }

// Title returns the child's title, as the host knows it.
func (c *Child) Title() string { return c.title }

// Emit sends an event to the host.
func (c *Child) Emit(name string, payload any) error { return c.peer.Emit(name, payload) }

// Call calls one of the host's services: Peer.Call. Handlers of the host's
// run on the host, and this returns when the call answers.
func (c *Child) Call(ctx context.Context, method string, args ...any) (json.RawMessage, error) {
	return c.peer.Call(ctx, method, args...)
}

// OnEvent takes the host's events: Peer.OnEvent. Events that came before
// the listener are held.
func (c *Child) OnEvent(name string, fn func(json.RawMessage)) (off func()) {
	return c.peer.OnEvent(name, fn)
}

// Refresh asks for a frame: call it after changing state outside input or
// a call, as from a goroutine.
func (c *Child) Refresh() {
	c.mustBuild.Store(true)
	c.dirty.Store(true)
}

// Serve runs the child until the host closes the link. It must be called
// in the child's main goroutine, with the environment link.Spawn set: it
// reads the socket, the token, the size and the directory from it.
func Serve(opts ServeOptions) error {
	if opts.View == nil {
		return errors.New("mygoembed: Serve needs a View")
	}
	dial, err := link.DialFromEnv()
	if err != nil {
		return err
	}
	w, h := dial.W, dial.H
	if w <= 0 || h <= 0 {
		w, h = 640, 480
	}
	scale := dial.Scale
	if scale <= 0 {
		scale = 1
	}
	title := opts.Title
	if title == "" {
		title = dial.Title
	}
	tester := ui.NewTester(opts.View, w, h)
	tester.SetScale(scale)

	ring, info, err := newRing(dial.Dir, 1, deviceSize(w, h, scale), scale)
	if err != nil {
		dial.Close()
		return err
	}
	c := &Child{
		peer:    nil, // set by Welcome, before Start
		dir:     dial.Dir,
		title:   title,
		tester:  tester,
		ring:    ring,
		info:    info,
		gen:     1,
		sizeW:   w,
		sizeH:   h,
		scale:   scale,
		cmds:    make(chan func(*Child), 256),
		service: opts.Service,
	}
	if err := dial.Hello(info, link.SizeInfo{W: w, H: h, Scale: scale}, title); err != nil {
		ring.Close()
		dial.Close()
		return err
	}
	surface, err := dial.Welcome()
	if err != nil {
		ring.Close()
		dial.Close()
		return err
	}
	// The host may have resized while the hello was in flight.
	if surface.W > 0 && surface.H > 0 {
		c.sizeW, c.sizeH = surface.W, surface.H
		if surface.Scale > 0 {
			c.scale = surface.Scale
		}
		tester.SetSize(c.sizeW, c.sizeH)
		tester.SetScale(c.scale)
	}
	c.peer = dial.Peer()
	dial.Start(c.setup)
	if opts.OnReady != nil {
		opts.OnReady(c)
	}
	c.loop()
	// The ring the child ends on, not the one it started with: a resize
	// replaced it.
	c.ring.Close()
	dial.Close()
	// The host removes the session directory when it closes. A host that was
	// killed first leaves it, with the socket and no rings; the child is the
	// last one out then, so it takes what remains.
	_ = os.RemoveAll(dial.Dir)
	return nil
}

// setup registers the child's handlers before the peer reads a message.
func (c *Child) setup(p *link.Peer) {
	p.OnMessage(c.onMessage)
	p.HandleAny(func(ctx context.Context, method string, args []json.RawMessage) (any, error) {
		if c.service == nil {
			return nil, fmt.Errorf("mygoembed: no service %q", method)
		}
		type result struct {
			v   any
			err error
		}
		done := make(chan result, 1)
		c.post(func(*Child) {
			v, err := c.service(method, args)
			// The service changed the state the view reads: the next frame
			// must build the view again, not only re-render its scene.
			c.mustBuild.Store(true)
			c.dirty.Store(true)
			done <- result{v, err}
		})
		select {
		case r := <-done:
			return r.v, r.err
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-p.Done():
			return nil, link.ErrClosed
		}
	})
}

// onMessage takes the host's transport messages, on the read goroutine.
func (c *Child) onMessage(m *link.Message) bool {
	switch m.T {
	case "resize":
		if m.Size != nil {
			size := *m.Size
			c.post(func(c *Child) { c.resize(size) })
		}
	case "input":
		if m.Input != nil {
			ev := *m.Input
			c.post(func(c *Child) { c.applyInput(ev) })
		}
	case "frame-ack":
		if acked := c.acked.Load(); m.Seq > acked {
			c.acked.Store(m.Seq)
		}
	default:
		return false
	}
	return true
}

// post runs fn on the UI goroutine. The read goroutine waits when the queue
// is full, which bounds how far the host may run ahead of the frames.
func (c *Child) post(fn func(*Child)) {
	select {
	case c.cmds <- fn:
	case <-c.peer.Done():
	}
}

// loop is the child's UI goroutine: commands and the frame clock.
func (c *Child) loop() {
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()
	last := time.Now()
	c.publish(true)
	for {
		select {
		case fn := <-c.cmds:
			fn(c)
			c.dirty.Store(true)
		case now := <-ticker.C:
			if d := now.Sub(last); d > time.Millisecond {
				c.tester.Advance(d)
				last = now
			}
			if c.tester.FramePending() || c.dirty.Swap(false) {
				c.publish(false)
			}
		case <-c.peer.Done():
			return
		}
	}
}

// resize takes the host's size: the view lays out anew, and the ring is
// replaced as the pixels change size. Sizes that arrive together wait for
// one rebuild, so a layout that jitters by a pixel cannot make a ring per
// frame.
func (c *Child) resize(size link.SizeInfo) {
	if size.W <= 0 || size.H <= 0 {
		log.Printf("mygoembed: resize to %dx%d", size.W, size.H)
		return
	}
	c.pendingSize = &size
	if c.resizeDue {
		return
	}
	c.resizeDue = true
	time.AfterFunc(80*time.Millisecond, func() { c.post((*Child).applySize) })
}

// applySize takes the newest size asked for, on the UI goroutine.
func (c *Child) applySize() {
	c.resizeDue = false
	size := c.pendingSize
	c.pendingSize = nil
	if size == nil {
		return
	}
	scale := size.Scale
	if scale <= 0 {
		scale = 1
	}
	c.sizeW, c.sizeH, c.scale = size.W, size.H, scale
	c.tester.SetSize(size.W, size.H)
	c.tester.SetScale(scale)
	c.rebuild(c.tester)
}

// rebuild draws a frame at the tester's size and gives the host a new ring
// for it.
func (c *Child) rebuild(tester *ui.Tester) {
	tester.Frame()
	psize := deviceSize(c.sizeW, c.sizeH, c.scale)
	pw, ph := psize.X, psize.Y
	// The renderer rounds its pixels up; trust the frame it drew over the
	// arithmetic.
	if img := tester.Image(); img != nil {
		pw, ph = img.Rect.Dx(), img.Rect.Dy()
	}
	ring, info, err := newRing(c.dir, c.gen+1, image.Point{pw, ph}, c.scale)
	if err != nil {
		log.Printf("mygoembed: ring: %v", err)
		return
	}
	c.gen++
	c.ring.Close()
	c.ring, c.info = ring, info
	if debug {
		log.Printf("mygoembed: ring %d for %dx%d at %v", c.gen, pw, ph, c.scale)
	}
	c.prev = nil
	c.slots = [link.RingSlots]uint64{}
	if err := c.peer.Send(&link.Message{T: "surface", Frame: &c.info,
		Size: &link.SizeInfo{W: c.sizeW, H: c.sizeH, Scale: c.scale}}); err != nil {
		log.Printf("mygoembed: surface: %v", err)
		return
	}
	c.publish(true)
}

// publish renders the view, and sends the frame if it changed, or always
// when force.
func (c *Child) publish(force bool) {
	// A service or Refresh changed the state the view reads: build it anew.
	// Input settled its own frames, and animations were drawn by Advance, so
	// their frames need no build here: a view that animates must not render
	// twenty frames for every publish.
	if c.mustBuild.Swap(false) {
		c.tester.Frame()
	}
	img := c.tester.Image()
	if img == nil {
		return
	}
	if img.Rect.Dx() != c.ring.W || img.Rect.Dy() != c.ring.H {
		log.Printf("mygoembed: frame %dx%d, ring %dx%d", img.Rect.Dx(), img.Rect.Dy(), c.ring.W, c.ring.H)
		return
	}
	box := link.Rect{W: img.Rect.Dx(), H: img.Rect.Dy()}
	if !force && c.prev != nil {
		damage, changed := link.Damage(c.prev, img.Pix, img.Rect.Dx(), img.Rect.Dy(), img.Stride)
		if !changed {
			return
		}
		box = damage
	}
	slot := c.freeSlot()
	if slot < 0 {
		c.dropped++
		return
	}
	c.seq++
	if err := c.ring.WriteSlot(slot, img.Pix); err != nil {
		log.Printf("mygoembed: frame: %v", err)
		return
	}
	c.slots[slot] = c.seq
	c.prev = img.Pix
	if err := c.peer.Send(&link.Message{T: "frame", Seq: c.seq, Slot: slot, Damage: []link.Rect{box}}); err != nil {
		log.Printf("mygoembed: frame message: %v", err)
	}
}

// freeSlot returns a slot the host acknowledged, or -1 when all are out.
func (c *Child) freeSlot() int {
	acked := c.acked.Load()
	for i := range c.slots {
		if c.slots[i] <= acked {
			return i
		}
	}
	return -1
}

// newRing makes a ring of link.RingSlots frames of the size given, at scale.
func newRing(dir string, gen int, size image.Point, scale float32) (*link.FrameRing, link.FrameInfo, error) {
	file := fmt.Sprintf("frames-%d.bin", gen)
	ring, err := link.CreateRing(dir, file, size.X, size.Y, link.RingSlots, scale)
	if err != nil {
		return nil, link.FrameInfo{}, err
	}
	return ring, link.FrameInfo{File: file, Slots: link.RingSlots,
		W: size.X, H: size.Y, Stride: 4 * size.X, Scale: scale}, nil
}

// deviceSize returns the pixels a w×h DIPs view draws at scale, as the
// renderer rounds them: up.
func deviceSize(w, h int, scale float32) image.Point {
	return image.Point{
		X: int(math.Ceil(float64(w) * float64(scale))),
		Y: int(math.Ceil(float64(h) * float64(scale))),
	}
}

// applyInput gives the tester one of the host's input events.
func (c *Child) applyInput(ev link.InputEvent) {
	t := c.tester
	switch ev.Kind {
	case link.InputDown:
		switch ev.Button {
		case 1:
			t.RightClickAt(ev.X, ev.Y)
		case 0:
			t.Press(ev.X, ev.Y)
		default:
			// ui.Tester presses the primary and the secondary button only;
			// the middle one needs ui's own surface API (DESIGN.md, P2).
			log.Printf("mygoembed: no middle button in ui.Tester")
		}
	case link.InputUp:
		t.Release(ev.X, ev.Y)
	case link.InputMove:
		t.Move(ev.X, ev.Y)
	case link.InputScroll:
		if ev.Precise {
			t.Scroll(ev.X, ev.Y, ev.DX, ev.DY)
		} else {
			t.Wheel(ev.X, ev.Y, ev.DX, ev.DY)
		}
	case link.InputZoom:
		t.Pinch(ev.X, ev.Y, ev.Scale)
	case link.InputKeyDown:
		t.HoldModifiers(ui.Modifiers(ev.Mods))
		if ev.Key == 0 && ev.Physical != 0 {
			t.KeyAt(ui.Modifiers(ev.Mods), ui.Key(ev.Physical))
		} else {
			t.KeyDown(ui.Key(ev.Key))
		}
	case link.InputKeyUp:
		if ev.Key == 0 && ev.Physical != 0 {
			// KeyAt pressed and released: nothing to do.
		} else {
			t.KeyUp(ui.Key(ev.Key))
		}
	case link.InputText:
		t.Type(ev.Text)
	case link.InputCompose:
		t.Compose(ev.Text, ev.Caret)
	case link.InputCommand:
		t.Command(ev.Text)
	case link.InputFocus:
		t.SetFocused(ev.Focused)
	case link.InputDrop:
		t.DropFiles(ev.X, ev.Y, ev.Files...)
	default:
		log.Printf("mygoembed: input %q", ev.Kind)
	}
}
