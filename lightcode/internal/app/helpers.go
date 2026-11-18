package app

import (
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"lightcode/internal/icons"
	"lightcode/internal/theme"
)

func isMac() bool { return runtime.GOOS == "darwin" }

// itoa is strconv.Itoa for the view's labels.
func itoa(n int) string { return strconv.Itoa(n) }

// relPath returns path relative to root, with slashes.
func relPath(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// fileIcon is the icon of a file, by its extension.
func fileIcon(name string) *ui.SVG {
	switch ext(name) {
	case "png", "jpg", "jpeg", "gif", "webp", "bmp", "ico", "svg":
		return icons.Image
	}
	return icons.File
}

func ext(name string) string {
	e := strings.ToLower(filepath.Ext(name))
	return strings.TrimPrefix(e, ".")
}

// fileColor colors a file icon by its language, as the boards do.
func fileColor(name string) ui.Color {
	switch ext(name) {
	case "go":
		return ui.Hex("#4ec9b0")
	case "md", "mdx":
		return ui.Hex("#519aba")
	case "json", "yaml", "yml", "toml", "ini":
		return ui.Hex("#e2c07a")
	case "js", "mjs", "cjs", "ts", "tsx", "jsx":
		return ui.Hex("#e5c07b")
	case "css", "scss", "less":
		return ui.Hex("#c678dd")
	case "html", "htm", "vue", "svelte":
		return ui.Hex("#e06c75")
	case "png", "jpg", "jpeg", "svg", "webp", "gif", "ico":
		return ui.Hex("#98c379")
	case "sh", "bash", "ps1", "zsh", "py":
		return ui.Hex("#56b6c2")
	case "rs":
		return ui.Hex("#e06c75")
	default:
		return theme.Muted
	}
}

// WindowTitle formats the title with the modified mark.
func (a *App) WindowTitle() string {
	e := a.Col().Current()
	t := "轻码 IDE"
	if e != nil {
		t = e.Doc.Name() + " — 轻码 IDE"
		if e.Doc.Dirty() {
			t = "● " + t
		}
	}
	if a.WS != nil {
		t += " · " + a.WS.Name()
	}
	return t
}
