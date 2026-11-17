package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"lightcode/internal/document"
	"lightcode/internal/icons"
	"lightcode/internal/symbols"
	"lightcode/internal/theme"
)

// outlineOf returns the symbols of an editor's file, parsed again when
// the buffer changed.
func (a *App) outlineOf(e *document.Editor) []symbols.Symbol {
	if a.outline.path == e.Doc.Path && a.outline.version == e.Doc.Buffer.Version() {
		return a.outline.syms
	}
	syms := symbols.File(e.Doc.Path, e.Doc.Buffer.String())
	a.outline.path = e.Doc.Path
	a.outline.version = e.Doc.Buffer.Version()
	a.outline.syms = syms
	return syms
}

// outlinePanel is the outline under the explorer's tree: the symbols of
// the active file, the one at the caret marked.
func (a *App) outlinePanel(c *ui.Context) {
	e := a.Col().Current()
	ui.Column(c).Grow(1).Background(theme.Panel).Children(func() {
		ui.Row(c).Height(theme.SectionH).PaddingX(8).Gap(4).AlignItems(ui.Center).
			Children(func() {
				collapse := ui.ButtonBase(c).Size(18, 18).Radius(3).Label("折叠大纲").Tooltip("折叠大纲").
					Children(func() { ui.Icon(c, icons.ChevronDown).FontSize(11).TextColor(theme.Faint) })
				collapse.OnClick(func() { a.OutlineOpen = false })
				ui.Text(c, "大纲").FontSize(11).Bold().TextColor(theme.Muted)
				ui.Box(c).Grow(1)
				if e != nil {
					ui.Text(c, e.Doc.Language()).FontSize(10).TextColor(theme.Faint)
				}
			})
		if e == nil {
			ui.Text(c, "打开文件后显示大纲").FontSize(11).TextColor(theme.Faint).Padding(8, 10)
			return
		}
		all := a.outlineOf(e)
		if len(all) == 0 {
			ui.Text(c, "这个文件没有可列出的声明").FontSize(11).TextColor(theme.Faint).Padding(8, 10)
			return
		}
		cursor := e.State.CursorPos().Line
		current := -1
		for i, s := range all {
			if s.Line-1 <= cursor {
				current = i
			}
		}
		// Big generated files have thousands of declarations: list the
		// first ones and say how many are left, rather than build every
		// row on every frame.
		const maxRows = 240
		syms := all
		if len(syms) > maxRows {
			syms = syms[:maxRows]
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Children(func() {
				for i, s := range syms {
					a.outlineRow(c, s, i == current)
				}
				if len(all) > maxRows {
					ui.Text(c, "还有 "+itoa(len(all)-maxRows)+" 个符号未列出").
						FontSize(10).TextColor(theme.Faint).Padding(4, 10)
				}
			})
		})
	})
	if a.OutlineOpen {
		ui.Row(c).Height(1).Background(theme.BorderSoft)
	}
}

func (a *App) outlineRow(c *ui.Context, s symbols.Symbol, active bool) {
	row := ui.Row(c).Height(22).PaddingX(8).Gap(5).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Tooltip(s.Kind + " " + s.Name)
	row.Children(func() {
		ui.Box(c).Width(float32(s.Depth) * 10)
		ui.Icon(c, icons.File).FontSize(12).TextColor(theme.Faint)
		ui.Text(c, s.Name).FontSize(11).TextColor(theme.Muted).SingleLine().Grow(1)
		ui.Text(c, itoa(s.Line)).FontSize(9).TextColor(theme.Faint)
	})
	if active {
		row.Background(theme.AccentSoft)
	} else if row.Hovered() {
		row.Background(theme.Hover)
	}
	row.OnClick(func() {
		if e := a.Col().Current(); e != nil {
			line := s.Line - 1
			if line < 0 {
				line = 0
			}
			e.State.GoTo(editorPos(line, s.Col-1))
			e.State.ScrollTo(line)
		}
	})
}

// cToEditorPos turns a symbol's position into an editor position.
func editorPos(line, col int) editor.Pos {
	if line < 0 {
		line = 0
	}
	if col < 0 {
		col = 0
	}
	return editor.Pos{Line: line, Col: col}
}
