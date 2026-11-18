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

	"lightcode/internal/document"
)

// writeTestPNG writes a small PNG beside the workspace's files.
func writeTestPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

const testSVG = `<svg width="64" height="48" viewBox="0 0 64 48">` +
	`<rect width="64" height="48" fill="#5b9cff"/><circle cx="32" cy="24" r="10" fill="#ffffff"/></svg>`

// TestImageViewer shows a PNG as a picture: no editor, a read-only note and
// the picture's size.
func TestImageViewer(t *testing.T) {
	a, dir := testWorkspace(t)
	pngPath := filepath.Join(dir, "shot.png")
	writeTestPNG(t, pngPath, 6, 4)
	a.resetColumns()
	a.OpenFile(pngPath)
	doc := a.Col().Current().Doc
	if doc.Kind() != document.KindImage || len(doc.Data()) == 0 {
		t.Fatalf("document = %+v kind=%v", doc.Path, doc.Kind())
	}
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Frame()
	if !tt.HasText("只读预览 · 6×4") {
		t.Errorf("no viewer note; texts %q", tt.Texts())
	}
	if !tt.HasText("shot.png") {
		t.Errorf("no tab; texts %q", tt.Texts())
	}
}

// TestSVGPreview parses an SVG and draws it; a broken one says so instead of
// failing the frame.
func TestSVGPreview(t *testing.T) {
	a, dir := testWorkspace(t)
	svgPath := filepath.Join(dir, "mark.svg")
	if err := os.WriteFile(svgPath, []byte(testSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	a.resetColumns()
	a.OpenFile(svgPath)
	if a.Col().Current().Doc.Kind() != document.KindSVG {
		t.Fatalf("kind = %v", a.Col().Current().Doc.Kind())
	}
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Frame()
	if !tt.HasText("只读预览 · 64×48") {
		t.Errorf("SVG viewer note missing; texts %q", tt.Texts())
	}
}

// TestBrokenSVGSaysSo keeps a malformed picture from breaking the frame.
func TestBrokenSVGSaysSo(t *testing.T) {
	a, dir := testWorkspace(t)
	badPath := filepath.Join(dir, "bad.svg")
	if err := os.WriteFile(badPath, []byte(`<svg><path d="M0 0`), 0o644); err != nil {
		t.Fatal(err)
	}
	a.resetColumns()
	a.OpenFile(badPath)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Frame()
	if !tt.HasText("无法预览这个文件") {
		t.Errorf("no error note; texts %q", tt.Texts())
	}
}

// TestMarkdownPreviewToggle renders a Markdown document, then goes back to
// its source.
func TestMarkdownPreviewToggle(t *testing.T) {
	a, dir := testWorkspace(t)
	a.resetColumns()
	a.OpenFile(filepath.Join(dir, "README.md"))
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Frame()
	if err := tt.Click("预览"); err != nil {
		t.Fatalf("preview button: %v", err)
	}
	tt.Frame()
	if !tt.HasText("A test workspace.") {
		t.Fatalf("rendered Markdown missing; texts %q", tt.Texts())
	}
	if !tt.HasText("源码") {
		t.Errorf("the button did not turn into 源码; texts %q", tt.Texts())
	}
	if strings.Contains(strings.Join(tt.Texts(), "\n"), "# Demo") {
		t.Errorf("the source is still shown")
	}
	if err := tt.Click("源码"); err != nil {
		t.Fatalf("source button: %v", err)
	}
	tt.Frame()
	if e := a.Col().Current(); e != nil && e.MarkdownPreview {
		t.Errorf("the document stayed in preview")
	}
	if !tt.HasText("预览") {
		t.Errorf("the button did not turn back into 预览; texts %q", tt.Texts())
	}
}

// TestViewerRestores keeps a picture in the restored session: the file is
// reopened as a viewer, not as text.
func TestViewerRestores(t *testing.T) {
	a, dir := testWorkspace(t)
	pngPath := filepath.Join(dir, "restored.png")
	writeTestPNG(t, pngPath, 2, 2)
	a.resetColumns()
	a.OpenFile(pngPath)
	a.SaveState()
	path := a.StatePath
	a2 := New(dir, "")
	a2.StatePath = path
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "restored.png") {
		t.Errorf("the session does not list the picture: %s", data)
	}
}

// TestViewerNotSaved keeps Ctrl+S from writing a picture.
func TestViewerNotSaved(t *testing.T) {
	a, dir := testWorkspace(t)
	pngPath := filepath.Join(dir, "readonly.png")
	writeTestPNG(t, pngPath, 2, 2)
	a.resetColumns()
	a.OpenFile(pngPath)
	before, err := os.ReadFile(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	a.SaveActive()
	after, err := os.ReadFile(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("the picture was rewritten")
	}
}
