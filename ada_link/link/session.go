package link

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// Surface is what the child shows: its size in DIPs, the device pixels per
// DIP it drew frames at, and its title.
type Surface struct {
	W, H  int
	Scale float32
	Title string
}

// Stats counts a session's frames.
type Stats struct {
	// Frames received, and Dropped because the host held all its frame
	// buffers: a host slower than the child loses frames rather than memory.
	Frames, Dropped uint64
}

// Session is the host's end of a link: the child's frames, input and Go
// services.
type Session struct {
	peer *Peer
	dir  string
	cmd  *childProcess

	// ringMu guards the ring's life: frame reads hold it for reading, so a
	// Close cannot unmap the pixels under them.
	ringMu sync.RWMutex

	mu        sync.Mutex
	ring      *FrameRing
	pool      *framePool
	lastSeq   uint64
	acked     uint64
	onFrame   func(*Frame)
	pending   *Frame
	onSurface func(Surface)
	surface   Surface
	wantW     int
	wantH     int
	wantScale float32
	closed    bool
	closeErr  error
	onClose   func(error)
	stats     Stats

	shutOnce sync.Once
	done     chan struct{}
}

// Peer returns the session's message layer: calls, events and handlers.
func (s *Session) Peer() *Peer { return s.peer }

// Directory returns the session's directory, where the frame rings live.
func (s *Session) Directory() string { return s.dir }

// Surface returns the child's last reported surface.
func (s *Session) Surface() Surface {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.surface
}

// Stats returns the session's counters.
func (s *Session) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

// Done is closed once the session ended, whether by Close, the child
// exiting, or the connection failing.
func (s *Session) Done() <-chan struct{} { return s.done }

// Err returns why the session ended, nil while it runs.
func (s *Session) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeErr
}

// IsClosed reports whether the session ended.
func (s *Session) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// OnFrame takes the child's frames, on the session's read goroutine. The
// callback must copy or draw the frame and call Release, which acknowledges
// it: until then the child keeps the slot. A callback that keeps a frame
// pins one of the child's slots and, past the ring's, drops frames.
//
// A frame may arrive before the callback is set: the last one waits, and is
// delivered as OnFrame takes it.
func (s *Session) OnFrame(fn func(*Frame)) {
	s.mu.Lock()
	s.onFrame = fn
	pending := s.pending
	s.pending = nil
	s.mu.Unlock()
	if pending != nil && fn != nil {
		fn(pending)
	} else if pending != nil {
		pending.Release()
	}
}

// OnSurface takes the child's surface reports: its size as the host asked,
// or its own when it starts.
func (s *Session) OnSurface(fn func(Surface)) {
	s.mu.Lock()
	s.onSurface = fn
	s.mu.Unlock()
}

// OnClose takes the reason the session ended.
func (s *Session) OnClose(fn func(error)) {
	s.mu.Lock()
	s.onClose = fn
	s.mu.Unlock()
}

// Resize asks the child to draw at w×h DIPs, scale device pixels per DIP.
// The child answers with a surface report, and the next frames are its
// size. Repeated asks of the same size are free.
func (s *Session) Resize(w, h int, scale float32) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("link: size %dx%d", w, h)
	}
	if scale <= 0 {
		scale = 1
	}
	s.mu.Lock()
	if w == s.wantW && h == s.wantH && scale == s.wantScale {
		s.mu.Unlock()
		return nil
	}
	s.wantW, s.wantH, s.wantScale = w, h, scale
	s.mu.Unlock()
	return s.peer.Send(&Message{T: "resize", Size: &SizeInfo{W: w, H: h, Scale: scale}})
}

// Input forwards one input event to the child.
func (s *Session) Input(ev InputEvent) error {
	return s.peer.Send(&Message{T: "input", Input: &ev})
}

// Call calls one of the child's services: Peer.Call.
func (s *Session) Call(ctx context.Context, method string, args ...any) (json.RawMessage, error) {
	return s.peer.Call(ctx, method, args...)
}

// Emit sends an event to the child: Peer.Emit.
func (s *Session) Emit(name string, payload any) error { return s.peer.Emit(name, payload) }

// Handle serves a method the child calls: Peer.Handle.
func (s *Session) Handle(method string, fn Handler) { s.peer.Handle(method, fn) }

// OnEvent takes the child's events: Peer.OnEvent.
func (s *Session) OnEvent(name string, fn func(json.RawMessage)) (off func()) {
	return s.peer.OnEvent(name, fn)
}

// Close ends the session: it tells the child, closes the socket and stops
// the child process if it did not exit by itself.
func (s *Session) Close() {
	s.peer.Send(&Message{T: "bye"})
	s.shutdown(ErrClosed)
}

func (s *Session) ackFrame(seq uint64) {
	s.mu.Lock()
	if seq <= s.acked {
		s.mu.Unlock()
		return
	}
	s.acked = seq
	s.mu.Unlock()
	if err := s.peer.Send(&Message{T: "frame-ack", Seq: seq}); err != nil && !errors.Is(err, ErrClosed) {
		log.Printf("link: frame ack: %v", err)
	}
}

