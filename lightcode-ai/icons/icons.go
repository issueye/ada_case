// Package icons holds the SVGs the assistant panel draws. Only SVG: the
// editor's boards never use emoji or characters as icons, and the panel
// follows the same rule. Each is on a 24 grid with a 1.6 stroke, colored by
// the text color where it shows.
package icons

import "github.com/egoist/mygo/ui"

func icon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

var (
	Sparkles   = icon(`<path d="M12 4l1.7 4.8L18.5 10.5l-4.8 1.7L12 17l-1.7-4.8L5.5 10.5l4.8-1.7z"/><path d="M18.5 16.5l.6 1.6 1.6.6-1.6.6-.6 1.6-.6-1.6-1.6z"/>`)
	PanelLeft  = icon(`<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/>`)
	PanelRight = icon(`<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M15 4v16"/>`)
	Split      = icon(`<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M12 4v16"/>`)
	Settings   = icon(`<path d="M4 7h10M18 7h2M4 17h4M12 17h8"/><circle cx="16" cy="7" r="2"/><circle cx="10" cy="17" r="2"/>`)
	Close      = icon(`<path d="M6 6l12 12M18 6 6 18"/>`)
	Plus       = icon(`<path d="M12 5v14M5 12h14"/>`)
	ArrowRight = icon(`<path d="M5 12h14M12 5l7 7-7 7"/>`)
	File       = icon(`<path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4"/>`)
)
