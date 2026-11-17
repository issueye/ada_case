// Package settings keeps the app's state between runs: the files open,
// the tab chosen, the sidebar and the AI panel, and the zoom. It writes
// one JSON document, replaced atomically, and reads it back without
// failing on a missing, half-written or older file.
package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Version is the schema of the state this build writes.
const Version = 2

// State is what the next run starts with.
type State struct {
	// Version is the schema the file has.
	Version int `json:"version"`
	// Workspace is the folder the state belongs to: the files and the
	// layout hold only while the app opens the same folder.
	Workspace string `json:"workspace,omitempty"`
	// Files are the open files of the first editor column, relative to
	// the workspace root; SplitFiles those of the second.
	Files      []string `json:"files,omitempty"`
	SplitFiles []string `json:"splitFiles,omitempty"`
	// Active is the index of the chosen tab, ActiveCol the chosen column.
	Active    int `json:"active"`
	ActiveCol int `json:"activeCol,omitempty"`
	// SplitPercent is the share of the editor area the left column of a split takes.
	SplitPercent float32 `json:"splitPercent,omitempty"`
	// SidebarOpen and SidebarView are the sidebar's state.
	SidebarOpen bool `json:"sidebarOpen"`
	SidebarView int  `json:"sidebarView"`
	// AIDocked is whether the AI panel shows docked rather than as the rail.
	AIDocked bool `json:"aiDocked"`
	// Zoom is the window content's zoom, 1 for none.
	Zoom float32 `json:"zoom,omitempty"`
}

// Default is the state a first run starts with.
func Default() State {
	return State{Version: Version, SidebarOpen: true, AIDocked: true, Zoom: 1}
}

// Path is where the app keeps its state: LIGHTCODE_STATE when set, else
// lightcode/state.json in the user's config folder.
func Path() string {
	if p := os.Getenv("LIGHTCODE_STATE"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "lightcode", "state.json")
}

// Load reads path. A missing file is not an error: it returns Default.
// An unreadable or corrupt file returns Default and the error, so that a
// caller can start fresh and, say, warn.
func Load(path string) (State, error) {
	st := Default()
	if path == "" {
		return st, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return st, nil
		}
		return st, err
	}
	var read = st
	if err := json.Unmarshal(data, &read); err != nil {
		return st, err
	}
	return migrate(read), nil
}

// migrate fills what an older file leaves out and clamps what a hand
// edit could break.
func migrate(in State) State {
	out := in
	out.Version = Version
	if out.SidebarView < 0 || out.SidebarView > 4 {
		out.SidebarView = 0
	}
	if out.Zoom < 0.25 || out.Zoom > 5 {
		out.Zoom = 1
	}
	if out.Active < 0 {
		out.Active = 0
	}
	if out.ActiveCol < 0 || out.ActiveCol > 1 {
		out.ActiveCol = 0
	}
	if out.SplitPercent < 0 || out.SplitPercent > 100 {
		out.SplitPercent = 0
	}
	return out
}

// Save writes st to path, replacing the file atomically so that a crash
// mid-write leaves the previous one.
func Save(path string, st State) error {
	if path == "" {
		return nil
	}
	st.Version = Version
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
