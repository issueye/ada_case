// Package aiplugin carries the assistant across a process boundary: a
// child process reads requests on its standard input, streams answers on
// its standard output, and the app talks to it through Client.
//
// The vocabulary follows ada_link's link.Peer — hello, welcome, call,
// reply, event, bye — and its framing, a 4-byte little-endian length and a
// JSON payload. What differs is the transport: a spawned child's private
// pipes rather than a socket, so there is no token to check and no
// directory to clean up, and the assistant's text rather than pixels.
//
// The plugin side is plugins/aiprovider; this package is the host's client
// and the protocol both ends share.
package aiplugin

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Proto is the version of the wire protocol both ends speak. A peer that
// speaks another is refused: the request and event shapes travel with it.
const Proto = 1

// MaxMessage bounds one message. Answers stream in small deltas, so the
// bound only catches a mistake; a larger message ends the session.
const MaxMessage = 16 << 20

// Name and Version identify the plugin in its welcome and in the
// capabilities view.
const (
	Name    = "lightcode-ai"
	Version = "0.1.0"
)

// ErrClosed is returned by calls on a peer whose pipes ended.
var ErrClosed = errors.New("aiplugin: closed")

// Methods the plugin serves.
const (
	// MethodStream asks for one answer: a StreamRequest, answered when the
	// answer ends, and streamed meanwhile as EventDelta.
	MethodStream = "ai.stream"
	// MethodCancel stops one answer by its request ID.
	MethodCancel = "ai.cancel"
)

// EventDelta names a piece of an answer, a Delta.
const EventDelta = "delta"

// Message is one wire message. Fields are left empty when they do not
// apply to T, which is one of "hello", "welcome", "call", "reply",
// "event", "bye" or "error".
type Message struct {
	T string `json:"t"`

	// hello and welcome: Proto is the wire version; the welcome also
	// names the plugin and the methods it serves.
	Proto   int      `json:"proto,omitempty"`
	Name    string   `json:"name,omitempty"`
	Version string   `json:"version,omitempty"`
	Methods []string `json:"methods,omitempty"`

	// call, reply and event.
	ID      int64             `json:"id,omitempty"`
	Method  string            `json:"m,omitempty"`
	Args    []json.RawMessage `json:"a,omitempty"`
	OK      *bool             `json:"ok,omitempty"`
	Value   json.RawMessage   `json:"v,omitempty"`
	Error   string            `json:"e,omitempty"`
	Event   string            `json:"n,omitempty"`
	Payload json.RawMessage   `json:"p,omitempty"`
}

// Config is where and how to ask: an OpenAI-compatible endpoint, the
// model, the key and the sampling knobs. It crosses the pipe with every
// request, so the plugin keeps the key in memory only.
type Config struct {
	// BaseURL ends before /chat/completions, as https://api.openai.com/v1.
	BaseURL string `json:"baseURL,omitempty"`
	Model   string `json:"model,omitempty"`
	APIKey  string `json:"apiKey,omitempty"`
	// Temperature is the sampling temperature, 0 for the API's default.
	Temperature float32 `json:"temperature,omitempty"`
	// MaxTokens caps the reply, 0 for the API's default.
	MaxTokens int `json:"maxTokens,omitempty"`
}

// DefaultConfig is the OpenAI endpoint with a small, cheap model.
func DefaultConfig() Config {
	return Config{BaseURL: "https://api.openai.com/v1", Model: "gpt-4o-mini", Temperature: 0.2}
}

// Normalize fills what the config leaves out.
func (c Config) Normalize() Config {
	if c.BaseURL == "" {
		c.BaseURL = DefaultConfig().BaseURL
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if c.Model == "" {
		c.Model = DefaultConfig().Model
	}
	return c
}

// ChatMessage is one turn of a conversation.
type ChatMessage struct {
	// Role is "system", "user" or "assistant".
	Role    string `json:"role"`
	Content string `json:"content"`
}

// StreamRequest asks for one answer. RequestID ties its deltas and its
// cancel together.
type StreamRequest struct {
	RequestID string        `json:"requestID"`
	Config    Config        `json:"config"`
	Messages  []ChatMessage `json:"messages"`
}

// Delta is one piece of an answer.
type Delta struct {
	RequestID string `json:"requestID"`
	Text      string `json:"text"`
}

// StreamResult is an answer's whole text, with what arrived before an
// error.
type StreamResult struct {
	RequestID string `json:"requestID"`
	Text      string `json:"text"`
}

// CancelRequest stops the answer of RequestID.
type CancelRequest struct {
	RequestID string `json:"requestID"`
}

// Result is what a call answers: the reply's value, or its error.
type Result struct {
	Value json.RawMessage
	Err   error
}

// Handler serves one method. It runs on a goroutine of its own, with the
// peer's context, which ends with the connection.
type Handler func(ctx context.Context, args []json.RawMessage) (any, error)

// WriteMessage writes one length-prefixed message.
func WriteMessage(w io.Writer, m *Message) error {
	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(payload) > MaxMessage {
		return fmt.Errorf("aiplugin: message of %d bytes is over %d", len(payload), MaxMessage)
	}
	buf := make([]byte, 4+len(payload))
	binary.LittleEndian.PutUint32(buf, uint32(len(payload)))
	copy(buf[4:], payload)
	_, err = w.Write(buf)
	return err
}

// ReadMessage reads one message the other end wrote.
func ReadMessage(r io.Reader) (*Message, error) {
	var head [4]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(head[:])
	if n > MaxMessage {
		return nil, fmt.Errorf("aiplugin: message of %d bytes is over %d", n, MaxMessage)
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	var m Message
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, fmt.Errorf("aiplugin: malformed message: %w", err)
	}
	return &m, nil
}
