package mygoembed

import (
	"errors"
	"image"
	"log"
	"os"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/learningdog1/ada_link/link"
)

// debug is set by ADALINK_DEBUG for the demo's logs.
var debug = os.Getenv("ADALINK_DEBUG") != ""

// Host places a child's frames in a MyGo window of native UI, and sends it
// the window's input.
//
//	win := mygo.NewWindow(mygo.WindowOptions{
//		Title:   "Shell",
//		Content: ui.View(nil), // replaced below
//	})
//	panel := mygoembed.Attach(win, session)
//	win.SetContent(ui.View(panel.View)) // or set Content before Attach
//
// The view draws the latest frame, asks the child for the element's size,
// and forwards pointer, key, text and drop input. Frames are applied on the
// main thread, one per main-loop turn: a burst of frames keeps the newest.
type Host struct {
	win  *mygo.Window
	sess *link.Session

	mu        sync.Mutex
	pending   *link.Frame
	scheduled bool

	// main thread only: the pixels the bitmap's texture is made from, and
	// the element's box, for input coordinates.
	buf   *image.RGBA
	bmp   *ui.Bitmap
	ready bool
	rect  ui.Rect
	// framePix is the last frame's size in device pixels: while it does not
	// match the element's, the frame is drawn to contain, not stretched.
	framePixW, framePixH int
	// asked is the DIPs and scale the child was last asked for.
	askedW, askedH int
	askedScale     float32
	// radius rounds the frame the host draws; 0 draws the pixels as they
	// come. A panel mounted as a card sets it so its square frame takes the
	// card's corners.
	radius float32

	onReady func()
}

// Attach connects a window and a session. The window must show native UI
// (WindowOptions.Content); the session is what link.Spawn made.
func Attach(win *mygo.Window, sess *link.Session) *Host {
	h := &Host{win: win, sess: sess}
	sess.OnFrame(h.frame)
	win.OnFocus(func() { _ = sess.Input(link.InputEvent{Kind: link.InputFocus, Focused: true}) })
	win.OnBlur(func() { _ = sess.Input(link.InputEvent{Kind: link.InputFocus, Focused: false}) })
	win.OnFileDrop(h.drop)
	win.OnClose(func(*mygo.CloseEvent) { sess.Close() })
	// A frame after every resize keeps the newly uncovered area from holding
	// what the surface had there before, and lets the view ask the child for
	// its new size.
	win.OnResize(func() { win.Invalidate() })
	return h
}

// SetRadius rounds the frame the host draws, so a square surface takes the
// rounded corners of the card around it. Zero draws the pixels untouched.
func (h *Host) SetRadius(radius float32) { h.radius = radius }

// Session returns the session the host shows.
func (h *Host) Session() *link.Session { return h.sess }

// Ready reports whether a frame arrived and the view draws one.
func (h *Host) Ready() bool { return h.ready }

// OnReady calls fn once the first frame is applied, on the main thread.
func (h *Host) OnReady(fn func()) { h.onReady = fn }

// Close closes the session.
func (h *Host) Close() { h.sess.Close() }

// frame takes a child's frame on the session's read goroutine, and schedules
// one main-thread turn for the newest: frames that arrive meanwhile replace
// each other, and their buffers go back to the pool.
func (h *Host) frame(f *link.Frame) {
	h.mu.Lock()
	old := h.pending
	h.pending = f
	need := !h.scheduled
	h.scheduled = true
	h.mu.Unlock()
	if old != nil {
		old.Release()
	}
	if need {
		h.win.Update(h.apply)
	}
}

// apply is the main-thread turn: copy the newest frame into the host's
// buffer and hand it to the bitmap, whose texture the GPU re-uploads.
func (h *Host) apply() {
	h.mu.Lock()
	f := h.pending
	h.pending = nil
	h.scheduled = false
	h.mu.Unlock()
	if f == nil {
		return
	}
	defer f.Release()
	if h.win.IsDestroyed() {
		return
	}
	if h.buf == nil || h.buf.Rect.Dx() != f.Image.Rect.Dx() || h.buf.Rect.Dy() != f.Image.Rect.Dy() {
		h.buf = image.NewRGBA(f.Image.Rect)
		h.bmp = ui.NewBitmap(h.buf)
	}
	copy(h.buf.Pix, f.Image.Pix)
	h.bmp.Update(h.buf)
	h.framePixW, h.framePixH = f.Image.Rect.Dx(), f.Image.Rect.Dy()
	if !h.ready {
		h.ready = true
		if h.onReady != nil {
			h.onReady()
		}
	}
}

