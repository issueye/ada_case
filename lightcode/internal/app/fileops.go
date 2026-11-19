package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"lightcode/internal/document"
	"lightcode/internal/theme"
	"lightcode/internal/workspace"
)

// fileOpKind is the file operation a dialog asks about.
type fileOpKind int

const (
	opNewFile fileOpKind = iota
	opNewFolder
	opRename
	opDelete
)

// fileOp is the file operation dialog's state.
type fileOp struct {
	Open bool
	Kind fileOpKind
	// Dir is the folder the operation works in, Path the file or folder
	// it renames or deletes, Name the field's text.
	Dir   string
	Path  string
	Name  string
	Error string
}

// Selected is the explorer's chosen entry, "" for none.
func (a *App) selectedDir() string {
	if a.Selected != "" {
		if info, err := os.Stat(a.Selected); err == nil {
			if info.IsDir() {
				return a.Selected
			}
			return filepath.Dir(a.Selected)
		}
	}
	if e := a.Col().Current(); e != nil {
		return e.Doc.Dir()
	}
	if a.WS != nil {
		return a.WS.Root
	}
	return ""
}

// BeginNewFile asks for a new file's name in dir.
func (a *App) BeginNewFile(dir string) {
	a.FileOp = fileOp{Open: true, Kind: opNewFile, Dir: dir, Name: ""}
}

// BeginNewFolder asks for a new folder's name in dir.
func (a *App) BeginNewFolder(dir string) {
	a.FileOp = fileOp{Open: true, Kind: opNewFolder, Dir: dir, Name: ""}
}

// BeginRename asks for a new name for path.
func (a *App) BeginRename(path string) {
	a.FileOp = fileOp{Open: true, Kind: opRename, Path: path, Dir: filepath.Dir(path), Name: filepath.Base(path)}
}

// BeginDelete asks whether to delete path.
func (a *App) BeginDelete(path string) {
	a.FileOp = fileOp{Open: true, Kind: opDelete, Path: path, Dir: filepath.Dir(path), Name: filepath.Base(path)}
}

// fileOpTitle is the dialog's heading.
func fileOpTitle(op fileOp) string {
	switch op.Kind {
	case opNewFile:
		return "新建文件"
	case opNewFolder:
		return "新建文件夹"
	case opRename:
		return "重命名"
	default:
		return "删除"
	}
}

// fileOpConfirm runs the operation the dialog asked about.
func (a *App) fileOpConfirm() {
	op := a.FileOp
	name := strings.TrimSpace(op.Name)
	if a.WS == nil {
		a.FileOp.Error = "没有打开的工作区"
		return
	}
	if op.Kind != opDelete {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
			a.FileOp.Error = "名称不能为空，也不能包含路径分隔符"
			return
		}
	}
	var target string
	switch op.Kind {
	case opNewFile, opNewFolder:
		target = filepath.Join(op.Dir, name)
		if _, err := os.Stat(target); err == nil {
			a.FileOp.Error = "已存在：" + name
			return
		}
		if op.Kind == opNewFile {
			if err := os.WriteFile(target, nil, 0o644); err != nil {
				a.FileOp.Error = err.Error()
				return
			}
		} else if err := os.MkdirAll(target, 0o755); err != nil {
			a.FileOp.Error = err.Error()
			return
		}
	case opRename:
		if d := a.DocAt(op.Path); d != nil && d.Dirty() {
			a.FileOp.Error = "文件有未保存的修改，先保存再重命名"
			return
		}
		target = filepath.Join(op.Dir, name)
		if target == op.Path {
			a.FileOp = fileOp{}
			return
		}
		if _, err := os.Stat(target); err == nil {
			a.FileOp.Error = "已存在：" + name
			return
		}
		if err := os.Rename(op.Path, target); err != nil {
			a.FileOp.Error = err.Error()
			return
		}
		a.closePath(op.Path)
	case opDelete:
		if d := a.DocAt(op.Path); d != nil && d.Dirty() {
			a.FileOp.Error = "文件有未保存的修改，先保存再删除"
			return
		}
		if err := os.RemoveAll(op.Path); err != nil {
			a.FileOp.Error = err.Error()
			return
		}
		a.closePath(op.Path)
	}
	a.FileOp = fileOp{}
	a.WS.Reload()
	if a.Repo != nil {
		a.refreshGit()
	}
	switch op.Kind {
	case opNewFile:
		a.OpenFile(target)
		a.say("已创建 " + name)
	case opNewFolder:
		a.say("已创建文件夹 " + name)
	case opRename:
		a.say("已重命名为 " + name)
	case opDelete:
		a.say("已删除 " + op.Name)
	}
}

// DocAt returns the open document of a path, nil when it is not open.
func (a *App) DocAt(path string) *document.Document {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	return a.Docs[abs]
}

// closePath closes every tab of path or, for a folder, of what it holds.
func (a *App) closePath(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	for _, col := range a.Cols {
		for i := len(col.Editors) - 1; i >= 0; i-- {
			e := col.Editors[i]
			if e.Doc.Path == abs || strings.HasPrefix(e.Doc.Path, abs+string(filepath.Separator)) {
				col.Remove(i)
			}
		}
	}
	a.collapseEmptyColumns()
	a.pruneDocs()
	a.updateTitle()
	a.SaveState()
}

// fileOpOverlay draws the file operation dialog.
func (a *App) fileOpOverlay(c *ui.Context) {
	if !a.FileOp.Open {
		return
	}
	op := &a.FileOp
	appDialog(c, &op.Open, func() {
		ui.Text(c, fileOpTitle(*op)).FontSize(14).Bold().TextColor(theme.Text)
		if op.Kind == opDelete {
			ui.Text(c, "删除 "+op.Name+"？文件夹会连同其中的文件一起删除，此操作不可撤销。").
				FontSize(11).TextColor(theme.Muted).LineHeight(1.5)
		} else {
			ui.Text(c, filepath.ToSlash(op.Dir)).FontSize(10).TextColor(theme.Faint).SingleLine()
			input := ui.TextInput(c, &op.Name).Placeholder("名称").FontSize(13).AutoFocus()
			if input.Submitted() {
				a.fileOpConfirm()
			}
		}
		if op.Error != "" {
			ui.Text(c, op.Error).FontSize(10).TextColor(theme.Danger).SingleLine()
		}
		ui.Box(c).Height(4)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Clicked() {
				a.FileOp = fileOp{}
			}
			if ui.Button(c, fileOpButton(*op)).Clicked() {
				a.fileOpConfirm()
			}
		})
	})
}

func fileOpButton(op fileOp) string {
	if op.Kind == opDelete {
		return "删除"
	}
	return "确定"
}

// fileOpMenuItems is the explorer row's menu: the file operations, in the
// app's own style.
func (a *App) fileOpMenuItems(e *workspace.Entry) []menuItem {
	dir := e.Path
	if !e.IsDir {
		dir = filepath.Dir(e.Path)
	}
	return []menuItem{
		{label: "新建文件", hint: "N", run: func() { a.BeginNewFile(dir) }},
		{label: "新建文件夹", run: func() { a.BeginNewFolder(dir) }},
		{sep: true},
		{label: "重命名", hint: "F2", run: func() { a.BeginRename(e.Path) }},
		{label: "删除", run: func() { a.BeginDelete(e.Path) }},
		{sep: true},
		{label: "在文件管理器中显示", run: func() { mygo.Shell.ShowItemInFolder(e.Path) }},
	}
}
