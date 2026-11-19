package app

import (
	"context"
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"lightcode/internal/git"
	"lightcode/internal/icons"
	"lightcode/internal/theme"
)

// attachRepo finds the workspace's repository and reads its status; a
// folder outside Git leaves the SCM view empty.
func (a *App) attachRepo() {
	a.Repo = nil
	a.Git = nil
	a.GitErr = ""
	a.GitLog = nil
	a.Diff = nil
	if a.WS == nil {
		return
	}
	r, err := git.Open(a.WS.Root)
	if err != nil {
		return
	}
	a.Repo = r
	a.refreshGit()
}

// refreshGit reads the status and recent commits, which the SCM panel and
// the status bar show. It never blocks the window: git runs on a goroutine
// and its result comes back on the main thread. One refresh runs at a time;
// the others ask for another round, so a burst of saves spawns one git.
// Tests, and a call before the window is made, read git at once.
func (a *App) refreshGit() {
	if a.Repo == nil {
		return
	}
	if a.Win == nil {
		a.refreshGitNow(a.Repo)
		return
	}
	if a.gitBusy {
		a.gitQueued = true
		return
	}
	a.gitBusy = true
	a.startGitRefresh(a.Repo)
}

// refreshGitNow is refreshGit on the calling goroutine.
func (a *App) refreshGitNow(repo *git.Repo) {
	ctx := context.Background()
	st, err := repo.Status(ctx)
	if err != nil {
		a.GitErr = err.Error()
		return
	}
	a.GitErr = ""
	a.Git = st
	a.Branch = st.Branch
	if logs, err := repo.Log(ctx, 20); err == nil {
		a.GitLog = logs
	}
}

// startGitRefresh reads the status off the main thread, and the history
// only while the SCM panel shows it, then publishes both with win.Update.
func (a *App) startGitRefresh(repo *git.Repo) {
	win := a.Win
	withLog := a.SidebarOpen && a.SidebarView == 2
	go func() {
		ctx := context.Background()
		st, err := repo.Status(ctx)
		var logs []git.LogEntry
		if err == nil && withLog {
			logs, _ = repo.Log(ctx, 20)
		}
		win.Update(func() {
			a.gitBusy = false
			if a.Repo != repo {
				// Another workspace opened meanwhile: its status may be
				// what was queued, so read the current repository.
				if a.gitQueued {
					a.gitQueued = false
					a.refreshGit()
				}
				return
			}
			if err != nil {
				a.GitErr = err.Error()
			} else {
				a.GitErr = ""
				a.Git = st
				a.Branch = st.Branch
				if withLog {
					a.GitLog = logs
				}
			}
			if a.gitQueued {
				a.gitQueued = false
				a.refreshGit()
			}
		})
	}()
}

// scmPanel is the sidebar's source control view: the branch, the three
// groups of changes, the commit box and recent commits.
func (a *App) scmPanel(c *ui.Context) {
	if a.Repo == nil {
		ui.Column(c).Grow(1).Padding(14).Gap(8).Children(func() {
			ui.Icon(c, icons.GitBranch).FontSize(22).TextColor(theme.Faint)
			ui.Text(c, "这个文件夹不是 Git 仓库").FontSize(12).TextColor(theme.Text)
			ui.Text(c, "在终端里 git init 后点刷新。").FontSize(11).TextColor(theme.Muted)
			if ui.Button(c, "刷新").Clicked() {
				a.attachRepo()
			}
		})
		return
	}
	ui.Column(c).Grow(1).Children(func() {
		a.scmBranchRow(c)
		if a.GitErr != "" {
			ui.Text(c, a.GitErr).FontSize(10).TextColor(theme.Danger).Padding(4, 10).SingleLine()
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Children(func() {
				a.scmGroups(c)
				a.scmHistory(c)
			})
		})
		a.scmCommitBox(c)
	})
}

func (a *App) scmBranchRow(c *ui.Context) {
	ui.Row(c).Height(28).PaddingX(8).Gap(5).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icons.GitBranch).FontSize(13).TextColor(theme.Accent)
		name := "无分支"
		if a.Git != nil && a.Git.Branch != "" {
			name = a.Git.Branch
		}
		ui.Text(c, name).FontSize(11).TextColor(theme.Text).SingleLine().Grow(1)
		if a.Git != nil && a.Git.Ahead > 0 {
			ui.Text(c, upArrow()+itoa(a.Git.Ahead)).FontSize(9).TextColor(theme.Faint)
		}
		if a.Git != nil && a.Git.Behind > 0 {
			ui.Text(c, downArrow()+itoa(a.Git.Behind)).FontSize(9).TextColor(theme.Faint)
		}
		iconButton(c, icons.Refresh, "刷新源代码管理", 18, func() { a.refreshGit() })
	})
}

