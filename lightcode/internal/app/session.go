package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"lightcode/internal/document"
	"lightcode/internal/settings"
	"lightcode/internal/theme"
)

// restoreState reads the state of the last run and applies it to the open
// workspace: the files open, the tab chosen, the sidebar, the AI panel
// and the zoom.
func (a *App) restoreState() {
	st, err := settings.Load(a.StatePath)
	if err != nil {
		a.say("状态文件无法读取，已重置：" + err.Error())
	}
	a.Zoom = st.Zoom
	a.SidebarOpen = st.SidebarOpen
	a.SidebarView = st.SidebarView
	if st.AIMode >= AIDock && st.AIMode <= AIRail {
		a.AI.Mode = st.AIMode
	}
	a.AI.SetBase = st.AIBaseURL
	a.AI.SetModel = st.AIModel
	a.initAI()
	if a.WS == nil || !samePath(st.Workspace, a.WS.Root) {
		return
	}
	for _, rel := range st.Files {
		path := filepath.Join(a.WS.Root, filepath.FromSlash(rel))
		d, err := a.Doc(path)
		if err != nil {
			continue
		}
		a.Cols[0].Open(d)
		a.restored = true
	}
	if len(st.SplitFiles) > 0 {
		a.Cols = append(a.Cols, &document.Column{})
		for _, rel := range st.SplitFiles {
			path := filepath.Join(a.WS.Root, filepath.FromSlash(rel))
			d, err := a.Doc(path)
			if err != nil {
				continue
			}
			a.Cols[1].Open(d)
			a.restored = true
		}
		if st.SplitPercent > 0 && st.SplitPercent < 100 {
			a.SplitPercent = st.SplitPercent
		}
		if st.ActiveCol == 1 {
			a.ActiveCol = 1
		}
	}
	if a.Cols[0].Active >= 0 && a.Cols[0].Active < len(a.Cols[0].Editors) {
		a.Cols[0].Active = st.Active
	}
}

// SaveState writes the session, so the next run starts where this one is.
func (a *App) SaveState() {
	if a.StatePath == "" {
		return
	}
	st := settings.Default()
	st.SidebarOpen = a.SidebarOpen
	st.SidebarView = a.SidebarView
	st.AIMode = a.AI.Mode
	st.AIBaseURL = a.AI.Config.BaseURL
	st.AIModel = a.AI.Config.Model
	st.Zoom = a.Zoom
	st.Active = a.Cols[0].Active
	st.ActiveCol = a.ActiveCol
	st.SplitPercent = a.SplitPercent
	if a.WS != nil {
		st.Workspace = a.WS.Root
		st.Files = a.columnFiles(a.Cols[0])
		if len(a.Cols) > 1 {
			st.SplitFiles = a.columnFiles(a.Cols[1])
		}
	}
	if err := settings.Save(a.StatePath, st); err != nil {
		a.say("无法保存会话：" + err.Error())
	}
}

// Start begins the session: it saves the state and watches the open
// files for changes made outside the app.
func (a *App) Start() {
	a.SaveState()
	a.startWatch()
	go func() {
		t := time.NewTicker(1500 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			win := a.Win
			if win == nil || win.IsDestroyed() {
				return
			}
			win.Update(a.checkDisk)
		}
	}()
}

// checkDisk runs on the main thread: it reloads clean documents whose
// files changed, marks dirty ones, and pins edited preview tabs.
func (a *App) checkDisk() {
	for _, d := range a.Docs {
		if !d.ChangedOnDisk() {
			continue
		}
		if d.Dirty() {
			d.SetExternalChanged(true)
			continue
		}
		if err := d.Reload(); err == nil {
			a.say("已从磁盘重新加载 " + d.Name())
		}
	}
	a.syncPreviews()
	a.gitTick++
	if a.Repo != nil && a.gitTick%2 == 0 {
		a.refreshGit()
	}
}

// syncPreviews pins preview tabs whose text was edited.
func (a *App) syncPreviews() {
	for _, col := range a.Cols {
		for _, e := range col.Editors {
			e.SyncPreview()
		}
	}
}

// SetZoom zooms the window's content.
func (a *App) SetZoom(z float32) {
	if z < 0.5 {
		z = 0.5
	}
	if z > 2 {
		z = 2
	}
	a.Zoom = z
	a.say(fmt.Sprintf("缩放 %d%%", int(z*100+0.5)))
	a.SaveState()
}

// OpenPreview shows path as a preview tab, as a single click in the
// explorer does.
func (a *App) OpenPreview(path string) {
	d, err := a.Doc(path)
	if err != nil {
		a.say("无法打开 " + filepath.Base(path) + "：" + err.Error())
		return
	}
	a.Col().OpenPreview(d)
	a.pruneDocs()
	a.updateTitle()
	a.SaveState()
}

// OpenFile shows path as a pinned tab.
func (a *App) OpenFile(path string) {
	d, err := a.Doc(path)
	if err != nil {
		a.say("无法打开 " + filepath.Base(path) + "：" + err.Error())
		return
	}
	a.Col().Open(d)
	a.pruneDocs()
	a.say("")
	a.updateTitle()
	a.SaveState()
}

// pruneDocs forgets documents no tab shows, as a preview replaced by
// another one.
func (a *App) pruneDocs() {
	for path, d := range a.Docs {
		if !a.hasDoc(d) {
			delete(a.Docs, path)
		}
	}
}

// hasDoc reports whether any column shows d.
func (a *App) hasDoc(d *document.Document) bool {
	for _, col := range a.Cols {
		if col.Has(d) {
			return true
		}
	}
	return false
}

// columnHasPath reports whether any column shows the file at path.
func (a *App) columnHasPath(path string) bool {
	for _, col := range a.Cols {
		for _, e := range col.Editors {
			if e.Doc.Path == path {
				return true
			}
		}
	}
	return false
}

// columnFiles returns the files of a column relative to the workspace.
func (a *App) columnFiles(col *document.Column) []string {
	var out []string
	if a.WS == nil {
		return out
	}
	for _, e := range col.Editors {
		rel, err := relPath(a.WS.Root, e.Doc.Path)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		out = append(out, rel)
	}
	return out
}

// samePath reports whether two paths name the same folder, as Windows
// compares them without case.
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ca, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	cb, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(ca), filepath.Clean(cb))
}

// externalBanner asks what to do with a dirty file that changed on disk.
func (a *App) externalBanner(c *ui.Context, d *document.Document) {
	ui.Row(c).Height(34).PaddingX(14).Gap(10).AlignItems(ui.Center).
		Background(theme.WarningSoft).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.WarningLine).Children(func() {
		ui.Text(c, d.Name()+" 已在磁盘上改变，本窗口还有未保存的修改。").FontSize(11).
			TextColor(theme.Warning).SingleLine().Grow(1)
		if ui.Button(c, "重新加载").Clicked() {
			if err := d.Reload(); err == nil {
				a.say("已重新加载 " + d.Name())
			}
			a.updateTitle()
			a.SaveState()
		}
		if ui.Button(c, "保留我的").Clicked() {
			d.SetExternalChanged(false)
			a.say("保留本窗口的修改；保存会覆盖磁盘上的版本")
		}
	})
}
