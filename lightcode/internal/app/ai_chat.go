package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"lightcode/internal/aiplugin"
	"lightcode/internal/icons"
	"lightcode/internal/theme"
	"lightcode/plugins/aiprovider"
)

// How the assistant shows.
const (
	AIDock  = 0
	AIFloat = 1
	AIRail  = 2
)

// aiMessage is one turn of the conversation.
type aiMessage struct {
	// Role is "user" or "assistant".
	Role    string
	Content string
	// Diff holds the file changes the assistant answered with.
	Diff []*aiDiffCard
	// Err is why this turn failed, for the retry.
	Err string
	// Full is the text received; Content the part shown so far, which
	// the view reveals a few runes at a time, as a typewriter.
	Full string
}

// aiDiffCard is one file's change in an answer: the text before and after,
// shown as a diff and applied only when the user clicks.
type aiDiffCard struct {
	Path    string
	Patch   editor.FilePatch
	Old     *editor.Buffer
	New     *editor.Buffer
	State   *editor.DiffState
	Open    bool
	Applied bool
	Err     string
}

// aiState is the assistant's state across frames.
type aiState struct {
	Mode      int
	Messages  []aiMessage
	Draft     string
	Streaming bool
	Err       string
	Config    aiplugin.Config
	// SettingsOpen edits the endpoint and the key.
	SettingsOpen bool
	SetBase      string
	SetModel     string
	SetKey       string
	// RailHover is whether the collapsed rail card is hovered, for its
	// preview.
	RailHover bool
	// RevealPending is whether the answer still has runes to show, as the
	// typewriter catches up with the text that arrived.
	RevealPending bool

	// client is the plugin process the answers come from, made on the
	// first question.
	client *aiplugin.Client
	cancel context.CancelFunc
	seq    uint64
}

// initAI fills the assistant's configuration: the saved endpoint and
// model, the key from the system store or the environment.
func (a *App) initAI() {
	a.AI.Config = aiplugin.DefaultConfig()
	if a.AI.SetBase != "" {
		a.AI.Config.BaseURL = a.AI.SetBase
	}
	if a.AI.SetModel != "" {
		a.AI.Config.Model = a.AI.SetModel
	}
	a.AI.Config.APIKey = a.loadAIKey()
}

// loadAIKey reads the key from the system's secret store, falling back to
// OPENAI_API_KEY for machines without one.
func (a *App) loadAIKey() string {
	if key, err := mygo.Secrets.Get("ai.key"); err == nil && len(key) > 0 {
		return string(key)
	}
	return os.Getenv("OPENAI_API_KEY")
}

// SetAIMode docks, floats or rails the assistant.
func (a *App) SetAIMode(mode int) {
	if mode < AIDock || mode > AIRail {
		mode = AIDock
	}
	a.AI.Mode = mode
	a.SaveState()
}

// ToggleAI collapses the docked panel to the rail, or brings it back.
func (a *App) ToggleAI() {
	if a.AI.Mode == AIRail {
		a.SetAIMode(AIDock)
		return
	}
	a.SetAIMode(AIRail)
}

// NewAIChat clears the conversation.
func (a *App) NewAIChat() {
	if a.AI.Streaming {
		a.CancelAI()
	}
	// A stream still running belongs to the cleared conversation: its
	// updates stop, as its reveal frames do.
	a.AI.seq++
	a.AI.Messages = nil
	a.AI.RevealPending = false
	a.AI.Err = ""
}

