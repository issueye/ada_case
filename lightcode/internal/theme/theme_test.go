package theme

import "testing"

// TestUseSwitches checks the two palettes and that switching back restores
// the dark colors.
func TestUseSwitches(t *testing.T) {
	Use(false)
	if IsLight() {
		t.Fatalf("dark palette reports light")
	}
	darkBG := Background
	Use(true)
	if !IsLight() {
		t.Fatalf("light palette reports dark")
	}
	if Background == darkBG {
		t.Errorf("the light background matches the dark one")
	}
	if Window().Dark {
		t.Errorf("the light window theme says dark")
	}
	Use(false)
	if Window().Dark != true {
		t.Errorf("the dark window theme says light")
	}
}
