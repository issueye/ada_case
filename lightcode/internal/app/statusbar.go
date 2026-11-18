package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"lightcode/internal/icons"
	"lightcode/internal/theme"
)

// statusBar is the 24 DIPs at the bottom: the branch and the message at
// the left, the editor's counts and settings at the right.
func (a *App) statusBar(c *ui.Context) {
	e := a.Col().Current()
	ui.Row(c).Height(theme.StatusBarH).Background(theme.PanelDeep).
		BorderWidth(1, 0, 0, 0).BorderColor(theme.BorderSoft).
		PaddingX(8).AlignItems(ui.Center).Gap(2).Children(func() {
		if a.Branch != "" {
			seg := statusSegment(c, icons.GitBranch, a.Branch)
			seg.OnClick(func() { a.ShowSidebar(2) })
		} else {
			seg := statusSegment(c, icons.GitBranch, "无 Git 仓库")
			seg.OnClick(func() { a.ShowSidebar(2) })
		}
		if a.Message != "" {
			ui.Box(c).Width(8)
			ui.Text(c, a.Message).FontSize(11).TextColor(theme.Muted).SingleLine()
		}
		ui.Box(c).Grow(1)
		if e != nil {
			pos := e.State.CursorPos()
			statusSegmentText(c, fmt.Sprintf("行 %d，列 %d", pos.Line+1, pos.Col+1), nil)
			statusSegmentText(c, "空格: 4", nil)
			statusSegmentText(c, e.Doc.Encoding(), nil)
			statusSegmentText(c, e.Doc.LineEnding(), nil)
			statusSegmentText(c, e.Doc.Language(), nil)
		} else {
			statusSegmentText(c, "无打开文件", nil)
		}
	})
}

// statusSegment is one part of the status bar with an icon.
func statusSegment(c *ui.Context, ic *ui.SVG, label string) ui.Element {
	seg := ui.Row(c).Height(theme.StatusBarH).PaddingX(6).Gap(4).
		AlignItems(ui.Center).Cursor(ui.CursorPointer).Tooltip(label)
	seg.Children(func() {
		ui.Icon(c, ic).FontSize(12).TextColor(theme.Faint)
		ui.Text(c, label).FontSize(11).TextColor(theme.Muted).SingleLine()
	})
	if seg.Hovered() {
		seg.Background(theme.Hover)
	}
	return seg
}

// statusSegmentText is a part of the status bar with text only.
func statusSegmentText(c *ui.Context, label string, run func()) ui.Element {
	seg := ui.Row(c).Height(theme.StatusBarH).PaddingX(6).AlignItems(ui.Center).
		Cursor(ui.CursorPointer)
	seg.Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(theme.Muted).SingleLine()
	})
	if run != nil {
		seg.OnClick(run)
	}
	if seg.Hovered() {
		seg.Background(theme.Hover)
	}
	return seg
}

// gitBranch reads the branch of a folder's repository, "" when it has
// none. It reads .git/HEAD once, as the branch changes under the app as
// the user checks out in a terminal (M3 watches it properly).
func gitBranch(root string) string {
	data, err := os.ReadFile(filepath.Join(root, ".git", "HEAD"))
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	if ref, ok := strings.CutPrefix(line, "ref: refs/heads/"); ok {
		return ref
	}
	if len(line) >= 7 && strings.Trim(line, "0123456789abcdef") == "" {
		return "detached at " + line[:7]
	}
	return ""
}
