package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"lightcode/internal/icons"
	"lightcode/internal/theme"
)

// console is one terminal of the bottom pane: its shell, the reason it
// could not start, and the number its tab shows.
type console struct {
	term   *terminal.Terminal
	err    string
	number int
}

// title is what the console's tab shows.
func (c *console) title() string { return "终端 " + strconv.Itoa(c.number) }

// toggleTerminal shows or hides the bottom console pane, starting a
// console the first time it opens.
func (a *App) toggleTerminal() {
	if !a.Prefs.TerminalEnabled {
		a.say("终端已在扩展页里关闭")
		return
	}
	if a.TermOpen {
		a.TermOpen = false
		a.SaveState()
		return
	}
	a.TermOpen = true
	a.ensureConsole()
	a.SaveState()
}

// newTerminal opens the pane with another console, the one shown.
func (a *App) newTerminal() {
	if !a.Prefs.TerminalEnabled {
		a.say("终端已在扩展页里关闭")
		return
	}
	a.TermOpen = true
	a.addConsole()
	a.SaveState()
}

// ensureConsole starts the pane's first console, or brings the open
// pane's console back into view.
func (a *App) ensureConsole() {
	if len(a.Terms) == 0 {
		a.addConsole()
		return
	}
	a.selectConsole(a.TermActive)
}

// addConsole adds a console after the others and shows it.
func (a *App) addConsole() *console {
	a.termSeq++
	c := &console{number: a.termSeq}
	a.Terms = append(a.Terms, c)
	a.TermActive = len(a.Terms) - 1
	a.startConsole(c)
	a.termFocus = true
	return c
}

// selectConsole shows the console at i.
func (a *App) selectConsole(i int) {
	if i < 0 || i >= len(a.Terms) {
		return
	}
	a.TermActive = i
	a.termFocus = true
}

// activeConsole is the console the pane shows, nil without one.
func (a *App) activeConsole() *console {
	if a.TermActive < 0 || a.TermActive >= len(a.Terms) {
		return nil
	}
	return a.Terms[a.TermActive]
}

// startConsole starts c's shell in the workspace, or keeps the reason it
// could not start.
func (a *App) startConsole(c *console) {
	if c.term != nil {
		c.term.Close()
		c.term = nil
	}
	c.err = ""
	if a.WS == nil {
		c.err = "还没有打开文件夹"
		return
	}
	dir := a.WS.Root
	t, err := terminal.New(terminal.Options{
		Dir:  dir,
		Font: terminal.Font{Family: theme.MonoFont, Size: a.Prefs.TerminalFontSize},
		OpenLink: func(link string) bool {
			return a.openTerminalLink(link, dir)
		},
	})
	if err != nil {
		c.err = err.Error()
		return
	}
	c.term = t
}

// closeConsole closes the console at i and shows its neighbour; closing
// the pane's last console closes the pane.
func (a *App) closeConsole(i int) {
	if i < 0 || i >= len(a.Terms) {
		return
	}
	if t := a.Terms[i].term; t != nil {
		t.Close()
	}
	a.Terms = append(a.Terms[:i], a.Terms[i+1:]...)
	if len(a.Terms) == 0 {
		a.TermOpen = false
		a.TermActive = 0
		a.SaveState()
		return
	}
	if a.TermActive >= i {
		a.TermActive = max(0, a.TermActive-1)
	}
	if a.TermActive >= len(a.Terms) {
		a.TermActive = len(a.Terms) - 1
	}
	a.termFocus = true
	a.SaveState()
}

// closeActiveConsole closes the console the pane shows.
func (a *App) closeActiveConsole() { a.closeConsole(a.TermActive) }

var linkPos = regexp.MustCompile(`^(.*?)(?::(\d+))?(?::(\d+))?$`)

// openTerminalLink opens a path:line:col the terminal printed, if it is
// in the workspace.
func (a *App) openTerminalLink(link, dir string) bool {
	m := linkPos.FindStringSubmatch(strings.TrimSpace(link))
	if m == nil {
		return false
	}
	path := m[1]
	if path == "" {
		return false
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, filepath.FromSlash(path))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	if a.WS != nil {
		rel, err := filepath.Rel(a.WS.Root, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			return false
		}
	}
	if info, err := os.Stat(abs); err != nil || info.IsDir() {
		return false
	}
	a.OpenFile(abs)
	line, col := 0, 0
	if m[2] != "" {
		line, _ = strconv.Atoi(m[2])
	}
	if m[3] != "" {
		col, _ = strconv.Atoi(m[3])
	}
	if e := a.Col().Current(); e != nil && line > 0 {
		l := line - 1
		if l >= e.Doc.Buffer.LineCount() {
			l = e.Doc.Buffer.LineCount() - 1
		}
		e.State.GoTo(editor.Pos{Line: l, Col: max(0, col-1)})
		e.State.ScrollTo(l)
	}
	return true
}

