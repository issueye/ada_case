package app

import (
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"lightcode.ai/client/protocol"

	"lightcode/internal/theme"
)

// How the assistant shows.
const (
	AIDock  = 0
	AIFloat = 1
	AIRail  = 2
)

// aiState is the assistant's configuration in the editor. The conversation,
// the input and the panel itself live in the plugin; the editor keeps only
// what the plugin cannot own: where the panel mounts, where the endpoint
// and the key come from, and the dialog that edits them.
type aiState struct {
	Mode   int
	Config protocol.Config
	// SettingsOpen edits the endpoint and the key.
	SettingsOpen bool
	SetBase      string
	SetModel     string
	SetKey       string
}

// initAI fills the assistant's configuration: the saved endpoint and
// model, the key from the system store or the environment.
func (a *App) initAI() {
	a.AI.Config = protocol.Config{
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-4o-mini",
	}
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
	a.AIEdge.dirty = true
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

// OpenAISettings opens the endpoint and key dialog.
func (a *App) OpenAISettings() {
	a.AI.SettingsOpen = true
	a.AI.SetBase = a.AI.Config.BaseURL
	a.AI.SetModel = a.AI.Config.Model
	a.AI.SetKey = ""
}

// SaveAISettings saves the endpoint, the model and the key; the key goes
// to the system's store when it has one. The panel gets the new values at
// once.
func (a *App) SaveAISettings() {
	if base := strings.TrimSpace(a.AI.SetBase); base != "" {
		a.AI.Config.BaseURL = base
	}
	if model := strings.TrimSpace(a.AI.SetModel); model != "" {
		a.AI.Config.Model = model
	}
	if key := strings.TrimSpace(a.AI.SetKey); key != "" {
		// Keep the key for this session even when the store refuses it, so
		// the panel still works; the store is what survives a restart.
		a.AI.Config.APIKey = key
		_ = mygo.Secrets.Set("ai.key", []byte(key))
	}
	a.AI.SettingsOpen = false
	a.AIEdge.dirty = true
	a.SaveState()
}

// aiSettingsOverlay is the endpoint and key dialog. It stays in the editor
// because the key is the editor's to keep, in the system's store.
func (a *App) aiSettingsOverlay(c *ui.Context) {
	if !a.AI.SettingsOpen {
		return
	}
	ui.Modal(c, &a.AI.SettingsOpen, func() {
		ui.Column(c).Width(460).Gap(10).Padding(18).Radius(8).
			Background(theme.Panel).Border(1, theme.Border).
			Shadow(0, 14, 36, 0, ui.RGBA(0, 0, 0, 0.55)).Children(func() {
			ui.Text(c, "AI 设置").FontSize(14).Bold().TextColor(theme.Text)
			ui.Text(c, "兼容 OpenAI 的 /chat/completions 流式接口。Key 存在系统凭据库（Windows Credential Manager / macOS Keychain）；AI 面板在独立进程里，只拿得到这次请求要用的值。").
				FontSize(10).TextColor(theme.Muted).LineHeight(1.5)
			ui.Text(c, "Base URL").FontSize(10).TextColor(theme.Faint)
			ui.TextInput(c, &a.AI.SetBase).Placeholder("https://api.openai.com/v1").FontSize(12)
			ui.Text(c, "模型").FontSize(10).TextColor(theme.Faint)
			ui.TextInput(c, &a.AI.SetModel).Placeholder("gpt-4o-mini").FontSize(12)
			ui.Text(c, "API Key").FontSize(10).TextColor(theme.Faint)
			ui.TextInput(c, &a.AI.SetKey).Placeholder("sk-…（留空保留原来的）").FontSize(12)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.AI.SettingsOpen = false
				}
				if ui.Button(c, "保存").Clicked() {
					a.SaveAISettings()
				}
			})
		})
	})
}

// providerText names the endpoint the panel asks.
func providerText(base, model string) string {
	if model == "" {
		model = "未配置"
	}
	return base + " · " + model
}
