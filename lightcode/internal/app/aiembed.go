package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/learningdog1/ada_link/link"
	"github.com/learningdog1/ada_link/mygoembed"

	"lightcode.ai/client/protocol"

	"lightcode/internal/icons"
	"lightcode/internal/theme"
)

// The rectangles the editor offers the panel: the dock's width, and the
// floating and rail cards' sizes.
const (
	aiFloatW = 380
	aiFloatH = 640
	aiRailW  = 48
	aiRailH  = 172
)

// aiEdge is the assistant's mounted panel: the link session, the embedded
// host that draws its frames, and the answers whose cards are still on
// screen.
type aiEdge struct {
	sess    *link.Session
	host    *mygoembed.Host
	ready   bool
	err     string
	answers map[string][]aiAnswerCard
	// dirty asks the next frame to push the configuration to the panel.
	dirty bool
}

// aiAnswerCard is one file's patch of an answer, kept so a later click can
// apply it to the file as it is then.
type aiAnswerCard struct {
	path  string
	patch editor.FilePatch
}

// aiMountDock is the panel's place in the layout: a column at the right.
func (a *App) aiMountDock(c *ui.Context) {
	ui.Column(c).Width(theme.AIDockW).FillHeight().Background(theme.Panel).
		BorderWidth(0, 1, 0, 0).BorderColor(theme.BorderSoft).Children(func() {
		a.aiMount(c)
	})
}

// aiMountFloat is the panel floating over the code, where the user left it.
func (a *App) aiMountFloat(c *ui.Context) {
	if a.AI.Mode != AIFloat {
		return
	}
	x, y := a.aiPosition(AIFloat)
	ui.Overlay(c, func() {
		ui.Column(c).Absolute().Left(x).Top(y).Width(aiFloatW).Height(aiFloatH).
			Radius(10).Background(theme.Panel).Border(1, theme.Border).
			Shadow(0, 16, 40, 0, ui.RGBA(0, 0, 0, 0.55)).Children(func() {
			a.aiMount(c)
		})
	})
}

// aiMountRail is the collapsed panel: a small card clear of the layout.
func (a *App) aiMountRail(c *ui.Context) {
	if a.AI.Mode != AIRail {
		return
	}
	x, y := a.aiPosition(AIRail)
	ui.Overlay(c, func() {
		ui.Column(c).Absolute().Left(x).Top(y).Width(aiRailW).Height(aiRailH).
			Radius(10).Background(theme.Panel).Border(1, theme.Border).
			Shadow(0, 12, 30, 0, ui.RGBA(0, 0, 0, 0.5)).Children(func() {
			a.aiMount(c)
		})
	})
}

// aiMount draws the panel's frames, or why it has none.
func (a *App) aiMount(c *ui.Context) {
	e := &a.AIEdge
	if e.err != "" {
		a.aiMountError(c, e.err)
		return
	}
	if e.sess != nil && e.sess.IsClosed() {
		e.err = "AI 插件进程已退出"
		a.aiMountError(c, e.err)
		return
	}
	h := a.ensureAIEdge()
	if h == nil {
		return
	}
	if e.dirty {
		a.pushAIConfig()
	}
	ui.Box(c).Grow(1).Children(func() { h.View(c) })
}

// aiMountError shows why the panel is missing, and the way back: the plugin
// is its own project, so the editor says how to build it.
func (a *App) aiMountError(c *ui.Context, msg string) {
	ui.Column(c).Grow(1).Padding(14).Gap(10).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Sparkles).FontSize(14).TextColor(theme.AI)
			ui.Text(c, "AI 助手").FontSize(11).Bold().TextColor(theme.Text)
		})
		ui.Text(c, msg).FontSize(10).TextColor(theme.Danger).LineHeight(1.6)
		if ui.Button(c, "重试").Clicked() {
			a.ResetAIEdge()
		}
		if ui.Button(c, "AI 设置").Clicked() {
			a.OpenAISettings()
		}
	})
}

// ensureAIEdge starts the panel's process once, and mounts its frames on the
// window.
func (a *App) ensureAIEdge() *mygoembed.Host {
	e := &a.AIEdge
	if e.host != nil && !e.sess.IsClosed() {
		return e.host
	}
	if a.Win == nil {
		e.err = "AI 面板需要窗口"
		return nil
	}
	cmd, err := aiPluginCommand()
	if err != nil {
		e.err = err.Error()
		return nil
	}
	sess, err := link.Spawn(link.SpawnOptions{
		Command: cmd, W: aiFloatW, H: aiFloatH, Scale: 1,
		Title: "lightcode-ai", Timeout: 10 * time.Second,
	})
	if err != nil {
		e.err = "AI 插件未启动：" + err.Error()
		return nil
	}
	a.registerAIHandlers(sess)
	h := mygoembed.Attach(a.Win, sess)
	e.sess, e.host, e.err, e.ready = sess, h, "", false
	e.answers = map[string][]aiAnswerCard{}
	h.OnReady(func() {
		a.AIEdge.ready = true
		a.AIEdge.dirty = true
		a.Win.Invalidate()
	})
	e.dirty = true
	return h
}

