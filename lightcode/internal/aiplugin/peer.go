package aiplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
)

// Peer is a message connection: calls, replies and events, both ways. It
// is link.Peer without the frame ring, on a child's standard streams.
type Peer struct {
	rw io.ReadWriteCloser

	// wmu serializes writes: a frame is written whole or not at all.
	wmu sync.Mutex

	mu      sync.Mutex
	next    int64
	nextSub uint64
	waits   map[int64]chan Result
	handles map[string]Handler
	events  map[string][]*eventSub
	msgFn   func(*Message) bool
	closed  bool
	err     error

	ctx    context.Context
	cancel context.CancelFunc

	once      sync.Once
	done      chan struct{}
	startOnce sync.Once
}

// eventSub is one listener of an event, with the id that removes it.
type eventSub struct {
	id uint64
	fn func(json.RawMessage)
}

// NewPeer takes connected pipes and starts its read loop.
func NewPeer(rw io.ReadWriteCloser) *Peer {
	p := NewQuietPeer(rw)
	p.Start()
	return p
}

// NewQuietPeer makes a peer whose read loop waits for Start, so that a
// caller can register its handlers and its OnMessage before any message
// arrives: the plugin does this after its welcome, the host after its
// hello.
func NewQuietPeer(rw io.ReadWriteCloser) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Peer{
		rw:      rw,
		waits:   map[int64]chan Result{},
		handles: map[string]Handler{},
		events:  map[string][]*eventSub{},
		ctx:     ctx,
		cancel:  cancel,
		done:    make(chan struct{}),
	}
}

// Start starts the read loop, once.
func (p *Peer) Start() {
	p.startOnce.Do(func() { go p.read() })
}

// Context is the peer's context: it ends when the connection does, so a
// handler that streams stops with the plugin.
func (p *Peer) Context() context.Context { return p.ctx }

// Send writes one message.
func (p *Peer) Send(m *Message) error {
	if p.isClosed() {
		return p.closeErr()
	}
	p.wmu.Lock()
	defer p.wmu.Unlock()
	if err := WriteMessage(p.rw, m); err != nil {
		p.closeWith(err)
		return err
	}
	return nil
}

// Handle serves method: a call whose Method is method runs fn. Calls to
// methods nobody handles answer an error.
func (p *Peer) Handle(method string, fn Handler) {
	p.mu.Lock()
	p.handles[method] = fn
	p.mu.Unlock()
}

// OnMessage takes the messages that are neither calls, replies nor
// events, such as "welcome" or "error". fn returns whether it took the
// message. It runs on the read goroutine, so it must not wait for a call.
func (p *Peer) OnMessage(fn func(*Message) bool) {
	p.mu.Lock()
	p.msgFn = fn
	p.mu.Unlock()
}

// Call calls method with args, which json.Marshal takes, and returns the
// reply's value. A context that ends first stops waiting; the peer still
// takes the reply when it comes.
func (p *Peer) Call(ctx context.Context, method string, args ...any) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	payloads := make([]json.RawMessage, len(args))
	for i, a := range args {
		b, err := json.Marshal(a)
		if err != nil {
			return nil, fmt.Errorf("aiplugin: argument %d of %s: %w", i+1, method, err)
		}
		payloads[i] = b
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

// Emit sends an event: name, and payload, which json.Marshal takes.
func (p *Peer) Emit(name string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("aiplugin: event %q: %w", name, err)
	}
	return p.Send(&Message{T: "event", Event: name, Payload: b})
}

// OnEvent calls fn for events named name, on the read goroutine. fn must
// not wait for the peer: a call it makes does, and the read loop cannot
// answer it meanwhile. Use a goroutine for that. The returned function
// stops the listener.
func (p *Peer) OnEvent(name string, fn func(json.RawMessage)) (off func()) {
	p.mu.Lock()
	p.nextSub++
	sub := &eventSub{id: p.nextSub, fn: fn}
	p.events[name] = append(p.events[name], sub)
	p.mu.Unlock()
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		subs := p.events[name]
		for i, s := range subs {
			if s == sub {
				p.events[name] = append(subs[:i:i], subs[i+1:]...)
				return
			}
		}
	}
}

// Done is closed when the connection ends.
func (p *Peer) Done() <-chan struct{} { return p.done }

// Err is why the connection ended, nil while it is open.
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
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	return ErrClosed
}

func (p *Peer) closeWith(err error) {
	p.once.Do(func() {
		p.mu.Lock()
		p.closed = true
		if p.err == nil {
			p.err = err
		}
		waits := p.waits
		p.waits = map[int64]chan Result{}
		p.mu.Unlock()
		p.cancel()
		close(p.done)
		p.rw.Close()
		for _, ch := range waits {
			select {
			case ch <- Result{Err: p.closeErr()}:
			default:
			}
		}
	})
}

// read runs the read loop, and closes the peer when the connection fails.
func (p *Peer) read() {
	for {
		m, err := ReadMessage(p.rw)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				p.closeWith(ErrClosed)
			} else {
				p.closeWith(err)
			}
			return
		}
		switch m.T {
		case "call":
			go p.serveCall(m)
		case "reply":
			p.deliver(m)
		case "event":
			p.deliverEvent(m)
		case "bye":
			p.closeWith(ErrClosed)
			return
		default:
			p.mu.Lock()
			fn := p.msgFn
			p.mu.Unlock()
			if fn == nil || !fn(m) {
				log.Printf("aiplugin: unexpected message %q", m.T)
			}
		}
	}
}

func (p *Peer) serveCall(m *Message) {
	p.mu.Lock()
	fn := p.handles[m.Method]
	p.mu.Unlock()
	var (
		v   any
		err error
	)
	if fn == nil {
		err = fmt.Errorf("aiplugin: no method %q", m.Method)
	} else {
		v, err = fn(p.ctx, m.Args)
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
			reply.Error = fmt.Sprintf("aiplugin: cannot encode result: %v", merr)
		} else {
			reply.Value = b
		}
	}
	if err := p.Send(reply); err != nil && !errors.Is(err, ErrClosed) {
		log.Printf("aiplugin: cannot answer %s: %v", m.Method, err)
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
		r.Err = errors.New(m.Error)
	}
	ch <- r
}

func (p *Peer) deliverEvent(m *Message) {
	p.mu.Lock()
	subs := append([]*eventSub(nil), p.events[m.Event]...)
	p.mu.Unlock()
	// No holding: the host listens before it calls, and the plugin only
	// emits events for calls it is answering.
	for _, s := range subs {
		s.fn(m.Payload)
	}
}