// CloseTerminal stops every shell when the window closes.
func (a *App) CloseTerminal() {
	for _, c := range a.Terms {
		if c.term != nil {
			c.term.Close()
			c.term = nil
		}
	}
	a.Terms = nil
}

// terminalPane is the bottom terminal: a tab per console, the console the
// tabs chose, or why it could not start.
func (a *App) terminalPane(c *ui.Context) {
	ui.Column(c).FillHeight().Background(theme.Background).
		BorderWidth(1, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		a.consoleTabs(c)
		a.consoleBody(c)
	})
}

// consoleTabs is the pane's head: one tab per console, the button adding
// another, and the one closing the pane.
func (a *App) consoleTabs(c *ui.Context) {
	ui.Row(c).Height(30).PaddingX(6).Gap(2).AlignItems(ui.Center).
		Background(theme.PanelDeep).Children(func() {
		ui.ScrollHorizontal(c).Grow(1).Children(func() {
			ui.Row(c).Height(30).Gap(2).Children(func() {
				for i, con := range a.Terms {
					a.consoleTab(c, i, con)
				}
			})
		})
		iconButton(c, icons.Plus, "新建终端", 20, a.newTerminal)
		iconButton(c, icons.Close, "关闭终端面板", 20, func() {
			a.TermOpen = false
			a.SaveState()
		})
	})
}

// consoleTab is one console's tab: the label choosing the console, and the
// button beside it closing it. The two are siblings, so closing does not
// also choose the tab.
func (a *App) consoleTab(c *ui.Context, i int, con *console) {
	active := i == a.TermActive
	tab := ui.Row(c).Height(30).PaddingX(8).Gap(4).AlignItems(ui.Center).MaxWidth(180)
	tab.Children(func() {
		color := theme.Muted
		if active {
			color = theme.Text
		}
		pick := ui.Row(c).Height(30).Gap(6).AlignItems(ui.Center).Grow(1).
			Cursor(ui.CursorPointer).Tooltip(con.title())
		pick.Children(func() {
			ui.Icon(c, icons.Terminal).FontSize(12).TextColor(theme.Faint)
			ui.Text(c, con.title()).FontSize(10).TextColor(color).SingleLine()
		})
		pick.OnClick(func() { a.selectConsole(i) })
		closeTab := ui.ButtonBase(c).Size(16, 16).Radius(3).
			Label("关闭" + con.title()).Tooltip("关闭" + con.title()).
			Children(func() { ui.Icon(c, icons.Close).FontSize(10).TextColor(theme.Faint) })
		closeTab.OnClick(func() { a.closeConsole(i) })
		if closeTab.Hovered() {
			closeTab.Background(theme.Active)
		}
	})
	if active {
		tab.Background(theme.Background).BorderWidth(2, 0, 0, 0).BorderColor(theme.Accent)
	} else if tab.Hovered() {
		tab.Background(theme.Hover)
	}
}

// consoleBody shows the chosen console, its starting state, or why it
// could not start.
func (a *App) consoleBody(c *ui.Context) {
	con := a.activeConsole()
	if con == nil {
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, "还没有终端，用上方的 + 新建一个。").FontSize(11).TextColor(theme.Faint)
		})
		return
	}
	if con.err != "" {
		ui.Column(c).Grow(1).Padding(14).Gap(6).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "终端无法启动").FontSize(12).Bold().TextColor(theme.Text)
				if ui.Button(c, "重试").Clicked() {
					a.startConsole(con)
					a.termFocus = true
				}
			})
			ui.Text(c, con.err).FontSize(10).TextColor(theme.Danger).LineHeight(1.5)
			ui.Text(c, "libghostty-vt 由 mygo build / mygo dev 随应用打包；用 go run 时首次会尝试下载到缓存。").
				FontSize(10).TextColor(theme.Muted).LineHeight(1.5)
		})
		return
	}
	if con.term == nil {
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, "正在启动终端…").FontSize(11).TextColor(theme.Faint)
		})
		return
	}
	view := terminal.View(c, con.term).Grow(1).FillWidth()
	if a.termFocus {
		// A console the user just opened or chose takes the keyboard.
		view.AutoFocus()
		a.termFocus = false
	}
}
