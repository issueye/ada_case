package document

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// ErrPicture is returned when saving an image, which is a preview and never
// edited.
var ErrPicture = errors.New("图片以预览打开，不能保存")

// pictureExts are the extensions the app previews as pictures, with the
// name the status bar shows.
var pictureExts = map[string]string{
	".png":  "PNG",
	".jpg":  "JPEG",
	".jpeg": "JPEG",
	".gif":  "GIF",
	".webp": "WebP",
	".bmp":  "BMP",
	".svg":  "SVG",
}

// pictureFormat returns the format of an image file, "" for other files.
func pictureFormat(path string) string {
	return pictureExts[strings.ToLower(filepath.Ext(path))]
}

// picture holds the decoded image of a file the editor previews.
type picture struct {
	src    ui.ImageSource
	format string
}

// openPicture reads path as a picture, and reports whether it is one: a
// raster the app cannot decode is an error, while an SVG it cannot draw is
// only text, which opens as source.
func openPicture(path string, data []byte) (*picture, bool, error) {
	format := pictureFormat(path)
	if format == "" {
		return nil, false, nil
	}
	src, err := ui.DecodeImage(data)
	if err != nil {
		if format == "SVG" && isText(data) {
			return nil, false, nil
		}
		return nil, true, fmt.Errorf("无法预览图片 %s：%w", filepath.Base(path), err)
	}
	return &picture{src: src, format: format}, true, nil
}

// loadPicture rereads a picture's file, keeping its format.
func loadPicture(path string) (*picture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p, _, err := openPicture(path, data)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrPicture
	}
	return p, nil
}

// isText reports whether data reads as text, for an SVG this build cannot
// draw: its source still opens.
func isText(data []byte) bool {
	_, _, err := decode(data)
	return err == nil
}
