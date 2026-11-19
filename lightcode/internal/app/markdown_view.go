package app

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"lightcode/internal/document"
	"lightcode/internal/theme"
)

// markdownExts are the files the app can show as a rendered Markdown
// document.
var markdownExts = map[string]bool{
	".md":       true,
	".markdown": true,
	".mdown":    true,
	".mkd":      true,
	".mdx":      true,
}

// isMarkdown reports whether path is a Markdown document.
func isMarkdown(path string) bool {
	return markdownExts[strings.ToLower(filepath.Ext(path))]
}

// ToggleMarkdownPreview switches the active Markdown tab between its
// source and the rendered document.
func (a *App) ToggleMarkdownPreview() {
	e := a.Col().Current()
	if e == nil {
		a.say("没有打开的文件")
		return
	}
	if !isMarkdown(e.Doc.Path) {
		a.say(e.Doc.Name() + " 不是 Markdown 文档")
		return
	}
	a.toggleMarkdownView(e)
}

// toggleMarkdownView flips a tab's Markdown view mode, whichever column
// shows it.
func (a *App) toggleMarkdownView(e *document.Editor) {
	e.MarkdownView = !e.MarkdownView
	if e.MarkdownView {
		a.say(e.Doc.Name() + " 以文档视图显示")
	} else {
		a.say(e.Doc.Name() + " 以源码显示")
	}
}

// markdownView shows a Markdown document as it reads: the rendered
// document, scrolling, selectable.
func (a *App) markdownView(c *ui.Context, e *document.Editor) {
	ui.Column(c).Grow(1).Background(theme.Background).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(20, 28).Gap(4).Children(func() {
				markdown.View(c, e.Doc.Buffer.String(), nil)
			})
		})
	})
}