// ResetAIEdge drops the panel and its process; the next frame starts a new
// one.
func (a *App) ResetAIEdge() {
	e := &a.AIEdge
	if e.sess != nil {
		e.sess.Close()
	}
	e.sess, e.host, e.ready, e.err = nil, nil, false, ""
	e.answers = nil
	e.dirty = true
}

// RestartAI replaces the panel process: the next frame starts a new one.
func (a *App) RestartAI() { a.ResetAIEdge() }

// CloseAI stops the panel process, as the window closes.
func (a *App) CloseAI() { a.ResetAIEdge() }

// pushAIConfig sends the endpoint, the key, the palette and the mount to
// the panel, and tells it where the mount sits.
func (a *App) pushAIConfig() {
	e := &a.AIEdge
	if e.sess == nil {
		return
	}
	x, y := a.aiOrigin()
	cfg := protocol.Configure{
		BaseURL: a.AI.Config.BaseURL,
		Model:   a.AI.Config.Model,
		APIKey:  a.AI.Config.APIKey,
		Light:   theme.IsLight(),
		Mode:    a.AI.Mode,
		X:       x,
		Y:       y,
	}
	e.dirty = false
	sess := e.sess
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = sess.Call(ctx, protocol.MethodConfigure, cfg)
		_ = sess.Emit(protocol.EventOrigin, protocol.Origin{X: x, Y: y})
	}()
}

// aiOrigin is where the mount sits in the window: the dock takes no
// position of its own.
func (a *App) aiOrigin() (float32, float32) {
	if a.AI.Mode == AIDock {
		return 0, 0
	}
	return a.aiPosition(a.AI.Mode)
}

// registerAIHandlers serves the panel's calls: the open file, its diffs, the
// mount and the settings.
func (a *App) registerAIHandlers(sess *link.Session) {
	sess.Handle(protocol.MethodContext, func(_ context.Context, _ []json.RawMessage) (any, error) {
		var out protocol.Context
		mygo.RunOnMain(func() { out.Messages = a.aiContextMessages() })
		return out, nil
	})
	sess.Handle(protocol.MethodAnswer, func(_ context.Context, args []json.RawMessage) (any, error) {
		var req protocol.AnswerRequest
		if len(args) == 0 || json.Unmarshal(args[0], &req) != nil {
			return protocol.AnswerResult{}, nil
		}
		var res protocol.AnswerResult
		mygo.RunOnMain(func() { res.Cards = a.aiCards(req) })
		return res, nil
	})
	sess.Handle(protocol.MethodApply, func(_ context.Context, args []json.RawMessage) (any, error) {
		var req protocol.ApplyRequest
		if len(args) == 0 || json.Unmarshal(args[0], &req) != nil {
			return protocol.ApplyResult{Err: "无法读取应用请求"}, nil
		}
		var res protocol.ApplyResult
		mygo.RunOnMain(func() { res.Err = a.aiApply(req) })
		return res, nil
	})
	sess.Handle(protocol.MethodSetMode, func(_ context.Context, args []json.RawMessage) (any, error) {
		var req protocol.ModeRequest
		if len(args) > 0 && json.Unmarshal(args[0], &req) == nil {
			mygo.RunOnMain(func() { a.SetAIMode(req.Mode) })
		}
		return nil, nil
	})
	sess.Handle(protocol.MethodSettings, func(_ context.Context, _ []json.RawMessage) (any, error) {
		mygo.RunOnMain(a.OpenAISettings)
		return nil, nil
	})
	sess.OnEvent(protocol.EventMove, func(raw json.RawMessage) {
		var req protocol.MoveRequest
		if json.Unmarshal(raw, &req) != nil {
			return
		}
		mygo.RunOnMain(func() { a.aiMove(req) })
	})
}

// aiContextMessages builds the next question's context: the contract, then
// the open file or the selection.
func (a *App) aiContextMessages() []protocol.ChatMessage {
	out := []protocol.ChatMessage{{
		Role: "system",
		Content: "你是轻码 IDE 的编程助手。用户要求改代码时，请用 git unified diff 格式回答" +
			"（放在 ```diff 代码块里），每个文件一个 diff；不要输出内联建议、不要声称已经修改文件。" +
			"改动由用户在界面上确认后应用。其余问题用简洁的中文回答。",
	}}
	e := a.Col().Current()
	if e == nil {
		return out
	}
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
	return append(out, protocol.ChatMessage{
		Role: "system", Content: "当前文件 " + rel + "：\n```\n" + text + "\n```",
	})
}

