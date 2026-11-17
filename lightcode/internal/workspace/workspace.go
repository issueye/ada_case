// Package workspace is the folder the IDE works in: the explorer's tree,
// loaded a directory at a time, and the file index that quick open
// searches.
package workspace

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Entry is a file or a folder of the explorer. A folder reads its children
// the first time the tree shows it open, so a large repository opens
// without walking it.
type Entry struct {
	Name     string
	Path     string
	IsDir    bool
	Open     bool // the tree shows a folder's children while true
	loaded   bool
	children []*Entry
	err      error
}

// Children returns the entries of a folder, reading the directory the
// first time and caching it. It returns nil for a file.
func (e *Entry) Children() []*Entry {
	if !e.IsDir {
		return nil
	}
	if e.loaded {
		return e.children
	}
	e.loaded = true
	items, err := os.ReadDir(e.Path)
	if err != nil {
		e.err = err
		return nil
	}
	e.children = fromDirItems(e.Path, items)
	return e.children
}

// Reload rereads a folder's children.
func (e *Entry) Reload() {
	e.loaded = false
	e.children = nil
}

// Loaded reports whether the folder's children were read.
func (e *Entry) Loaded() bool { return e.loaded }

// CachedChildren returns the children read so far, without reading the
// folder.
func (e *Entry) CachedChildren() []*Entry { return e.children }

// Workspace is a folder and its file index.
type Workspace struct {
	Root string

	mu       sync.Mutex
	top      []*Entry
	index    []string
	building bool
	name     string
}

// Open reads the top level of root; the rest loads as the tree opens it.
func Open(root string) (*Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &os.PathError{Op: "open", Path: abs, Err: os.ErrInvalid}
	}
	w := &Workspace{Root: abs, name: filepath.Base(abs)}
	items, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	w.top = fromDirItems(abs, items)
	w.BuildIndex()
	return w, nil
}

// Name is the folder's name, shown in the title bar and sidebar.
func (w *Workspace) Name() string { return w.name }

// Top returns the top-level entries.
func (w *Workspace) Top() []*Entry { return w.top }

// Reload rereads the whole tree and its index.
func (w *Workspace) Reload() error {
	if err := w.ReloadTree(); err != nil {
		return err
	}
	w.BuildIndex()
	return nil
}

// ReloadTree rereads the tree, keeping which folders are open, without
// the index.
func (w *Workspace) ReloadTree() error {
	items, err := os.ReadDir(w.Root)
	if err != nil {
		return err
	}
	open := map[string]bool{}
	var walk func(es []*Entry)
	walk = func(es []*Entry) {
		for _, e := range es {
			if e.IsDir && e.Open {
				open[e.Path] = true
				walk(e.children)
			}
		}
	}
	walk(w.top)
	w.top = fromDirItems(w.Root, items)
	func() {
		var reopen func(es []*Entry)
		reopen = func(es []*Entry) {
			for _, e := range es {
				if e.IsDir && open[e.Path] {
					e.Open = true
					e.Children()
					reopen(e.children)
				}
			}
		}
		reopen(w.top)
	}()
	return nil
}

func skipDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".hg", ".svn", "node_modules", "vendor", ".venv", "venv",
		"dist", "build", "target", ".idea", "__pycache__", ".next":
		return true
	}
	return false
}

// splitDir splits the names of a directory into entries, folders first.
func fromDirItems(dir string, items []fs.DirEntry) []*Entry {
	out := make([]*Entry, 0, len(items))
	for _, it := range items {
		name := it.Name()
		if skipDir(name) {
			continue
		}
		out = append(out, &Entry{
			Name:  name,
			Path:  filepath.Join(dir, name),
			IsDir: it.IsDir(),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		li, lj := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if li != lj {
			return li < lj
		}
		return out[i].Name < out[j].Name
	})
	return out
}
