package app

import (
	"github.com/egoist/mygo"

	"lightcode/internal/theme"
)

// ToggleTheme switches between the dark and the light board.
func (a *App) ToggleTheme() { a.UseTheme(!theme.IsLight()) }

// UseTheme switches the palette, the open editors and the diff or AI cards
// already built, and saves it in settings.json.
func (a *App) UseTheme(light bool) {
	if light {
		a.Prefs.Theme = "light"
	} else {
		a.Prefs.Theme = "dark"
	}
	theme.Use(light)
	a.applyTheme()
	savePrefs(a.Prefs)
	a.SyncNativeTheme()
	if light {
		a.say("已切换到浅色主题")
	} else {
		a.say("已切换到深色主题")
	}
}

// SyncNativeTheme makes the window's menus and dialogs follow the palette,
// through the main loop.
func (a *App) SyncNativeTheme() {
	if a.Win == nil {
		return
	}
	wantLight := a.Prefs.Theme == "light"
	a.Win.Update(func() {
		if wantLight {
			mygo.Theme.SetSource(mygo.ThemeLight)
		} else {
			mygo.Theme.SetSource(mygo.ThemeDark)
		}
	})
}

// applyTheme gives the open editors, the diff page and the AI cards the
// current palette.
func (a *App) applyTheme() {
	t := theme.Window()
	for _, col := range a.Cols {
		for _, e := range col.Editors {
			e.State.Theme = theme.Code(t)
		}
	}
	if a.Diff != nil && a.Diff.State != nil {
		a.Diff.State.Theme = theme.Code(t)
	}
	// The panel draws its own cards, so it gets the palette it should use.
	a.AIEdge.dirty = true
}
