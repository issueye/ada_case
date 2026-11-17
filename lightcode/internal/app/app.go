// Package app is the window of 轻码 IDE: the chrome (title bar, activity
// bar, sidebar, tabs, status bar), the editors it shows, the command
// palette, and the AI panel.
//
// The view is a function of App's state; commands change that state and
// MyGo builds the next frame.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"lightcode/internal/command"
	"lightcode/internal/document"
	"lightcode/internal/git"
	"lightcode/internal/settings"
	"lightcode/internal/symbols"
	"lightcode/internal/theme"
	"lightcode/internal/workspace"

	"github.com/egoist/mygo/plugins/terminal"
)

// PaletteMode is what the palette lists.
type PaletteMode int

// Palette modes.
const (
	PaletteFiles PaletteMode = iota
	PaletteCommands
)

// App is the window's state.
type App struct {
	Win *mygo.Window
	WS  *workspace.Workspace
	// Cols are the editor columns; a split shows two, and ActiveCol is
	// the one commands act on.
	Cols      []*document.Column
	ActiveCol int
	// SplitPercent is the share of the editor area the left column of a split takes.
	SplitPercent float32
	Docs         map[string]*document.Document
	Cmds         *command.Registry

	// SidebarView is the activity the sidebar shows: 0 files, 1 search,
	// 2 source control, 3 run, 4 extensions.
	SidebarView int
	SidebarOpen bool
	// AI is the assistant: its mode (docked, floating or the rail), its
	// conversation, its stream and its configuration.
	AI aiState

	PaletteOpen  bool
	PaletteMode  PaletteMode
	PaletteQuery string
	PaletteSel   int

	// Message shows in the status bar, as the outcome of a command.
	Message string
	// CloseDoc is the document whose tab asks whether to save first.
	CloseDoc *document.Document
	// ClosePrompt shows the save-before-close dialog.
	ClosePrompt bool
	// Branch is the workspace's git branch, "" without a repository.
	Branch string
	// Search is the sidebar's search panel.
	Search searchState
	// Selected is the explorer's chosen entry.
	Selected string
	// FileOp is the new/rename/delete dialog.
	FileOp fileOp
	// watchCancel stops the file watch, when one started.
	watchCancel context.CancelFunc
	// Repo is the workspace's Git repository, nil outside one; Git is its
	// last status, GitMsg the commit message, GitErr git's last error, and
	// GitLog its recent commits. Diff is the open diff page.
	Repo   *git.Repo
	Git    *git.Status
	GitMsg string
	GitErr string
	GitLog []git.LogEntry
	Diff   *diffTab
	// Term is the bottom terminal, its error, whether the pane shows,
	// and the share of the height it takes.
	Term        *terminal.Terminal
	TermErr     string
	TermOpen    bool
	TermPercent float32
	// SettingsOpen shows the settings page in the active column.
	SettingsOpen bool
	// viewW and viewH are the window's content size, for dragging overlays.
	viewW, viewH float32
	// aiDrag is a pointer drag of the floating assistant.
	aiDrag aiDragState
	// tabMenu is the tab menu a right click opened.
	tabMenu tabMenuState
	// Prefs are the app's preferences, from settings.json.
	Prefs preferences
	// Capability toggles in the extensions view.
	Capabilities capabilityState
	// Preferences are loaded once, before the window.
	PrefsLoaded bool
	gitTick     int
	DiscardPath string
	DiscardOpen bool
	// OutlineOpen shows the outline under the explorer's tree, its
	// percent the room the tree keeps.
	OutlineOpen    bool
	OutlinePercent float32
	outline        struct {
		path    string
		version uint64
		syms    []symbols.Symbol
	}
	// StatePath is where the session is kept; Zoom is the content's zoom.
	StatePath string
	Zoom      float32

	// restored reports that the last session brought files back.
	restored bool

	// initialFile opens once the workspace is ready.
	initialFile string
}

