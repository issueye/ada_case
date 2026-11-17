package app

import (
	"github.com/egoist/mygo/ui"

	"lightcode/internal/icons"
	"lightcode/internal/theme"
)

// activityBar is the 48 DIPs at the left: the views of the sidebar, the
// settings at the bottom.
func (a *App) activityBar(c *ui.Context) {
	ui.Column(c).Width(theme.ActivityW).FillHeight().Background(theme.PanelDeep).
		BorderWidth(0, 1, 0, 0).BorderColor(theme.BorderSoft).
		AlignItems(ui.Center).PaddingY(6).Children(func() {
		items := []struct {
			icon  *ui.SVG
			label string
			view  int
			hint  string
		}{
			{icons.Explorer, "资源管理器", 0, "Ctrl+Shift+E"},
			{icons.Search, "搜索", 1, "Ctrl+Shift+F"},
			{icons.GitBranch, "源代码管理", 2, "Ctrl+Shift+G"},
			{icons.Run, "运行与调试", 3, ""},
			{icons.Extensions, "扩展", 4, "Ctrl+Shift+X"},
		}
		for _, it := range items {
			a.activityItem(c, it.icon, it.label, it.view, it.hint)
		}
		ui.Box(c).Grow(1)
		a.activityItem(c, icons.Settings, "设置", -1, "")
	})
}

func (a *App) activityItem(c *ui.Context, ic *ui.SVG, label string, view int, hint string) {
	selected := view >= 0 && a.SidebarOpen && a.SidebarView == view
	row := ui.Row(c).Width(40).Height(40).Radius(6).AlignItems(ui.Center).
		Justify(ui.Center).Cursor(ui.CursorPointer).Tooltip(label)
	row.Children(func() {
		color := theme.Muted
		if selected {
			color = theme.Accent
		}
		ui.Icon(c, ic).FontSize(19).TextColor(color)
	})
	if selected {
		row.Background(theme.AccentSoft)
	} else if row.Hovered() {
		row.Background(theme.Hover)
	}
	row.OnClick(func() {
		if view < 0 {
			a.OpenPalette(PaletteCommands)
			a.say("设置页将在 M5 提供，先看命令面板")
			return
		}
		a.ShowSidebar(view)
	})
	ui.Box(c).Height(4)
}
