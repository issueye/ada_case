package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestPicturePreviewTab opens a PNG in a tab: the preview shows its name,
// format and size, and saving refuses it.
func TestPicturePreviewTab(t *testing.T) {
	a, dir := testWorkspace(t)
	path := filepath.Join(dir, "shot.png")
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(0, 0, color.NRGBA{R: 255, G: 80, B: 40, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	a.OpenFile(path)
	d := a.Col().Current().Doc
	if !d.IsPicture() || d.PictureFormat() != "PNG" {
		t.Fatalf("not a PNG picture: %v %q", d.IsPicture(), d.PictureFormat())
	}
	tt := ui.NewTester(a.View, 1280, 820)
	for _, want := range []string{"shot.png", "PNG", "4 × 3"} {
		if !tt.HasText(want) {
			t.Errorf("the preview misses %q; texts %q", want, tt.Texts())
		}
	}
	a.SaveActive()
	if !strings.Contains(a.Message, "图片预览") {
		t.Errorf("saving said %q, want a picture preview refusal", a.Message)
	}
}
