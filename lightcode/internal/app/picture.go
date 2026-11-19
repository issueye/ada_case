package app

import (
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"lightcode/internal/document"
	"lightcode/internal/icons"
	"lightcode/internal/theme"
)

// pictureView shows an image file as a preview: its name, format and size
// above it, and the picture fit to the room below.
func (a *App) pictureView(c *ui.Context, d *document.Document) {
	ui.Column(c).Grow(1).Background(theme.Background).Children(func() {
		ui.Row(c).Height(28).PaddingX(12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.File).FontSize(13).TextColor(fileColor(d.Name()))
			ui.Text(c, d.Name()).FontSize(11).Bold().TextColor(theme.Text).SingleLine()
			ui.Text(c, d.PictureFormat()).FontSize(10).TextColor(theme.Muted)
			if w, h := d.PictureSize(); w > 0 && h > 0 {
				ui.Text(c, fmt.Sprintf("%d × %d", int(w), int(h))).FontSize(10).TextColor(theme.Faint)
			}
			ui.Box(c).Grow(1)
			iconButton(c, icons.Explorer, "在文件管理器中显示", 20, func() {
				mygo.Shell.ShowItemInFolder(d.Path)
			})
		})
		ui.Box(c).Height(1).Background(theme.BorderSoft)
		ui.Box(c).Grow(1).FillWidth().Padding(16).Background(theme.PanelDeep).Children(func() {
			ui.Image(c, d.Picture()).Fill().Fit(ui.ScaleDown)
		})
	})
}
