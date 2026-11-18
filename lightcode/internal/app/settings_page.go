package app

import (
	"fmt"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"lightcode/internal/icons"
	"lightcode/internal/theme"
)

// applyPrefs puts the editor and terminal preferences on the open views.
func (a *App) applyPrefs() {
	for _, col := range a.Cols {
		for _, e := range col.Editors {
			e.State.Font.Size = a.Prefs.EditorFontSize
			e.State.SoftWrap = a.Prefs.SoftWrap
		}
	}
	if a.Term != nil {
		a.Term.SetFont(terminal.Font{Family: theme.MonoFont, Size: a.Prefs.TerminalFontSize})
	}
}

// settingsPage is the settings view, in place of the editor.
func (a *App) settingsPage(c *ui.Context) {
	ui.Column(c).FillHeight().Background(theme.Background).Children(func() {
		ui.Row(c).Height(36).PaddingX(12).Gap(8).AlignItems(ui.Center).
			Background(theme.Panel).BorderWidth(0, 0, 1, 0).BorderColor(theme.BorderSoft).Children(func() {
			ui.Icon(c, icons.Settings).FontSize(14).TextColor(theme.Muted)
			ui.Text(c, "设置").FontSize(12).Bold().TextColor(theme.Text)
			ui.Text(c, prefsPath()).FontSize(9).TextColor(theme.Faint).SingleLine().Grow(1)
			iconButton(c, icons.Close, "关闭设置", 22, func() { a.SettingsOpen = false })
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(16).Gap(10).Children(func() {
				a.settingsSection(c, "外观")
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "主题").FontSize(11).TextColor(theme.Muted).Width(120)
					dark := ui.Button(c, "深色")
					if !theme.IsLight() {
						dark.Background(theme.AccentSoft)
					}
					if dark.Clicked() {
						a.UseTheme(false)
					}
					light := ui.Button(c, "浅色")
					if theme.IsLight() {
						light.Background(theme.AccentSoft)
					}
					if light.Clicked() {
						a.UseTheme(true)
					}
				})
				a.settingsSection(c, "编辑器")
				a.settingsFontRow(c, "字号", &a.Prefs.EditorFontSize)
				wrap := a.Prefs.SoftWrap
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "软换行").FontSize(11).TextColor(theme.Muted).Width(120)
					ui.Switch(c, &wrap).Label("软换行")
					if wrap != a.Prefs.SoftWrap {
						a.Prefs.SoftWrap = wrap
						a.applyPrefs()
						savePrefs(a.Prefs)
					}
				})
				a.settingsSection(c, "终端")
				a.settingsFontRow(c, "字号", &a.Prefs.TerminalFontSize)
				term := a.Prefs.TerminalEnabled
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "启用终端").FontSize(11).TextColor(theme.Muted).Width(120)
					ui.Switch(c, &term).Label("启用终端")
					if term != a.Prefs.TerminalEnabled {
						a.Prefs.TerminalEnabled = term
						if !term {
							a.TermOpen = false
						}
						savePrefs(a.Prefs)
					}
				})
				a.settingsSection(c, "AI")
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "接口与 Key").FontSize(11).TextColor(theme.Muted).Width(120)
					if ui.Button(c, "打开 AI 设置").Clicked() {
						a.OpenAISettings()
					}
				})
				a.settingsSection(c, "会话")
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "状态文件").FontSize(11).TextColor(theme.Muted).Width(120)
					ui.Text(c, a.StatePath).FontSize(10).TextColor(theme.Faint).SingleLine()
				})
			})
		})
	})
}

// settingsFontRow is a size with minus and plus buttons.
func (a *App) settingsFontRow(c *ui.Context, label string, value *float32) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(theme.Muted).Width(120)
		if ui.Button(c, "−").Clicked() && *value > 8 {
			*value -= 1
			a.appliedPrefs()
		}
		ui.Text(c, fmt.Sprintf("%d px", int(*value))).FontSize(11).TextColor(theme.Text).Width(48)
		if ui.Button(c, "+").Clicked() && *value < 40 {
			*value += 1
			a.appliedPrefs()
		}
	})
}

