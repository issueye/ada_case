package app

import (
	"github.com/egoist/mygo/ui"

	"lightcode/internal/icons"
	"lightcode/internal/theme"
	"lightcode/internal/workspace"
)

// sidebar is the column of the chosen activity.
func (a *App) sidebar(c *ui.Context) {
	ui.Column(c).Width(theme.SidebarW).FillHeight().Background(theme.Panel).
		BorderWidth(0, 1, 0, 0).BorderColor(theme.BorderSoft).Children(func() {
		a.sidebarHead(c)
		switch a.SidebarView {
		case 0:
			a.explorer(c)
		case 1:
			a.searchPanel(c)
		case 2:
			a.scmPanel(c)
		case 3:
			a.placeholder(c, icons.Run, "运行与调试", "运行配置与调试会话，计划在 M5 之后评估。")
		case 4:
			a.placeholder(c, icons.Extensions, "扩展", "能力面板：Git / AI / 终端 / 预览器的开关与 provider 列表，计划在 M5 提供。")
		}
	})
}

// sidebarHead is the panel head: its title and its actions.
func (a *App) sidebarHead(c *ui.Context) {
	titles := []string{"资源管理器", "搜索", "源代码管理", "运行与调试", "扩展"}
	title := "资源管理器"
	if a.SidebarView >= 0 && a.SidebarView < len(titles) {
		title = titles[a.SidebarView]
	}
	ui.Row(c).Height(theme.PanelHeadH).PaddingX(12).Gap(4).AlignItems(ui.Center).
		Children(func() {
			ui.Text(c, title).FontSize(11).Bold().TextColor(theme.Text).SingleLine().Grow(1)
			if a.SidebarView == 0 && a.WS != nil {
				iconButton(c, icons.NewFile, "新建文件", 22, func() { a.BeginNewFile(a.selectedDir()) })
				iconButton(c, icons.Refresh, "刷新", 22, a.RefreshWorkspace)
				iconButton(c, icons.Collapse, "全部折叠", 22, a.collapseAll)
			}
		})
}

// explorer shows the folder's tree.
func (a *App) explorer(c *ui.Context) {
	if a.WS == nil {
		ui.Column(c).Grow(1).Center().Gap(8).PaddingX(16).Children(func() {
			ui.Icon(c, icons.Explorer).FontSize(24).TextColor(theme.Faint)
			ui.Text(c, "尚未打开文件夹").FontSize(12).TextColor(theme.Muted)
			if ui.Button(c, "打开文件夹").Clicked() {
				a.OpenFolderCommand()
			}
		})
		return
	}
	ui.Column(c).Grow(1).Children(func() {
		ui.Row(c).Height(theme.SectionH).PaddingX(12).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.WS.Name()).FontSize(11).Bold().TextColor(theme.Muted).SingleLine().Grow(1)
			if a.OutlineOpen {
				iconButton(c, icons.Menu, "折叠大纲", 18, func() { a.OutlineOpen = false })
			} else {
				iconButton(c, icons.Menu, "显示大纲", 18, func() { a.OutlineOpen = true })
			}
		})
		if a.OutlineOpen {
			ui.SplitVerticalPercent(c, &a.OutlinePercent, func() {
				a.fileTree(c)
			}, func() {
				a.outlinePanel(c)
			}).Grow(1)
		} else {
			a.fileTree(c)
		}
	})
}

// fileTree is the explorer's scrolling tree.
func (a *App) fileTree(c *ui.Context) {
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Children(func() {
			a.treeLevel(c, a.WS.Top(), 0)
		})
	})
}

// treeLevel builds the rows of a folder, and of the open folders under
// it, in order.
func (a *App) treeLevel(c *ui.Context, entries []*workspace.Entry, depth int) {
	if depth > 12 {
		return
	}
	for _, e := range entries {
		a.treeRow(c, e, depth)
		if e.IsDir && e.Open {
			a.treeLevel(c, e.Children(), depth+1)
		}
	}
}

func (a *App) treeRow(c *ui.Context, e *workspace.Entry, depth int) {
	active := false
	if a.columnHasPath(e.Path) {
		active = true
	}
	row := ui.Row(c).Height(theme.RowH).PaddingX(6).Gap(4).
		AlignItems(ui.Center).Cursor(ui.CursorPointer)
	row.Children(func() {
		ui.Box(c).Width(float32(depth) * 12)
		if e.IsDir {
			chev := icons.ChevronRight
			if e.Open {
				chev = icons.ChevronDown
			}
			ui.Icon(c, chev).FontSize(12).TextColor(theme.Faint)
			folder := icons.Folder
			if e.Open {
				folder = icons.FolderOpen
			}
			ui.Icon(c, folder).FontSize(15).TextColor(theme.Muted)
		} else {
			ui.Box(c).Width(12)
			ui.Icon(c, fileIcon(e.Name)).FontSize(14).TextColor(fileColor(e.Name))
		}
		color := theme.Muted
		if active {
			color = theme.Text
		} else if e.IsDir {
			color = theme.Text
		}
		ui.Text(c, e.Name).FontSize(12).TextColor(color).SingleLine().Grow(1)
		if !e.IsDir {
			if d, ok := a.Docs[e.Path]; ok && d.Dirty() {
				ui.Icon(c, icons.Dot).FontSize(9).TextColor(theme.Accent).Tooltip("未保存")
			}
		}
	})
	if active {
		row.Background(theme.Active)
	} else if row.Hovered() {
		row.Background(theme.Hover)
	}
	row.ContextMenu(func(m *ui.Menu) {
		a.fileOpMenuItems(m, e)
	})
	row.OnClick(func() {
		a.Selected = e.Path
		if e.IsDir {
			e.Open = !e.Open
			return
		}
		if row.DoubleClicked() {
			a.OpenFile(e.Path)
			return
		}
		a.OpenPreview(e.Path)
	})
}

func (a *App) collapseAll() {
	var walk func(es []*workspace.Entry)
	walk = func(es []*workspace.Entry) {
		for _, e := range es {
			if e.IsDir {
				e.Open = false
				if e.Loaded() {
					walk(e.CachedChildren())
				}
			}
		}
	}
	if a.WS != nil {
		walk(a.WS.Top())
	}
	a.say("已折叠")
}

// placeholder is an empty page of a sidebar view whose milestone is later.
func (a *App) placeholder(c *ui.Context, ic *ui.SVG, title, detail string) {
	ui.Column(c).Grow(1).Padding(18, 14).Gap(8).Children(func() {
		ui.Icon(c, ic).FontSize(22).TextColor(theme.Faint)
		ui.Text(c, title).FontSize(13).Bold().TextColor(theme.Text)
		ui.Text(c, detail).FontSize(11).TextColor(theme.Muted).LineHeight(1.6)
	})
}

// iconButton is a flat icon button of the chrome.
func iconButton(c *ui.Context, ic *ui.SVG, label string, size float32, run func()) ui.Element {
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Size(size, size).Radius(4).
		Children(func() { ui.Icon(c, ic).FontSize(size - 6).TextColor(theme.Faint) })
	handled := b
	if run != nil {
		b.OnClick(run)
	}
	if b.Hovered() {
		b.Background(theme.Active)
	}
	return handled
}