// handleMessage takes the session's transport messages.
func (s *Session) handleMessage(m *Message) bool {
	switch m.T {
	case "surface":
		if m.Frame != nil {
			s.openRing(m.Frame)
		}
	case "frame":
		s.handleFrame(m)
	case "error":
		log.Printf("link: child: %s", m.Error)
	default:
		return false
	}
	return true
}

func (s *Session) openRing(info *FrameInfo) {
	ring, err := OpenRing(s.dir, *info)
	if err != nil {
		log.Printf("link: surface: %v", err)
		s.shutdown(err)
		return
	}
	s.ringMu.Lock()
	s.mu.Lock()
	old := s.ring
	if s.pool == nil || old == nil || old.W != ring.W || old.H != ring.H {
		s.pool = newFramePool(ring.W, ring.H, ring.Slots)
	}
	s.ring = ring
	if info.Scale > 0 {
		s.surface.Scale = info.Scale
	}
	if ring.Scale > 0 {
		s.surface.Scale = ring.Scale
	}
	if s.surface.Scale > 0 {
		s.surface.W = int(float32(ring.W)/s.surface.Scale + 0.5)
		s.surface.H = int(float32(ring.H)/s.surface.Scale + 0.5)
	} else {
		s.surface.W, s.surface.H = ring.W, ring.H
	}
	cb := s.onSurface
	surface := s.surface
	s.mu.Unlock()
	if old != nil {
		old.Close()
	}
	s.ringMu.Unlock()
	if cb != nil {
		cb(surface)
	}
}

// handleFrame copies one frame out of the ring and hands it to the callback.
func (s *Session) handleFrame(m *Message) {
	s.ringMu.RLock()
	s.mu.Lock()
	ring, pool := s.ring, s.pool
	if ring == nil || pool == nil {
		s.mu.Unlock()
		s.ringMu.RUnlock()
		return
	}
	if m.Slot < 0 || m.Slot >= ring.Slots || m.Seq <= s.lastSeq {
		s.mu.Unlock()
		s.ringMu.RUnlock()
		return
	}
	s.lastSeq = m.Seq
	cb := s.onFrame
	s.mu.Unlock()

	img := pool.get()
	if img == nil {
		// The host holds every buffer: acknowledge and drop, so the child
		// keeps drawing rather than stalling.
		s.ringMu.RUnlock()
		s.mu.Lock()
		s.stats.Dropped++
		s.mu.Unlock()
		s.ackFrame(m.Seq)
		return
	}
	if err := ring.ReadSlot(m.Slot, img.Pix); err != nil {
		log.Printf("link: frame %d: %v", m.Seq, err)
		s.ringMu.RUnlock()
		pool.put(img)
		s.ackFrame(m.Seq)
		return
	}
	s.ringMu.RUnlock()

	f := &Frame{Seq: m.Seq, Image: img, Damage: m.Damage, s: s}
	s.mu.Lock()
	s.stats.Frames++
	s.mu.Unlock()
	if cb != nil {
		cb(f)
		return
	}
	// Nobody listens yet: keep only the newest.
	s.mu.Lock()
	old := s.pending
	s.pending = f
	s.mu.Unlock()
	if old != nil {
		old.Release()
	}
}

// shutdown ends the session once: the peer, the child process, the ring and
// the directory.
func (s *Session) shutdown(err error) {
	s.shutOnce.Do(func() {
		s.mu.Lock()
		s.closed, s.closeErr = true, err
		pending, cbClose := s.pending, s.onClose
		s.pending = nil
		s.mu.Unlock()
		close(s.done)
		if pending != nil {
			pending.Release()
		}
		// Frames being copied hold the read lock; closing waits for them.
		s.ringMu.Lock()
		s.mu.Lock()
		ring := s.ring
		s.ring = nil
		s.mu.Unlock()
		if ring != nil {
			ring.Close()
		}
		s.ringMu.Unlock()
		s.peer.Close()
		if s.cmd != nil {
			s.cmd.stop(2 * time.Second)
		}
		// The peer's read loop may still be returning; the directory is the
		// host's to remove, as the child only wrote in it.
		_ = removeDir(s.dir)
		if cbClose != nil {
			cbClose(err)
		}
	})
}

// startPeer wires the read loop to the session, after the handshake.
func (s *Session) startPeer(peer *Peer) {
	s.peer = peer
	peer.OnMessage(s.handleMessage)
	go func() {
		<-peer.Done()
		err := peer.Err()
		if s.cmd != nil {
			if werr := s.cmd.wait(0); werr == nil && errors.Is(err, ErrClosed) {
				err = fmt.Errorf("link: the child exited")
			}
		}
		s.shutdown(err)
	}()
}

// checkHello validates a child's hello against the session's token.
func checkHello(m *Message, token string) error {
	if m.T != "hello" {
		return fmt.Errorf("link: the child's first message is %q, not hello", m.T)
	}
	if m.Proto != Proto {
		return fmt.Errorf("link: the child speaks protocol %d, not %d", m.Proto, Proto)
	}
	if subtle.ConstantTimeCompare([]byte(m.Token), []byte(token)) != 1 {
		return errors.New("link: wrong token")
	}
	if m.Frame == nil {
		return errors.New("link: hello without a frame ring")
	}
	return nil
}
