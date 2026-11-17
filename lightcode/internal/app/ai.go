package app

import (
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"

	"lightcode/internal/icons"
	"lightcode/internal/theme"
)

// aiPanel is the assistant docked at the right, 380 DIPs wide.
func (a *App) aiPanel(c *ui.Context) {
	ui.Column(c).Width(theme.AIDockW).FillHeight().Background(theme.Panel).
		BorderWidth(0, 1, 0, 0).BorderColor(theme.BorderSoft).Children(func() {
		a.aiHeader(c)
		a.aiBody(c)
		a.aiInput(c)
	})
}

// aiFloat is the assistant floating over the editor, 380×640.
func (a *App) aiFloat(c *ui.Context) {
	if a.AI.Mode != AIFloat {
		return
	}
	ui.Overlay(c, func() {
		card := ui.Column(c).Absolute().Right(14).Top(float32(theme.TitleBarH)+16).Width(380).Height(640).
			Radius(10).Background(theme.Panel).Border(1, theme.Border).
			Shadow(0, 16, 40, 0, ui.RGBA(0, 0, 0, 0.55))
		card.Children(func() {
			a.aiHeader(c)
			a.aiBody(c)
			a.aiInput(c)
		})
	})
}

// aiHeader is the title row: the modes, the settings and the collapse.
func (a *App) aiHeader(c *ui.Context) {
	ui.Row(c).Height(theme.PanelHeadH).PaddingX(10).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icons.Sparkles).FontSize(15).TextColor(theme.AI)
		ui.Text(c, "AI 助手").FontSize(11).Bold().TextColor(theme.Text)
		if a.AI.Streaming {
			ui.Text(c, "生成中").FontSize(9).TextColor(theme.AI).Padding(1, 5).
				Radius(3).Background(theme.AISoft)
		}
		ui.Box(c).Grow(1)
		ui.Segmented(c, &a.AI.Mode, "停靠", "悬浮", "收起").FontSize(9).
			OnChange(func() { a.SaveState() })
		iconButton(c, icons.Settings, "AI 设置", 20, a.OpenAISettings)
		iconButton(c, icons.History, "新对话", 20, a.NewAIChat)
	})
}

// aiBody is the conversation: the intro with suggestions, or the turns
// with their answers and diff cards.
func (a *App) aiBody(c *ui.Context) {
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(10).Gap(8).Children(func() {
			if len(a.AI.Messages) == 0 {
				a.aiIntro(c)
				return
			}
			pending := false
			for i := range a.AI.Messages {
				m := &a.AI.Messages[i]
				if m.Role == "user" {
					a.aiUserMessage(c, m.Content)
					continue
				}
				if a.revealAI(m) {
					pending = true
				}
				a.aiAnswer(c, m)
			}
			if a.AI.Streaming || pending {
				ui.Box(c).Height(1).ScrollIntoView()
			}
			if pending {
				c.After(16 * time.Millisecond)
			}
		})
	})
}

// aiRailPeek previews the last answer while the rail card is hovered.
func (a *App) aiRailPeek(c *ui.Context) {
	_, height := c.Size()
	top := float32(theme.TitleBarH) + (height-float32(theme.TitleBarH)-float32(theme.StatusBarH))/2 - 100
	if top < float32(theme.TitleBarH)+8 {
		top = float32(theme.TitleBarH) + 8
	}
	ui.Overlay(c, func() {
		card := ui.Column(c).Absolute().Right(float32(theme.AIRailW)+10).Top(top).Width(330).
			Padding(12).Gap(6).Radius(8).Background(theme.Panel).Border(1, theme.AI).
			Shadow(0, 12, 30, 0, ui.RGBA(0, 0, 0, 0.5))
		card.Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icons.Sparkles).FontSize(13).TextColor(theme.AI)
				ui.Text(c, "AI 助手").FontSize(11).Bold().TextColor(theme.Text)
			})
			excerpt := a.lastAnswerExcerpt()
			if excerpt == "" {
				excerpt = "展开对话，让 AI 改代码；回答只产出 diff，由你确认后应用。"
			}
			ui.Text(c, excerpt).FontSize(11).TextColor(theme.Muted).LineHeight(1.5)
			ui.Text(c, "点击卡片展开").FontSize(9).TextColor(theme.Faint)
		})
	})
}

