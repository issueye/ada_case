package app

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"lightcode/internal/aiplugin"
)

// aiClient returns the assistant's plugin client, making it once. The
// process itself starts with the first question, not here: a session that
// never asks never spawns one.
func (a *App) aiClient() (*aiplugin.Client, error) {
	if a.AI.client != nil {
		return a.AI.client, nil
	}
	cmd := aiPluginCommand()
	if len(cmd) == 0 {
		return nil, errors.New("找不到 AI 插件进程：把 LIGHTCODE_AI_PLUGIN 指向插件可执行文件")
	}
	c := aiplugin.New(aiplugin.Options{Command: cmd})
	a.AI.client = c
	return c, nil
}

// CloseAI stops the plugin process, as the window closes.
func (a *App) CloseAI() {
	if a.AI.client == nil {
		return
	}
	_ = a.AI.client.Close()
	a.AI.client = nil
}

// RestartAI replaces the plugin process: the next question starts a new
// one. It is what a stuck plugin needs, without restarting the editor.
func (a *App) RestartAI() {
	if a.AI.client == nil {
		return
	}
	a.AI.client.Restart()
}

// aiPluginStatus reports the plugin's state for the capabilities view,
// without starting it.
func (a *App) aiPluginStatus() aiplugin.Status {
	if a.AI.client == nil {
		return aiplugin.Status{Name: aiplugin.Name, Version: aiplugin.Version}
	}
	return a.AI.client.Status()
}

// aiPluginText is one line about the plugin, for the capabilities view.
func aiPluginText(st aiplugin.Status) string {
	switch {
	case st.Running && st.PID > 0:
		return "进程插件 · 运行中 · pid " + strconv.Itoa(st.PID)
	case st.Err != "":
		return "进程插件 · 上次退出：" + st.Err
	default:
		return "进程插件 · 按需启动（首次提问时拉起）"
	}
}

// aiPluginCommand is how the plugin process is reached: the app spawns
// itself with a subcommand, so a build carries its plugin; LIGHTCODE_AI_PLUGIN
// points at a separate executable instead, for a plugin built on its own.
func aiPluginCommand() []string {
	if v := strings.TrimSpace(os.Getenv("LIGHTCODE_AI_PLUGIN")); v != "" {
		return []string{v}
	}
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	return []string{exe, "ai-provider"}
}
