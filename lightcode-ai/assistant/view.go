package assistant

import (
	"encoding/json"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"

	"lightcode.ai/client/icons"
	"lightcode.ai/client/protocol"
	"lightcode.ai/client/theme"
)

// View draws the panel for the mount the host gave it.
func (s *State) View(c *ui.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.advanceLocked() {
		c.After(16 * time.Millisecond)
	}
	if s.mode == ModeRail {
		s.rail(c)
		return
	}
	s.panel(c)
}

// panel is the docked or floating panel: its own header, conversation and
// input.
func (s *State) panel(c *ui.Context) {
	ui.Column(c).Grow(1).Background(theme.Panel).Children(func() {
		s.header(c)
		s.body(c)
		s.input(c)
	})
}

// header is the panel's window-like title bar: a grip that drags the mount,
// the title, and the controls.
func (s *State) header(c *ui.Context) {
	ui.Row(c).Height(theme.HeaderH).Background(theme.PanelDeep).PaddingX(8).Gap(6).
		AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(theme.BorderSoft).
		Children(func() {
			grip := ui.Row(c).Width(150).Shrink(0).Gap(6).AlignItems(ui.Center)
			if s.mode != ModeDock {
				grip.Cursor(ui.CursorGrab).Tooltip("拖动标题移动 AI 面板").
					HandleInput(s.dragHandler(s.mode))
			}
			grip.Children(func() {
				ui.Icon(c, icons.Sparkles).FontSize(15).TextColor(theme.AI)
				ui.Text(c, "AI 助手").FontSize(11).Bold().TextColor(theme.Text)
				if s.streaming {
					ui.Text(c, "生成中").FontSize(9).TextColor(theme.AI).Padding(1, 5).
						Radius(3).Background(theme.AISoft)
				}
			})
			ui.Box(c).Grow(1)
			modes := []struct {
				icon  *ui.SVG
				label string
			}{
				{icons.PanelLeft, "停靠"},
				{icons.Split, "悬浮"},
				{icons.PanelRight, "收起"},
			}
			for i, m := range modes {
				color := theme.Muted
				if s.mode == i {
					color = theme.Accent
				}
				btn := ui.ButtonBase(c).Size(24, 22).Radius(4).Label(m.label).Tooltip(m.label)
				btn.Children(func() { ui.Icon(c, m.icon).FontSize(13).TextColor(color) })
				if s.mode == i {
					btn.Background(theme.AccentSoft)
				}
				mode := i
				btn.OnClick(func() { s.Bridge.SetMode(mode) })
			}
			iconButton(c, icons.Settings, "AI 设置", s.Bridge.Settings)
			iconButton(c, icons.Close, "收起 AI 面板", func() { s.Bridge.SetMode(ModeRail) })
		})
}

// body is the conversation: the intro with suggestions, or the turns with
// their answers and diff cards.
func (s *State) body(c *ui.Context) {
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(10).Gap(8).Children(func() {
			if len(s.messages) == 0 {
				s.intro(c)
				return
			}
			for i := range s.messages {
				m := &s.messages[i]
				if m.Role == "user" {
					userMessage(c, m.Content)
					continue
				}
				s.answer(c, m)
			}
			if s.streaming || s.reveal {
				// Keep the end of the answer in view while it grows.
				ui.Box(c).Height(1).ScrollIntoView()
			}
		})
	})
}

// answer is an assistant turn: its markdown, its diff cards and its error.
func (s *State) answer(c *ui.Context, m *Message) {
	if m.Content != "" {
		ui.Column(c).Children(func() { markdown.View(c, m.Content, nil) })
	}
	if m.Err != "" {
		ui.Text(c, "请求失败："+m.Err).FontSize(10).TextColor(theme.Danger)
		if ui.Button(c, "重试").Clicked() {
			s.Retry()
		}
	}
	for _, card := range m.Cards {
		if card.Hidden {
			continue
		}
		s.card(c, card)
	}
	if s.streaming {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "停止").Clicked() {
				s.Cancel()
			}
		})
	}
}

