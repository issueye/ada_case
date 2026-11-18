// Package link carries a MyGo app's user interface and its Go services to
// another process.
//
// The host runs the window and the native controls around an area; the
// child runs the view that fills that area, renders it offscreen and hands
// the frames over. The two talk over a Unix domain socket with
// length-prefixed JSON, and frames travel through a memory-mapped file that
// both processes map, so pixels are copied once from the child's renderer
// into the ring, and once from the ring into the host's texture.
//
// The protocol is described in docs/protocol.md. Its types are:
//
//   - Session, the host's end: Spawn or Dial makes one.
//   - Peer, the message layer both ends share: calls, replies, events.
//   - FrameRing, the shared pixel ring the child publishes to.
//   - InputEvent, the pointer, key and text events the host forwards.
//
// The MyGo and ui side lives in package mygoembed: a host places the frame
// in its view (mygoembed.Attach), and a child serves its ui.View headless
// (mygoembed.Serve).
package link

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
)

// Proto is the version of the wire protocol this package speaks. Both ends
// refuse a peer that speaks another: the messages, and the meaning of input
// events and keys, are tied to the MyGo version that built them.
const Proto = 1

// MaxMessage bounds one message; a larger one closes the connection. Frames
// never travel in messages: only their metadata does.
const MaxMessage = 16 << 20

// ErrClosed is returned by calls on a peer whose connection ended.
var ErrClosed = errors.New("link: session closed")

// Message is one wire message. Fields are left empty when they do not apply
// to T, which is one of "hello", "welcome", "surface", "resize", "input",
// "frame", "frame-ack", "call", "reply", "event", "bye", "error".
type Message struct {
	T string `json:"t"`

	// hello and welcome: Proto is the wire version, Token the session's
	// secret (hello only), and the welcome carries what the child shows.
	Proto    int      `json:"proto,omitempty"`
	Token    string   `json:"token,omitempty"`
	Title    string   `json:"title,omitempty"`
	Services []string `json:"services,omitempty"`
	App      string   `json:"app,omitempty"`

	// call, reply and event.
	ID      int64             `json:"id,omitempty"`
	Method  string            `json:"m,omitempty"`
	Args    []json.RawMessage `json:"a,omitempty"`
	OK      *bool             `json:"ok,omitempty"`
	Value   json.RawMessage   `json:"v,omitempty"`
	Error   string            `json:"e,omitempty"`
	Event   string            `json:"n,omitempty"`
	Payload json.RawMessage   `json:"p,omitempty"`

	// surface and resize: the child's pixels and the host's room.
	Frame *FrameInfo `json:"frame,omitempty"`
	Size  *SizeInfo  `json:"size,omitempty"`

	// frame and frame-ack: which slot of the ring holds seq, where it
	// changed, and how much the host took.
	Seq    uint64 `json:"seq,omitempty"`
	Slot   int    `json:"slot,omitempty"`
	Damage []Rect `json:"damage,omitempty"`

	// input: one pointer, key, text or focus event for the child.
	Input *InputEvent `json:"ev,omitempty"`
}

// FrameInfo names the ring a child publishes frames through.
type FrameInfo struct {
	// File is the ring's file, relative to the session's directory.
	File string `json:"file"`
	// Slots is how many frames the ring may hold before the host takes one.
	Slots int `json:"slots"`
	// W, H and Stride are the slot's pixels, in device pixels.
	W      int `json:"w,omitempty"`
	H      int `json:"h,omitempty"`
	Stride int `json:"stride,omitempty"`
	// Scale is device pixels per DIP the frames were drawn at.
	Scale float32 `json:"scale,omitempty"`
}

// SizeInfo is a size in DIPs, and the scale the child draws at.
type SizeInfo struct {
	W     int     `json:"w,omitempty"`
	H     int     `json:"h,omitempty"`
	Scale float32 `json:"scale,omitempty"`
}

// Rect is a whole-pixel rectangle.
type Rect struct {
	X int `json:"x,omitempty"`
	Y int `json:"y,omitempty"`
	W int `json:"w,omitempty"`
	H int `json:"h,omitempty"`
}

// Empty reports whether the rectangle covers nothing.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Result is what a call answers: the reply's value, or its error.
type Result struct {
	Value json.RawMessage
	Err   error
}

// Handler serves one method for a peer. It runs on a goroutine of its own,
// and must not block the peer's read loop: waits belong to the method.
type Handler func(ctx context.Context, args []json.RawMessage) (any, error)

