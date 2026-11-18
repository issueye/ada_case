package app

import (
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"lightcode/internal/aiplugin"
)

// TestAIPluginCommand checks the default: the app spawns itself with the
// plugin subcommand, so one binary carries the editor and its assistant.
func TestAIPluginCommand(t *testing.T) {
	t.Setenv("LIGHTCODE_AI_PLUGIN", "")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := aiPluginCommand()
	if len(cmd) != 2 || cmd[0] != exe || cmd[1] != "ai-provider" {
		t.Fatalf("command = %v, want [%s ai-provider]", cmd, exe)
	}
}

// TestAIPluginCommandSeparate points the app at a plugin built on its own.
func TestAIPluginCommandSeparate(t *testing.T) {
	t.Setenv("LIGHTCODE_AI_PLUGIN", `C:\tools\lightcode-ai.exe`)
	cmd := aiPluginCommand()
	if len(cmd) != 1 || cmd[0] != `C:\tools\lightcode-ai.exe` {
		t.Fatalf("command = %v", cmd)
	}
}

// TestAIPluginText says what the plugin is doing, including why it last
// stopped: the capabilities view shows this line.
func TestAIPluginText(t *testing.T) {
	for _, tc := range []struct {
		st   aiplugin.Status
		want string
	}{
		{aiplugin.Status{Running: true, PID: 4242}, "进程插件 · 运行中 · pid 4242"},
		{aiplugin.Status{Err: "exit status 1"}, "进程插件 · 上次退出：exit status 1"},
		{aiplugin.Status{}, "进程插件 · 按需启动（首次提问时拉起）"},
	} {
		if got := aiPluginText(tc.st); got != tc.want {
			t.Errorf("text = %q, want %q", got, tc.want)
		}
	}
}

// TestAIPluginRow shows the plugin's row in the capabilities view before
// anything was asked: no process runs until a question needs one.
func TestAIPluginRow(t *testing.T) {
	a, _ := testWorkspace(t)
	a.ShowSidebar(4)
	tt := ui.NewTester(a.View, 1280, 820)
	if !tt.HasText("AI 进程插件") {
		t.Fatalf("no plugin row; texts %q", tt.Texts())
	}
	if !tt.HasText("进程插件 · 按需启动（首次提问时拉起）") {
		t.Errorf("idle plugin text missing; texts %q", tt.Texts())
	}
	if a.AI.client != nil {
		t.Errorf("the capabilities view started a plugin process")
	}
}

// TestAIPluginClose keeps closing safe before anything ran.
func TestAIPluginClose(t *testing.T) {
	a, _ := testWorkspace(t)
	a.CloseAI()
	a.RestartAI()
	if a.AI.client != nil {
		t.Errorf("closing left a client")
	}
	if st := a.aiPluginStatus(); st.Running || st.PID != 0 {
		t.Errorf("status after close = %+v", st)
	}
}

// TestAIPluginStatusWithoutClient reports the plugin's identity before it
// ever ran.
func TestAIPluginStatusWithoutClient(t *testing.T) {
	a, _ := testWorkspace(t)
	st := a.aiPluginStatus()
	if st.Name != aiplugin.Name || !strings.Contains(aiPluginText(st), "按需启动") {
		t.Fatalf("status = %+v, text = %q", st, aiPluginText(st))
	}
}
