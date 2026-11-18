package link

// InputKind names what an InputEvent reports.
type InputKind string

// The kinds of input. They mirror ui.InputEvent's kinds, plus focus and
// drop, which MyGo reports elsewhere than an element's input.
const (
	InputDown    InputKind = "down"
	InputUp      InputKind = "up"
	InputMove    InputKind = "move"
	InputScroll  InputKind = "scroll"
	InputZoom    InputKind = "zoom"
	InputKeyDown InputKind = "keydown"
	InputKeyUp   InputKind = "keyup"
	InputText    InputKind = "text"
	InputCompose InputKind = "compose"
	// InputCommand is an edit command of the menus, as "copy" or "paste".
	InputCommand InputKind = "command"
	// InputFocus takes or gives the keyboard: Focused says which.
	InputFocus InputKind = "focus"
	// InputDrop is files dropped on the child's area, in Files.
	InputDrop InputKind = "drop"
	// InputClipboard is the child asking the host for its clipboard, or
	// telling it the child copied something: the host answers with
	// InputClipboard, Text carrying the text and Write telling the
	// direction. Version 1 passes text only.
	InputClipboard InputKind = "clipboard"
)

// InputEvent is one input event for the child, in the child's DIPs.
//
// X and Y are relative to the child's area, not the host's window. Key,
// Physical and Mods are ui.Key, ui.Key and ui.Modifiers values: the
// protocol pins them to the MyGo version both processes are built with
// (docs/protocol.md).
type InputEvent struct {
	Kind InputKind `json:"k"`

	// The pointer.
	X      float32 `json:"x,omitempty"`
	Y      float32 `json:"y,omitempty"`
	Button int     `json:"b,omitempty"`
	Clicks int     `json:"c,omitempty"`
	DX     float32 `json:"dx,omitempty"`
	DY     float32 `json:"dy,omitempty"`
	Scale  float32 `json:"s,omitempty"`
	// Precise and Momentum are InputScroll's, as a touchpad's.
	Precise  bool `json:"p,omitempty"`
	Momentum bool `json:"m,omitempty"`

	// The keyboard: Key is what the layout types and Physical where the key
	// is on a US keyboard. Repeat marks a key held down.
	Key      uint16 `json:"key,omitempty"`
	Physical uint16 `json:"pk,omitempty"`
	Mods     uint8  `json:"mods,omitempty"`
	Repeat   bool   `json:"r,omitempty"`
	// Text is what InputText typed or InputCompose composed, and InputCommand
	// names the command; Caret is InputCompose's caret, in runes.
	Text  string `json:"t,omitempty"`
	Caret int    `json:"caret,omitempty"`

	// Focused is InputFocus's: whether the child has the keyboard.
	Focused bool `json:"f,omitempty"`
	// Files are InputDrop's paths.
	Files []string `json:"files,omitempty"`
	// Write tells InputClipboard's direction: true when the child copied
	// what Text holds, false when it asks for the host's clipboard.
	Write bool `json:"w,omitempty"`
}
