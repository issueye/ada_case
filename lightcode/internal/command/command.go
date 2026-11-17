// Package command is the app's command registry: one table of commands
// that menus, the command palette, the keymap and the tests all share.
//
// A command's Run is a closure made where the app is wired, so the
// registry has no dependency on the views.
package command

import "sort"

// Command is one action of the app.
type Command struct {
	// ID is stable and dotted, as "file.save": menus and keybindings
	// refer to it, tests too.
	ID string
	// Title is what the palette and menus show.
	Title string
	// Category groups the palette, as "文件" or "Git".
	Category string
	// Key is a keybinding in VS Code's spelling, as "CmdOrCtrl+S" or
	// "CmdOrCtrl+Shift+P"; empty for none.
	Key string
	// When guards the command, as "!terminalFocus": empty always runs.
	When string
	// Run does it.
	Run func()
}

// Registry keeps the commands in a stable order.
type Registry struct {
	order []*Command
	byID  map[string]*Command
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{byID: map[string]*Command{}}
}

// Add registers c; a repeated ID replaces it.
func (r *Registry) Add(c *Command) *Command {
	if old, ok := r.byID[c.ID]; ok {
		for i, o := range r.order {
			if o == old {
				r.order[i] = c
				break
			}
		}
	} else {
		r.order = append(r.order, c)
	}
	r.byID[c.ID] = c
	return c
}

// Get returns the command with id, or nil.
func (r *Registry) Get(id string) *Command { return r.byID[id] }

// All returns the commands sorted by category and title.
func (r *Registry) All() []*Command {
	out := make([]*Command, len(r.order))
	copy(out, r.order)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Title < out[j].Title
	})
	return out
}

// Run runs the command with id, reporting whether it exists.
func (r *Registry) Run(id string) bool {
	c, ok := r.byID[id]
	if !ok || c.Run == nil {
		return false
	}
	c.Run()
	return true
}