// Peer is a message connection: calls, replies, events and the transport
// messages of a session, both ways.
type Peer struct {
	conn io.ReadWriteCloser

	wmu sync.Mutex

	mu      sync.Mutex
	next    int64
	subNext uint64
	waits   map[int64]chan Result
	handles map[string]Handler
	anyFn   func(ctx context.Context, method string, args []json.RawMessage) (any, error)
	events  map[string][]eventSub
	held    map[string][]json.RawMessage
	msgFn   func(*Message) bool
	closed  bool
	ctx     context.Context
	cancel  context.CancelFunc

	closeOnce sync.Once
	startOnce sync.Once
	done      chan struct{}
	err       atomic.Value
}

// NewPeer takes a connection and starts its read loop. A net.Conn suits
// it, and so does a child's standard streams through PipeConn: the peer
// only reads, writes and closes.
func NewPeer(conn io.ReadWriteCloser) *Peer {
	p := newPeer(conn)
	p.start()
	return p
}

// NewQuietPeer makes a peer whose read loop waits for Start, so that a
// caller can register its handlers and its OnMessage before any message
// arrives: a plugin does this before its handshake.
func NewQuietPeer(conn io.ReadWriteCloser) *Peer { return newPeer(conn) }

// Start starts the read loop, once.
func (p *Peer) Start() { p.start() }

// newPeer makes a peer whose read loop waits for start, so that a caller
// can register its handlers before any message arrives.
func newPeer(conn io.ReadWriteCloser) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Peer{
		conn:    conn,
		waits:   map[int64]chan Result{},
		handles: map[string]Handler{},
		events:  map[string][]eventSub{},
		held:    map[string][]json.RawMessage{},
		done:    make(chan struct{}),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// start starts the peer's read loop, once.
func (p *Peer) start() {
	p.startOnce.Do(func() { go p.read() })
}

// eventSub is one listener of an event.
type eventSub struct {
	id uint64
	fn func(json.RawMessage)
}

// Send writes one message.
func (p *Peer) Send(m *Message) error {
	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(payload) > MaxMessage {
		return fmt.Errorf("link: message of %d bytes is over %d", len(payload), MaxMessage)
	}
	buf := make([]byte, 4+len(payload))
	binary.LittleEndian.PutUint32(buf, uint32(len(payload)))
	copy(buf[4:], payload)
	p.wmu.Lock()
	defer p.wmu.Unlock()
	_, err = p.conn.Write(buf)
	return err
}

// Handle serves method: a call whose Method is method runs fn. A peer
// answers calls to methods it does not handle with an error.
func (p *Peer) Handle(method string, fn Handler) {
	p.mu.Lock()
	p.handles[method] = fn
	p.mu.Unlock()
}

// OnMessage takes the transport messages, those that are no calls, replies
// or events: "welcome", "surface", "frame", "frame-ack", "resize" and
// "error". fn returns whether it took the message; one it leaves is logged.
// It runs on the read goroutine, so it must not wait for one.
func (p *Peer) OnMessage(fn func(*Message) bool) {
	p.mu.Lock()
	p.msgFn = fn
	p.mu.Unlock()
}

// Call calls method with args, which json.Marshal takes, and returns the
// reply's value. A context that ends first cancels the call; the peer still
// takes the reply when it comes.
func (p *Peer) Call(ctx context.Context, method string, args ...any) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	payloads := make([]json.RawMessage, len(args))
	for i, a := range args {
		b, err := json.Marshal(a)
		if err != nil {
			return nil, fmt.Errorf("link: argument %d of %s: %w", i+1, method, err)
		}
		payloads[i] = b
	}
	if p.isClosed() {
		return nil, p.closeErr()
	}
	ch := make(chan Result, 1)
	id := p.addWait(ch)
	defer p.forgetWait(id)
	if err := p.Send(&Message{T: "call", ID: id, Method: method, Args: payloads}); err != nil {
		return nil, err
	}
	select {
	case r := <-ch:
		return r.Value, r.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.done:
		// A reply may have landed with the close: take it before the error.
		select {
		case r := <-ch:
			return r.Value, r.Err
		default:
			return nil, p.closeErr()
		}
	}
}

// Emit sends a typed event: name, and payload, which json.Marshal takes.
func (p *Peer) Emit(name string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("link: event %q: %w", name, err)
	}
	return p.Send(&Message{T: "event", Event: name, Payload: b})
}

// OnEvent calls fn for events named name, on the peer's read goroutine, with
// the event's payload. Events that came before the first listener are held,
// up to maxHeldEvents of each name, and delivered to it then: a child that
// emits its state as it starts does not race the host that listens for it.
// fn must not wait for the peer: a call it makes does, and the read loop
// cannot answer it meanwhile. Use a goroutine for that.
func (p *Peer) OnEvent(name string, fn func(json.RawMessage)) (off func()) {
	p.mu.Lock()
	p.subNext++
	id := p.subNext
	p.events[name] = append(p.events[name], eventSub{id: id, fn: fn})
	held := p.held[name]
	delete(p.held, name)
	p.mu.Unlock()
	for _, payload := range held {
		fn(payload)
	}
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		subs := p.events[name]
		for i, s := range subs {
			if s.id == id {
				p.events[name] = append(subs[:i:i], subs[i+1:]...)
				return
			}
		}
	}
}

