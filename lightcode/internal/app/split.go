package app

import (
	"fmt"

	"lightcode/internal/document"
)

// ToggleSplit shows a second editor column beside the first, or closes it
// and moves its tabs into the first. `Ctrl+\` runs it.
func (a *App) ToggleSplit() {
	if len(a.Cols) > 1 {
		first := a.Cols[0]
		for _, e := range a.Cols[1].Editors {
			first.Open(e.Doc)
		}
		a.Cols = a.Cols[:1]
		a.ActiveCol = 0
		a.say("已关闭分屏")
	} else {
		a.Cols = append(a.Cols, &document.Column{})
		a.ActiveCol = 1
		if e := a.Cols[0].Current(); e != nil {
			a.Cols[1].Open(e.Doc)
		}
		if a.SplitPercent <= 0 || a.SplitPercent >= 100 {
			a.SplitPercent = 50
		}
		a.say(fmt.Sprintf("已拆分编辑器，左栏占 %d%%（拖动分隔条调整）", int(a.SplitPercent)))
	}
	a.collapseEmptyColumns()
	a.pruneDocs()
	a.updateTitle()
	a.SaveState()
}