// appliedPrefs applies and saves preferences after a change.
func (a *App) appliedPrefs() {
	a.applyPrefs()
	savePrefs(a.Prefs)
}

func (a *App) settingsSection(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(11).Bold().TextColor(theme.Text).PaddingY(6)
}

// capabilitiesPanel is the extensions view: the toggles and the provider
// list, read-only where the app has no extension runtime.
func (a *App) capabilitiesPanel(c *ui.Context) {
	ui.Column(c).Grow(1).Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(12).Gap(10).Children(func() {
				ui.Text(c, "能力").FontSize(12).Bold().TextColor(theme.Text)
				ui.Text(c, "这个版本没有可安装的扩展；这里是已编译进来的能力与它们的 provider。").
					FontSize(10).TextColor(theme.Muted).LineHeight(1.5)
				a.capabilityRow(c, "源代码管理", "Git CLI", a.Repo != nil)
				ai := a.Prefs.AIEnabled
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).Children(func() {
						ui.Text(c, "AI 助手").FontSize(11).TextColor(theme.Text)
						ui.Text(c, providerText(a.AI.Config.BaseURL, a.AI.Config.Model)).FontSize(9).TextColor(theme.Faint).SingleLine()
					})
					ui.Switch(c, &ai).Label("启用 AI 助手")
					if ai != a.Prefs.AIEnabled {
						a.Prefs.AIEnabled = ai
						if !ai && a.AI.Mode == AIDock {
							a.AI.Mode = AIRail
						}
						savePrefs(a.Prefs)
					}
				})
				a.aiPluginRow(c)
				term := a.Prefs.TerminalEnabled
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).Children(func() {
						ui.Text(c, "终端").FontSize(11).TextColor(theme.Text)
						status := "libghostty-vt"
						if a.TermErr != "" {
							status = "不可用：" + a.TermErr
						}
						ui.Text(c, status).FontSize(9).TextColor(theme.Faint).SingleLine()
					})
					ui.Switch(c, &term).Label("启用终端")
					if term != a.Prefs.TerminalEnabled {
						a.Prefs.TerminalEnabled = term
						if !term {
							a.TermOpen = false
						}
						savePrefs(a.Prefs)
					}
				})
				a.capabilityRow(c, "搜索", "Go 内置实现（有 ripgrep 时优先）", true)
				a.capabilityRow(c, "预览器", "ui/markdown", true)
				ui.Box(c).Height(6)
				ui.Text(c, "内置扩展（构建期）").FontSize(10).Bold().TextColor(theme.Muted)
				for _, p := range []string{"ui/editor · 编辑器", "editor.DiffView · 差异", "ui/markdown · Markdown 预览", "plugins/terminal · 终端"} {
					ui.Text(c, p).FontSize(10).TextColor(theme.Faint)
				}
			})
		})
	})
}

// aiPluginRow shows the assistant's process plugin: whether it runs, and
// the button that replaces it.
func (a *App) aiPluginRow(c *ui.Context) {
	st := a.aiPanelStatusOf()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Children(func() {
			ui.Text(c, "AI 进程插件").FontSize(11).TextColor(theme.Text)
			ui.Text(c, aiPanelText(st)).FontSize(9).TextColor(theme.Faint).SingleLine()
		})
		if st.Running {
			if ui.Button(c, "重启").Height(20).Padding(0, 6).Radius(3).Label("重启插件").Clicked() {
				a.RestartAI()
			}
		}
	})
}

func (a *App) capabilityRow(c *ui.Context, name, provider string, on bool) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Children(func() {
			ui.Text(c, name).FontSize(11).TextColor(theme.Text)
			ui.Text(c, provider).FontSize(9).TextColor(theme.Faint).SingleLine()
		})
		label, color := "可用", theme.Success
		if !on {
			label, color = "不可用", theme.Faint
		}
		ui.Text(c, label).FontSize(9).TextColor(color)
	})
}