// SendAI sends the draft and the current file's context.
func (a *App) SendAI() {
	text := strings.TrimSpace(a.AI.Draft)
	if text == "" || a.AI.Streaming {
		return
	}
	if a.AI.Config.APIKey == "" {
		a.AI.SettingsOpen = true
		a.AI.SetBase = a.AI.Config.BaseURL
		a.AI.SetModel = a.AI.Config.Model
		a.AI.Err = "先填 API Key（保存在系统凭据库）"
		return
	}
	for i := range a.AI.Messages {
		if a.AI.Messages[i].Full != "" {
			a.AI.Messages[i].Content = a.AI.Messages[i].Full
		}
	}
	a.AI.Draft = ""
	a.AI.Err = ""
	a.AI.Messages = append(a.AI.Messages, aiMessage{Role: "user", Content: text})
	a.AI.Messages = append(a.AI.Messages, aiMessage{Role: "assistant"})
	answer := &a.AI.Messages[len(a.AI.Messages)-1]

	a.AI.seq++
	seq := a.AI.seq
	ctx, cancel := context.WithCancel(context.Background())
	a.AI.cancel = cancel
	a.AI.Streaming = true
	msgs := a.aiPrompt(text)

	style := func(piece string) {
		answer.Full += piece
	}
	finalize := func(err error) {
		if a.AI.seq != seq {
			return
		}
		a.AI.Streaming = false
		if err != nil && !errors.Is(err, context.Canceled) {
			answer.Err = err.Error()
			a.AI.Err = err.Error()
			return
		}
		answer.Diff = a.parseAIDiff(answer.Full)
		if a.Win == nil {
			answer.Content = answer.Full
		}
	}
	if a.Win == nil {
		// No window: the provider runs inline, the way search does. The
		// window's path is the plugin process, below.
		_, err := aiprovider.Stream(ctx, a.AI.Config, msgs, style)
		finalize(err)
		return
	}
	client, err := a.aiClient()
	if err != nil {
		finalize(err)
		return
	}
	win := a.Win
	// Deltas arrive far faster than frames: append them in batches, at
	// most every 50 ms, and let the typewriter catch up between them, so
	// the window rebuilds some twenty times a second rather than sixty.
	go func() {
		var pending strings.Builder
		last := time.Now()
		flush := func() {
			if pending.Len() == 0 {
				return
			}
			text := pending.String()
			pending.Reset()
			last = time.Now()
			win.Update(func() {
				if a.AI.seq != seq {
					return
				}
				answer.Full += text
				a.advanceAIReveal(answer)
			})
		}
		_, err := client.Stream(ctx, a.AI.Config, msgs, func(piece string) {
			pending.WriteString(piece)
			if time.Since(last) >= 50*time.Millisecond {
				flush()
			}
		})
		flush()
		win.Update(func() { finalize(err) })
		a.revealAITail(seq, answer)
	}()
}

// revealAITail shows the rest of the answer after the stream ends, a step
// every 50 ms on the main thread, until all of it shows. It stops as soon
// as the conversation changes, as a new chat or a new question does.
func (a *App) revealAITail(seq uint64, answer *aiMessage) {
	win := a.Win
	if win == nil {
		return
	}
	for i := 0; i < 400; i++ {
		time.Sleep(50 * time.Millisecond)
		more := false
		mygo.RunOnMain(func() {
			if a.AI.seq != seq {
				return
			}
			more = a.advanceAIReveal(answer)
		})
		if !more {
			return
		}
		win.Invalidate()
	}
}

// advanceAIReveal shows more of the answer's text, as a typewriter paced by
// the time between frames rather than by the frames themselves: a big
// backlog catches up in a few seconds, the tail types out at a readable
// rate. It runs on the main thread, and reports whether more is left.
func (a *App) advanceAIReveal(m *aiMessage) bool {
	if m.Full == "" || m.Full == m.Content {
		if m.Full != "" {
			m.Content = m.Full
		}
		a.AI.RevealPending = false
		return false
	}
	runes := []rune(m.Full)
	shown := utf8.RuneCountInString(m.Content)
	if shown >= len(runes) {
		m.Content = m.Full
		a.AI.RevealPending = false
		return false
	}
	remaining := len(runes) - shown
	step := remaining/12 + 3
	if !a.AI.Streaming {
		// Nothing more comes: finish within some frames.
		step = remaining/8 + 3
	}
	if next := min(shown+step, len(runes)); next > shown {
		m.Content = string(runes[:next])
		shown = next
	}
	a.AI.RevealPending = shown < len(runes)
	return a.AI.RevealPending
}

