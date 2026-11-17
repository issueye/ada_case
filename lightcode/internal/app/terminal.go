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

// toggleTerminal shows or hides the bottom terminal, making it the first
// time the pane opens.
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
	a.ensureTerminal()
	a.SaveState()
}

// ensureTerminal makes the terminal, once.
func (a *App) ensureTerminal() {
	if a.Term != nil || a.TermErr != "" || a.WS == nil {
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
		a.TermErr = err.Error()
		return
	}
	a.Term = t
	a.TermErr = ""
}

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

// CloseTerminal stops the shell when the window closes.
func (a *App) CloseTerminal() {
	if a.Term != nil {
		a.Term.Close()
		a.Term = nil
	}
}

// terminalPane is the bottom terminal: its head, the terminal, or why it
// could not start.
func (a *App) terminalPane(c *ui.Context) {
	ui.Column(c).FillHeight().Background(theme.Background).
		BorderWidth(1, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Row(c).Height(26).PaddingX(8).Gap(6).AlignItems(ui.Center).
			Background(theme.PanelDeep).Children(func() {
			ui.Icon(c, icons.Terminal).FontSize(13).TextColor(theme.Muted)
			ui.Text(c, "终端").FontSize(10).Bold().TextColor(theme.Muted)
			if a.WS != nil {
				ui.Text(c, a.WS.Name()).FontSize(9).TextColor(theme.Faint).SingleLine()
			}
			ui.Box(c).Grow(1)
			if a.TermErr != "" {
				if ui.Button(c, "重试").Clicked() {
					a.TermErr = ""
					a.ensureTerminal()
				}
			}
			iconButton(c, icons.Close, "关闭终端", 18, func() {
				a.TermOpen = false
				a.SaveState()
			})
		})
		if a.TermErr != "" {
			ui.Column(c).Grow(1).Padding(14).Gap(6).Children(func() {
				ui.Text(c, "终端无法启动").FontSize(12).Bold().TextColor(theme.Text)
				ui.Text(c, a.TermErr).FontSize(10).TextColor(theme.Danger).LineHeight(1.5)
				ui.Text(c, "libghostty-vt 由 mygo build / mygo dev 随应用打包；用 go run 时首次会尝试下载到缓存。").
					FontSize(10).TextColor(theme.Muted).LineHeight(1.5)
			})
			return
		}
		if a.Term == nil {
			ui.Column(c).Grow(1).Center().Children(func() {
				ui.Text(c, "正在启动终端…").FontSize(11).TextColor(theme.Faint)
			})
			return
		}
		terminal.View(c, a.Term).Grow(1).FillWidth()
	})
}
