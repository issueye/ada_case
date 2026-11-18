// Package theme holds the assistant panel's colors: the same boards the
// editor uses, kept here as values rather than shared code, so this project
// does not depend on the editor's internals.
package theme

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// Mount sizes, in DIPs: the rectangles the editor gives the panel.
const (
	HeaderH = 36
	DockW   = 380
	FloatW  = 380
	FloatH  = 640
	RailW   = 48
	RailH   = 172
)

// Colors of the current palette.
var (
	Background ui.Color
	Panel      ui.Color
	PanelDeep  ui.Color
	Border     ui.Color
	BorderSoft ui.Color
	Text       ui.Color
	Muted      ui.Color
	Faint      ui.Color
	Accent     ui.Color
	AccentSoft ui.Color
	AI         ui.Color
	AISoft     ui.Color
	Hover      ui.Color
	Active     ui.Color
	Danger     ui.Color
	Success    ui.Color
	Added      ui.Color

	light bool
)

func init() { Use(false) }

// Use switches the palette: light is the editor's light board.
func Use(isLight bool) {
	light = isLight
	if isLight {
		Background, Panel, PanelDeep = ui.Hex("#ffffff"), ui.Hex("#f6f7f9"), ui.Hex("#eceff3")
		Border, BorderSoft = ui.Hex("#d8dee8"), ui.Hex("#e6eaf0")
		Text, Muted, Faint = ui.Hex("#1f2430"), ui.Hex("#5b6472"), ui.Hex("#8a93a2")
		Accent, AccentSoft = ui.Hex("#3b82f6"), ui.Hex("#e3edff")
		AI, AISoft = ui.Hex("#7c5cff"), ui.Hex("#ece7ff")
		Hover, Active = ui.Hex("#eef1f6"), ui.Hex("#e1e7f2")
		Danger, Success, Added = ui.Hex("#d1242f"), ui.Hex("#1a7f37"), ui.Hex("#1a7f37")
		return
	}
	Background, Panel, PanelDeep = ui.Hex("#0f1216"), ui.Hex("#14181e"), ui.Hex("#11151a")
	Border, BorderSoft = ui.Hex("#242a33"), ui.Hex("#1c222a")
	Text, Muted, Faint = ui.Hex("#e6e9ef"), ui.Hex("#8b95a5"), ui.Hex("#5d6675")
	Accent, AccentSoft = ui.Hex("#5b9cff"), ui.Hex("#1e2b3d")
	AI, AISoft = ui.Hex("#a78bfa"), ui.Hex("#2a2340")
	Hover, Active = ui.Hex("#1a2028"), ui.Hex("#1f2630")
	Danger, Success, Added = ui.Hex("#f87171"), ui.Hex("#4ade80"), ui.Hex("#3fb950")
}

// IsLight reports the light board.
func IsLight() bool { return light }

// Window is the theme the panel's widgets read when they take no color.
func Window() *ui.Theme {
	t := &ui.Theme{
		Dark:           !light,
		Background:     Background,
		Surface:        Panel,
		SurfaceHover:   Hover,
		SurfacePressed: Active,
		Border:         Border,
		Text:           Text,
		TextMuted:      Muted,
		Accent:         Accent,
		AccentHover:    Accent,
		AccentPressed:  Accent,
		Danger:         Danger,
		Warning:        ui.Hex("#fbbf24"),
		Success:        Success,
		Selection:      AccentSoft,
		Focus:          Accent,
		Inverse:        Text,
		InverseText:    Background,
		Scrollbar:      ui.Hex("#3a4352"),
		Radius:         6,
		Spacing:        4,
		FontSize:       13,
	}
	if light {
		t.AccentText = ui.RGB(255, 255, 255)
	} else {
		t.AccentText = ui.RGB(8, 14, 22)
		t.InactiveAccent = Accent
	}
	return t
}

// Code is the editor theme the diff cards draw with: the tokens the editor
// uses, so a card reads like the pane it will change.
func Code() *editor.Theme {
	ct := editor.DefaultTheme(Window())
	ct.Background = Background
	ct.Text = Text
	ct.LineNumber = Faint
	ct.ActiveLineNumber = Muted
	ct.Added, ct.Deleted = Added, ui.Hex("#f85149")
	ct.Modified = ui.Hex("#d29922")
	if light {
		ct.ActiveLine = ui.Hex("#f3f6fa")
		ct.Selection = ui.Hex("#cfe1ff")
		ct.Cursor = ui.Hex("#1f2430")
	} else {
		ct.ActiveLine = ui.Hex("#161b22")
		ct.Selection = ui.Hex("#233a58")
		ct.Cursor = ui.Hex("#d7e2f0")
	}
	return ct
}
