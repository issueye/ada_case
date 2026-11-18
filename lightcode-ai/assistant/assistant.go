// Package assistant is the assistant's own UI: the panel, the conversation,
// the input and the diff cards all live and draw here, in the plugin
// process. The editor only gives it a rectangle, forwards input and does the
// work the panel cannot (read the open file, apply a patch, keep the key).
package assistant

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui/editor"

	"lightcode.ai/client/protocol"
	"lightcode.ai/client/provider"
	"lightcode.ai/client/theme"
)

// The mounts the editor can give the panel: docked at the right edge,
// floating over the code, or a rail card.
const (
	ModeDock  = 0
	ModeFloat = 1
	ModeRail  = 2
)

// Bridge is what the panel asks of the editor: the parts that need the
// open file, the window or the credentials.
type Bridge interface {
	// Context returns the system prompt and the open file's text.
	Context(ctx context.Context) ([]protocol.ChatMessage, error)
	// Answer hands the answer's text to the editor, which parses its diffs
	// and returns the cards with the file's current text.
	Answer(ctx context.Context, requestID, text string) (protocol.AnswerResult, error)
	// Apply writes one card's change into its file.
	Apply(ctx context.Context, requestID string, index int) (string, error)
	// SetMode asks for another mount.
	SetMode(mode int)
	// Settings opens the editor's endpoint/key dialog.
	Settings()
	// MoveTo asks the host to put a floating mount at x, y.
	MoveTo(target int, x, y float32)
}

// Message is one turn of the conversation.
type Message struct {
	// Role is "user" or "assistant".
	Role string
	// Full is the text received; Content the part shown so far, which the
	// panel reveals a few runes at a time, as a typewriter.
	Full    string
	Content string
	// Cards are the answer's file changes.
	Cards []*Card
	// Err is why this turn failed.
	Err string
}

// Card is one file's change in an answer. Old and New come from the editor,
// which read the file; the panel only draws them.
type Card struct {
	Index   int
	Path    string
	Old     string
	New     string
	Err     string
	Open    bool
	Applied bool
	// Hidden is the user discarding the card; it is not drawn.
	Hidden bool

	state *editor.DiffState
}

// State is the panel's state across frames.
type State struct {
	// Bridge talks to the editor; Refresh asks for another frame.
	Bridge  Bridge
	Refresh func()

	mu        sync.Mutex
	light     bool
	mode      int
	originX   float32
	originY   float32
	config    protocol.Config
	messages  []Message
	draft     string
	streaming bool
	reveal    bool
	err       string
	cancel    context.CancelFunc
	seq       uint64
}

// New makes the panel's state.
func New(bridge Bridge, refresh func()) *State {
	return &State{Bridge: bridge, Refresh: refresh}
}

// Mode is the mount the panel draws for.
func (s *State) Mode() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
}

// Configure takes the editor's push: endpoint, key, palette and mount.
func (s *State) Configure(raw json.RawMessage) error {
	var cfg protocol.Configure
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	s.mu.Lock()
	s.config = protocol.Config{BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: cfg.APIKey}
	s.light = cfg.Light
	s.mode = cfg.Mode
	s.originX, s.originY = cfg.X, cfg.Y
	s.mu.Unlock()
	theme.Use(cfg.Light)
	if s.Refresh != nil {
		s.Refresh()
	}
	return nil
}

// Origin takes the host's report of where the mount sits, which a drag
// needs to turn local pointer coordinates into window ones.
func (s *State) Origin(x, y float32) {
	s.mu.Lock()
	s.originX, s.originY = x, y
	s.mu.Unlock()
}

// SetOrigin is Origin for the tests' spelling.
func (s *State) SetOrigin(x, y float32) { s.Origin(x, y) }

// Send sends the draft with the editor's context.
func (s *State) Send() {
	s.mu.Lock()
	text := strings.TrimSpace(s.draft)
	if text == "" || s.streaming {
		s.mu.Unlock()
		return
	}
	if s.config.APIKey == "" {
		s.err = "先在编辑器的 AI 设置里填 API Key"
		s.mu.Unlock()
		s.Bridge.Settings()
		s.refresh()
		return
	}
	s.draft = ""
	s.err = ""
	s.messages = append(s.messages, Message{Role: "user", Content: text}, Message{Role: "assistant"})
	idx := len(s.messages) - 1
	s.seq++
	seq := s.seq
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.streaming = true
	cfg := s.config
	s.mu.Unlock()
	s.refresh()
	go s.run(ctx, seq, idx, text, cfg)
}

