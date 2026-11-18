package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"lightcode/internal/document"
	"lightcode/internal/theme"
)

// viewerView shows a picture document: an image or an SVG, as large as the
// pane allows and centered.
func (a *App) viewerView(c *ui.Context, d *document.Document) {
	ui.Box(c).Grow(1).Padding(18).Children(func() {
		src, err := a.viewerSource(d)
		if err != nil {
			ui.Column(c).Grow(1).Gap(6).Children(func() {
				ui.Text(c, "无法预览这个文件").FontSize(12).Bold().TextColor(theme.Text)
				ui.Text(c, err.Error()).FontSize(10).TextColor(theme.Danger)
			})
			return
		}
		ui.Image(c, src).Fit(ui.Contain).Grow(1)
	})
}

// viewerSource decodes a picture once and keeps it: decoding a large image
// every frame would cost more than the rest of the window.
func (a *App) viewerSource(d *document.Document) (ui.ImageSource, error) {
	if a.viewers == nil {
		a.viewers = map[*document.Document]ui.ImageSource{}
	}
	if src, ok := a.viewers[d]; ok {
		return src, nil
	}
	if d.Kind() == document.KindSVG {
		svg, err := ui.ParseSVG(d.Data())
		if err != nil {
			return nil, err
		}
		a.viewers[d] = svg
		return svg, nil
	}
	bmp, err := ui.DecodeBitmap(d.Data())
	if err != nil {
		return nil, err
	}
	a.viewers[d] = bmp
	return bmp, nil
}

// forgetViewer drops a picture's decoded form, after its file changed.
func (a *App) forgetViewer(d *document.Document) {
	delete(a.viewers, d)
}

// viewerSize is the picture's size in pixels, for the breadcrumb.
func (a *App) viewerSize(d *document.Document) (int, int) {
	src, err := a.viewerSource(d)
	if err != nil || src == nil {
		return 0, 0
	}
	switch s := src.(type) {
	case *ui.Bitmap:
		return s.Size()
	case *ui.SVG:
		w, h := s.Size()
		return int(w + 0.5), int(h + 0.5)
	}
	return 0, 0
}

// viewerLabel is the breadcrumb's note about a picture: its size and that it
// is only shown.
func (a *App) viewerLabel(d *document.Document) string {
	w, h := a.viewerSize(d)
	if w <= 0 || h <= 0 {
		return "只读预览"
	}
	return fmt.Sprintf("只读预览 · %d×%d", w, h)
}

// markdownView shows a Markdown document rendered, scrolled like the
// editor; the file stays the buffer the editor would edit.
func (a *App) markdownView(c *ui.Context, d *document.Document) {
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(22).Gap(10).Children(func() {
			markdown.View(c, d.Buffer.String(), nil)
		})
	})
}

// previewControls fills the breadcrumb row's right side: how a picture is
// shown, or whether a Markdown document shows rendered or as its source.
func (a *App) previewControls(c *ui.Context, col *document.Column) {
	doc := a.docOf(col)
	if doc == nil {
		return
	}
	if doc.IsViewer() {
		ui.Text(c, a.viewerLabel(doc)).FontSize(9).TextColor(theme.Faint)
		return
	}
	if doc.Language() != "Markdown" {
		return
	}
	label := "预览"
	if e := col.Current(); e != nil && e.MarkdownPreview {
		label = "源码"
	}
	if ui.Button(c, "").Height(18).Padding(0, 6).Radius(3).Label(label).Clicked() {
		if e := col.Current(); e != nil {
			e.MarkdownPreview = !e.MarkdownPreview
		}
	}
}

// ToggleMarkdownPreview switches the chosen Markdown document between its
// source and the rendered page.
func (a *App) ToggleMarkdownPreview() {
	e := a.Col().Current()
	if e == nil || e.Doc.Language() != "Markdown" {
		a.say("当前文件不是 Markdown")
		return
	}
	e.MarkdownPreview = !e.MarkdownPreview
}
