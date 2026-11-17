package document

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// ErrBinary is returned for files that hold NUL bytes.
var ErrBinary = errors.New("binary file")

// ErrEncoding is returned for text this app cannot decode.
var ErrEncoding = errors.New("unreadable text encoding")

// Encodings the app reads and writes.
const (
	EncUTF8    = "UTF-8"
	EncUTF8BOM = "UTF-8 BOM"
	EncGBK     = "GBK"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Document is one file open in the app.
type Document struct {
	Path string
	// Buffer is the text as it is edited.
	Buffer *editor.Buffer
	// Base is the text as it was on disk, which the gutter marks the
	// changes against.
	Base *editor.Buffer

	encoding string
	modTime  time.Time
	size     int64
	readOnly bool
	// external marks a file that changed on disk while it has edits here.
	external bool
}

// Open reads path into a Document.
func Open(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	text, enc, err := decode(data)
	if err != nil {
		return nil, err
	}
	d := &Document{
		Path:     path,
		encoding: enc,
		modTime:  info.ModTime(),
		size:     info.Size(),
		readOnly: info.Mode().Perm()&0o200 == 0,
	}
	d.set(text)
	return d, nil
}

// decode reads text in UTF-8 with or without a BOM, or in GBK. It rejects
// binary files and encodings it cannot prove, rather than showing
// mojibake.
func decode(data []byte) (string, string, error) {
	if bytes.IndexByte(data, 0) >= 0 {
		return "", "", ErrBinary
	}
	if bytes.HasPrefix(data, utf8BOM) {
		rest := data[len(utf8BOM):]
		if utf8.Valid(rest) {
			return string(rest), EncUTF8BOM, nil
		}
		return "", "", ErrEncoding
	}
	if utf8.Valid(data) {
		return string(data), EncUTF8, nil
	}
	// GBK (GB18030's subset): accept it only when what it decodes to
	// encodes back to the same bytes.
	dec := simplifiedchinese.GB18030.NewDecoder()
	decoded, err := dec.Bytes(data)
	if err != nil {
		return "", "", ErrEncoding
	}
	enc := simplifiedchinese.GB18030.NewEncoder()
	round, err := enc.Bytes(decoded)
	if err != nil || !bytes.Equal(round, data) {
		return "", "", ErrEncoding
	}
	return string(decoded), EncGBK, nil
}

func (d *Document) set(text string) {
	lang := editor.LanguageFor(filepath.Base(d.Path))
	buf := editor.NewBuffer(text)
	buf.SetLanguage(lang)
	base := editor.NewBuffer(text)
	base.SetLanguage(lang)
	d.Buffer, d.Base = buf, base
}

// Name is the file's base name.
func (d *Document) Name() string { return filepath.Base(d.Path) }

// Dir is the folder the file is in.
func (d *Document) Dir() string { return filepath.Dir(d.Path) }

// Dirty reports unsaved changes.
func (d *Document) Dirty() bool { return d.Buffer.Modified() }

// ReadOnly reports whether the file denies writing.
func (d *Document) ReadOnly() bool { return d.readOnly }

// Encoding is the file's encoding, as "UTF-8", "UTF-8 BOM" or "GBK".
func (d *Document) Encoding() string { return d.encoding }

// Language is the name of the editor's language, as "Go".
func (d *Document) Language() string {
	if l := d.Buffer.Language(); l != nil {
		return l.Name
	}
	return "Plain Text"
}

// LineEnding is "LF" or "CRLF".
func (d *Document) LineEnding() string {
	if d.Buffer.LineEnding() == "\r\n" {
		return "CRLF"
	}
	return "LF"
}

// Save writes the buffer to disk and marks it saved, in the encoding the
// file was read in.
func (d *Document) Save() error {
	var buf bytes.Buffer
	if _, err := d.Buffer.WriteTo(&buf); err != nil {
		return err
	}
	data, err := d.encode(buf.String())
	if err != nil {
		return err
	}
	info, err := os.Stat(d.Path)
	if err == nil && info.Mode().Perm()&0o200 == 0 {
		return os.ErrPermission
	}
	tmp := d.Path + ".lighttmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, d.Path); err != nil {
		os.Remove(tmp)
		return err
	}
	if info, err := os.Stat(d.Path); err == nil {
		d.modTime, d.size = info.ModTime(), info.Size()
	}
	d.Buffer.MarkSaved()
	d.Base.Set(d.Buffer.String())
	d.external = false
	return nil
}

func (d *Document) encode(text string) ([]byte, error) {
	switch d.encoding {
	case EncGBK:
		return simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(text))
	case EncUTF8BOM:
		return append(append([]byte{}, utf8BOM...), text...), nil
	default:
		return []byte(text), nil
	}
}

// ChangedOnDisk reports whether the file changed after it was read or
// saved.
func (d *Document) ChangedOnDisk() bool {
	info, err := os.Stat(d.Path)
	if err != nil {
		return true
	}
	return !info.ModTime().Equal(d.modTime) || info.Size() != d.size
}

// Reload replaces the buffer with the file's text, which loses changes.
func (d *Document) Reload() error {
	data, err := os.ReadFile(d.Path)
	if err != nil {
		return err
	}
	info, err := os.Stat(d.Path)
	if err != nil {
		return err
	}
	text, enc, err := decode(data)
	if err != nil {
		return err
	}
	d.encoding = enc
	d.set(text)
	d.modTime, d.size = info.ModTime(), info.Size()
	d.external = false
	return nil
}

// SetExternalChanged marks that the file changed on disk while this
// document holds edits, for the view to ask what to do.
func (d *Document) SetExternalChanged(changed bool) { d.external = changed }

// ExternalChanged reports a file changed on disk under unsaved edits.
func (d *Document) ExternalChanged() bool { return d.external }
