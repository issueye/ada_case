package app

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/egoist/mygo/ui"

	"lightcode/internal/icons"
	"lightcode/internal/theme"
	"lightcode/internal/workspace"
)

// paletteEntry is one row of quick open or the command palette.
type paletteEntry struct {
	title  string
	detail string
	hint   string
	file   string
	run    func()
}

// paletteEntries filters the palette's source by the query.
func (a *App) paletteEntries() []paletteEntry {
	query := a.PaletteQuery
	switch a.PaletteMode {
	case PaletteCommands:
		var out []paletteEntry
		for _, c := range a.Cmds.All() {
			score, ok := workspace.Match(query, c.Title)
			if !ok {
				continue
			}
			out = append(out, paletteEntry{title: c.Title, detail: c.Category, hint: c.Key, run: func() { a.RunCommand(c.ID) }})
			_ = score
		}
		return out
	default:
		if a.WS == nil {
			return nil
		}
		scored := make([]struct {
			entry paletteEntry
			score int
		}, 0, 64)
		for _, rel := range a.WS.Files() {
			score, ok := workspace.Match(query, rel)
			if !ok {
				continue
			}
			dir := filepath.ToSlash(filepath.Dir(rel))
			if dir == "." {
				dir = ""
			}
			entry := paletteEntry{
				title:  filepath.Base(rel),
				detail: dir,
				file:   rel,
				run:    func() { a.openRel(rel) },
			}
			scored = append(scored, struct {
				entry paletteEntry
				score int
			}{entry, score})
		}
		sort.SliceStable(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
		out := make([]paletteEntry, len(scored))
		for i := range scored {
			out[i] = scored[i].entry
		}
		return out
	}
}

// paletteOverlay shows the palette above the window.
func (a *App) paletteOverlay(c *ui.Context) {
	if !a.PaletteOpen {
		return
	}
	entries := a.paletteEntries()
	sel := a.PaletteSel
	if sel >= len(entries) {
		sel = len(entries) - 1
	}
	if sel < 0 {
		sel = 0
	}
	maxRows := min(len(entries), 8)
	placeholder := "搜索文件…"
	if a.PaletteMode == PaletteCommands {
		placeholder = "输入命令…"
	}
	ui.Overlay(c, func() {
		back := ui.Column(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).
			Background(ui.RGBA(0, 0, 0, 0.32)).Modal().
			AlignItems(ui.Center).Padding(72, 0, 0, 0)
		back.Children(func() {
			panel := ui.Column(c).Width(600).MaxHeight(432).Clip().Radius(8).
				Background(theme.Panel).Border(1, theme.Border).
				Shadow(0, 14, 36, 0, ui.RGBA(0, 0, 0, 0.55))
			panel.Children(func() {
				ui.Row(c).Height(40).PaddingX(12).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, icons.Search).FontSize(14).TextColor(theme.Faint)
					input := ui.TextInput(c, &a.PaletteQuery).Placeholder(placeholder).
						FontSize(13).Grow(1).AutoFocus()
					input.OnTextKey(func(k ui.TextKey) bool {
						switch k.Key {
						case ui.KeyDown:
							if a.PaletteSel+1 < len(entries) {
								a.PaletteSel++
							}
							return true
						case ui.KeyUp:
							if a.PaletteSel > 0 {
								a.PaletteSel--
							}
							return true
						case ui.KeyEnter:
							a.runPalette()
							return true
						case ui.KeyEscape:
							a.closePalette()
							return true
						}
						return false
					})
					mode := "文件"
					if a.PaletteMode == PaletteCommands {
						mode = "命令"
					}
					ui.Text(c, mode).FontSize(10).TextColor(theme.Faint)
				})
				ui.Box(c).Height(1).Background(theme.BorderSoft)
				ui.Column(c).Grow(1).PaddingY(4).Children(func() {
					if len(entries) == 0 {
						ui.Row(c).Height(38).PaddingX(12).AlignItems(ui.Center).Children(func() {
							ui.Text(c, "没有匹配项").FontSize(12).TextColor(theme.Faint)
						})
						return
					}
					for i := 0; i < maxRows; i++ {
						a.paletteRow(c, entries[i], i == sel)
					}
				})
				ui.Box(c).Height(1).Background(theme.BorderSoft)
				ui.Row(c).Height(26).PaddingX(12).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Enter 打开 · ↑↓ 选择 · Esc 关闭").FontSize(10).TextColor(theme.Faint)
					ui.Box(c).Grow(1)
					ui.Text(c, fmt.Sprintf("%d 项", len(entries))).FontSize(10).TextColor(theme.Faint)
				})
			})
			if panel.PressedOutside() {
				a.closePalette()
			}
			if panel.OverlayShortcut(0, ui.KeyEscape) {
				a.closePalette()
			}
		})
	})
}

func (a *App) paletteRow(c *ui.Context, e paletteEntry, active bool) {
	row := ui.Row(c).Height(30).PaddingX(12).Gap(8).AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	row.Children(func() {
		icon := icons.Menu
		color := theme.Faint
		if e.file != "" {
			icon = fileIcon(e.file)
			color = fileColor(e.file)
		}
		ui.Icon(c, icon).FontSize(14).TextColor(color)
		ui.Text(c, e.title).FontSize(12).TextColor(theme.Text).SingleLine()
		if e.detail != "" {
			ui.Text(c, e.detail).FontSize(11).TextColor(theme.Faint).SingleLine().Grow(1)
		} else {
			ui.Box(c).Grow(1)
		}
		if e.hint != "" {
			ui.Text(c, e.hint).FontSize(10).TextColor(theme.Faint).SingleLine()
		}
	})
	if active {
		row.Background(theme.AccentSoft)
	} else if row.Hovered() {
		row.Background(theme.Hover)
	}
	row.OnClick(func() {
		e.run()
		a.closePalette()
	})
}

func (a *App) runPalette() {
	entries := a.paletteEntries()
	sel := a.PaletteSel
	if sel >= len(entries) {
		sel = len(entries) - 1
	}
	if sel < 0 || sel >= len(entries) {
		return
	}
	entries[sel].run()
	a.closePalette()
}

func (a *App) closePalette() {
	a.PaletteOpen = false
	a.PaletteQuery = ""
	a.PaletteSel = 0
}

// closeConfirmOverlay asks whether to save a dirty file before closing
// its tab.
func (a *App) closeConfirmOverlay(c *ui.Context) {
	if !a.ClosePrompt || a.CloseDoc == nil {
		return
	}
	d := a.CloseDoc
	ui.Modal(c, &a.ClosePrompt, func() {
		ui.Column(c).Width(420).Gap(10).Padding(18).Radius(8).
			Background(theme.Panel).Border(1, theme.Border).
			Shadow(0, 14, 36, 0, ui.RGBA(0, 0, 0, 0.55)).
			Children(func() {
				ui.Text(c, "是否保存对 "+d.Name()+" 的更改？").FontSize(14).Bold().TextColor(theme.Text)
				ui.Text(c, "不保存会丢失这次编辑。").FontSize(11).TextColor(theme.Muted)
				ui.Box(c).Height(6)
				ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
					if ui.Button(c, "取消").Clicked() {
						a.CloseDoc = nil
						a.ClosePrompt = false
					}
					if ui.Button(c, "不保存").Clicked() {
						a.ClosePrompt = false
						a.discardClose()
					}
					if ui.Button(c, "保存").Clicked() {
						a.ClosePrompt = false
						a.saveClose()
					}
				})
			})
	})
}
