package app

import (
	"context"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"lightcode/internal/icons"
	"lightcode/internal/theme"
)

// diffTab is the diff page of one file: the index's text against the
// working tree, or the index against HEAD when staged, and the hunks git
// found.
type diffTab struct {
	Path   string
	Staged bool
	Old    *editor.Buffer
	New    *editor.Buffer
	State  *editor.DiffState
	Hunks  []editor.PatchHunk
	// Err is why the page could not be built, as git's message.
	Err string
}

// openDiff builds the diff page of a file.
func (a *App) openDiff(path string, staged bool) {
	if a.Repo == nil {
		if a.WS != nil {
			a.OpenFile(filepath.Join(a.WS.Root, filepath.FromSlash(path)))
		}
		return
	}
	ctx := context.Background()
	var oldText, newText string
	if staged {
		oldText, _ = a.Repo.HeadText(ctx, path)
		newText, _ = a.Repo.IndexText(ctx, path)
	} else {
		oldText, _ = a.Repo.IndexText(ctx, path)
		if text, err := a.Repo.WorktreeText(path); err == nil {
			newText = text
		}
	}
	fp, _, err := a.Repo.HunkDiff(ctx, path, staged)
	if err != nil {
		a.say("无法读取差异：" + err.Error())
		return
	}
	lang := editor.LanguageFor(filepath.Base(path))
	oldBuf := editor.NewBuffer(oldText)
	oldBuf.SetLanguage(lang)
	newBuf := editor.NewBuffer(newText)
	newBuf.SetLanguage(lang)
	state := &editor.DiffState{
		Old:          oldBuf,
		New:          newBuf,
		Layout:       editor.DiffSplit,
		Font:         theme.EditorFont(),
		Theme:        theme.Code(theme.Window()),
		ContextLines: 3,
		Inline:       editor.DiffInlineWordAlt,
		SplitPercent: 45,
		PopoverMenu:  true,
	}
	a.Diff = &diffTab{Path: path, Staged: staged, Old: oldBuf, New: newBuf, State: state, Hunks: fp.Hunks}
}

// diffView draws the diff page in place of the active column's editor.
func (a *App) diffView(c *ui.Context) {
	d := a.Diff
	ui.Column(c).Grow(1).Background(theme.Background).Children(func() {
		ui.Row(c).Height(36).PaddingX(8).Gap(8).AlignItems(ui.Center).
			Background(theme.Panel).BorderWidth(0, 0, 1, 0).BorderColor(theme.BorderSoft).
			Children(func() {
				iconButton(c, icons.ArrowLeft, "关闭差异", 22, func() { a.Diff = nil })
				ui.Icon(c, fileIcon(d.Path)).FontSize(14).TextColor(fileColor(d.Path))
				ui.Text(c, d.Path).FontSize(12).TextColor(theme.Text).SingleLine()
				label := "未暂存"
				if d.Staged {
					label = "已暂存"
				}
				ui.Text(c, label).FontSize(9).Padding(1, 5).Radius(3).
					Background(theme.AccentSoft).TextColor(theme.Accent)
				ui.Text(c, itoa(len(d.Hunks))+" 个块").FontSize(9).TextColor(theme.Faint)
				ui.Box(c).Grow(1)
				if !d.Staged {
					if ui.Button(c, "暂存全部").Clicked() {
						a.stageFile(d.Path)
						a.openDiff(d.Path, false)
					}
				} else {
					if ui.Button(c, "取消暂存").Clicked() {
						a.unstageFile(d.Path)
						a.Diff = nil
					}
				}
				iconButton(c, icons.Refresh, "刷新差异", 22, func() { a.openDiff(d.Path, d.Staged) })
			})
		if d.Err != "" {
			ui.Text(c, d.Err).FontSize(11).TextColor(theme.Danger).Padding(8, 12)
		}
		if !d.Staged && len(d.Hunks) > 0 {
			a.diffHunkBar(c, d)
		}
		// As the code area, the diff's sides build in the app menus'
		// metrics, so their context menus read like the app's.
		ui.Themed(c, theme.Menu()).Grow(1).FillWidth().Children(func() {
			editor.DiffView(c, d.State).Grow(1).FillWidth()
		})
	})
}

// diffHunkBar is the row of hunks above the diff: each can be staged on
// its own, as the index gets a blob of the lines.
func (a *App) diffHunkBar(c *ui.Context, d *diffTab) {
	ui.Row(c).Height(38).Background(theme.PanelDeep).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.BorderSoft).Children(func() {
		ui.ScrollHorizontal(c).Grow(1).Children(func() {
			ui.Row(c).Height(38).PaddingX(8).Gap(6).AlignItems(ui.Center).Children(func() {
				for i, h := range d.Hunks {
					a.diffHunkCard(c, d, i, h)
				}
			})
		})
	})
}

func (a *App) diffHunkCard(c *ui.Context, d *diffTab, i int, h editor.PatchHunk) {
	card := ui.Row(c).Height(26).Padding(0, 8).Gap(6).Radius(4).
		Background(theme.Panel).Border(1, theme.BorderSoft).AlignItems(ui.Center)
	card.Children(func() {
		ui.Text(c, "块 "+itoa(i+1)).FontSize(9).Bold().TextColor(theme.Muted)
		ui.Text(c, "旧 "+hunkRange(h.OldStart, h.OldLines)+" → 新 "+hunkRange(h.NewStart, h.NewLines)).
			FontSize(9).TextColor(theme.Faint)
		stage := ui.Button(c, "").Height(20).Padding(0, 6).Radius(3).Label("暂存块 " + itoa(i+1)).
			Children(func() { ui.Text(c, "暂存").FontSize(9).TextColor(theme.Accent) })
		if stage.Clicked() {
			a.stageHunk(d.Path, i)
			a.openDiff(d.Path, false)
		}
	})
	if card.Hovered() {
		card.Border(1, theme.Border)
	}
}

func hunkRange(start, lines int) string {
	if lines == 1 {
		return itoa(start)
	}
	return itoa(start) + "–" + itoa(start+lines-1)
}

// stageHunk stages one hunk of a file's working-tree diff.
func (a *App) stageHunk(path string, i int) {
	if a.Repo == nil {
		return
	}
	if err := a.Repo.StageHunk(context.Background(), path, i); err != nil {
		a.say("暂存块失败：" + err.Error())
		return
	}
	a.refreshGit()
	a.say("已暂存 " + path + " 的块 " + itoa(i+1))
}
