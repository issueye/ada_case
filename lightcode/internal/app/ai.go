package app

import (
	"github.com/egoist/mygo/ui"

	"lightcode/internal/icons"
	"lightcode/internal/theme"
)

// aiPanel is the AI assistant docked at the right, 380 DIPs wide. It
// shows the conversation; the rail of aiRail replaces it when collapsed.
//
// The provider lands in M4: for now the panel explains the contract, that
// AI answers with diffs the user applies.
func (a *App) aiPanel(c *ui.Context) {
	ui.Column(c).Width(theme.AIDockW).FillHeight().Background(theme.Panel).
		BorderWidth(0, 1, 0, 0).BorderColor(theme.BorderSoft).Children(func() {
		ui.Row(c).Height(theme.PanelHeadH).PaddingX(10).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Sparkles).FontSize(15).TextColor(theme.AI)
			ui.Text(c, "AI 助手").FontSize(11).Bold().TextColor(theme.Text)
			ui.Text(c, "diff 预览").FontSize(10).TextColor(theme.AI).Padding(1, 5).
				Radius(3).Background(theme.AISoft)
			ui.Box(c).Grow(1)
			iconButton(c, icons.History, "对话历史（M4）", 22, func() { a.say("对话历史将在 M4 提供") })
			iconButton(c, icons.PanelRight, "收起 AI 面板", 22, a.ToggleAI)
		})
		ui.Box(c).Height(1).Background(theme.BorderSoft)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(12).Gap(8).Children(func() {
				ui.Text(c, "让 AI 改代码").FontSize(13).Bold().TextColor(theme.Text)
				ui.Text(c, "描述你要的更改，AI 会返回一份 diff。应用前由你确认，不做内联建议，也不会自动修改文件。").
					FontSize(11).TextColor(theme.Muted).LineHeight(1.6)
				a.aiSuggestion(c, "给这段 Go 代码加上错误处理")
				a.aiSuggestion(c, "解释当前选中的 hunk 做了什么")
				a.aiSuggestion(c, "为这个文件补一组表驱动测试")
				ui.Box(c).Height(6)
				ui.Text(c, "上下文").FontSize(10).Bold().TextColor(theme.Faint)
				ui.Row(c).Gap(6).Children(func() {
					chip(c, icons.File, "当前文件")
					if e := a.Col().Current(); e != nil {
						chip(c, icons.File, e.Doc.Name())
					}
				})
			})
		})
		ui.Box(c).Height(1).Background(theme.BorderSoft)
		ui.Row(c).Height(52).Padding(8, 10).Gap(8).AlignItems(ui.Center).Children(func() {
			input := ui.TextInput(c, &a.AIDraft).Placeholder("向 AI 发送消息…（M4）").FontSize(12).Grow(1)
			if input.Submitted() && a.AIDraft != "" {
				a.say("AI 提供商将在 M4 接入")
				a.AIDraft = ""
			}
			send := ui.ButtonBase(c).Size(30, 30).Radius(6).Label("发送").Tooltip("发送").
				Children(func() { ui.Icon(c, icons.ArrowRight).FontSize(15).TextColor(theme.AI) })
			send.OnClick(func() {
				if a.AIDraft != "" {
					a.say("AI 提供商将在 M4 接入")
					a.AIDraft = ""
				}
			})
			if send.Hovered() {
				send.Background(theme.AISoft)
			}
		})
	})
}

// aiSuggestion is a quick prompt of the empty conversation.
func (a *App) aiSuggestion(c *ui.Context, text string) {
	row := ui.Row(c).Padding(7, 10).Radius(5).Gap(6).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Border(1, theme.BorderSoft).Background(theme.PanelDeep)
	row.Children(func() {
		ui.Icon(c, icons.ArrowRight).FontSize(11).TextColor(theme.AI)
		ui.Text(c, text).FontSize(11).TextColor(theme.Muted).SingleLine().Grow(1)
	})
	if row.Hovered() {
		row.Border(1, theme.Border).Background(theme.Hover)
	}
	row.OnClick(func() {
		a.AIDraft = text
		a.say("AI 提供商将在 M4 接入")
	})
}

// chip is a small labeled box of the panel.
func chip(c *ui.Context, ic *ui.SVG, label string) {
	ui.Row(c).Height(22).Padding(0, 8).Gap(4).Radius(11).AlignItems(ui.Center).
		Background(theme.AISoft).Children(func() {
		ui.Icon(c, ic).FontSize(11).TextColor(theme.AI)
		ui.Text(c, label).FontSize(10).TextColor(theme.AI).SingleLine()
	})
}

// aiRail is the collapsed assistant: a 48×172 card floating at the right
// of the editor, which grows back into the dock when clicked.
func (a *App) aiRail(c *ui.Context) {
	_, height := c.Size()
	top := float32(theme.TitleBarH) + (height-float32(theme.TitleBarH)-float32(theme.StatusBarH))/2 - 86
	if top < float32(theme.TitleBarH)+10 {
		top = float32(theme.TitleBarH) + 10
	}
	ui.Overlay(c, func() {
		card := ui.Column(c).Absolute().Right(14).Top(top).Width(48).Height(172).
			Radius(10).Background(theme.Panel).Border(1, theme.Border).
			Shadow(0, 12, 30, 0, ui.RGBA(0, 0, 0, 0.5)).
			AlignItems(ui.Center).PaddingY(8).Gap(6)
		card.Children(func() {
			ui.Icon(c, icons.Sparkles).FontSize(20).TextColor(theme.AI).Tooltip("AI 助手（已收起）")
			ui.Box(c).Width(24).Height(1).Background(theme.BorderSoft)
			expand := ui.ButtonBase(c).Size(32, 32).Radius(6).Label("展开 AI 面板").Tooltip("展开 AI 助手").
				Children(func() { ui.Icon(c, icons.PanelRight).FontSize(15).TextColor(theme.Muted) })
			expand.OnClick(a.ToggleAI)
			if expand.Hovered() {
				expand.Background(theme.AISoft)
			}
			newChat := ui.ButtonBase(c).Size(32, 32).Radius(6).Label("新对话").Tooltip("新对话（M4）").
				Children(func() { ui.Icon(c, icons.Plus).FontSize(14).TextColor(theme.Muted) })
			newChat.OnClick(func() { a.say("AI 对话将在 M4 提供") })
			if newChat.Hovered() {
				newChat.Background(theme.AISoft)
			}
			ui.Box(c).Grow(1)
			ui.Text(c, "AI").FontSize(9).Bold().TextColor(theme.AI)
		})
		if card.Hovered() {
			card.Border(1, theme.AI)
		}
	})
}
