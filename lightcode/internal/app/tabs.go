package app

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"lightcode/internal/document"
	"lightcode/internal/icons"
	"lightcode/internal/theme"
)

// columnTabs is the row of open files above one editor column.
func (a *App) columnTabs(c *ui.Context, idx int) {
	col := a.Cols[idx]
	ui.Row(c).Height(theme.TabStripH).Background(theme.Panel).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.BorderSoft).Children(func() {
		if len(col.Editors) == 0 {
			ui.Row(c).Grow(1)
		} else {
			ui.ScrollHorizontal(c).Grow(1).Children(func() {
				ui.Row(c).Height(theme.TabStripH).Children(func() {
					for i, e := range col.Editors {
						a.tab(c, idx, i, e, i == col.Active)
					}
				})
			})
		}
		ui.Box(c).Width(1).Height(theme.TabStripH).Background(theme.BorderSoft)
		ui.Row(c).PaddingX(6).Gap(2).AlignItems(ui.Center).Children(func() {
			split := ui.ButtonBase(c).Size(26, 24).Radius(4).Label("拆分编辑器").Tooltip(a.splitHint()).
				Children(func() { ui.Icon(c, icons.Split).FontSize(14).TextColor(theme.Faint) })
			split.OnClick(a.ToggleSplit)
			more := ui.ButtonBase(c).Size(26, 24).Radius(4).Label("更多标签操作").Tooltip("更多").
				Children(func() { ui.Icon(c, icons.Ellipsis).FontSize(14).TextColor(theme.Faint) })
			more.OnClick(func() { a.OpenPalette(PaletteCommands) })
		})
	})
}

func (a *App) splitHint() string {
	if len(a.Cols) > 1 {
		return "关闭分屏（Ctrl+\\）"
	}
	return "拆分编辑器（Ctrl+\\）"
}

func (a *App) tab(c *ui.Context, idx, i int, e *document.Editor, chosen bool) {
	activeCol := idx == a.ActiveCol
	tab := ui.Row(c).Height(theme.TabStripH).PaddingX(10).Gap(6).
		AlignItems(ui.Center).Cursor(ui.CursorPointer).MaxWidth(220)
	tab.Children(func() {
		ui.Icon(c, fileIcon(e.Doc.Name())).FontSize(14).TextColor(fileColor(e.Doc.Name()))
		nameColor := theme.Muted
		if chosen && activeCol {
			nameColor = theme.Text
		} else if chosen {
			nameColor = theme.Muted
		}
		name := ui.Text(c, e.Doc.Name()).FontSize(12).TextColor(nameColor).SingleLine().Grow(1)
		if e.Preview {
			name.Italic()
		}
		if e.Doc.Dirty() {
			ui.Icon(c, icons.Dot).FontSize(9).TextColor(theme.Accent).Tooltip("未保存")
		} else {
			closeTab := ui.ButtonBase(c).Size(18, 18).Radius(3).Label("关闭 " + e.Doc.Name()).Tooltip("关闭").
				Children(func() { ui.Icon(c, icons.Close).FontSize(11).TextColor(theme.Faint) })
			closeTab.OnClick(func() { a.closeTabIn(idx, e.Doc) })
			if closeTab.Hovered() {
				closeTab.Background(theme.Active)
			}
		}
	})
	if chosen && activeCol {
		tab.Background(theme.Background).BorderWidth(2, 0, 0, 0).BorderColor(theme.Accent)
	} else if tab.Hovered() {
		tab.Background(theme.Hover)
	}
	tab.OnClick(func() {
		a.ActiveCol = idx
		a.Cols[idx].Active = i
	})
	if tab.RightClicked() {
		a.tabMenu = tabMenuState{open: true, idx: idx, doc: e.Doc}
	}
	if a.tabMenu.open && a.tabMenu.idx == idx && a.tabMenu.doc == e.Doc {
		ui.Popover(c, tab, &a.tabMenu.open, func() {
			a.tabMenuBody(c, idx, e.Doc)
		})
	}
}

// closeTabIn closes d in column idx, asking to save it first when dirty,
// and collapses the split when its last tab goes.
func (a *App) closeTabIn(idx int, d *document.Document) {
	if d.Dirty() {
		a.CloseDoc = d
		a.ClosePrompt = true
		return
	}
	col := a.Cols[idx]
	for i, e := range col.Editors {
		if e.Doc == d {
			col.Remove(i)
			break
		}
	}
	a.collapseEmptyColumns()
	a.pruneDocs()
	a.updateTitle()
	a.SaveState()
}

// collapseEmptyColumns drops a split column that has no tab left, and
// keeps the first one.
func (a *App) collapseEmptyColumns() {
	for len(a.Cols) > 1 {
		idx := -1
		for i, col := range a.Cols {
			if len(col.Editors) == 0 && i > 0 {
				idx = i
				break
			}
		}
		if idx < 0 {
			return
		}
		a.Cols = append(a.Cols[:idx], a.Cols[idx+1:]...)
		if a.ActiveCol >= len(a.Cols) {
			a.ActiveCol = len(a.Cols) - 1
		}
	}
}

