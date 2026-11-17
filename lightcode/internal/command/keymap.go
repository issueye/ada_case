package command

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// Chord is a parsed keybinding: modifiers and a key.
type Chord struct {
	Mods ui.Modifiers
	Key  ui.Key
}

// ParseChord parses a VS Code style keybinding, as "CmdOrCtrl+Shift+S".
// It accepts CmdOrCtrl, Cmd, Ctrl, Alt and Shift, then one key name.
func ParseChord(spec string) (Chord, bool) {
	var ch Chord
	parts := strings.Split(strings.TrimSpace(spec), "+")
	if len(parts) == 0 {
		return ch, false
	}
	keyName := parts[len(parts)-1]
	for _, p := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "cmdorctrl":
			ch.Mods |= ui.Cmd
		case "cmd":
			ch.Mods |= ui.Super
		case "ctrl":
			ch.Mods |= ui.Ctrl
		case "alt", "option":
			ch.Mods |= ui.Alt
		case "shift":
			ch.Mods |= ui.Shift
		default:
			return ch, false
		}
	}
	key, ok := keys[strings.ToLower(keyName)]
	if !ok {
		return ch, false
	}
	ch.Key = key
	return ch, true
}

var keys = map[string]ui.Key{
	"s": ui.KeyS, "w": ui.KeyW, "p": ui.KeyP, "b": ui.KeyB, "f": ui.KeyF,
	"g": ui.KeyG, "n": ui.KeyN, "o": ui.KeyO, "q": ui.KeyQ, "r": ui.KeyR,
	"t": ui.KeyT, "h": ui.KeyH, "j": ui.KeyJ, "k": ui.KeyK, "l": ui.KeyL,
	"m": ui.KeyM, "u": ui.KeyU, "v": ui.KeyV, "x": ui.KeyX, "y": ui.KeyY,
	"z": ui.KeyZ, "a": ui.KeyA, "c": ui.KeyC, "d": ui.KeyD, "e": ui.KeyE,
	"i": ui.KeyI,
	"0": ui.Key0, "1": ui.Key1, "2": ui.Key2, "3": ui.Key3, "4": ui.Key4,
	"5": ui.Key5, "6": ui.Key6, "7": ui.Key7, "8": ui.Key8, "9": ui.Key9,
	"escape": ui.KeyEscape, "enter": ui.KeyEnter, "tab": ui.KeyTab,
	"space": ui.KeySpace, "backspace": ui.KeyBackspace, "delete": ui.KeyDelete,
	"home": ui.KeyHome, "end": ui.KeyEnd, "pageup": ui.KeyPageUp,
	"pagedown": ui.KeyPageDown, "up": ui.KeyUp, "down": ui.KeyDown,
	"left": ui.KeyLeft, "right": ui.KeyRight,
	"f1": ui.KeyF1, "f2": ui.KeyF2, "f3": ui.KeyF3, "f4": ui.KeyF4,
	"f5": ui.KeyF5, "f6": ui.KeyF6, "f7": ui.KeyF7, "f8": ui.KeyF8,
	"f9": ui.KeyF9, "f10": ui.KeyF10, "f11": ui.KeyF11, "f12": ui.KeyF12,
	"`": ui.KeyBackquote, "-": ui.KeyMinus, "=": ui.KeyEqual,
	"[": ui.KeyBracketLeft, "]": ui.KeyBracketRight,
	",": ui.KeyComma, ".": ui.KeyPeriod, "/": ui.KeySlash,
	"\\": ui.KeyBackslash, ";": ui.KeySemicolon, "'": ui.KeyQuote,
}