// CancelAI stops the stream, keeping what arrived.
func (a *App) CancelAI() {
	if a.AI.cancel != nil {
		a.AI.cancel()
		a.AI.cancel = nil
	}
	a.AI.Streaming = false
}

// RetryAI sends the last user message again.
func (a *App) RetryAI() {
	var last string
	for i := len(a.AI.Messages) - 1; i >= 0; i-- {
		if a.AI.Messages[i].Role == "user" {
			last = a.AI.Messages[i].Content
			a.AI.Messages = a.AI.Messages[:i]
			break
		}
	}
	if last == "" {
		return
	}
	a.AI.Draft = last
	a.SendAI()
}

// aiPrompt builds the messages: the contract, the open file, the recent
// turns and the request.
func (a *App) aiPrompt(request string) []aiplugin.ChatMessage {
	var out []aiplugin.ChatMessage
	system := "你是轻码 IDE 的编程助手。用户要求改代码时，请用 git unified diff 格式回答" +
		"（放在 ```diff 代码块里），每个文件一个 diff；不要输出内联建议、不要声称已经修改文件。" +
		"改动由用户在界面上确认后应用。其余问题用简洁的中文回答。"
	out = append(out, aiplugin.ChatMessage{Role: "system", Content: system})
	if e := a.Col().Current(); e != nil {
		text := e.Doc.Buffer.String()
		if selected := e.State.SelectedText(); selected != "" {
			text = selected
		}
		if len(text) > 8000 {
			text = text[:8000] + "\n…（已截断）"
		}
		rel := e.Doc.Path
		if a.WS != nil {
			if r, err := relPath(a.WS.Root, e.Doc.Path); err == nil {
				rel = r
			}
		}
		out = append(out, aiplugin.ChatMessage{Role: "system", Content: "当前文件 " + rel + "：\n```\n" + text + "\n```"})
	}
	for _, m := range a.AI.Messages {
		if m.Role == "user" {
			out = append(out, aiplugin.ChatMessage{Role: m.Role, Content: m.Content})
		} else if m.Full != "" {
			out = append(out, aiplugin.ChatMessage{Role: m.Role, Content: m.Full})
		}
	}
	return out
}

// parseAIDiff finds the file patches in an answer and builds their cards.
func (a *App) parseAIDiff(content string) []*aiDiffCard {
	files, err := editor.ParsePatch(content)
	if err != nil || len(files) == 0 {
		return nil
	}
	var cards []*aiDiffCard
	for _, fp := range files {
		path := fp.NewName
		if path == "" {
			path = fp.OldName
		}
		if path == "" || a.WS == nil {
			continue
		}
		oldText := ""
		if d, err := a.Doc(filepath.Join(a.WS.Root, filepath.FromSlash(path))); err == nil {
			oldText = d.Buffer.String()
		}
		newText, err := fp.Apply(oldText)
		if err != nil {
			cards = append(cards, &aiDiffCard{Path: path, Patch: fp, Err: err.Error()})
			continue
		}
		oldBuf := editor.NewBuffer(oldText)
		newBuf := editor.NewBuffer(newText)
		lang := editor.LanguageFor(filepath.Base(path))
		oldBuf.SetLanguage(lang)
		newBuf.SetLanguage(lang)
		cards = append(cards, &aiDiffCard{Path: path, Patch: fp, Old: oldBuf, New: newBuf})
	}
	return cards
}

// diffStateOf builds the card's diff view once it opens.
func (c *aiDiffCard) diffState() *editor.DiffState {
	if c.State == nil && c.Old != nil && c.New != nil {
		c.State = &editor.DiffState{
			Old: c.Old, New: c.New, Layout: editor.DiffUnified,
			Font: theme.EditorFont(), Theme: theme.Code(theme.Window()),
			ContextLines: 2, Inline: editor.DiffInlineWordAlt,
		}
	}
	return c.State
}

