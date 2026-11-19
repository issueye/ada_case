package document

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// encodePNG makes a w×h PNG for a picture test.
func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.NRGBA{R: 255, G: 80, B: 40, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestOpenPNGPreview opens a PNG as a read-only picture: its size and
// format, a refused save, and a reload after it changed.
func TestOpenPNGPreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shot.png")
	data := encodePNG(t, 4, 3)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !d.IsPicture() || d.PictureFormat() != "PNG" {
		t.Fatalf("not a PNG picture: %v %q", d.IsPicture(), d.PictureFormat())
	}
	if w, h := d.PictureSize(); w != 4 || h != 3 {
		t.Errorf("size = %v×%v, want 4×3", w, h)
	}
	if !d.ReadOnly() || d.Dirty() {
		t.Errorf("read-only %v, dirty %v", d.ReadOnly(), d.Dirty())
	}
	if err := d.Save(); !errors.Is(err, ErrPicture) {
		t.Errorf("Save = %v, want ErrPicture", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Errorf("the picture file changed: %v", err)
	}

	if err := os.WriteFile(path, encodePNG(t, 2, 2), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := d.Reload(); err != nil {
		t.Fatal(err)
	}
	if w, h := d.PictureSize(); w != 2 || h != 2 {
		t.Errorf("reloaded size = %v×%v, want 2×2", w, h)
	}
}

// TestOpenSVGPreview opens an SVG as a picture with its own size.
func TestOpenSVGPreview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mark.svg")
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="12" height="8"><rect x="0" y="0" width="12" height="8" fill="#f00"/></svg>`)
	if err := os.WriteFile(path, svg, 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !d.IsPicture() || d.PictureFormat() != "SVG" {
		t.Fatalf("not an SVG picture: %v %q", d.IsPicture(), d.PictureFormat())
	}
	if w, h := d.PictureSize(); w != 12 || h != 8 {
		t.Errorf("size = %v×%v, want 12×8", w, h)
	}
}

// TestOpenSVGAsText opens an SVG this build cannot draw as its source.
func TestOpenSVGAsText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.svg")
	svg := []byte(`<svg><`)
	if err := os.WriteFile(path, svg, 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.IsPicture() {
		t.Errorf("a broken SVG opened as a picture")
	}
	if got := d.Buffer.String(); got != string(svg) {
		t.Errorf("source = %q, want %q", got, svg)
	}
}
