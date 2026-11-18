package document

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func writeFile(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpenSaveReload(t *testing.T) {
	path := writeFile(t, "package main\n\nfunc main() {}\n")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Language() != "Go" {
		t.Errorf("language = %q", d.Language())
	}
	if d.Dirty() {
		t.Errorf("a new document is dirty")
	}
	d.Buffer.Set("package main\n\nfunc main() { println(\"hi\") }\n")
	if !d.Dirty() {
		t.Fatalf("Set did not mark the document dirty")
	}
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	if d.Dirty() {
		t.Errorf("still dirty after Save")
	}
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "println") {
		t.Errorf("saved text = %q", text)
	}
	if d.ChangedOnDisk() {
		t.Errorf("the document reports a change it saved")
	}
	d.Buffer.Set("package main\n\n// reloaded\n")
	if err := d.Reload(); err != nil {
		t.Fatal(err)
	}
	if d.Dirty() {
		t.Errorf("Reload kept the modified mark")
	}
	if got := d.Buffer.String(); !strings.Contains(got, "func main()") {
		t.Errorf("Reload text = %q", got)
	}
}

func TestLineEndingRoundTrip(t *testing.T) {
	path := writeFile(t, "a\r\nb\r\n")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.LineEnding() != "CRLF" {
		t.Fatalf("line ending = %q", d.LineEnding())
	}
	d.Buffer.Set("a\r\nb\r\nc\r\n")
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(text); got != "a\r\nb\r\nc\r\n" {
		t.Errorf("saved %q, want CRLF lines", got)
	}
}

func TestBinaryRejected(t *testing.T) {
	path := writeFile(t, "abc\x00def")
	if _, err := Open(path); err != ErrBinary {
		t.Errorf("Open binary = %v, want ErrBinary", err)
	}
}

func TestGBKRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gbk.txt")
	text := "中文测试\n第二行\n"
	data, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Encoding() != EncGBK {
		t.Fatalf("encoding = %q", d.Encoding())
	}
	if got := d.Buffer.String(); got != text {
		t.Fatalf("decoded = %q", got)
	}
	d.Buffer.Set(text + "第三行\n")
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	back, err := simplifiedchinese.GB18030.NewDecoder().Bytes(saved)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(back); got != text+"第三行\n" {
		t.Errorf("saved text decodes to %q", got)
	}
}

func TestUTF8BOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bom.txt")
	body := "hello\n"
	if err := os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, body...), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Encoding() != EncUTF8BOM {
		t.Fatalf("encoding = %q", d.Encoding())
	}
	if got := d.Buffer.String(); got != body {
		t.Errorf("BOM leaked into the buffer: %q", got)
	}
	if err := d.Save(); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(saved), "\xEF\xBB\xBF") {
		t.Errorf("BOM lost on save: %q", saved)
	}
}

func TestChangedOnDisk(t *testing.T) {
	path := writeFile(t, "one\n")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.ChangedOnDisk() {
		t.Fatalf("changed right after opening")
	}
	if err := os.Chtimes(path, time.Now(), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if !d.ChangedOnDisk() {
		t.Errorf("a newer mtime went unnoticed")
	}
}

func TestViewerKinds(t *testing.T) {
	for path, want := range map[string]Kind{
		"a.png": KindImage, "b.JPG": KindImage, "c.webp": KindImage, "d.bmp": KindImage, "e.svg": KindSVG, "f.go": KindText, "g.md": KindText,
	} {
		if got := ViewerKind(path); got != want {
			t.Errorf("ViewerKind(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestOpenViewerKeepsBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pic.png")
	data := []byte{0x89, 'P', 'N', 'G', 0, 0, 0}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := OpenAny(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind() != KindImage || !d.IsViewer() {
		t.Fatalf("kind = %v", d.Kind())
	}
	if string(d.Data()) != string(data) {
		t.Errorf("bytes = %v", d.Data())
	}
	if d.Dirty() || !d.ReadOnly() {
		t.Errorf("viewer dirty=%v readOnly=%v", d.Dirty(), d.ReadOnly())
	}
	if err := d.Save(); err != ErrViewerReadOnly {
		t.Errorf("Save = %v, want ErrViewerReadOnly", err)
	}
	if d.Language() != "PNG" || d.LineEnding() != "—" {
		t.Errorf("language=%q lineEnding=%q", d.Language(), d.LineEnding())
	}
}

func TestViewerReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pic.svg")
	if err := os.WriteFile(path, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := OpenAny(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind() != KindSVG || d.Language() != "SVG" {
		t.Fatalf("kind=%v language=%q", d.Kind(), d.Language())
	}
	if err := os.WriteFile(path, []byte("<svg><rect/></svg>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := d.Reload(); err != nil {
		t.Fatal(err)
	}
	if string(d.Data()) != "<svg><rect/></svg>" {
		t.Errorf("bytes after reload = %q", d.Data())
	}
}
