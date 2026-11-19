package app

import (
	"github.com/egoist/mygo/ui"

	"lightcode/internal/theme"
)

// dialogWidth is how wide the app's dialogs are.
const dialogWidth float32 = 420

// appDialog shows fn's content in the app's own dialog: a panel of the
// app's panel color over a dimmed window, as the command palette and the
// diff window, with one border and one shadow. ui.Modal would add the
// theme's own panel behind this one, a second, white background around it
// in the light palette.
func appDialog(c *ui.Context, open *bool, fn func()) {
	ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.36))
		panel.Width(dialogWidth).Gap(10).Padding(18).Radius(8).
			Background(theme.Panel).Border(1, theme.Border).
			Shadow(0, 18, 48, 0, ui.RGBA(0, 0, 0, 0.55))
		panel.Children(fn)
	})
}