// maxHeldEvents bounds the events a name holds before a listener comes.
const maxHeldEvents = 64

// Done is closed when the connection ends.
func (p *Peer) Done() <-chan struct{} { return p.done }

// Err returns why the connection ended, nil while it is open.
func (p *Peer) Err() error { return p.closeErr() }

// Close ends the connection.
func (p *Peer) Close() { p.closeWith(ErrClosed) }

func (p *Peer) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func (p *Peer) addWait(ch chan Result) int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.next++
	id := p.next
	p.waits[id] = ch
	return id
}

func (p *Peer) forgetWait(id int64) {
	p.mu.Lock()
	delete(p.waits, id)
	p.mu.Unlock()
}

func (p *Peer) closeErr() error {
	if v := p.err.Load(); v != nil {
		return v.(error)
	}
	return ErrClosed
}

func (p *Peer) closeWith(err error) {
	p.closeOnce.Do(func() {
		p.err.Store(err)
		p.cancel()
		p.mu.Lock()
		p.closed = true
		waits := p.waits
		p.waits = map[int64]chan Result{}
		p.mu.Unlock()
		close(p.done)
		p.conn.Close()
		for _, ch := range waits {
			select {
			case ch <- Result{Err: err}:
			default:
			}
		}
	})
}

// read runs the peer's read loop, and closes the peer when the connection
// fails.
func (p *Peer) read() {
	for {
		payload, err := readMessage(p.conn)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				p.closeWith(ErrClosed)
			} else {
				p.closeWith(err)
			}
			return
		}
		var m Message
		if err := json.Unmarshal(payload, &m); err != nil {
			log.Printf("link: malformed message: %v", err)
			continue
		}
		switch m.T {
		case "call":
			go p.serveCall(&m)
		case "reply":
			p.deliver(&m)
		case "event":
			p.deliverEvent(&m)
		case "bye":
			p.closeWith(ErrClosed)
			return
		default:
			p.mu.Lock()
			fn := p.msgFn
			p.mu.Unlock()
			if fn == nil || !fn(&m) {
				log.Printf("link: unexpected message %q", m.T)
			}
		}
	}
}

func readMessage(r io.Reader) ([]byte, error) {
	var head [4]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(head[:])
	if n > MaxMessage {
		return nil, fmt.Errorf("link: message of %d bytes is over %d", n, MaxMessage)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// HandleAny serves the calls no Handle took, with their method name: a
// child whose services a router names at run time, as the child of
// mygoembed.Serve, registers its router once here.
func (p *Peer) HandleAny(fn func(ctx context.Context, method string, args []json.RawMessage) (any, error)) {
	p.mu.Lock()
	p.anyFn = fn
	p.mu.Unlock()
}

func (p *Peer) serveCall(m *Message) {
	p.mu.Lock()
	fn := p.handles[m.Method]
	anyFn := p.anyFn
	p.mu.Unlock()
	var (
		v   any
		err error
	)
	switch {
	case fn != nil:
		v, err = fn(p.ctx, m.Args)
	case anyFn != nil:
		v, err = anyFn(p.ctx, m.Method, m.Args)
	default:
		err = fmt.Errorf("link: no method %q", m.Method)
	}
	ok := err == nil
	reply := &Message{T: "reply", ID: m.ID, OK: &ok}
	if err != nil {
		reply.Error = err.Error()
	} else if v != nil {
		b, merr := json.Marshal(v)
		if merr != nil {
			no := false
			reply.OK = &no
			reply.Error = fmt.Sprintf("link: cannot encode result: %v", merr)
		} else {
			reply.Value = b
		}
	}
	if err := p.Send(reply); err != nil {
		log.Printf("link: cannot answer %s: %v", m.Method, err)
	}
}

func (p *Peer) deliver(m *Message) {
	p.mu.Lock()
	ch := p.waits[m.ID]
	delete(p.waits, m.ID)
	p.mu.Unlock()
	if ch == nil {
		return // a call canceled before its reply
	}
	r := Result{Value: m.Value}
	if m.OK != nil && !*m.OK {
		r.Err = fmt.Errorf("%s", m.Error)
	}
	ch <- r
}

func (p *Peer) deliverEvent(m *Message) {
	p.mu.Lock()
	subs := append([]eventSub(nil), p.events[m.Event]...)
	if len(subs) == 0 {
		held := append(p.held[m.Event], m.Payload)
		if len(held) > maxHeldEvents {
			held = held[len(held)-maxHeldEvents:]
		}
		p.held[m.Event] = held
	}
	p.mu.Unlock()
	for _, s := range subs {
		s.fn(m.Payload)
	}
}