// lastAnswerExcerpt is the tail of the last answer for the rail's preview.
func (a *App) lastAnswerExcerpt() string {
	for i := len(a.AI.Messages) - 1; i >= 0; i-- {
		m := &a.AI.Messages[i]
		if m.Role != "assistant" {
			continue
		}
		text := m.Full
		if text == "" {
			text = m.Content
		}
		if text == "" {
			continue
		}
		r := []rune(text)
		if len(r) > 320 {
			r = r[len(r)-320:]
		}
		return string(r)
	}
	return ""
}

// revealAI shows a few more runes of the answer, as a typewriter: the
// network may deliver a whole reply at once, the view still types it.
// It reports whether more is left to show.
func (a *App) revealAI(m *aiMessage) bool {
	if m.Full == "" || m.Full == m.Content {
		if m.Full != "" {
			m.Content = m.Full
		}
		return false
	}
	runes := []rune(m.Full)
	shown := utf8.RuneCountInString(m.Content)
	if shown >= len(runes) {
		m.Content = m.Full
		return false
	}
	remaining := len(runes) - shown
	step := 2
	if !a.AI.Streaming || remaining > 300 {
		step = remaining/40 + 2
	}
	next := shown + step
	if next > len(runes) {
		next = len(runes)
	}
	m.Content = string(runes[:next])
	return next < len(runes)
}

// aiIntro is what the panel shows before the first question.
func (a *App) aiIntro(c *ui.Context) {
	ui.Text(c, "让 AI 改代码").FontSize(13).Bold().TextColor(theme.Text)
	ui.Text(c, "描述你要的更改，AI 会返回一份 diff。应用前由你确认，不做内联建议，也不会自动修改文件。").
		FontSize(11).TextColor(theme.Muted).LineHeight(1.6)
	a.aiSuggestion(c, "给这段 Go 代码加上错误处理")
	a.aiSuggestion(c, "解释当前选中的代码做了什么")
	a.aiSuggestion(c, "为这个文件补一组表驱动测试")
	ui.Box(c).Height(4)
	ui.Text(c, "上下文").FontSize(10).Bold().TextColor(theme.Faint)
	a.aiContextChips(c)
	if a.AI.Err != "" {
		ui.Text(c, a.AI.Err).FontSize(10).TextColor(theme.Danger)
	}
}

// aiUserMessage is a question, as a bubble.
func (a *App) aiUserMessage(c *ui.Context, text string) {
	row := ui.Row(c).Justify(ui.End).Children(func() {
		ui.Column(c).Padding(7, 10).Radius(7).Background(theme.AISoft).MaxWidth(300).
			Children(func() {
				ui.Text(c, text).FontSize(11).TextColor(theme.Text).LineHeight(1.5)
			})
	})
	_ = row
}

// aiAnswer is an assistant turn: its markdown, its diff cards and its
// error with a retry.
func (a *App) aiAnswer(c *ui.Context, m *aiMessage) {
	if m.Content != "" {
		ui.Column(c).Children(func() {
			markdown.View(c, m.Content, nil)
		})
	}
	if m.Err != "" {
		ui.Text(c, "请求失败："+m.Err).FontSize(10).TextColor(theme.Danger)
		if ui.Button(c, "重试").Clicked() {
			a.RetryAI()
		}
	}
	for _, card := range m.Diff {
		a.aiDiffCard(c, card)
	}
	if a.AI.Streaming {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "停止").Clicked() {
				a.CancelAI()
			}
		})
	}
}

// aiDiffCard is one file's change, applied only on the user's click.
func (a *App) aiDiffCard(c *ui.Context, card *aiDiffCard) {
	box := ui.Column(c).Padding(8).Gap(6).Radius(6).Background(theme.PanelDeep).
		Border(1, theme.BorderSoft)
	box.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, fileIcon(card.Path)).FontSize(13).TextColor(fileColor(card.Path))
			ui.Text(c, card.Path).FontSize(11).TextColor(theme.Text).SingleLine().Grow(1)
			if card.Applied {
				ui.Text(c, "已应用").FontSize(9).TextColor(theme.Added)
			} else if card.Err != "" {
				ui.Text(c, "无法应用").FontSize(9).TextColor(theme.Danger)
			} else {
				if ui.Button(c, "应用").Clicked() {
					a.ApplyAIDiff(card)
				}
				if ui.Button(c, "忽略").Clicked() {
					a.DiscardAIDiff(card)
				}
			}
			if ui.Button(c, "差异").Clicked() {
				card.Open = !card.Open
			}
		})
		if card.Err != "" {
			ui.Text(c, card.Err).FontSize(9).TextColor(theme.Danger).SingleLine()
		}
		if card.Open {
			if st := card.diffState(); st != nil {
				editor.DiffView(c, st).Height(220)
			}
		}
	})
}

