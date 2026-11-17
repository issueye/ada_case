package app

import (
	"github.com/egoist/mygo/ui"

	"lightcode/internal/command"
	"lightcode/internal/theme"
)

// View builds the window.
func (a *App) View(c *ui.Context) {
	c.SetTheme(theme.Window())
	if a.Zoom != 0 && a.Zoom != 1 {
		c.SetZoom(a.Zoom)
	}
	a.syncPreviews()
	a.bindWindowKeys(c)
	ui.Column(c).Fill().Background(theme.Panel).Children(func() {
		a.titleBar(c)
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			a.activityBar(c)
			if a.SidebarOpen {
				a.sidebar(c)
			}
			ui.Column(c).Grow(1).Children(func() {
				if a.TermOpen {
					ui.SplitVerticalPercent(c, &a.TermPercent, func() {
						a.editorArea(c)
					}, func() {
						a.terminalPane(c)
					}).Grow(1)
				} else {
					a.editorArea(c)
				}
			})
			switch a.AI.Mode {
			case AIDock:
				if a.Prefs.AIEnabled {
					a.aiPanel(c)
				}
			}
		})
		a.statusBar(c)
	})
	a.paletteOverlay(c)
	a.closeConfirmOverlay(c)
	a.fileOpOverlay(c)
	a.discardOverlay(c)
	a.aiFloat(c)
	if a.Prefs.AIEnabled && a.AI.Mode == AIRail && a.AI.RailHover {
		a.aiRailPeek(c)
	}
	a.aiSettingsOverlay(c)
}

// bindWindowKeys binds the commands' keys for the whole window: the
// editor takes some of them itself (RunKey in its OnInput), and while a
// dialog is open none run.
func (a *App) bindWindowKeys(c *ui.Context) {
	for _, cmd := range a.Cmds.All() {
		if cmd.Key == "" {
			continue
		}
		ch, ok := command.ParseChord(cmd.Key)
		if !ok {
			continue
		}
		id := cmd.ID
		c.OnShortcut(ch.Mods, ch.Key, func() { a.RunCommand(id) })
	}
}

// editorArea is the middle column: one editor column, or two split by a
// divider the user drags.
func (a *App) editorArea(c *ui.Context) {
	if a.SettingsOpen {
		a.settingsPage(c)
		return
	}
	if len(a.Cols) > 1 {
		ui.SplitPercent(c, &a.SplitPercent, func() {
			a.columnView(c, 0)
		}, func() {
			a.columnView(c, 1)
		}).Grow(1)
		return
	}
	a.columnView(c, 0)
}

// columnView shows one column: its tabs, breadcrumbs, editor or empty
// page, and the assistant's rail when it is collapsed.
func (a *App) columnView(c *ui.Context, idx int) {
	col := a.Cols[idx]
	ui.Column(c).Grow(1).Background(theme.Background).Children(func() {
		a.columnTabs(c, idx)
		a.breadcrumbs(c, col)
		ui.Box(c).Height(1).Background(theme.BorderSoft)
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Grow(1).Background(theme.Background).Children(func() {
				if a.Diff != nil && idx == a.ActiveCol {
					a.diffView(c)
					return
				}
				if e := col.Current(); e != nil {
					if e.Doc.ExternalChanged() {
						a.externalBanner(c, e.Doc)
					}
					e.State.OnInput = a.RunKey
					editorView(c, e)
				} else {
					a.emptyColumn(c, col)
				}
			})
			if idx == a.ActiveCol && a.AI.Mode == AIRail && a.Prefs.AIEnabled {
				a.aiRail(c)
			}
		})
	})
}