// upArrow and downArrow are the arrows the branch row shows; text, not
// icons, as a count's prefix.
func upArrow() string   { return "↑" }
func downArrow() string { return "↓" }

// scmGroups lists staged, changed, untracked and conflicted files.
func (a *App) scmGroups(c *ui.Context) {
	if a.Git == nil {
		return
	}
	a.scmGroup(c, "暂存的更改", a.Git.Staged, "staged")
	a.scmGroup(c, "更改", a.Git.Changes, "changes")
	a.scmGroup(c, "未跟踪", a.Git.Untracked, "untracked")
	a.scmGroup(c, "冲突", a.Git.Conflicts, "conflicts")
}

func (a *App) scmGroup(c *ui.Context, title string, entries []git.Entry, kind string) {
	if len(entries) == 0 {
		return
	}
	ui.Row(c).Height(24).PaddingX(10).Gap(4).AlignItems(ui.Center).
		Background(theme.PanelDeep).Children(func() {
		ui.Icon(c, icons.ChevronDown).FontSize(10).TextColor(theme.Faint)
		ui.Text(c, title).FontSize(10).Bold().TextColor(theme.Muted).SingleLine().Grow(1)
		ui.Text(c, itoa(len(entries))).FontSize(9).TextColor(theme.Faint)
	})
	for _, e := range entries {
		a.scmEntryRow(c, e, kind)
	}
}

func (a *App) scmEntryRow(c *ui.Context, e git.Entry, kind string) {
	status := string(e.Y)
	if kind == "staged" {
		status = string(e.X)
	}
	if kind == "untracked" {
		status = "U"
	}
	if kind == "conflicts" {
		status = "!"
	}
	row := ui.Row(c).Height(22).Padding(0, 6, 0, 14).Gap(5).AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	row.Children(func() {
		ui.Icon(c, icons.File).FontSize(13).TextColor(fileColor(e.Path))
		ui.Text(c, e.Path).FontSize(11).TextColor(theme.Muted).SingleLine().Grow(1)
		ui.Text(c, status).FontSize(10).Bold().TextColor(statusColor(kind))
	})
	if row.Hovered() {
		row.Background(theme.Hover)
	}
	row.OnClick(func() { a.openDiff(e.Path, kind == "staged") })
	if row.RightClicked() {
		a.rowMenu = rowMenu{open: true, kind: menuSCM, path: e.Path, scmKind: kind}
	}
	if a.rowMenu.open && a.rowMenu.kind == menuSCM && a.rowMenu.path == e.Path && a.rowMenu.scmKind == kind {
		a.menuPopup(c, row, &a.rowMenu.open, a.scmMenuItems(e, kind), func() { a.rowMenu.open = false })
	}
}

func statusColor(kind string) ui.Color {
	switch kind {
	case "staged":
		return theme.Added
	case "conflicts":
		return theme.Danger
	case "untracked":
		return theme.Muted
	default:
		return theme.Warning
	}
}

// scmMenuItems is the SCM row's menu, drawn by the app like the others.
func (a *App) scmMenuItems(e git.Entry, kind string) []menuItem {
	var items []menuItem
	switch kind {
	case "staged":
		items = append(items, menuItem{label: "取消暂存", run: func() { a.unstageFile(e.Path) }})
	case "untracked", "changes":
		items = append(items, menuItem{label: "暂存", run: func() { a.stageFile(e.Path) }})
	}
	return append(items,
		menuItem{sep: true},
		menuItem{label: "丢弃更改", run: func() { a.discardFile(e.Path, kind) }},
		menuItem{label: "在编辑器中打开", run: func() {
			a.OpenFile(filepath.Join(a.WS.Root, filepath.FromSlash(e.Path)))
		}},
	)
}

// scmCommitBox is the message field and the commit button.
func (a *App) scmCommitBox(c *ui.Context) {
	staged := 0
	if a.Git != nil {
		staged = len(a.Git.Staged)
	}
	ui.Box(c).Height(1).Background(theme.BorderSoft)
	ui.Column(c).Padding(8, 10).Gap(6).Children(func() {
		msg := ui.TextInput(c, &a.GitMsg).Placeholder("提交信息（Ctrl+Enter 提交）").FontSize(11)
		submit := msg.Submitted()
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, itoa(staged)+" 个更改已暂存").FontSize(9).TextColor(theme.Faint).Grow(1)
			commit := ui.Button(c, "提交").Disabled(staged == 0 || a.GitMsg == "")
			if commit.Clicked() || submit && staged > 0 && a.GitMsg != "" {
				a.commit()
			}
		})
	})
}

