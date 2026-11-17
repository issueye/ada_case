package app

import (
	"context"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"

	"lightcode/internal/workspace"
)

// startWatch watches the workspace for changes made outside the app, as
// in a terminal: it asks for hints only (which directory changed, not
// which file), then rereads the tree and refreshes the index. A machine
// that cannot watch — or a folder the watch cannot cover — keeps the 1.5 s
// poll of session.go.
func (a *App) startWatch() {
	if a.WS == nil || a.Win == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.watchCancel = cancel
	root := a.WS.Root
	win := a.Win
	opts := mygo.FileWatchOptions{
		Recursive: true,
		HintsOnly: true,
		Debounce:  120 * time.Millisecond,
		MaxDelay:  600 * time.Millisecond,
		Ignore: func(p string, isDir bool) bool {
			return isDir && workspace.ShouldSkipDir(filepath.Base(p))
		},
	}
	w, err := mygo.WatchFiles(ctx, root, opts)
	if err != nil {
		a.say("文件监视不可用（" + err.Error() + "），改用 1.5 s 轮询")
		return
	}
	go func() {
		defer w.Close()
		for {
			events, err := w.Next(ctx)
			if err != nil {
				return
			}
			if len(events) == 0 {
				continue
			}
			win.Update(func() {
				if a.WS == nil || a.WS.Root != root {
					return
				}
				a.refreshTree()
				a.checkDisk()
			})
		}
	}()
}

// refreshTree rereads the explorer's tree and starts an index refresh in
// the background; the files open keep their caret and text.
func (a *App) refreshTree() {
	if a.WS == nil {
		return
	}
	if err := a.WS.ReloadTree(); err != nil {
		a.say("刷新目录失败：" + err.Error())
		return
	}
	a.WS.StartIndexRefresh(func() {
		if a.Win != nil {
			a.Win.Invalidate()
		}
	})
}