// DiscardAIDiff forgets one of the answer's cards.
func (a *App) DiscardAIDiff(card *aiDiffCard) {
	for i := range a.AI.Messages {
		m := &a.AI.Messages[i]
		for j, c2 := range m.Diff {
			if c2 == card {
				m.Diff = append(m.Diff[:j], m.Diff[j+1:]...)
				return
			}
		}
	}
}

// aiInput is the message field and the send or stop button.
func (a *App) aiInput(c *ui.Context) {
	ui.Box(c).Height(1).Background(theme.BorderSoft)
	ui.Column(c).Padding(8, 10).Gap(4).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			input := ui.TextInput(c, &a.AI.Draft).Placeholder("描述改动，Enter 发送…").FontSize(12).Grow(1)
			if input.Submitted() {
				a.SendAI()
			}
			if a.AI.Streaming {
				stop := ui.ButtonBase(c).Size(30, 30).Radius(6).Label("停止生成").Tooltip("停止").
					Children(func() { ui.Icon(c, icons.Close).FontSize(13).TextColor(theme.AI) })
				stop.OnClick(a.CancelAI)
				if stop.Hovered() {
					stop.Background(theme.AISoft)
				}
			} else {
				send := ui.ButtonBase(c).Size(30, 30).Radius(6).Label("发送").Tooltip("发送（Enter）").
					Children(func() { ui.Icon(c, icons.ArrowRight).FontSize(15).TextColor(theme.AI) })
				send.OnClick(a.SendAI)
				if send.Hovered() {
					send.Background(theme.AISoft)
				}
			}
		})
		ui.Text(c, "Enter 发送 · 选中代码会作为上下文 · diff 需点「应用」").
			FontSize(9).TextColor(theme.Faint).SingleLine()
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
	row.OnClick(func() { a.AI.Draft = text })
}

// chip is a small labeled box of the panel.
func chip(c *ui.Context, ic *ui.SVG, label string) {
	ui.Row(c).Height(22).Padding(0, 8).Gap(4).Radius(11).AlignItems(ui.Center).
		Background(theme.AISoft).Children(func() {
		ui.Icon(c, ic).FontSize(11).TextColor(theme.AI)
		ui.Text(c, label).FontSize(10).TextColor(theme.AI).SingleLine()
	})
}

// aiRail is the collapsed assistant: a 48×172 card in its own 62 DIP
// strip at the right of the editor.
func (a *App) aiRail(c *ui.Context) {
	ui.Column(c).Width(theme.AIRailW).FillHeight().Background(theme.Background).
		BorderWidth(0, 1, 0, 0).BorderColor(theme.BorderSoft).
		Justify(ui.Center).AlignItems(ui.Start).Children(func() {
		card := ui.Column(c).Width(48).Height(172).Radius(10).Background(theme.Panel).Border(1, theme.Border).
			Shadow(0, 12, 30, 0, ui.RGBA(0, 0, 0, 0.5)).
			AlignItems(ui.Center).PaddingY(8).Gap(6)
		card.Children(func() {
			ui.Icon(c, icons.Sparkles).FontSize(20).TextColor(theme.AI).Tooltip("AI 助手（已收起）")
			ui.Box(c).Width(24).Height(1).Background(theme.BorderSoft)
			expand := ui.ButtonBase(c).Size(32, 32).Radius(6).Label("展开 AI 面板").Tooltip("展开 AI 助手").
				Children(func() { ui.Icon(c, icons.PanelRight).FontSize(15).TextColor(theme.Muted) })
			expand.OnClick(func() { a.SetAIMode(AIDock) })
			if expand.Hovered() {
				expand.Background(theme.AISoft)
			}
			newChat := ui.ButtonBase(c).Size(32, 32).Radius(6).Label("新对话").Tooltip("新对话").
				Children(func() { ui.Icon(c, icons.Plus).FontSize(14).TextColor(theme.Muted) })
			newChat.OnClick(a.NewAIChat)
			if newChat.Hovered() {
				newChat.Background(theme.AISoft)
			}
			ui.Box(c).Grow(1)
			ui.Text(c, "AI").FontSize(9).Bold().TextColor(theme.AI)
		})
		a.AI.RailHover = card.Hovered()
		if card.Hovered() {
			card.Border(1, theme.AI)
		}
	})
}
