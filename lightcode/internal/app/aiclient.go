package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"lightcode.ai/client/protocol"
)

// aiPluginName is the panel's executable: it is its own project and its own
// build, not a subcommand of the editor.
func aiPluginName() string {
	if runtime.GOOS == "windows" {
		return "lightcode-ai.exe"
	}
	return "lightcode-ai"
}

// aiPluginCommand finds the panel: LIGHTCODE_AI_PLUGIN first, then the
// editor's own directory (where a build puts it), then the working
// directory, then PATH.
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
	return nil, fmt.Errorf("找不到 AI 面板 %s：先构建它（cd lightcode-ai && go build -o ../lightcode/%s .），或设置 LIGHTCODE_AI_PLUGIN", name, name)
}

// isFile reports whether path is an existing regular file.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// aiPanelStatus is what the capabilities view says about the panel.
type aiPanelStatus struct {
	Running bool
	Err     string
}

// aiPanelStatusOf reports the panel's state, without starting it.
func (a *App) aiPanelStatusOf() aiPanelStatus {
	e := &a.AIEdge
	if e.err != "" {
		return aiPanelStatus{Err: e.err}
	}
	if e.sess != nil && !e.sess.IsClosed() {
		return aiPanelStatus{Running: true}
	}
	return aiPanelStatus{}
}

// aiPanelText is one line about the panel, for the capabilities view.
func aiPanelText(st aiPanelStatus) string {
	switch {
	case st.Running:
		return "运行中 · " + protocol.Name + " v" + protocol.Version
	case st.Err != "":
		return "未启动：" + st.Err
	default:
		return "按需启动（首次显示面板时拉起）"
	}
}