// ApplyAIDiff writes the card's new text into the file, as the user
// confirmed it.
func (a *App) ApplyAIDiff(c *aiDiffCard) {
	if c == nil || c.New == nil || a.WS == nil {
		return
	}
	path := filepath.Join(a.WS.Root, filepath.FromSlash(c.Path))
	d, err := a.Doc(path)
	if err != nil {
		c.Err = err.Error()
		return
	}
	d.Buffer.Set(c.New.String())
	c.Applied = true
	a.updateTitle()
	a.refreshGit()
	a.say("已应用 AI 对 " + c.Path + " 的改动（可撤销保存前）")
}

// OpenAISettings opens the endpoint and key dialog.
func (a *App) OpenAISettings() {
	a.AI.SettingsOpen = true
	a.AI.SetBase = a.AI.Config.BaseURL
	a.AI.SetModel = a.AI.Config.Model
	a.AI.SetKey = ""
}

// SaveAISettings saves the endpoint, the model and the key; the key goes
// to the system's store when it has one.
func (a *App) SaveAISettings() {
	base := strings.TrimSpace(a.AI.SetBase)
	model := strings.TrimSpace(a.AI.SetModel)
	if base != "" {
		a.AI.Config.BaseURL = base
	}
	if model != "" {
		a.AI.Config.Model = model
	}
	if key := strings.TrimSpace(a.AI.SetKey); key != "" {
		a.AI.Config.APIKey = key
		if err := mygo.Secrets.Set("ai.key", []byte(key)); err != nil {
			a.AI.Err = "Key 未能存入系统凭据库：" + err.Error()
		}
	}
	a.AI.SettingsOpen = false
	a.AI.Err = ""
	a.SaveState()
}

// aiSettingsOverlay is the endpoint and key dialog.
func (a *App) aiSettingsOverlay(c *ui.Context) {
	if !a.AI.SettingsOpen {
		return
	}
	ui.Modal(c, &a.AI.SettingsOpen, func() {
		ui.Column(c).Width(460).Gap(10).Padding(18).Radius(8).
			Background(theme.Panel).Border(1, theme.Border).
			Shadow(0, 14, 36, 0, ui.RGBA(0, 0, 0, 0.55)).Children(func() {
			ui.Text(c, "AI 设置").FontSize(14).Bold().TextColor(theme.Text)
			ui.Text(c, "兼容 OpenAI 的 /chat/completions 流式接口。Key 存在系统凭据库（Windows Credential Manager / macOS Keychain）。").
				FontSize(10).TextColor(theme.Muted).LineHeight(1.5)
			ui.Text(c, "Base URL").FontSize(10).TextColor(theme.Faint)
			ui.TextInput(c, &a.AI.SetBase).Placeholder("https://api.openai.com/v1").FontSize(12)
			ui.Text(c, "模型").FontSize(10).TextColor(theme.Faint)
			ui.TextInput(c, &a.AI.SetModel).Placeholder("gpt-4o-mini").FontSize(12)
			ui.Text(c, "API Key").FontSize(10).TextColor(theme.Faint)
			ui.TextInput(c, &a.AI.SetKey).Placeholder("sk-…（留空保留原来的）").FontSize(12)
			if a.AI.Err != "" {
				ui.Text(c, a.AI.Err).FontSize(10).TextColor(theme.Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.AI.SettingsOpen = false
					a.AI.Err = ""
				}
				if ui.Button(c, "保存").Clicked() {
					a.SaveAISettings()
				}
			})
		})
	})
}

// aiContextChips shows what the next request carries.
func (a *App) aiContextChips(c *ui.Context) {
	ui.Row(c).Gap(6).Children(func() {
		chip(c, icons.File, "当前文件")
		if e := a.Col().Current(); e != nil {
			chip(c, icons.File, e.Doc.Name())
		}
	})
}
