package link

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/learningdog1/ada_link/internal/shm"
)

// The ring's header, at the start of its file:
//
//	0   magic "ADLNKFG1"
//	8   slots   uint32
//	12  width   uint32, in device pixels
//	16  height  uint32
//	20  stride  uint32, bytes per row (4×width)
//	24  scale   float32, device pixels per DIP
//	28  reserved
//
// Each slot follows the 64-byte header, height×stride bytes of premultiplied
// RGBA in Go's image.RGBA layout, the bytes of an image.RGBA's Pix.
const (
	ringHeaderSize = 64
	ringMagic      = "ADLNKFG1"
	// RingSlots is how far the child may run ahead of the host: three frames
	// of latency before it must drop one.
	RingSlots = 3
)

// FrameRing is a frame ring: slots of premultiplied RGBA that the child
// writes and the host reads, in a file both map.
type FrameRing struct {
	// Dir is the session's directory; File the ring's file in it.
	Dir, File string

	// W, H and Stride are the pixels of a slot; Scale the device pixels per
	// DIP the child drew at.
	W, H, Stride int
	Scale        float32
	Slots        int

	m *shm.Map
}

// CreateRing makes a ring of slots frames of w×h device pixels, at scale.
func CreateRing(dir, file string, w, h, slots int, scale float32) (*FrameRing, error) {
	if slots < 2 || slots > 8 {
		return nil, fmt.Errorf("link: %d frame slots", slots)
	}
	if w <= 0 || h <= 0 || w > 16384 || h > 16384 {
		return nil, fmt.Errorf("link: frame size %dx%d", w, h)
	}
	slot := 4 * w * h
	size := ringHeaderSize + slots*slot
	m, err := shm.Create(filepath.Join(dir, file), size)
	if err != nil {
		return nil, err
	}
	var hdr [ringHeaderSize]byte
	copy(hdr[0:8], ringMagic)
	binary.LittleEndian.PutUint32(hdr[8:], uint32(slots))
	binary.LittleEndian.PutUint32(hdr[12:], uint32(w))
	binary.LittleEndian.PutUint32(hdr[16:], uint32(h))
	binary.LittleEndian.PutUint32(hdr[20:], uint32(4*w))
	binary.LittleEndian.PutUint32(hdr[24:], math.Float32bits(scale))
	if err := m.Write(hdr[:], 0); err != nil {
		m.Close()
		return nil, err
	}
	return &FrameRing{Dir: dir, File: file, W: w, H: h, Stride: 4 * w, Scale: scale, Slots: slots, m: m}, nil
}

// OpenRing maps the ring info names, in dir.
func OpenRing(dir string, info FrameInfo) (*FrameRing, error) {
	if filepath.Base(info.File) != info.File {
		return nil, fmt.Errorf("link: frame file %q is not a name", info.File)
	}
	if info.W <= 0 || info.H <= 0 || info.W > 16384 || info.H > 16384 || info.Stride != 4*info.W {
		return nil, fmt.Errorf("link: frame ring %dx%d stride %d", info.W, info.H, info.Stride)
	}
	slots := info.Slots
	if slots <= 0 {
		slots = RingSlots
	}
	slot := info.Stride * info.H
	size := ringHeaderSize + slots*slot
	m, err := shm.Open(filepath.Join(dir, info.File), size)
	if err != nil {
		return nil, err
	}
	// The header is the child's; read it again rather than trust the message,
	// so a malformed or truncated ring is noticed here.
	var hdr [ringHeaderSize]byte
	if err := m.Read(hdr[:], 0); err != nil {
		m.Close()
		return nil, err
	}
	if string(hdr[0:8]) != ringMagic {
		m.Close()
		return nil, fmt.Errorf("link: %s is not a frame ring", info.File)
	}
	h := struct{ slots, w, hh, stride uint32 }{
		binary.LittleEndian.Uint32(hdr[8:]),
		binary.LittleEndian.Uint32(hdr[12:]),
		binary.LittleEndian.Uint32(hdr[16:]),
		binary.LittleEndian.Uint32(hdr[20:]),
	}
	if int(h.slots) != slots || int(h.w) != info.W || int(h.hh) != info.H ||
		int(h.stride) != info.Stride || uint32(size) != ringHeaderSize+uint32(slot)*h.slots {
		m.Close()
		return nil, fmt.Errorf("link: %s holds %dx%d stride %d slots %d", info.File, h.w, h.hh, h.stride, h.slots)
	}
	return &FrameRing{Dir: dir, File: info.File, W: info.W, H: info.H, Stride: info.Stride,
		Scale: info.Scale, Slots: slots, m: m}, nil
}

// SlotOffset returns where slot i's pixels start.
func (r *FrameRing) SlotOffset(i int) int64 {
	return int64(ringHeaderSize + i*r.Stride*r.H)
}