// New makes the app; root is the folder to open, or "" for the working
// directory. file, when set, is opened after the folder.
func New(root, file string) *App {
	a := &App{
		Docs:           map[string]*document.Document{},
		Cmds:           command.NewRegistry(),
		SidebarOpen:    true,
		StatePath:      settings.Path(),
		Zoom:           1,
		SplitPercent:   50,
		OutlineOpen:    true,
		OutlinePercent: 58,
		TermPercent:    28,
	}
	a.resetColumns()
	a.initialFile = file
	if root == "" {
		root, _ = os.Getwd()
	}
	if info, err := os.Stat(root); err == nil && !info.IsDir() {
		dir := filepath.Dir(root)
		if a.initialFile == "" {
			a.initialFile = root
		}
		root = dir
	}
	if ws, err := workspace.Open(root); err == nil {
		a.WS = ws
		a.Branch = gitBranch(root)
	}
	a.registerCommands()
	a.attachRepo()
	a.Prefs = loadPrefs()
	theme.Use(a.Prefs.Theme == "light")
	a.restoreState()
	a.applyPrefs()
	a.openInitial()
	return a
}

// registerCommands fills the command table: menus, the palette and the
// keymap all come from it.
func (a *App) registerCommands() {
	add := func(id, title, category, key string, run func()) {
		a.Cmds.Add(&command.Command{ID: id, Title: title, Category: category, Key: key, Run: run})
	}
	add("file.openFolder", "打开文件夹…", "文件", "CmdOrCtrl+O", a.OpenFolderCommand)
	add("file.save", "保存", "文件", "CmdOrCtrl+S", a.SaveActive)
	add("file.saveAll", "全部保存", "文件", "", a.SaveAll)
	add("file.close", "关闭标签", "文件", "CmdOrCtrl+W", a.CloseActive)
	add("file.reload", "重新加载", "文件", "", a.ReloadActive)
	add("file.newFile", "新建文件", "文件", "CmdOrCtrl+N", func() { a.BeginNewFile(a.selectedDir()) })
	add("file.newFolder", "新建文件夹", "文件", "", func() { a.BeginNewFolder(a.selectedDir()) })
	add("file.rename", "重命名", "文件", "F2", func() {
		if a.Selected == "" {
			a.say("先在资源管理器里选中文件或文件夹")
			return
		}
		a.BeginRename(a.Selected)
	})
	add("file.delete", "删除", "文件", "", func() {
		if a.Selected == "" {
			a.say("先在资源管理器里选中文件或文件夹")
			return
		}
		a.BeginDelete(a.Selected)
	})
	add("view.explorer", "资源管理器", "视图", "CmdOrCtrl+Shift+E", func() { a.ShowSidebar(0) })
	add("view.search", "搜索", "视图", "CmdOrCtrl+Shift+F", func() { a.ShowSidebar(1) })
	add("view.scm", "源代码管理", "视图", "CmdOrCtrl+Shift+G", func() { a.ShowSidebar(2) })
	add("view.extensions", "扩展", "视图", "CmdOrCtrl+Shift+X", func() { a.ShowSidebar(4) })
	add("view.toggleSidebar", "切换侧栏", "视图", "CmdOrCtrl+B", a.ToggleSidebar)
	add("view.toggleAI", "切换 AI 面板", "视图", "", a.ToggleAI)
	add("view.zoomIn", "放大", "视图", "CmdOrCtrl+=", func() { a.SetZoom(a.Zoom + 0.1) })
	add("view.zoomOut", "缩小", "视图", "CmdOrCtrl+-", func() { a.SetZoom(a.Zoom - 0.1) })
	add("view.zoomReset", "重置缩放", "视图", "CmdOrCtrl+0", func() { a.SetZoom(1) })
	add("view.split", "拆分 / 关闭编辑器", "视图", "CmdOrCtrl+\\", a.ToggleSplit)
	add("terminal.toggle", "终端", "视图", "CmdOrCtrl+`", a.toggleTerminal)
	add("view.settings", "设置", "文件", "", func() { a.SettingsOpen = true })
	add("view.toggleTheme", "切换明暗主题", "视图", "CmdOrCtrl+Shift+T", a.ToggleTheme)
	add("palette.files", "快速打开…", "转到", "CmdOrCtrl+P", func() { a.OpenPalette(PaletteFiles) })
	add("palette.commands", "命令面板…", "视图", "CmdOrCtrl+Shift+P", func() { a.OpenPalette(PaletteCommands) })
	add("git.refresh", "刷新源代码管理", "Git", "", a.refreshGit)
	add("git.commit", "提交", "Git", "", a.commit)
	add("workspace.refresh", "刷新资源管理器", "文件", "", a.RefreshWorkspace)
	add("help.shortcuts", "键盘快捷键", "帮助", "", func() { a.say("Ctrl+P 快速打开 · Ctrl+Shift+P 命令面板 · Ctrl+B 侧栏 · Ctrl+S 保存") })
}

