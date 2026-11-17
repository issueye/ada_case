package app

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotBaseline compares the main window to a baseline image, so
// that a layout or theme regression fails the tests. Set
// LIGHTCODE_UPDATE_BASELINE=1 to regenerate the baseline.
func TestScreenshotBaseline(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "main.go"))
	a.AI.Mode = AIDock
	a.SidebarOpen = true
	a.SidebarView = 0
	tt := ui.NewTester(a.View, 1280, 820)
	img := tt.Image()

	basePath := filepath.Join("testdata", "shots", "editor.png")
	if os.Getenv("LIGHTCODE_UPDATE_BASELINE") == "" {
		base, err := readPNG(basePath)
		if err == nil {
			if diff := pixelDiff(base, img); diff > 0.01 {
				t.Fatalf("the window differs from %s in %.2f%% of pixels", basePath, diff*100)
			}
			return
		}
		if !os.IsNotExist(err) {
			t.Fatalf("baseline: %v", err)
		}
	}
	if err := writePNG(basePath, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote baseline %s", basePath)
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// pixelDiff is the share of pixels whose channels differ by more than a
// rounding step, 0 when the images match.
func pixelDiff(a, b image.Image) float64 {
	ab, bb := a.Bounds(), b.Bounds()
	if ab.Dx() != bb.Dx() || ab.Dy() != bb.Dy() {
		return 1
	}
	total := ab.Dx() * ab.Dy()
	diff := 0
	for y := ab.Min.Y; y < ab.Max.Y; y++ {
		for x := ab.Min.X; x < ab.Max.X; x++ {
			ar, ag, abl, aa := a.At(x, y).RGBA()
			br, bg, bbl, ba := b.At(x, y).RGBA()
			if absDiff(ar, br) > 8<<8 || absDiff(ag, bg) > 8<<8 ||
				absDiff(abl, bbl) > 8<<8 || absDiff(aa, ba) > 8<<8 {
				diff++
			}
		}
	}
	return float64(diff) / float64(total)
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}
