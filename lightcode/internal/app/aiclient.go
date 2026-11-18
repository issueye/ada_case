package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"lightcode.ai/client/protocol"

	"lightcode/internal/aiplugin"
)

// aiClient returns the assistant's plugin client, making it once. The
// process itself starts with the first question, not here: a session that
// never asks never spawns one.
func (a *App) aiClient() (*aiplugin.Client, error) {
	if a.AI.client != nil {
		return a.AI.client, nil
	}
	cmd, err := aiPluginCommand()
	if err != nil {
		return nil, err
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
		return aiplugin.Status{Name: protocol.Name, Version: protocol.Version}
	}
	return a.AI.client.Status()
}

// aiPluginText is one line about the plugin, for the capabilities view.
func aiPluginText(st aiplugin.Status) string {
	switch {
	case st.Running && st.PID > 0:
		version := st.Version
		if version == "" {
			version = "?"
		}
		return "进程插件 · 运行中 · " + st.Name + " v" + version + " · pid " + strconv.Itoa(st.PID)
	case st.Err != "":
		return "进程插件 · 上次退出：" + st.Err
	default:
		return "进程插件 · 按需启动（首次提问时拉起独立进程）"
	}
}

// aiPluginName is the plugin's executable, which is its own project and
// its own build, not a subcommand of the editor.
func aiPluginName() string {
	if runtime.GOOS == "windows" {
		return "lightcode-ai.exe"
	}
	return "lightcode-ai"
}

// aiPluginCommand finds the plugin: LIGHTCODE_AI_PLUGIN first, then the
// editor's own directory (where a build puts it), then the working
// directory, then PATH. Since the plugin is a separate project, a missing
// binary is an error with the command that builds it, not a silent
// fallback into the editor.
func aiPluginCommand() ([]string, error) {
	if v := strings.TrimSpace(os.Getenv("LIGHTCODE_AI_PLUGIN")); v != "" {
		return []string{v}, nil
	}
	name := aiPluginName()
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), name)
		if isFile(cand) {
			return []string{cand}, nil
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		cand := filepath.Join(cwd, name)
		if isFile(cand) {
			return []string{cand}, nil
		}
	}
	if found, err := exec.LookPath(name); err == nil {
		return []string{found}, nil
	}
	return nil, fmt.Errorf("找不到 AI 插件 %s：先构建它（cd lightcode-ai && go build -o ../lightcode/%s .），或设置 LIGHTCODE_AI_PLUGIN", name, name)
}

// isFile reports whether path is an existing regular file.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
