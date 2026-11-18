package assets

import (
	"bytes"
	"image/png"
	"testing"
)

// TestIcon checks the embedded mark decodes and is square: a broken or
// replaced PNG would show as a blank title bar icon rather than a build
// error, so the test reads it.
func TestIcon(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(Icon))
	if err != nil {
		t.Fatalf("icon.png: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 256 || b.Dy() != 256 {
		t.Errorf("icon is %dx%d, want 256x256 (regenerate it from docs/logo.svg)", b.Dx(), b.Dy())
	}
}
