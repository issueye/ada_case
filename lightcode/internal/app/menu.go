package app

import (
	"github.com/egoist/mygo/ui"

	"lightcode/internal/theme"
)

// menuItem is one row of the app's own context menus.
type menuItem struct {
	label string
	hint  string
	run   func()
	// sep makes the item a separator line rather than a row.
	sep bool
}

// The rows whose context menus the app draws itself, rather than letting
// the system draw a white one that ignores the palette.
const (
	menuNone = iota
	menuExplorer
	menuSCM
)

// rowMenu is the explorer or SCM row whose context menu is open.
type rowMenu struct {
	open bool
	kind int
	path string
	// scmKind is the SCM group of an SCM row: "staged", "changes",
	// "untracked" or "conflicts".
	scmKind string
	isDir   bool
}

// menuPopup shows items below anchor while *open is true. It builds the
// panel with PopoverBase, not Popover: the theme's popover look would put
// its own background, border, padding and shadow behind the menu card and
// read as a second, larger background around it in the light palette.
func (a *App) menuPopup(c *ui.Context, anchor ui.Element, open *bool, items []menuItem, closeMenu func()) {
	ui.PopoverBase(c, anchor, open, func(panel ui.Element) {
		panel.Margin(4, 0, 0, 0)
		a.menuBody(c, items, closeMenu)
	})
}

// menuBody draws a context menu in the app's own style: the same width,
// row height and type as the tab menu, so every right click in the window
// looks alike. closeMenu shuts the menu the items belong to.
func (a *App) menuBody(c *ui.Context, items []menuItem, closeMenu func()) {
	ui.Column(c).Width(216).PaddingY(4).Radius(6).Background(theme.Panel).
		Border(1, theme.Border).Shadow(0, 4, 12, 0, ui.RGBA(0, 0, 0, 0.20)).
		Children(func() {
			for _, it := range items {
				if it.sep {
					ui.Box(c).Height(1).Background(theme.BorderSoft)
					continue
				}
				item := it
				row := ui.Row(c).Height(28).PaddingX(10).Gap(8).AlignItems(ui.Center).
					Cursor(ui.CursorPointer)
				row.Children(func() {
					ui.Text(c, item.label).FontSize(11).TextColor(theme.Text).Grow(1).SingleLine()
					if item.hint != "" {
						ui.Text(c, item.hint).FontSize(9).TextColor(theme.Faint)
					}
				})
				if row.Hovered() {
					row.Background(theme.Hover)
				}
				row.OnClick(func() {
					if closeMenu != nil {
						closeMenu()
					}
					if item.run != nil {
						item.run()
					}
				})
			}
		})
}
