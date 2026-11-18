// Package protocol is the contract between the editor and the assistant
// panel: what the editor pushes into the panel, and what the panel asks the
// editor to do. The panel draws its own UI, so the messages are about
// configuration and file work, not pixels — the pixels travel through
// ada_link's frame ring.
//
// The envelope is link.Message; the handshake, frames and input belong to
// mygoembed. This package only names what the two ends say to each other.
package protocol

// Name and Version identify the panel in the editor's capabilities view.
// Bump the version when a message changes meaning.
const (
	Name    = "lightcode-ai"
	Version = "0.3.0"
)

// Methods the editor calls on the panel, and the panel calls on the editor.
const (
	// MethodConfigure (editor → panel) replaces the panel's configuration:
	// endpoint, model, key, palette and the mount it draws for.
	MethodConfigure = "ai.configure"
	// MethodContext (panel → editor) returns the system prompt and the open
	// file's text for the next question.
	MethodContext = "editor.context"
	// MethodAnswer (panel → editor) hands an answer's text to the editor,
	// which parses its diffs and returns the cards to draw.
	MethodAnswer = "editor.answer"
	// MethodApply (panel → editor) writes one card's change into its file.
	MethodApply = "editor.apply"
	// MethodSetMode (panel → editor) asks for another mount.
	MethodSetMode = "editor.setMode"
	// MethodSettings (panel → editor) opens the editor's endpoint/key
	// dialog, where the key is stored in the system's credential store.
	MethodSettings = "ai.settings"
)

// EventMove is the panel telling the editor its header was dragged: the
// pointer's place in the window, in DIPs.
const EventMove = "ai.move"

// EventOrigin is the editor telling the panel where its mount sits, so a
// drag can compute the pointer's window position while the mount follows
// it.
const EventOrigin = "ai.origin"

// Config is where and how to ask: an OpenAI-compatible endpoint, the model,
// the key and the sampling knobs. The key crosses the process boundary with
// the configuration and stays in the panel's memory.
type Config struct {
	BaseURL     string  `json:"baseURL,omitempty"`
	Model       string  `json:"model,omitempty"`
	APIKey      string  `json:"apiKey,omitempty"`
	Temperature float32 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"maxTokens,omitempty"`
}

// ChatMessage is one turn of a conversation.
type ChatMessage struct {
	// Role is "system", "user" or "assistant".
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Configure is the editor's push of everything the panel needs: where to
// ask, how to look, and which mount the editor laid out.
type Configure struct {
	BaseURL string `json:"baseURL,omitempty"`
	Model   string `json:"model,omitempty"`
	APIKey  string `json:"apiKey,omitempty"`
	// Light picks the light palette, matching the editor's theme.
	Light bool `json:"light,omitempty"`
	// Mode is the mount the editor shows: 0 docked, 1 floating, 2 rail.
	Mode int `json:"mode"`
	// X and Y are the mount's top left in the window, in DIPs.
	X float32 `json:"x"`
	Y float32 `json:"y"`
}

// ContextRequest asks the editor for the next question's context.
type ContextRequest struct{}

// Context is the question's context, built by the editor from the open file
// and its selection.
type Context struct {
	Messages []ChatMessage `json:"messages,omitempty"`
}

// AnswerRequest hands an answer's whole text to the editor.
type AnswerRequest struct {
	RequestID string `json:"requestID"`
	Text      string `json:"text"`
}

// Card is one file's change, with the text the editor's buffer holds now.
type Card struct {
	Index int    `json:"index"`
	Path  string `json:"path"`
	Old   string `json:"old,omitempty"`
	New   string `json:"new,omitempty"`
	Err   string `json:"err,omitempty"`
}

// AnswerResult carries the cards of one answer.
type AnswerResult struct {
	Cards []Card `json:"cards,omitempty"`
}

// ApplyRequest writes one card's change into its file.
type ApplyRequest struct {
	RequestID string `json:"requestID"`
	Index     int    `json:"index"`
}

// ApplyResult is an apply's outcome: "" when the file changed.
type ApplyResult struct {
	Err string `json:"err,omitempty"`
}

// ModeRequest asks the editor for another mount.
type ModeRequest struct {
	Mode int `json:"mode"`
}

// MoveRequest asks the editor to put a floating mount at x, y: the
// pointer's place in the window, less where it took hold of the panel.
type MoveRequest struct {
	// Target is 1 for the floating panel, 2 the rail card, matching the
	// mount modes.
	Target int     `json:"target"`
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
}

// Origin is where the editor put a mount, in the window's DIPs.
type Origin struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
}