// commit records the index with the message.
func (a *App) commit() {
	if a.Repo == nil || a.GitMsg == "" {
		return
	}
	if err := a.Repo.Commit(context.Background(), a.GitMsg); err != nil {
		a.GitErr = "提交失败：" + err.Error()
		return
	}
	a.GitMsg = ""
	a.say("已提交")
	a.refreshGit()
	a.Diff = nil
}

func (a *App) stageFile(path string) {
	if a.Repo == nil {
		return
	}
	if err := a.Repo.StageFile(context.Background(), path); err != nil {
		a.say("暂存失败：" + err.Error())
		return
	}
	a.refreshGit()
	a.say("已暂存 " + path)
}

func (a *App) unstageFile(path string) {
	if a.Repo == nil {
		return
	}
	if err := a.Repo.UnstageFile(context.Background(), path); err != nil {
		a.say("取消暂存失败：" + err.Error())
		return
	}
	a.refreshGit()
	a.say("已取消暂存 " + path)
}

func (a *App) discardFile(path, kind string) {
	if a.Repo == nil {
		return
	}
	if kind == "untracked" {
		a.say("未跟踪的文件请用资源管理器删除")
		return
	}
	a.DiscardPath = path
	a.DiscardOpen = true
}

// confirmDiscard throws the changes away once the user confirms.
func (a *App) confirmDiscard() {
	path := a.DiscardPath
	a.DiscardPath = ""
	a.DiscardOpen = false
	if a.Repo == nil || path == "" {
		return
	}
	if err := a.Repo.DiscardFile(context.Background(), path); err != nil {
		a.say("丢弃失败：" + err.Error())
		return
	}
	a.refreshGit()
	a.Diff = nil
	a.say("已丢弃 " + path)
}

// discardOverlay asks before the working tree's changes go away.
func (a *App) discardOverlay(c *ui.Context) {
	if !a.DiscardOpen {
		return
	}
	path := a.DiscardPath
	ui.Modal(c, &a.DiscardOpen, func() {
		ui.Column(c).Width(420).Gap(10).Padding(18).Radius(8).
			Background(theme.Panel).Border(1, theme.Border).
			Shadow(0, 14, 36, 0, ui.RGBA(0, 0, 0, 0.55)).Children(func() {
			ui.Text(c, "丢弃 "+path+" 的更改？").FontSize(14).Bold().TextColor(theme.Text)
			ui.Text(c, "工作区会回到上次提交的样子，不能撤销。").FontSize(11).TextColor(theme.Muted)
			ui.Box(c).Height(4)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.DiscardPath = ""
					a.DiscardOpen = false
				}
				if ui.Button(c, "丢弃").Clicked() {
					a.confirmDiscard()
				}
			})
		})
	})
}

// scmHistory lists the recent commits under the changes.
func (a *App) scmHistory(c *ui.Context) {
	if len(a.GitLog) == 0 {
		return
	}
	ui.Row(c).Height(24).PaddingX(10).Gap(4).AlignItems(ui.Center).
		Background(theme.PanelDeep).Children(func() {
		ui.Icon(c, icons.History).FontSize(11).TextColor(theme.Faint)
		ui.Text(c, "提交历史").FontSize(10).Bold().TextColor(theme.Muted).SingleLine().Grow(1)
		ui.Text(c, itoa(len(a.GitLog))).FontSize(9).TextColor(theme.Faint)
	})
	for _, e := range a.GitLog {
		row := ui.Row(c).Height(30).Padding(0, 6, 0, 14).Gap(6).AlignItems(ui.Center)
		row.Children(func() {
			ui.Icon(c, icons.Commit).FontSize(12).TextColor(theme.Faint)
			ui.Column(c).Grow(1).Children(func() {
				ui.Text(c, e.Subject).FontSize(11).TextColor(theme.Muted).SingleLine()
				ui.Text(c, e.Hash+" · "+e.Author+" · "+e.When).FontSize(9).TextColor(theme.Faint).SingleLine()
			})
		})
		if row.Hovered() {
			row.Background(theme.Hover)
		}
	}
}
