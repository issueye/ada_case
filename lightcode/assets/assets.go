// Package assets holds the files the window reads at run time. They are
// embedded, so a built binary carries its mark rather than beside a data
// folder that may be missing.
package assets

import _ "embed"

// Icon is the app's mark: a 256×256 PNG, shown in the window's title bar
// and the taskbar. It is rendered from docs/logo.svg; change the SVG first,
// then regenerate the PNG, so the two stay one artwork.
//
//go:embed icon.png
var Icon []byte
