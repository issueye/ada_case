package app

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"lightcode/internal/search"
	"lightcode/internal/theme"
)

// searchState is the search panel's state: the query, its options, the
// matches of the last run and whether one runs now.
type searchState struct {
	Query   string
	Case    bool
	Regex   bool
	Results []search.Match
	Running bool
	Elapsed time.Duration
	Err     string

	seq    uint64
	cancel context.CancelFunc
	timer  *time.Timer
}

// ScheduleSearch starts a search after a short pause, so that typing does
// not search every keystroke.
func (a *App) ScheduleSearch() {
	s := &a.Search
	s.seq++
	seq := s.seq
	if s.timer != nil {
		s.timer.Stop()
	}
	if s.Query == "" {
		a.clearSearch()
		return
	}
	delay := 180 * time.Millisecond
	if a.Win == nil {
		a.startSearch(seq)
		return
	}
	s.timer = time.AfterFunc(delay, func() {
		a.Win.Update(func() {
			if s.seq == seq {
				a.startSearch(seq)
			}
		})
	})
}

// RunSearchNow searches at once, as Enter does.
func (a *App) RunSearchNow() {
	a.Search.seq++
	a.startSearch(a.Search.seq)
}

func (a *App) startSearch(seq uint64) {
	s := &a.Search
	if s.Query == "" {
		a.clearSearch()
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	if a.WS == nil {
		return
	}
	root := a.WS.Root
	opts := search.Options{CaseSensitive: s.Case, Regex: s.Regex, MaxResults: 500, Files: a.WS.Files()}
	query := s.Query
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.Running = true
	s.Err = ""
	s.Results = nil
	if a.Win == nil {
		var got []search.Match
		res, err := search.RunStream(ctx, root, query, opts, func(m search.Match) { got = append(got, m) })
		if a.Search.seq == seq {
			a.Search.Running = false
			a.Search.Results = got
			a.Search.Elapsed = res.Elapsed
			if err != nil && !errors.Is(err, context.Canceled) {
				a.Search.Err = err.Error()
			}
		}
		return
	}
	win := a.Win
	go func() {
		var pending []search.Match
		last := time.Now()
		flush := func() {
			if len(pending) == 0 {
				return
			}
			batch := pending
			pending = nil
			last = time.Now()
			win.Update(func() {
				if a.Search.seq != seq {
					return
				}
				a.Search.Results = append(a.Search.Results, batch...)
				a.Search.Running = true
			})
		}
		res, err := search.RunStream(ctx, root, query, opts, func(m search.Match) {
			pending = append(pending, m)
			if len(pending) >= 20 || time.Since(last) > 60*time.Millisecond {
				flush()
			}
		})
		flush()
		win.Update(func() {
			if a.Search.seq != seq {
				return
			}
			a.Search.Running = false
			a.Search.Elapsed = res.Elapsed
			if err != nil && !errors.Is(err, context.Canceled) {
				a.Search.Err = err.Error()
			}
		})
	}()
}

func (a *App) clearSearch() {
	if a.Search.cancel != nil {
		a.Search.cancel()
		a.Search.cancel = nil
	}
	a.Search.Results = nil
	a.Search.Running = false
	a.Search.Elapsed = 0
	a.Search.Err = ""
}

// openMatch opens the file of a match at its line.
func (a *App) openMatch(m search.Match) {
	if a.WS == nil {
		return
	}
	path := filepath.Join(a.WS.Root, filepath.FromSlash(m.Path))
	a.OpenFile(path)
	if e := a.Col().Current(); e != nil {
		line := m.Line - 1
		if line < 0 {
			line = 0
		}
		if line >= e.Doc.Buffer.LineCount() {
			line = e.Doc.Buffer.LineCount() - 1
		}
		e.State.GoTo(editor.Pos{Line: line, Col: m.Col - 1})
		e.State.ScrollTo(line)
	}
}

// searchPanel is the sidebar's search view: the field, its options, the
// matches and the status line.
func (a *App) searchPanel(c *ui.Context) {
	ui.Column(c).Grow(1).Padding(10, 10).Gap(8).Children(func() {
		field := ui.SearchField(c, &a.Search.Query).Placeholder("在文件夹中搜索").
			FontSize(12).AutoFocus()
		if field.Submitted() {
			a.RunSearchNow()
		} else if field.Changed() {
			a.ScheduleSearch()
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			casing := a.Search.Case
			regex := a.Search.Regex
			ui.Checkbox(c, &casing, "区分大小写").FontSize(10)
			ui.Checkbox(c, &regex, "正则").FontSize(10)
			ui.Box(c).Grow(1)
			if a.Search.Query != "" {
				if ui.Button(c, "搜索").Clicked() {
					a.RunSearchNow()
				}
			}
			if casing != a.Search.Case || regex != a.Search.Regex {
				a.Search.Case, a.Search.Regex = casing, regex
				a.ScheduleSearch()
			}
		})
		// The status line: running, an error, or what the last run found.
		ui.Row(c).Height(18).AlignItems(ui.Center).Children(func() {
			text := ""
			kindColor := theme.Faint
			switch {
			case a.Search.Err != "":
				text, kindColor = "搜索失败："+a.Search.Err, theme.Danger
			case a.Search.Running:
				text = "正在搜索…"
			case len(a.Search.Results) > 0:
				text = searchSummary(len(a.Search.Results), a.Search.Elapsed)
			case a.Search.Query != "":
				text = "没有结果"
			}
			if text != "" {
				ui.Text(c, text).FontSize(10).TextColor(kindColor).SingleLine()
			}
		})
		ui.Box(c).Height(1).Background(theme.BorderSoft)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Children(func() {
				for _, m := range a.Search.Results {
					a.matchRow(c, m)
				}
				if len(a.Search.Results) == 0 && !a.Search.Running && a.Search.Query == "" {
					a.searchHint(c)
				}
			})
		})
	})
}

// matchRow is one match: the file and line at the top, the text under.
func (a *App) matchRow(c *ui.Context, m search.Match) {
	row := ui.Column(c).Padding(4, 6).Gap(2).Radius(4).Cursor(ui.CursorPointer)
	row.Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, fileIcon(m.Path)).FontSize(12).TextColor(fileColor(m.Path))
			ui.Text(c, m.Path).FontSize(10).TextColor(theme.Faint).SingleLine().Grow(1)
			ui.Text(c, itoa(m.Line)).FontSize(10).TextColor(theme.Faint)
		})
		line := m.Text
		if len(line) > 120 {
			line = line[:120]
		}
		ui.Text(c, line).FontSize(11).TextColor(theme.Muted).SingleLine()
	})
	if row.Hovered() {
		row.Background(theme.Hover)
	}
	row.OnClick(func() { a.openMatch(m) })
}

func (a *App) searchHint(c *ui.Context) {
	ui.Column(c).Padding(10, 4).Gap(4).Children(func() {
		ui.Text(c, "搜索工作区").FontSize(12).Bold().TextColor(theme.Text)
		ui.Text(c, "输入关键词，Enter 立即搜索；默认忽略大小写，可切换正则。").
			FontSize(11).TextColor(theme.Muted).LineHeight(1.5)
		if a.WS != nil {
			ui.Text(c, "索引 "+itoa(a.WS.FileCount())+" 个文件（纯 Go 实现，无 rg 依赖）").
				FontSize(10).TextColor(theme.Faint)
		}
	})
}

func searchSummary(n int, d time.Duration) string {
	return itoa(n) + " 个结果 · " + d.Round(time.Millisecond).String()
}
