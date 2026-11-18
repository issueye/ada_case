// Package serve runs the assistant plugin: it answers the editor's hello,
// serves ai.stream and ai.cancel over the pipe, and owns the network call
// that produces the text.
//
// It knows nothing about the editor's window: the request carries the
// endpoint, the model and the key, and the answer leaves as deltas. That
// is the whole contract, and the only reason this project can be built,
// tested and shipped on its own.
package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/learningdog1/ada_link/link"

	"lightcode.ai/client/protocol"
	"lightcode.ai/client/provider"
)

// Options configure Run.
type Options struct {
	// Stdin and Stdout carry the protocol; nil means the process's own.
	Stdin  io.Reader
	Stdout io.Writer
	// Stderr takes the plugin's logs; nil discards them, as a windowed app
	// has no console to show them.
	Stderr io.Writer
	// Timeout bounds the editor's hello. Zero means ten seconds.
	Timeout time.Duration
}

// Run serves the plugin until the editor says bye or the pipe ends.
func Run(opts Options) error {
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}

	p := link.NewQuietPeer(link.NewPipeConn(opts.Stdin, opts.Stdout))
	hello := make(chan *link.Message, 1)
	p.OnMessage(func(m *link.Message) bool {
		if m.T != "hello" {
			return false
		}
		select {
		case hello <- m:
		default:
		}
		return true
	})
	s := &server{peer: p, streams: map[string]context.CancelFunc{}, stderr: opts.Stderr}
	p.Handle(protocol.MethodStream, s.stream)
	p.Handle(protocol.MethodCancel, s.cancel)
	p.Start()

	select {
	case h := <-hello:
		if h.Proto != protocol.Proto {
			return fmt.Errorf("serve: the editor speaks protocol %d, not %d", h.Proto, protocol.Proto)
		}
	case <-p.Done():
		return fmt.Errorf("serve: the editor exited before its hello: %v", p.Err())
	case <-time.After(opts.Timeout):
		return fmt.Errorf("serve: no hello in %v", opts.Timeout)
	}
	if err := p.Send(&link.Message{
		T: "welcome", Proto: protocol.Proto,
		Payload: mustJSON(protocol.Welcome{
			Name: protocol.Name, Version: protocol.Version,
			Methods: []string{protocol.MethodStream, protocol.MethodCancel},
		}),
	}); err != nil {
		return fmt.Errorf("serve: welcome: %w", err)
	}

	<-p.Done()
	if err := p.Err(); err != nil && !errors.Is(err, link.ErrClosed) {
		return err
	}
	return nil
}

// server is the plugin's state: its peer, and the answers that can still be
// stopped.
type server struct {
	peer    *link.Peer
	stderr  io.Writer
	mu      sync.Mutex
	streams map[string]context.CancelFunc
}

// stream answers one request, emitting its deltas as they arrive.
func (s *server) stream(ctx context.Context, args []json.RawMessage) (any, error) {
	if len(args) == 0 {
		return nil, errors.New("serve: stream needs a request")
	}
	var req protocol.StreamRequest
	if err := json.Unmarshal(args[0], &req); err != nil {
		return nil, fmt.Errorf("serve: cannot read the request: %w", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if req.RequestID != "" {
		s.mu.Lock()
		s.streams[req.RequestID] = cancel
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			delete(s.streams, req.RequestID)
			s.mu.Unlock()
		}()
	}
	text, err := Answer(ctx, req.Config, req.Messages, func(piece string) {
		_ = s.peer.Emit(protocol.EventDelta, protocol.Delta{RequestID: req.RequestID, Text: piece})
	})
	if err != nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil && ctx.Err() == nil {
		fmt.Fprintf(s.stderr, "serve: stream %s: %v\n", req.RequestID, err)
	}
	return protocol.StreamResult{RequestID: req.RequestID, Text: text}, err
}

// cancel stops the answer of a request ID, if it still runs.
func (s *server) cancel(_ context.Context, args []json.RawMessage) (any, error) {
	var req protocol.CancelRequest
	if len(args) > 0 {
		if err := json.Unmarshal(args[0], &req); err != nil {
			return nil, fmt.Errorf("serve: cannot read the cancel: %w", err)
		}
	}
	s.mu.Lock()
	cancel := s.streams[req.RequestID]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil, nil
}

// Answer asks an OpenAI-compatible endpoint for a reply, calling onDelta
// with each piece as it arrives. The protocol's stream method runs it; a
// test calls it directly, so both paths run the same code.
func Answer(ctx context.Context, cfg protocol.Config, msgs []protocol.ChatMessage, onDelta func(string)) (string, error) {
	cfg = cfg.Normalize()
	if cfg.APIKey == "" {
		return "", ErrNoKey
	}
	conversation := make([]provider.Message, len(msgs))
	for i, m := range msgs {
		conversation[i] = provider.Message{Role: m.Role, Content: m.Content}
	}
	client := provider.New(provider.Config{
		BaseURL:     cfg.BaseURL,
		Model:       cfg.Model,
		APIKey:      cfg.APIKey,
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
	})
	return client.Stream(ctx, conversation, onDelta)
}

// ErrNoKey is returned when the request carries no API key.
var ErrNoKey = errors.New("没有配置 API Key")

// mustJSON encodes a payload that cannot fail: the types are flat.
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