// aiCards parses an answer's diffs and reads the files, so the panel draws
// the change the editor would make. The patches stay here for apply.
func (a *App) aiCards(req protocol.AnswerRequest) []protocol.Card {
	if a.AIEdge.answers == nil {
		a.AIEdge.answers = map[string][]aiAnswerCard{}
	}
	files, err := editor.ParsePatch(req.Text)
	if err != nil || a.WS == nil {
		a.AIEdge.answers[req.RequestID] = nil
		return nil
	}
	var cards []protocol.Card
	var kept []aiAnswerCard
	for _, fp := range files {
		path := fp.NewName
		if path == "" {
			path = fp.OldName
		}
		if path == "" {
			continue
		}
		old := ""
		if d, err := a.Doc(filepath.Join(a.WS.Root, filepath.FromSlash(path))); err == nil {
			old = d.Buffer.String()
		}
		newText, err := fp.Apply(old)
		index := len(kept)
		card := protocol.Card{Index: index, Path: path, Old: old, New: newText}
		if err != nil {
			card.Err = err.Error()
		}
		kept = append(kept, aiAnswerCard{path: path, patch: fp})
		cards = append(cards, card)
	}
	a.AIEdge.answers[req.RequestID] = kept
	return cards
}

// aiApply writes one card's change into its file, against what the buffer
// holds now, and marks the file dirty for the user to save.
func (a *App) aiApply(req protocol.ApplyRequest) string {
	cards := a.AIEdge.answers[req.RequestID]
	if req.Index < 0 || req.Index >= len(cards) {
		return "找不到要应用的改动"
	}
	card := cards[req.Index]
	if a.WS == nil {
		return "没有打开的工作区"
	}
	path := filepath.Join(a.WS.Root, filepath.FromSlash(card.path))
	d, err := a.Doc(path)
	if err != nil {
		return err.Error()
	}
	newText, err := card.patch.Apply(d.Buffer.String())
	if err != nil {
		return err.Error()
	}
	d.Buffer.Set(newText)
	a.updateTitle()
	a.refreshGit()
	a.say("已应用 AI 对 " + card.path + " 的改动（保存前可撤销）")
	return ""
}

// aiMove puts a floating mount where the panel's drag asked, clamped to the
// window, and reports the new origin back.
func (a *App) aiMove(req protocol.MoveRequest) {
	w, h := float32(aiFloatW), float32(aiFloatH)
	var px, py *float32
	switch req.Target {
	case AIRail:
		w, h = aiRailW, aiRailH
		px, py = &a.Prefs.AIRailX, &a.Prefs.AIRailY
	default:
		px, py = &a.Prefs.AIFloatX, &a.Prefs.AIFloatY
	}
	*px, *py = a.clampAI(req.X, req.Y, w, h)
	savePrefs(a.Prefs)
	a.AIEdge.dirty = true
}

// aiPosition returns the top-left of the floating panel (AIFloat) or the
// rail card (AIRail), defaulting to the right edge and clamped to the window.
func (a *App) aiPosition(target int) (float32, float32) {
	w, h := float32(aiFloatW), float32(aiFloatH)
	x, y := a.Prefs.AIFloatX, a.Prefs.AIFloatY
	if target == AIRail {
		w, h = aiRailW, aiRailH
		x, y = a.Prefs.AIRailX, a.Prefs.AIRailY
	}
	thisW, thisH := a.viewW, a.viewH
	if thisW <= 0 || thisH <= 0 {
		thisW, thisH = 1280, 820
	}
	if x == 0 && y == 0 {
		x = thisW - w - 14
		if target == AIRail {
			y = (thisH - float32(theme.StatusBarH)) / 2
			y -= h / 2
		} else {
			y = float32(theme.TitleBarH) + 16
		}
	}
	x, y = a.clampAI(x, y, w, h)
	if target == AIRail {
		a.Prefs.AIRailX, a.Prefs.AIRailY = x, y
	} else {
		a.Prefs.AIFloatX, a.Prefs.AIFloatY = x, y
	}
	return x, y
}

// clampAI keeps a mount inside the window.
func (a *App) clampAI(x, y, w, h float32) (float32, float32) {
	W, H := a.viewW, a.viewH
	if W <= 0 || H <= 0 {
		W, H = 1280, 820
	}
	minX, minY := float32(8), float32(theme.TitleBarH)+8
	maxX := W - w - 8
	maxY := H - float32(theme.StatusBarH) - h - 8
	if maxX < minX {
		maxX = minX
	}
	if maxY < minY {
		maxY = minY
	}
	if x < minX {
		x = minX
	}
	if x > maxX {
		x = maxX
	}
	if y < minY {
		y = minY
	}
	if y > maxY {
		y = maxY
	}
	return x, y
}