// WriteSlot copies a frame's pixels into slot i. The child calls it before
// it tells the host about the frame.
func (r *FrameRing) WriteSlot(i int, pix []byte) error {
	if i < 0 || i >= r.Slots || len(pix) != r.Stride*r.H {
		return fmt.Errorf("link: slot %d of %d holds %d bytes, not %d", i, r.Slots, len(pix), r.Stride*r.H)
	}
	if r.m == nil {
		return fmt.Errorf("link: the ring is closed")
	}
	return r.m.Write(pix, r.SlotOffset(i))
}

// ReadSlot copies slot i's pixels into dst. The host calls it once the
// frame's message names seq and slot.
func (r *FrameRing) ReadSlot(i int, dst []byte) error {
	if i < 0 || i >= r.Slots || len(dst) < r.Stride*r.H {
		return fmt.Errorf("link: slot %d of %d holds %d bytes, not %d", i, r.Slots, r.Stride*r.H, len(dst))
	}
	if r.m == nil {
		return fmt.Errorf("link: the ring is closed")
	}
	return r.m.Read(dst[:r.Stride*r.H], r.SlotOffset(i))
}

// Close unmaps the ring and removes its file.
func (r *FrameRing) Close() {
	if r.m != nil {
		r.m.Close()
		r.m = nil
	}
	if r.Dir != "" && r.File != "" {
		_ = os.Remove(filepath.Join(r.Dir, r.File))
	}
}

// Damage returns the box that differs between prev and cur, and false when
// nothing does. Both are rows of stride bytes, w pixels of 4 bytes each.
// Only a box is reported, not every changed run: the host re-uploads
// whole frames today (ui.Bitmap.Update keeps its texture), so the box
// tells where to look, not what to copy. Comparing rows first keeps an
// unchanged frame cheap: a still UI costs one pass over its rows.
func Damage(prev, cur []byte, w, h, stride int) (Rect, bool) {
	if len(prev) != len(cur) || len(cur) < stride*h || stride != 4*w {
		return Rect{}, false
	}
	x0, y0, x1, y1 := w, h, -1, -1
	for y := 0; y < h; y++ {
		a := prev[y*stride : (y+1)*stride]
		b := cur[y*stride : (y+1)*stride]
		if bytes.Equal(a, b) {
			continue
		}
		if y0 > y {
			y0 = y
		}
		y1 = y
		for x := 0; x < w; x++ {
			if !bytes.Equal(a[4*x:4*x+4], b[4*x:4*x+4]) {
				if x < x0 {
					x0 = x
				}
				if x > x1 {
					x1 = x
				}
			}
		}
	}
	if y1 < y0 || x1 < x0 {
		return Rect{}, false
	}
	return Rect{X: x0, Y: y0, W: x1 - x0 + 1, H: y1 - y0 + 1}, true
}

// framePool recycles the host's frame images, so that receiving frames at
// the display's rate allocates nothing after the first few.
type framePool struct {
	mu   sync.Mutex
	w, h int
	free []*image.RGBA
	// made counts the images allocated: past max, get returns nil rather than
	// grow, so a host slower than its child drops frames instead of memory.
	made, max int
}

func newFramePool(w, h, max int) *framePool {
	if max < 2 {
		max = 2
	}
	return &framePool{w: w, h: h, max: max}
}

func (p *framePool) get() *image.RGBA {
	p.mu.Lock()
	defer p.mu.Unlock()
	if n := len(p.free); n > 0 {
		img := p.free[n-1]
		p.free = p.free[:n-1]
		return img
	}
	if p.made >= p.max {
		return nil
	}
	p.made++
	return image.NewRGBA(image.Rect(0, 0, p.w, p.h))
}

func (p *framePool) put(img *image.RGBA) {
	if img == nil || img.Rect.Dx() != p.w || img.Rect.Dy() != p.h {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.free) < p.max {
		p.free = append(p.free, img)
	}
}

// Frame is a frame the host received: its pixels, and where they changed.
// Call Release once the pixels are copied or drawn; they are recycled after
// it, and the child learns it may write the slot again.
type Frame struct {
	// Seq counts the child's frames; it starts at 1.
	Seq uint64
	// Image is premultiplied RGBA, of the child's device pixels.
	Image *image.RGBA
	// Damage is where the frame changed since the one before, for hosts
	// that copy only that; nil when the child sent no box.
	Damage []Rect

	s        *Session
	released bool
}

// Release returns the frame's pixels to the pool, and acknowledges it to
// the child. It is safe to call twice; a frame never released pins one of
// the child's slots.
func (f *Frame) Release() {
	if f == nil || f.released || f.s == nil {
		return
	}
	f.released = true
	f.s.ackFrame(f.Seq)
	f.s.pool.put(f.Image)
}
