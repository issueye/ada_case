// Package icons embeds the SVG icons of the chrome. Icons are only SVGs:
// the boards never use emoji or characters as icons, and neither does the
// app. Each is drawn on a 24 grid with a 1.6 stroke, colored by the text
// color of where it shows.
package icons

import "github.com/egoist/mygo/ui"

func icon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

// Activity bar.
var (
	Explorer   = icon(`<path d="M3 7a2 2 0 0 1 2-2h4l2 3h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>`)
	Search     = icon(`<circle cx="11" cy="11" r="7"/><path d="M16.5 16.5 21 21"/>`)
	GitBranch  = icon(`<path d="M6 3v12"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="6" r="3"/><path d="M18 9a9 9 0 0 1-9 9"/>`)
	Run        = icon(`<path d="M7 4.5 19 12 7 19.5z"/>`)
	Extensions = icon(`<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>`)
	Settings   = icon(`<path d="M4 7h10M18 7h2M4 17h4M12 17h8"/><circle cx="16" cy="7" r="2"/><circle cx="10" cy="17" r="2"/>`)
	Packages   = icon(`<path d="M12 3 3 7.5v9L12 21l9-4.5v-9z"/><path d="M3 7.5 12 12l9-4.5M12 12v9"/>`)
)

// Explorer.
var (
	ChevronRight = icon(`<path d="M9 6l6 6-6 6"/>`)
	ChevronDown  = icon(`<path d="M6 9l6 6 6-6"/>`)
	Folder       = icon(`<path d="M3 7a2 2 0 0 1 2-2h4l2 3h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>`)
	FolderOpen   = icon(`<path d="M3 8a2 2 0 0 1 2-2h3.5l2 3H19a2 2 0 0 1 2 2"/><path d="M3 8v9a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2l-2-6H6l-3 8z"/>`)
	File         = icon(`<path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4"/>`)
	NewFile      = icon(`<path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4"/><path d="M11.5 13.5v5M9 16h5"/>`)
	NewFolder    = icon(`<path d="M3 7a2 2 0 0 1 2-2h4l2 3h8a2 2 0 0 1 2 2v3"/><path d="M3 7v10a2 2 0 0 0 2 2h6"/><path d="M17 14v6M14 17h6"/>`)
	Refresh      = icon(`<path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/>`)
	Collapse     = icon(`<path d="M9 6l-4 6 4 6M15 6l4 6-4 6"/>`)
	Ellipsis     = icon(`<circle cx="6" cy="12" r="1.2"/><circle cx="12" cy="12" r="1.2"/><circle cx="18" cy="12" r="1.2"/>`)
	Menu         = icon(`<path d="M4 7h16M4 12h16M4 17h16"/>`)
	Sun          = icon(`<circle cx="12" cy="12" r="4"/><path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6l1.4 1.4M17 17l1.4 1.4M18.4 5.6L17 7M7 17l-1.4 1.4"/>`)
	Moon         = icon(`<path d="M20 14.5A8.5 8.5 0 0 1 9.5 4a8.5 8.5 0 1 0 10.5 10.5z"/>`)
)

// Editor chrome.
var (
	Close      = icon(`<path d="M6 6l12 12M18 6 6 18"/>`)
	Plus       = icon(`<path d="M12 5v14M5 12h14"/>`)
	Dot        = ui.MustParseSVG([]byte(`<svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="3.2"/></svg>`))
	Split      = icon(`<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M12 4v16"/>`)
	Terminal   = icon(`<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M7 9l4 3-4 3M12.5 16H17"/>`)
	History    = icon(`<path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/><path d="M12 7v5l3 2"/>`)
	Commit     = icon(`<circle cx="12" cy="12" r="3.5"/><path d="M3 12h5.5M15.5 12H21"/>`)
	ArrowLeft  = icon(`<path d="M19 12H5M12 19l-7-7 7-7"/>`)
	ArrowRight = icon(`<path d="M5 12h14M12 5l7 7-7 7"/>`)
	Check      = icon(`<path d="M5 12l5 5 9-9"/>`)
	Save       = icon(`<path d="M5 4h11l3 3v13H5z"/><path d="M8 4v5h7V4M8 20v-6h8v6"/>`)
	Wrap       = icon(`<path d="M4 6h16M4 12h11a3 3 0 0 1 0 6h-4"/><path d="M13 15l-2 3 2 3"/>`)
	Filter     = icon(`<path d="M3 5h18l-7 8v6l-4 2v-8z"/>`)
)

// Window controls, drawn where the view wants them (HideWindowButtons).
var (
	Minimize = icon(`<path d="M5 12h14"/>`)
	Maximize = icon(`<rect x="6" y="6" width="12" height="12" rx="1"/>`)
	Restore  = icon(`<rect x="4" y="8" width="11" height="11" rx="1"/><path d="M8 8V5a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1h-3"/>`)
	WindowX  = icon(`<path d="M6 6l12 12M18 6 6 18"/>`)
)