// View builds the element that shows the child: the child's frame, filling
// the element's content box.
func (h *Host) View(c *ui.Context) {
	if !h.ready {
		ui.Box(c).Fill().Children(func() {
			ui.Text(c, "Waiting for the child…")
		})
		return
	}
	// Focusable, but not AutoFocus: the panel shares the window with the
	// editor, and taking the keyboard when it mounts would leave the editor
	// unable to type until it is clicked. A click focuses it.
	ui.Box(c).Fill().
		Key(h).
		Focusable().
		HandleInput(h.input).
		Draw(func(p *ui.Painter, r ui.Rect) {
			h.rect = r
			h.requestSize(r, p.Scale())
			if h.bmp == nil {
				return
			}
			fw, fh := float32(h.framePixW), float32(h.framePixH)
			pw, ph := r.W*p.Scale(), r.H*p.Scale()
			fit := ui.FillBox
			if abs32(fw-pw) > 2 || abs32(fh-ph) > 2 {
				// The child has not drawn the element's size yet: contain it
				// rather than stretch a frame of another size across the
				// element, as a resize does before the child's next frame.
				fit = ui.Contain
			}
			if h.radius > 0 {
				// Inset by the border so the card's ring stays visible
				// around the frame.
				rr := ui.Rect{X: r.X + 1, Y: r.Y + 1, W: r.W - 2, H: r.H - 2}
				p.Clip(rr, h.radius, func() { p.Image(h.bmp, rr, fit) })
				return
			}
			p.Image(h.bmp, r, fit)
		})
}

// abs32 returns the absolute value of v.
func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// requestSize asks the child for the element's size when it changed.
func (h *Host) requestSize(r ui.Rect, scale float32) {
	if r.W < 1 || r.H < 1 || scale <= 0 {
		return
	}
	w, hgt := int(r.W+0.5), int(r.H+0.5)
	if w == h.askedW && hgt == h.askedH && scale == h.askedScale {
		return
	}
	oldW, oldH, oldScale := h.askedW, h.askedH, h.askedScale
	h.askedW, h.askedH, h.askedScale = w, hgt, scale
	if debug {
		log.Printf("mygoembed: resize %dx%d at %v (was %dx%d at %v)", w, hgt, scale, oldW, oldH, oldScale)
	}
	if err := h.sess.Resize(w, hgt, scale); err != nil {
		log.Printf("mygoembed: resize: %v", err)
	}
}

// input forwards one of the element's input events to the child.
func (h *Host) input(ev ui.InputEvent) bool {
	out := link.InputEvent{}
	// The element's input is local to its box, which is the child's DIPs.
	out.X, out.Y = ev.X, ev.Y
	switch ev.Kind {
	case ui.InputPointerDown:
		out.Kind = link.InputDown
		out.Button, out.Clicks = ev.Button, ev.Clicks
	case ui.InputPointerUp:
		out.Kind = link.InputUp
		out.Button, out.Clicks = ev.Button, ev.Clicks
	case ui.InputPointerMove:
		out.Kind = link.InputMove
	case ui.InputScroll:
		out.Kind = link.InputScroll
		out.DX, out.DY = ev.DX, ev.DY
		out.Precise, out.Momentum = ev.Precise, ev.Momentum
	case ui.InputZoom:
		out.Kind = link.InputZoom
		out.Scale = ev.Scale
	case ui.InputKeyDown:
		// A shortcut of the host's (Cmd+S) stays the host's: the child is
		// a panel of the host's app, not an app of its own.
		if ev.Mods&ui.Cmd != 0 {
			return false
		}
		out.Kind = link.InputKeyDown
		out.Key, out.Physical = uint16(ev.Key), uint16(ev.PhysicalKey)
		out.Mods, out.Repeat = uint8(ev.Mods), ev.Repeat
	case ui.InputKeyUp:
		if ev.Mods&ui.Cmd != 0 {
			return false
		}
		out.Kind = link.InputKeyUp
		out.Key, out.Physical = uint16(ev.Key), uint16(ev.PhysicalKey)
		out.Mods = uint8(ev.Mods)
	case ui.InputText:
		out.Kind = link.InputText
		out.Text = ev.Text
	case ui.InputCompose:
		out.Kind = link.InputCompose
		out.Text, out.Caret = ev.Text, ev.Caret
	case ui.InputCommand:
		out.Kind = link.InputCommand
		out.Text = ev.Text
	default:
		return false
	}
	if err := h.sess.Input(out); err != nil && !errors.Is(err, link.ErrClosed) {
		log.Printf("mygoembed: input: %v", err)
	}
	return true
}

// drop forwards files dropped on the window to the child, where they fall
// in the element's box.
func (h *Host) drop(e *mygo.FileDropEvent) {
	r := h.rect
	x, y := float32(e.X)-r.X, float32(e.Y)-r.Y
	if x < 0 || y < 0 || x >= r.W || y >= r.H {
		return
	}
	if err := h.sess.Input(link.InputEvent{Kind: link.InputDrop, X: x, Y: y, Files: e.Paths}); err != nil {
		log.Printf("mygoembed: drop: %v", err)
	}
}