// card is one file's change, applied only on the user's click.
func (s *State) card(c *ui.Context, card *Card) {
	ui.Column(c).Padding(8).Gap(6).Radius(6).Background(theme.PanelDeep).
		Border(1, theme.BorderSoft).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.File).FontSize(13).TextColor(theme.Muted)
			ui.Text(c, card.Path).FontSize(11).TextColor(theme.Text).SingleLine().Grow(1)
			if card.Applied {
				ui.Text(c, "已应用").FontSize(9).TextColor(theme.Success)
			} else if card.Err != "" {
				ui.Text(c, "无法应用").FontSize(9).TextColor(theme.Danger)
			} else {
				if ui.Button(c, "应用").Clicked() {
					s.Apply(card, s.seq)
				}
				if ui.Button(c, "忽略").Clicked() {
					card.Hidden = true
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

// diffState builds the card's diff view once it opens.
func (card *Card) diffState() *editor.DiffState {
	if card.state == nil && (card.Old != "" || card.New != "") {
		oldBuf := editor.NewBuffer(card.Old)
		newBuf := editor.NewBuffer(card.New)
		card.state = &editor.DiffState{
			Old: oldBuf, New: newBuf, Layout: editor.DiffUnified,
			Font:  ui.Font{Family: "JetBrains Mono", Size: 12, LineHeight: 1.5},
			Theme: theme.Code(), ContextLines: 2, Inline: editor.DiffInlineWordAlt,
		}
	}
	return card.state
}

// input is the message field and the send or stop button.
func (s *State) input(c *ui.Context) {
	ui.Box(c).Height(1).Background(theme.BorderSoft)
	ui.Column(c).Padding(8, 10).Gap(4).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c, &s.draft).Placeholder("描述改动，Enter 发送…").FontSize(12).Grow(1)
			if in.Submitted() {
				s.Send()
			}
			if s.streaming {
				stop := ui.ButtonBase(c).Size(30, 30).Radius(6).Label("停止生成").Tooltip("停止").
					Children(func() { ui.Icon(c, icons.Close).FontSize(13).TextColor(theme.AI) })
				stop.OnClick(s.Cancel)
				if stop.Hovered() {
					stop.Background(theme.AISoft)
				}
			} else {
				send := ui.ButtonBase(c).Size(30, 30).Radius(6).Label("发送").Tooltip("发送（Enter）").
					Children(func() { ui.Icon(c, icons.ArrowRight).FontSize(15).TextColor(theme.AI) })
				send.OnClick(s.Send)
				if send.Hovered() {
					send.Background(theme.AISoft)
				}
			}
		})
		ui.Text(c, "Enter 发送 · 上下文取自编辑器 · diff 需点「应用」").
			FontSize(9).TextColor(theme.Faint).SingleLine()
	})
}

// intro is what the panel shows before the first question.
func (s *State) intro(c *ui.Context) {
	ui.Text(c, "让 AI 改代码").FontSize(13).Bold().TextColor(theme.Text)
	ui.Text(c, "描述你要的更改，AI 会返回一份 diff。应用前由你确认，不做内联建议，也不会自动修改文件。").
		FontSize(11).TextColor(theme.Muted).LineHeight(1.6)
	s.suggestion(c, "给这段 Go 代码加上错误处理")
	s.suggestion(c, "解释当前选中的代码做了什么")
	if s.err != "" {
		ui.Text(c, s.err).FontSize(10).TextColor(theme.Danger)
	}
}

// suggestion is a quick prompt of the empty conversation.
func (s *State) suggestion(c *ui.Context, text string) {
	row := ui.Row(c).Padding(7, 10).Radius(5).Gap(6).AlignItems(ui.Center).
		Cursor(ui.CursorPointer).Border(1, theme.BorderSoft).Background(theme.PanelDeep)
	row.Children(func() {
		ui.Icon(c, icons.ArrowRight).FontSize(11).TextColor(theme.AI)
		ui.Text(c, text).FontSize(11).TextColor(theme.Muted).SingleLine().Grow(1)
	})
	if row.Hovered() {
		row.Border(1, theme.Border).Background(theme.Hover)
	}
	row.OnClick(func() { s.draft = text })
}

