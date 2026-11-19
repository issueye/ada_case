package document

import (
	"github.com/egoist/mygo/ui/editor"

	"lightcode/internal/theme"
)

// Editor shows one Document: its view state (caret, scroll, folds, find
// bar) beside the buffer the Document owns.
type Editor struct {
	Doc   *Document
	State *editor.State
	// Preview marks a tab opened by a single click in the explorer: it
	// is replaced by the next preview, and pinned by an edit or a
	// deliberate open.
	Preview bool
	// MarkdownView shows a Markdown document as it reads, in place of
	// its source, until toggled back.
	MarkdownView bool
	// previewVersion is the buffer version the preview opened at.
	previewVersion uint64
}

// NewEditor makes the view state of a document.
func NewEditor(d *Document) *Editor {
	return &Editor{
		Doc: d,
		State: &editor.State{
			Buffer:   d.Buffer,
			DiffBase: d.Base,
			Font:     theme.EditorFont(),
			Theme:    theme.Code(theme.Window()),
			Label:    d.Name(),
			// The code area's menu is the app's own: drawn in the window
			// with the palette, in Chinese, as the other context menus.
			PopoverMenu: true,
		},
		previewVersion: d.Buffer.Version(),
	}
}

// Pin keeps the tab of a preview open, as an edit or a double click does.
func (e *Editor) Pin() { e.Preview = false }

// SyncPreview pins the tab once its text changed since it opened as a
// preview.
func (e *Editor) SyncPreview() {
	if e.Preview && e.Doc.Buffer.Version() != e.previewVersion {
		e.Preview = false
	}
}

// Retarget makes the editor show another document, as a preview tab
// replaced by the next preview does.
func (e *Editor) Retarget(d *Document) {
	e.Doc = d
	e.State = NewEditor(d).State
	e.Preview = true
	e.previewVersion = d.Buffer.Version()
}

// Column is one group of tabs: the editors it shows and the one chosen.
type Column struct {
	Editors []*Editor
	Active  int
}

// Open shows d, choosing the editor that already shows it, or adding one
// after the current tab.
func (c *Column) Open(d *Document) *Editor {
	for _, e := range c.Editors {
		if e.Doc == d {
			e.Pin()
			c.Active = indexOf(c.Editors, e)
			return e
		}
	}
	e := NewEditor(d)
	if len(c.Editors) == 0 {
		c.Editors = append(c.Editors, e)
		c.Active = 0
		return e
	}
	at := c.Active + 1
	c.Editors = append(c.Editors, nil)
	copy(c.Editors[at+1:], c.Editors[at:])
	c.Editors[at] = e
	c.Active = at
	return e
}

// OpenPreview shows d as the preview tab: the editor showing a clean
// preview takes d's place, and a tab already showing d is focused.
func (c *Column) OpenPreview(d *Document) *Editor {
	for _, e := range c.Editors {
		if e.Doc == d {
			e.Preview = true
			e.previewVersion = d.Buffer.Version()
			c.Active = indexOf(c.Editors, e)
			return e
		}
	}
	if cur := c.Current(); cur != nil && cur.Preview && !cur.Doc.Dirty() {
		cur.Retarget(d)
		return cur
	}
	e := c.Open(d)
	e.Preview = true
	e.previewVersion = d.Buffer.Version()
	return e
}

// Current returns the editor of the chosen tab, or nil.
func (c *Column) Current() *Editor {
	if c.Active < 0 || c.Active >= len(c.Editors) {
		return nil
	}
	return c.Editors[c.Active]
}

// Remove closes the tab at i.
func (c *Column) Remove(i int) {
	if i < 0 || i >= len(c.Editors) {
		return
	}
	c.Editors = append(c.Editors[:i], c.Editors[i+1:]...)
	if c.Active >= len(c.Editors) {
		c.Active = len(c.Editors) - 1
	}
}

// Has reports whether the column shows d.
func (c *Column) Has(d *Document) bool {
	for _, e := range c.Editors {
		if e.Doc == d {
			return true
		}
	}
	return false
}

func indexOf(es []*Editor, e *Editor) int {
	for i := range es {
		if es[i] == e {
			return i
		}
	}
	return -1
}