// RunCommand runs a command by ID.
func (a *App) RunCommand(id string) {
	if !a.Cmds.Run(id) {
		a.say("未知命令：" + id)
	}
}

// RunKey runs the command bound to an input event, for the editor's
// OnInput: the editor would take Ctrl+P and Ctrl+W itself, and the app's
// keymap wins over it, as terminals win over both.
func (a *App) RunKey(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputKeyDown {
		return false
	}
	for _, cmd := range a.Cmds.All() {
		if cmd.Key == "" {
			continue
		}
		if ch, ok := command.ParseChord(cmd.Key); ok && ch.Mods == ev.Mods && ch.Key == ev.Key {
			a.RunCommand(cmd.ID)
			return true
		}
	}
	return false
}

func (a *App) say(msg string) { a.Message = msg }

// resetColumns closes every editor column and shows one empty one, as
// opening another folder does.
func (a *App) resetColumns() {
	a.Cols = []*document.Column{{}}
	a.ActiveCol = 0
}

// Col returns the column commands act on.
func (a *App) Col() *document.Column {
	if a.ActiveCol < 0 || a.ActiveCol >= len(a.Cols) {
		a.ActiveCol = 0
	}
	if len(a.Cols) == 0 {
		a.resetColumns()
	}
	return a.Cols[a.ActiveCol]
}

// updateTitle gives the window its title and edited mark, after a tab
// opened, closed or saved.
func (a *App) updateTitle() {
	if a.Win == nil {
		return
	}
	a.Win.SetTitle(a.WindowTitle())
	dirty := false
	if e := a.Col().Current(); e != nil {
		dirty = e.Doc.Dirty()
	}
	a.Win.SetDocumentEdited(dirty)
}

// OpenPalette opens the palette in mode, focused on its query.
func (a *App) OpenPalette(mode PaletteMode) {
	a.PaletteOpen = true
	a.PaletteMode = mode
	a.PaletteQuery = ""
	a.PaletteSel = 0
}

// ToggleSidebar shows or hides the sidebar.
func (a *App) ToggleSidebar() {
	a.SidebarOpen = !a.SidebarOpen
	a.SaveState()
}

// ShowSidebar shows the sidebar on view.
func (a *App) ShowSidebar(view int) {
	if a.SidebarView == view && a.SidebarOpen {
		a.SidebarOpen = false
		a.SaveState()
		return
	}
	a.SidebarView = view
	a.SidebarOpen = true
	a.SaveState()
}

// OpenFolderCommand asks for a folder in the background and opens it.
func (a *App) OpenFolderCommand() {
	win := a.Win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:    win,
			Title:     "打开文件夹",
			Directory: true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		win.Update(func() { a.setWorkspace(paths[0]) })
	}()
}

func (a *App) setWorkspace(root string) {
	ws, err := workspace.Open(root)
	if err != nil {
		a.say("无法打开 " + root + "：" + err.Error())
		return
	}
	a.WS = ws
	a.Branch = gitBranch(root)
	a.resetColumns()
	a.Docs = map[string]*document.Document{}
	a.attachRepo()
	a.say("已打开 " + ws.Name())
	a.SaveState()
}

// RefreshWorkspace rereads the tree, keeping open folders.
func (a *App) RefreshWorkspace() {
	if a.WS == nil {
		return
	}
	if err := a.WS.Reload(); err != nil {
		a.say("刷新失败：" + err.Error())
		return
	}
	a.say("资源管理器已刷新")
}

