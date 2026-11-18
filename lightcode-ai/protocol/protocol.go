// Package protocol is the contract between the editor and this plugin:
// the messages that cross the pipe, and nothing about how either side
// draws its screen.
//
// The envelope is link.Message — call, reply, event and the hello/welcome
// handshake — so both ends share one transport implementation. This
// package only names what travels inside it.
package protocol

import "strings"

// Proto is the wire version both ends check in the handshake. The shapes
// below travel with it; a mismatch is refused rather than guessed.
const Proto = 1

// Name and Version identify this plugin in its welcome, and in the
// editor's capabilities view.
const (
	Name    = "lightcode-ai"
	Version = "0.2.0"
)

// Methods the plugin serves.
const (
	// MethodStream asks for one answer: a StreamRequest. The reply is a
	// StreamResult, and the text arrives meanwhile as EventDelta.
	MethodStream = "ai.stream"
	// MethodCancel stops one answer by its request ID.
	MethodCancel = "ai.cancel"
)

// EventDelta names a piece of an answer, a Delta.
const EventDelta = "delta"

// Welcome is the plugin's answer to the editor's hello: who it is and what
// it serves.
type Welcome struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Methods []string `json:"methods,omitempty"`
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
// cancel together: the editor may run more than one question, and a late
// piece of an older answer must not land in a newer one.
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
