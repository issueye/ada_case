package app

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// preferences are settings.json: the editor's and the terminal's look, and
// which capabilities are on. They are not the session (state.json).
type preferences struct {
	EditorFontSize   float32 `json:"editorFontSize,omitempty"`
	TerminalFontSize float32 `json:"terminalFontSize,omitempty"`
	SoftWrap         bool    `json:"softWrap,omitempty"`
	// Theme is "dark" or "light"; empty reads as dark.
	Theme           string `json:"theme,omitempty"`
	AIEnabled       bool   `json:"aiEnabled,omitempty"`
	TerminalEnabled bool   `json:"terminalEnabled,omitempty"`
}

// defaultPreferences is what a first run uses.
func defaultPreferences() preferences {
	return preferences{EditorFontSize: 13, TerminalFontSize: 13, AIEnabled: true, TerminalEnabled: true, Theme: "dark"}
}

// prefsPath is settings.json: LIGHTCODE_SETTINGS when set, else in the
// user's config folder beside state.json.
func prefsPath() string {
	if p := os.Getenv("LIGHTCODE_SETTINGS"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "lightcode", "settings.json")
}

// loadPrefs reads settings.json, falling back to the defaults on a missing
// or corrupt file.
func loadPrefs() preferences {
	p := defaultPreferences()
	path := prefsPath()
	if path == "" {
		return p
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return p
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return defaultPreferences()
	}
	if p.EditorFontSize < 8 || p.EditorFontSize > 40 {
		p.EditorFontSize = 13
	}
	if p.TerminalFontSize < 8 || p.TerminalFontSize > 40 {
		p.TerminalFontSize = 13
	}
	if p.Theme != "light" {
		p.Theme = "dark"
	}
	return p
}

// savePrefs writes settings.json atomically.
func savePrefs(p preferences) {
	path := prefsPath()
	if path == "" {
		return
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return
	}
	os.Rename(tmp, path)
}

// capabilityState is the extensions view's toggles and read-only provider
// list.
type capabilityState struct {
	// nothing yet: the toggles live in Prefs and show as enabled here.
}
