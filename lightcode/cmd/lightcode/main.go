// Command lightcode runs 轻码 IDE: a light code editor in Go, drawn by
// MyGo's native UI.
//
//	lightcode [folder] [file]
package main

import (
	"log"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"lightcode/internal/app"
	"lightcode/internal/theme"
)

func main() {
	var root, file string
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if len(os.Args) > 2 {
		file = os.Args[2]
	}
	mygo.App.WhenReady(func() {
		mygo.Theme.SetSource(mygo.ThemeDark)
		a := app.New(root, file)
		mygo.App.SetMenu(newMenu(a))
		a.Win = mygo.NewWindow(mygo.WindowOptions{
			Title:             "轻码 IDE",
			Width:             theme.WindowW,
			Height:            theme.WindowH,
			MinWidth:          theme.WindowMinW,
			MinHeight:         theme.WindowMinH,
			TitleBarStyle:     mygo.TitleBarHidden,
			HideWindowButtons: true,
			BackgroundColor:   "#0f1216",
			StateKey:          "lightcode.main",
			Content:           ui.View(a.View),
		})
		a.Win.SetTitle(a.WindowTitle())
		a.Win.OnClosed(a.CloseTerminal)
		a.Start()
		if demo := os.Getenv("LIGHTCODE_DEMO"); demo != "" {
			switch demo {
			case "terminal":
				a.RunCommand("terminal.toggle")
			case "settings":
				a.RunCommand("view.settings")
			case "capabilities":
				a.ShowSidebar(4)
			}
		}
		// LIGHTCODE_CAPTURE makes the app save a PNG of the window and
		// quit, as the screenshot regression of docs/dev-plan.md §6 does.
		if path := os.Getenv("LIGHTCODE_CAPTURE"); path != "" {
			win := a.Win
			go func() {
				time.Sleep(captureDelay())
				win.Update(func() {
					if png, err := win.CapturePage(); err == nil {
						os.WriteFile(path, png, 0o644)
					}
					win.Close()
				})
			}()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func captureDelay() time.Duration {
	if s := os.Getenv("LIGHTCODE_CAPTURE_DELAY"); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			return d
		}
	}
	return 1200 * time.Millisecond
}

// newMenu is the app's menu bar: the roles of a desktop app plus the
// commands the app adds.
func newMenu(a *app.App) *mygo.Menu {
	run := func(id string) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) { a.RunCommand(id) }
	}
	file := []*mygo.MenuItem{
		{Label: "新建文件", Accelerator: "CmdOrCtrl+N", Click: run("file.newFile")},
		{Label: "新建文件夹", Click: run("file.newFolder")},
		{Label: "打开文件夹…", Accelerator: "CmdOrCtrl+O", Click: run("file.openFolder")},
		mygo.Separator(),
		{Label: "保存", Accelerator: "CmdOrCtrl+S", Click: run("file.save")},
		{Label: "全部保存", Click: run("file.saveAll")},
		mygo.Separator(),
		{Label: "关闭标签", Accelerator: "CmdOrCtrl+W", Click: run("file.close")},
		{Role: mygo.RoleQuit},
	}
	view := []*mygo.MenuItem{
		{Label: "命令面板…", Accelerator: "CmdOrCtrl+Shift+P", Click: run("palette.commands")},
		{Label: "快速打开…", Accelerator: "CmdOrCtrl+P", Click: run("palette.files")},
		mygo.Separator(),
		{Label: "切换侧栏", Accelerator: "CmdOrCtrl+B", Click: run("view.toggleSidebar")},
		{Label: "拆分 / 关闭编辑器", Accelerator: "CmdOrCtrl+\\", Click: run("view.split")},
		{Label: "终端", Accelerator: "CmdOrCtrl+`", Click: run("terminal.toggle")},
		{Label: "设置", Click: run("view.settings")},
		{Label: "切换 AI 面板", Click: run("view.toggleAI")},
		mygo.Separator(),
		{Label: "放大", Accelerator: "CmdOrCtrl+=", Click: run("view.zoomIn")},
		{Label: "缩小", Accelerator: "CmdOrCtrl+-", Click: run("view.zoomOut")},
		{Label: "重置缩放", Accelerator: "CmdOrCtrl+0", Click: run("view.zoomReset")},
	}
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "文件", Submenu: file},
		{Role: mygo.RoleEditMenu},
		{Label: "视图", Submenu: view},
		{Role: mygo.RoleWindowMenu},
	})
}