// Doc returns the open document for path, reading it the first time.
func (a *App) Doc(path string) (*document.Document, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if d, ok := a.Docs[abs]; ok {
		// The file may have changed in a terminal or another editor
		// since it was read: reload it while it is unchanged here, and
		// warn instead of silently overwriting edits.
		if d.ChangedOnDisk() {
			if d.Dirty() {
				a.say(d.Name() + " 已在磁盘上改变；保存会覆盖磁盘上的版本")
			} else if err := d.Reload(); err == nil {
				a.say("已从磁盘重新加载 " + d.Name())
			}
		}
		return d, nil
	}
	d, err := document.Open(abs)
	if err != nil {
		return nil, err
	}
	a.Docs[abs] = d
	return d, nil
}

// OpenFileReply opens a file relative to the workspace root.
func (a *App) openRel(rel string) {
	if a.WS == nil {
		return
	}
	a.OpenFile(filepath.Join(a.WS.Root, filepath.FromSlash(rel)))
}

// SaveActive saves the chosen editor's document.
func (a *App) SaveActive() {
	e := a.Col().Current()
	if e == nil {
		a.say("没有打开的文件")
		return
	}
	if e.Doc.ReadOnly() {
		a.say(e.Doc.Name() + " 是只读文件")
		return
	}
	if err := e.Doc.Save(); err != nil {
		a.say("保存失败：" + err.Error())
		return
	}
	a.say("已保存 " + e.Doc.Name())
	a.updateTitle()
	a.refreshGit()
}

// SaveAll saves every dirty document.
func (a *App) SaveAll() {
	n := 0
	for _, d := range a.Docs {
		if d.Dirty() {
			if err := d.Save(); err == nil {
				n++
			}
		}
	}
	a.say(fmt.Sprintf("已保存 %d 个文件", n))
}

// ReloadActive reads the chosen file again, losing its changes.
func (a *App) ReloadActive() {
	e := a.Col().Current()
	if e == nil {
		return
	}
	if e.Doc.Dirty() {
		a.say("文件有未保存的修改，未重新加载")
		return
	}
	if err := e.Doc.Reload(); err != nil {
		a.say("重新加载失败：" + err.Error())
		return
	}
	a.say("已重新加载 " + e.Doc.Name())
	a.updateTitle()
}

// CloseActive closes the chosen tab, asking to save first when dirty.
func (a *App) CloseActive() {
	e := a.Col().Current()
	if e == nil {
		return
	}
	a.closeTabIn(a.ActiveCol, e.Doc)
}

// saveClose saves the document that asked to close, then closes it.
func (a *App) saveClose() {
	d := a.CloseDoc
	a.CloseDoc = nil
	if d == nil {
		return
	}
	if err := d.Save(); err != nil {
		a.say("保存失败：" + err.Error())
		return
	}
	a.removeDoc(d)
	a.say("已保存并关闭 " + d.Name())
	a.updateTitle()
	a.SaveState()
}

// discardClose closes without saving.
func (a *App) discardClose() {
	if a.CloseDoc == nil {
		return
	}
	d := a.CloseDoc
	a.CloseDoc = nil
	a.removeDoc(d)
	a.updateTitle()
	a.SaveState()
}

// removeDoc closes every tab showing d, as the save prompt does.
func (a *App) removeDoc(d *document.Document) {
	for _, col := range a.Cols {
		for i, e := range col.Editors {
			if e.Doc == d {
				col.Remove(i)
				break
			}
		}
	}
	a.collapseEmptyColumns()
	a.pruneDocs()
}

// openInitial opens the file the app was asked to show at startup, or the
// workspace's README.
func (a *App) openInitial() {
	if a.initialFile != "" {
		path := a.initialFile
		if !filepath.IsAbs(path) && a.WS != nil {
			path = filepath.Join(a.WS.Root, path)
		}
		a.OpenFile(path)
		return
	}
	if a.restored {
		return
	}
	if a.WS != nil {
		for _, name := range []string{"README.md", "readme.md", "go.mod"} {
			p := filepath.Join(a.WS.Root, name)
			if _, err := os.Stat(p); err == nil {
				a.OpenFile(p)
				return
			}
		}
	}
}

// Title is the window title: the active file and the folder.
func (a *App) Title() string {
	name := ""
	if e := a.Col().Current(); e != nil {
		name = e.Doc.Name()
	}
	if a.WS != nil {
		if name != "" {
			return name + " — " + a.WS.Name() + " — 轻码 IDE"
		}
		return a.WS.Name() + " — 轻码 IDE"
	}
	return "轻码 IDE"
}
