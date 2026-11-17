package command

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestParseChord(t *testing.T) {
	cases := []struct {
		spec  string
		mods  ui.Modifiers
		key   ui.Key
		valid bool
	}{
		{"CmdOrCtrl+S", ui.Cmd, ui.KeyS, true},
		{"CmdOrCtrl+Shift+P", ui.Cmd | ui.Shift, ui.KeyP, true},
		{"Ctrl+Enter", ui.Ctrl, ui.KeyEnter, true},
		{"Alt+F5", ui.Alt, ui.KeyF5, true},
		{"Hyper+S", 0, 0, false},
		{"CmdOrCtrl+", 0, 0, false},
	}
	for _, c := range cases {
		ch, ok := ParseChord(c.spec)
		if ok != c.valid {
			t.Errorf("%q: ok = %v, want %v", c.spec, ok, c.valid)
			continue
		}
		if !ok {
			continue
		}
		if ch.Mods != c.mods || ch.Key != c.key {
			t.Errorf("%q: got %v %v, want %v %v", c.spec, ch.Mods, ch.Key, c.mods, c.key)
		}
	}
}

func TestRegistryRun(t *testing.T) {
	r := NewRegistry()
	ran := false
	r.Add(&Command{ID: "a", Title: "A", Run: func() { ran = true }})
	if !r.Run("a") || !ran {
		t.Errorf("command did not run")
	}
	if r.Run("missing") {
		t.Errorf("missing command reported as run")
	}
}
