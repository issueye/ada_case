package app

import (
	"github.com/egoist/mygo/ui"

	"lightcode/internal/icons"
	"lightcode/internal/theme"
)

// titleBar draws the window's 40 DIPs of title bar: the app menu and the
// workspace at the left, the command center in the middle, the window
// controls at the right. The window hides its own title bar
// (HideWindowButtons), so the whole strip drags the window.
func (a *App) titleBar(c *ui.Context) {
	bar := ui.Row(c).Height(theme.TitleBarH).Background(theme.Panel).
		Padding(0, 10).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.BorderSoft).
		DragWindow()
	bar.Children(func() {
		menu := ui.ButtonBase(c).Label("主菜单").Tooltip("主菜单").Padding(5).Radius(4).
			Children(func() { ui.Icon(c, icons.Menu).FontSize(16).TextColor(theme.Muted) })
		if menu.Clicked() {
			a.OpenPalette(PaletteCommands)
		}
		ui.Text(c, "轻码").FontSize(12).Bold().TextColor(theme.Text).SingleLine()
		if a.WS != nil {
			ui.Text(c, a.WS.Name()).FontSize(12).TextColor(theme.Faint).SingleLine()
		}
		ui.Row(c).Grow(1).Justify(ui.Center).Children(func() {
			a.commandCenter(c)
		})
		a.windowControls(c)
	})
}

// commandCenter is the field in the middle of the title bar: a click
// opens quick open, as Ctrl+P does.
func (a *App) commandCenter(c *ui.Context) {
	center := ui.Row(c).Width(440).Height(26).Gap(6).Padding(0, 8).Radius(6).
		AlignItems(ui.Center).Background(theme.PanelDeep).
		Border(1, theme.BorderSoft).Cursor(ui.CursorPointer)
	center.OnClick(func() { a.OpenPalette(PaletteFiles) })
	center.Children(func() {
		ui.Icon(c, icons.Search).FontSize(13).TextColor(theme.Faint)
		label := "搜索文件与命令"
		if e := a.Col().Current(); e != nil {
			label = e.Doc.Path
			if a.WS != nil {
				if rel, err := relPath(a.WS.Root, e.Doc.Path); err == nil {
					label = rel
				}
			}
		}
		ui.Text(c, label).FontSize(12).TextColor(theme.Muted).SingleLine().Grow(1)
		hint := "Ctrl+P"
		if isMac() {
			hint = "⌘P"
		}
		ui.Text(c, hint).FontSize(11).TextColor(theme.Faint)
	})
	if center.Hovered() {
		center.Background(theme.Hover).Border(1, theme.Border)
	}
}

// windowControls draws minimize, maximize and close, as the window has no
// controls of its own (HideWindowButtons).
func (a *App) windowControls(c *ui.Context) {
	if a.Win == nil {
		return
	}
	win := a.Win
	minimize := ui.ButtonBase(c).Label("最小化").Tooltip("最小化").Size(40, 28).Radius(4).
		Children(func() { ui.Icon(c, icons.Minimize).FontSize(14).TextColor(theme.Muted) })
	minimize.OnClick(func() { win.Minimize() })
	maximize := ui.ButtonBase(c).Label("最大化").Tooltip("最大化").Size(40, 28).Radius(4).
		Children(func() {
			if win.IsMaximized() {
				ui.Icon(c, icons.Restore).FontSize(13).TextColor(theme.Muted)
			} else {
				ui.Icon(c, icons.Maximize).FontSize(12).TextColor(theme.Muted)
			}
		})
	maximize.OnClick(func() {
		if win.IsMaximized() {
			win.Unmaximize()
		} else {
			win.Maximize()
		}
	})
	closeBtn := ui.ButtonBase(c).Label("关闭窗口").Tooltip("关闭窗口").Size(40, 28).Radius(4).
		Children(func() { ui.Icon(c, icons.WindowX).FontSize(14).TextColor(theme.Muted) })
	closeBtn.OnClick(func() { win.Close() })
	if closeBtn.Hovered() {
		closeBtn.Background(theme.Danger)
	}
}
