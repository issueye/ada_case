// Package aiprovider is the assistant's process plugin: it is spawned by
// the app, reads the aiplugin protocol on its standard input, streams an
// OpenAI-compatible answer back on its standard output, and keeps the API
// key in memory only.
//
// The plugin owns the network call: the app's window never waits on the
// endpoint, a hung or crashed endpoint leaves the editor alone, and a new
// process replaces a dead one. When the answer's text must span processes
// it travels as small deltas, the shape aiplugin.Delta carries.
package aiprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"lightcode/internal/ai"
	"lightcode/internal/aiplugin"
)

// Options configure Serve.
type Options struct {
	// Stdin and Stdout carry the protocol; nil means the process's own.
	Stdin  io.Reader
	Stdout io.Writer
	// Stderr takes the plugin's logs; nil discards them, as a windowed app
	// has no console to show them.
	Stderr io.Writer
}

// Serve runs the plugin: it answers the host's hello, then serves its
// methods until the host says bye or the pipes end.
func Serve(opts Options) error {
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	hello, err := aiplugin.ReadMessage(opts.Stdin)
	if err != nil {
		return fmt.Errorf("aiprovider: read hello: %w", err)
	}
	if hello.T != "hello" {
		return fmt.Errorf("aiprovider: the host's first message is %q, not hello", hello.T)
	}
	if hello.Proto != aiplugin.Proto {
		return fmt.Errorf("aiprovider: the host speaks protocol %d, not %d", hello.Proto, aiplugin.Proto)
	}
	if err := aiplugin.WriteMessage(opts.Stdout, &aiplugin.Message{
		T: "welcome", Proto: aiplugin.Proto,
		Name: aiplugin.Name, Version: aiplugin.Version,
		Methods: []string{aiplugin.MethodStream, aiplugin.MethodCancel},
	}); err != nil {
		return fmt.Errorf("aiprovider: write welcome: %w", err)
	}

	s := &server{out: opts.Stdout, streams: map[string]context.CancelFunc{}, stderr: opts.Stderr}
	p := aiplugin.NewQuietPeer(stdioConn{r: opts.Stdin, w: opts.Stdout})
	p.Handle(aiplugin.MethodStream, s.stream)
	p.Handle(aiplugin.MethodCancel, s.cancel)
	p.Start()
	<-p.Done()
	if err := p.Err(); err != nil && !errors.Is(err, aiplugin.ErrClosed) {
		return err
	}
	return nil
}

// server is the plugin's state: the process's output stream, and the
// answers that can still be stopped.
type server struct {
	out    io.Writer
	stderr io.Writer

	mu      sync.Mutex
	streams map[string]context.CancelFunc
}

// stream answers one request, emitting its deltas as they arrive.
func (s *server) stream(ctx context.Context, args []json.RawMessage) (any, error) {
	if len(args) == 0 {
		return nil, errors.New("aiprovider: stream needs a request")
	}
	var req aiplugin.StreamRequest
	if err := json.Unmarshal(args[0], &req); err != nil {
		return nil, fmt.Errorf("aiprovider: cannot read the request: %w", err)
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
	text, err := Stream(ctx, req.Config, req.Messages, func(piece string) {
		_ = aiplugin.WriteMessage(s.out, &aiplugin.Message{
			T: "event", Event: aiplugin.EventDelta,
			Payload: mustJSON(aiplugin.Delta{RequestID: req.RequestID, Text: piece}),
		})
	})
	if err != nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil && ctx.Err() == nil {
		s.logf("stream %s: %v", req.RequestID, err)
	}
	return aiplugin.StreamResult{RequestID: req.RequestID, Text: text}, err
}

// cancel stops the answer of a request ID, if it still runs.
func (s *server) cancel(_ context.Context, args []json.RawMessage) (any, error) {
	var req aiplugin.CancelRequest
	if len(args) > 0 {
		if err := json.Unmarshal(args[0], &req); err != nil {
			return nil, fmt.Errorf("aiprovider: cannot read the cancel: %w", err)
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

// Stream asks an OpenAI-compatible endpoint for an answer, calling onDelta
// with each piece as it arrives. The window's assistant reaches it through
// Serve; a headless test or a host without a window calls it directly, so
// both paths run the same code.
func Stream(ctx context.Context, cfg aiplugin.Config, msgs []aiplugin.ChatMessage, onDelta func(string)) (string, error) {
	cfg = cfg.Normalize()
	if cfg.APIKey == "" {
		return "", ErrNoKey
	}
	conversation := make([]ai.Message, len(msgs))
	for i, m := range msgs {
		conversation[i] = ai.Message{Role: m.Role, Content: m.Content}
	}
	client := ai.New(ai.Config{
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

// stdioConn joins a process's standard streams into one connection.
type stdioConn struct {
	r io.Reader
	w io.Writer
}

func (c stdioConn) Read(p []byte) (int, error)  { return c.r.Read(p) }
func (c stdioConn) Write(p []byte) (int, error) { return c.w.Write(p) }
func (c stdioConn) Close() error                { return nil }

// mustJSON encodes a payload that cannot fail: the types here are flat.
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

// Logf writes a line to the plugin's standard error, when it has one.
func (s *server) logf(format string, args ...any) {
	if s.stderr == nil {
		return
	}
	fmt.Fprintf(s.stderr, "aiprovider: "+format+"\n", args...)
}