// userMessage is a question, as a bubble.
func userMessage(c *ui.Context, text string) {
	ui.Row(c).Justify(ui.End).Children(func() {
		ui.Column(c).Padding(7, 10).Radius(7).Background(theme.AISoft).MaxWidth(300).
			Children(func() {
				ui.Text(c, text).FontSize(11).TextColor(theme.Text).LineHeight(1.5)
			})
	})
}

// rail is the collapsed panel: the 48×172 card that floats clear of the
// layout.
func (s *State) rail(c *ui.Context) {
	ui.Column(c).Grow(1).Background(theme.Panel).AlignItems(ui.Center).PaddingY(8).Gap(6).
		Children(func() {
			grip := ui.Box(c).Size(32, 32).Center().Cursor(ui.CursorGrab).
				Tooltip("AI 助手（已收起）；拖动可移动").
				HandleInput(s.dragHandler(ModeRail))
			grip.Children(func() { ui.Icon(c, icons.Sparkles).FontSize(20).TextColor(theme.AI) })
			ui.Box(c).Width(24).Height(1).Background(theme.BorderSoft)
			expand := ui.ButtonBase(c).Size(32, 32).Radius(6).Label("展开 AI 面板").
				Tooltip("展开 AI 助手").
				Children(func() { ui.Icon(c, icons.PanelRight).FontSize(15).TextColor(theme.Muted) })
			expand.OnClick(func() { s.Bridge.SetMode(ModeDock) })
			if expand.Hovered() {
				expand.Background(theme.AISoft)
			}
			newChat := ui.ButtonBase(c).Size(32, 32).Radius(6).Label("新对话").Tooltip("新对话").
				Children(func() { ui.Icon(c, icons.Plus).FontSize(14).TextColor(theme.Muted) })
			newChat.OnClick(s.NewChat)
			if newChat.Hovered() {
				newChat.Background(theme.AISoft)
			}
			ui.Box(c).Grow(1)
			ui.Text(c, "AI").FontSize(9).Bold().TextColor(theme.AI)
		})
}

// dragHandler drags the panel's mount by its header grip.
//
// The pointer's window position is the mount's origin plus the local one:
// the mount follows the pointer, so a local coordinate alone would tell a
// frame late. The host reports the origin after each move.
func (s *State) dragHandler(target int) func(ui.InputEvent) bool {
	var dragging bool
	var grabX, grabY float32
	return func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button != 0 {
				return false
			}
			dragging = true
			grabX, grabY = ev.X, ev.Y
			return true
		case ui.InputPointerMove:
			if !dragging {
				return false
			}
			s.mu.Lock()
			ox, oy := s.originX, s.originY
			s.mu.Unlock()
			s.Bridge.MoveTo(target, ox+ev.X-grabX, oy+ev.Y-grabY)
			return true
		case ui.InputPointerUp:
			if !dragging {
				return false
			}
			dragging = false
			return true
		}
		return false
	}
}

// iconButton is a small square button with one icon.
func iconButton(c *ui.Context, ic *ui.SVG, label string, fn func()) {
	btn := ui.ButtonBase(c).Size(22, 22).Radius(4).Label(label).Tooltip(label)
	btn.Children(func() { ui.Icon(c, ic).FontSize(13).TextColor(theme.Muted) })
	btn.OnClick(fn)
	if btn.Hovered() {
		btn.Background(theme.Hover)
	}
}

// Service serves the host's calls: the push of configuration.
func (s *State) Service(method string, args []json.RawMessage) (any, error) {
	switch method {
	case protocol.MethodConfigure:
		if len(args) == 0 {
			return nil, nil
		}
		return nil, s.Configure(args[0])
	}
	return nil, nil
}
