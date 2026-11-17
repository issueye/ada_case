package workspace

import (
	"io/fs"
	"path/filepath"
	"sort"
)

// Files returns the repository's files as paths relative to the root, the
// list the index holds: nil until BuildIndex or a refresh fills it. Hidden
// folders and the usual build outputs stay out.
func (w *Workspace) Files() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.index
}

// FileCount is how many files the index holds.
func (w *Workspace) FileCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.index)
}

// IndexBuilding reports whether a refresh is walking the folder.
func (w *Workspace) IndexBuilding() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.building
}

// BuildIndex walks the folder and replaces the index, on the calling
// goroutine.
func (w *Workspace) BuildIndex() {
	files := walkFiles(w.Root)
	w.mu.Lock()
	w.index = files
	w.mu.Unlock()
}

// StartIndexRefresh walks the folder on a goroutine and replaces the
// index when done, then calls done from that goroutine (the app hands
// that to Window.Update). A refresh already running is joined rather
// than doubled.
func (w *Workspace) StartIndexRefresh(done func()) {
	w.mu.Lock()
	if w.building {
		w.mu.Unlock()
		return
	}
	w.building = true
	w.mu.Unlock()
	go func() {
		files := walkFiles(w.Root)
		w.mu.Lock()
		w.index = files
		w.building = false
		w.mu.Unlock()
		if done != nil {
			done()
		}
	}()
}

// InvalidateIndex asks for a refresh: quick open keeps the old list until
// the new one lands.
func (w *Workspace) InvalidateIndex() { w.StartIndexRefresh(nil) }

// ShouldSkipDir reports folders the explorer, index and search leave out.
func ShouldSkipDir(name string) bool { return skipDir(name) }

// walkFiles lists the files under root, relative and sorted.
func walkFiles(root string) []string {
	var out []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out
}