// run asks the endpoint for the answer, streaming it into the message.
func (s *State) run(ctx context.Context, seq uint64, idx int, question string, cfg protocol.Config) {
	contextMsgs, err := s.Bridge.Context(ctx)
	if err != nil {
		s.finish(seq, idx, err)
		return
	}
	s.mu.Lock()
	var history []protocol.ChatMessage
	for i, m := range s.messages {
		if i == idx {
			break
		}
		text := m.Content
		if m.Role == "assistant" && m.Full != "" {
			text = m.Full
		}
		if text != "" {
			history = append(history, protocol.ChatMessage{Role: m.Role, Content: text})
		}
	}
	s.mu.Unlock()
	msgs := append(append([]protocol.ChatMessage{}, contextMsgs...), history...)
	msgs = append(msgs, protocol.ChatMessage{Role: "user", Content: question})

	conversation := make([]provider.Message, len(msgs))
	for i, m := range msgs {
		conversation[i] = provider.Message{Role: m.Role, Content: m.Content}
	}
	_, err = provider.New(provider.Config{
		BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: cfg.APIKey,
		Temperature: cfg.Temperature, MaxTokens: cfg.MaxTokens,
	}).Stream(ctx, conversation, func(piece string) {
		s.mu.Lock()
		if s.seq != seq {
			s.mu.Unlock()
			return
		}
		s.messages[idx].Full += piece
		s.reveal = true
		s.mu.Unlock()
		s.refresh()
	})
	s.finish(seq, idx, err)
}

// finish settles a streamed answer: its error, and the diff cards the
// editor builds from its text.
func (s *State) finish(seq uint64, idx int, err error) {
	s.mu.Lock()
	if s.seq != seq || idx >= len(s.messages) {
		s.mu.Unlock()
		return
	}
	s.streaming = false
	s.messages[idx].Full = strings.TrimSpace(s.messages[idx].Full)
	full := s.messages[idx].Full
	s.mu.Unlock()
	if err != nil && notCanceled(err) {
		s.mu.Lock()
		s.messages[idx].Err = err.Error()
		s.err = err.Error()
		s.mu.Unlock()
	}
	s.refresh()
	if full == "" || err != nil {
		return
	}
	res, aerr := s.Bridge.Answer(context.Background(), s.requestID(seq), full)
	if aerr != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seq != seq || idx >= len(s.messages) {
		return
	}
	for _, c := range res.Cards {
		s.messages[idx].Cards = append(s.messages[idx].Cards, &Card{
			Index: c.Index, Path: c.Path, Old: c.Old, New: c.New, Err: c.Err,
		})
	}
	if s.Refresh != nil {
		s.Refresh()
	}
}

// Cancel stops the stream, keeping what arrived.
func (s *State) Cancel() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.streaming = false
	s.mu.Unlock()
	s.refresh()
}

// NewChat clears the conversation.
func (s *State) NewChat() {
	s.Cancel()
	s.mu.Lock()
	s.messages = nil
	s.err = ""
	s.reveal = false
	s.mu.Unlock()
	s.refresh()
}

// Retry asks the last question again.
func (s *State) Retry() {
	s.mu.Lock()
	for i := len(s.messages) - 1; i >= 0; i-- {
		if s.messages[i].Role == "user" {
			text := s.messages[i].Content
			s.messages = s.messages[:i]
			s.draft = text
			s.mu.Unlock()
			s.Send()
			return
		}
	}
	s.mu.Unlock()
}

// Apply asks the editor to write one card's change, and marks it applied.
func (s *State) Apply(card *Card, seq uint64) {
	if card.Applied {
		return
	}
	go func() {
		msg, err := s.Bridge.Apply(context.Background(), s.requestID(seq), card.Index)
		s.mu.Lock()
		if err != nil {
			card.Err = err.Error()
		} else if msg != "" {
			card.Err = msg
		} else {
			card.Applied = true
		}
		s.mu.Unlock()
		s.refresh()
	}()
}

// requestID is the id the editor files an answer's cards under.
func (s *State) requestID(seq uint64) string {
	return "answer-" + itoa(int(seq))
}

// advance reveals more of an answer's text, paced by the frames rather than
// by the stream: a backlog catches up in a few frames, the tail types out.
func (s *State) advance() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.advanceLocked()
}

// advanceLocked is advance for a caller already holding the lock.
func (s *State) advanceLocked() bool {
	more := false
	for i := range s.messages {
		m := &s.messages[i]
		if m.Full == "" || m.Full == m.Content {
			continue
		}
		runes := []rune(m.Full)
		shown := len([]rune(m.Content))
		if shown >= len(runes) {
			m.Content = m.Full
			continue
		}
		step := (len(runes)-shown)/12 + 3
		if !s.streaming {
			step = (len(runes)-shown)/8 + 3
		}
		if next := min(shown+step, len(runes)); next > shown {
			m.Content = string(runes[:next])
		}
		if m.Content != m.Full {
			more = true
		}
	}
	s.reveal = more
	return more
}

// refresh asks the host for a frame with the panel's new state.
func (s *State) refresh() {
	if s.Refresh != nil {
		s.Refresh()
	}
}

// ctxAlive reports whether an error is a cancellation, which the panel keeps
// quiet about: the user asked for it.
func notCanceled(err error) bool {
	return err != nil && !strings.Contains(err.Error(), "context canceled")
}

// itoa avoids pulling strconv in for one call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