// breadcrumbs shows the path of the active file.
func (a *App) breadcrumbs(c *ui.Context, col *document.Column) {
	doc := a.docOf(col)
	ui.Row(c).Height(theme.CrumbsH).Background(theme.Background).PaddingX(14).Gap(4).
		AlignItems(ui.Center).Children(func() {
		if doc == nil {
			return
		}
		parts := a.breadcrumbsFor(doc)
		for i, part := range parts {
			if i > 0 {
				ui.Icon(c, icons.ChevronRight).FontSize(10).TextColor(theme.Faint)
			}
			color := theme.Faint
			if i == len(parts)-1 {
				color = theme.Text
			}
			ui.Text(c, part).FontSize(11).TextColor(color).SingleLine()
		}
		if e := col.Current(); e != nil && isMarkdown(doc.Path) {
			// Markdown files switch between source and rendered document.
			ui.Box(c).Grow(1)
			label := "预览"
			if e.MarkdownView {
				label = "源码"
			}
			btn := ui.Row(c).Height(20).Padding(0, 8).Radius(3).AlignItems(ui.Center).
				Cursor(ui.CursorPointer).Tooltip("Markdown 文档视图（Ctrl+Shift+V）")
			btn.Children(func() {
				ui.Text(c, label).FontSize(10).TextColor(theme.Muted)
			})
			if btn.Hovered() {
				btn.Background(theme.Hover)
			}
			btn.OnClick(func() { a.toggleMarkdownView(e) })
		}
	})
}

// docOf is the document of a column's chosen editor, nil without one.
func (a *App) docOf(col *document.Column) *document.Document {
	if e := col.Current(); e != nil {
		return e.Doc
	}
	return nil
}

// breadcrumbsFor splits a document's path under the workspace root.
func (a *App) breadcrumbsFor(d *document.Document) []string {
	if a.WS != nil {
		if rel, err := relPath(a.WS.Root, d.Path); err == nil && rel != "" && rel != "." && !strings.HasPrefix(rel, "..") {
			return strings.Split(rel, "/")
		}
	}
	if dir := filepath.Dir(d.Path); dir != "" {
		return []string{filepath.Base(dir), d.Name()}
	}
	return []string{d.Name()}
}

// editorView shows the active editor.
func editorView(c *ui.Context, e *document.Editor) {
	editor.View(c, e.State).Grow(1).FillWidth()
}

// emptyColumn is the page shown with no open file in a column.
func (a *App) emptyColumn(c *ui.Context, col *document.Column) {
	ui.Column(c).Grow(1).Center().Gap(6).PaddingX(10).Children(func() {
		ui.Text(c, "轻码 IDE").FontSize(18).Bold().TextColor(theme.Text)
		ui.Text(c, "Go 写成的轻量编辑器").FontSize(11).TextColor(theme.Muted)
		ui.Box(c).Height(12)
		mod := "Ctrl"
		if isMac() {
			mod = "Cmd"
		}
		hint := func(keys, what string) {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, keys).FontSize(11).TextColor(theme.Text).Width(150).SingleLine()
				ui.Text(c, what).FontSize(11).TextColor(theme.Faint).SingleLine()
			})
		}
		hint(mod+"+P", "快速打开文件")
		hint(mod+"+Shift+P", "命令面板")
		hint(mod+"+\\", "拆分编辑器")
		hint(mod+"+B", "切换侧栏")
		ui.Box(c).Height(12)
		if ui.Button(c, "快速打开").Clicked() {
			a.OpenPalette(PaletteFiles)
		}
		if len(a.Cols) == 1 {
			if ui.Button(c, "拆分编辑器").Clicked() {
				a.ToggleSplit()
			}
		}
	})
}

// tabMenuState is the tab menu that a right click opened: which tab, the
// document, and whether it still shows.
type tabMenuState struct {
	open bool
	idx  int
	doc  *document.Document
}

// tabMenuBody draws the tab's menu in the app's own style, in a popover
// under the tab, rather than the system's white menu.
func (a *App) tabMenuBody(c *ui.Context, idx int, d *document.Document) {
	a.menuBody(c, []menuItem{
		{label: "关闭", hint: "Ctrl+W", run: func() { a.closeTabIn(idx, d) }},
		{label: "关闭其他", run: func() { a.closeOthersIn(idx, d) }},
		{label: "关闭已保存", run: func() { a.closeSavedIn(idx) }},
		{sep: true},
		{label: "复制路径", run: func() {
			mygo.Clipboard.WriteText(d.Path)
			a.say("已复制路径 " + d.Path)
		}},
		{label: "在文件管理器中显示", run: func() { mygo.Shell.ShowItemInFolder(d.Path) }},
	}, func() { a.tabMenu.open = false })
}

// closeOthersIn closes the clean tabs of a column except d.
func (a *App) closeOthersIn(idx int, d *document.Document) {
	col := a.Cols[idx]
	skipped := 0
	for i := len(col.Editors) - 1; i >= 0; i-- {
		e := col.Editors[i]
		if e.Doc == d {
			continue
		}
		if e.Doc.Dirty() {
			skipped++
			continue
		}
		col.Remove(i)
	}
	a.collapseEmptyColumns()
	a.pruneDocs()
	a.updateTitle()
	a.SaveState()
	if skipped > 0 {
		a.say("已关闭其他标签；有未保存修改的标签保留")
	} else {
		a.say("已关闭其他标签")
	}
}

// closeSavedIn closes the column's tabs that have no unsaved change.
func (a *App) closeSavedIn(idx int) {
	col := a.Cols[idx]
	closed := 0
	for i := len(col.Editors) - 1; i >= 0; i-- {
		if col.Editors[i].Doc.Dirty() {
			continue
		}
		col.Remove(i)
		closed++
	}
	a.collapseEmptyColumns()
	a.pruneDocs()
	a.updateTitle()
	a.SaveState()
	a.say("已关闭 " + itoa(closed) + " 个已保存标签")
}
