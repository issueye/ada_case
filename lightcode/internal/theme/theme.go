// Package theme holds the design tokens of 轻码 IDE: sizes and colors from
// the boards, turned into a *ui.Theme and an *editor.Theme. Use switches
// the palette between the dark and the light board.
//
// Views take colors and sizes from here, never literal values, so the
// boards stay the single source of truth.
package theme

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// Sizes of the chrome, in DIPs.
const (
	TitleBarH  = 40
	ActivityW  = 48
	SidebarW   = 240
	SidebarMin = 180
	SidebarMax = 360
	TabStripH  = 36
	CrumbsH    = 28
	StatusBarH = 24
	PanelHeadH = 34
	RowH       = 24
	SectionH   = 28
	TerminalH  = 200
	WindowMinW = 960
	WindowMinH = 600
	WindowW    = 1280
	WindowH    = 820
)

// Fonts: Inter for the chrome, JetBrains Mono for the code; both fall back
// to the system's families when not installed.
const (
	UIFont   = "Inter"
	MonoFont = "JetBrains Mono"
)

// Colors of the current palette, from the "主窗（深色）" and "主窗（浅色）"
// boards. Use sets them; views read them as they build, so switching is one
// assignment per color.
var (
	Background  ui.Color
	Panel       ui.Color
	PanelDeep   ui.Color
	Border      ui.Color
	BorderSoft  ui.Color
	Text        ui.Color
	Muted       ui.Color
	Faint       ui.Color
	Accent      ui.Color
	AccentSoft  ui.Color
	Hover       ui.Color
	Active      ui.Color
	Danger      ui.Color
	Warning     ui.Color
	Success     ui.Color
	Added       ui.Color
	Deleted     ui.Color
	Modified    ui.Color
	WarningSoft ui.Color
	WarningLine ui.Color
	// Scrollbar is the thumb of the scroll bars.
	Scrollbar ui.Color

	light bool
)

// palette is one board's colors.
type palette struct {
	Background, Panel, PanelDeep, Border, BorderSoft   ui.Color
	Text, Muted, Faint                                 ui.Color
	Accent, AccentSoft, Hover, Active                  ui.Color
	Danger, Warning, Success, Added, Deleted, Modified ui.Color
	WarningSoft, WarningLine, Scrollbar                ui.Color
}

var darkPalette = palette{
	Background: ui.Hex("#0f1216"), Panel: ui.Hex("#14181e"), PanelDeep: ui.Hex("#11151a"),
	Border: ui.Hex("#242a33"), BorderSoft: ui.Hex("#1c222a"),
	Text: ui.Hex("#e6e9ef"), Muted: ui.Hex("#8b95a5"), Faint: ui.Hex("#5d6675"),
	Accent: ui.Hex("#5b9cff"), AccentSoft: ui.Hex("#1e2b3d"),
	Hover: ui.Hex("#1a2028"), Active: ui.Hex("#1f2630"),
	Danger: ui.Hex("#f87171"), Warning: ui.Hex("#fbbf24"), Success: ui.Hex("#4ade80"),
	Added: ui.Hex("#3fb950"), Deleted: ui.Hex("#f85149"), Modified: ui.Hex("#d29922"),
	WarningSoft: ui.RGBA(251, 191, 36, 0.10), WarningLine: ui.RGBA(251, 191, 36, 0.32),
	Scrollbar: ui.Hex("#3a4352"),
}

var lightPalette = palette{
	Background: ui.Hex("#ffffff"), Panel: ui.Hex("#f6f7f9"), PanelDeep: ui.Hex("#eceff3"),
	Border: ui.Hex("#d8dee8"), BorderSoft: ui.Hex("#e6eaf0"),
	Text: ui.Hex("#1f2430"), Muted: ui.Hex("#5b6472"), Faint: ui.Hex("#8a93a2"),
	Accent: ui.Hex("#3b82f6"), AccentSoft: ui.Hex("#e3edff"),
	Hover: ui.Hex("#eef1f6"), Active: ui.Hex("#e1e7f2"),
	Danger: ui.Hex("#d1242f"), Warning: ui.Hex("#9a6700"), Success: ui.Hex("#1a7f37"),
	Added: ui.Hex("#1a7f37"), Deleted: ui.Hex("#cf222e"), Modified: ui.Hex("#9a6700"),
	WarningSoft: ui.RGBA(251, 191, 36, 0.18), WarningLine: ui.RGBA(200, 150, 20, 0.40),
	Scrollbar: ui.Hex("#c3cbd8"),
}

func init() { Use(false) }

// Use switches the palette: light is the light board, else the dark one.
func Use(isLight bool) {
	p := darkPalette
	if isLight {
		p = lightPalette
	}
	light = isLight
	Background, Panel, PanelDeep = p.Background, p.Panel, p.PanelDeep
	Border, BorderSoft = p.Border, p.BorderSoft
	Text, Muted, Faint = p.Text, p.Muted, p.Faint
	Accent, AccentSoft = p.Accent, p.AccentSoft
	Hover, Active = p.Hover, p.Active
	Danger, Warning, Success = p.Danger, p.Warning, p.Success
	Added, Deleted, Modified = p.Added, p.Deleted, p.Modified
	WarningSoft, WarningLine = p.WarningSoft, p.WarningLine
	Scrollbar = p.Scrollbar
}

// IsLight reports the light board.
func IsLight() bool { return light }

// Window returns the window theme of the current palette.
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
		Warning:        Warning,
		Success:        Success,
		Selection:      AccentSoft,
		Focus:          Accent,
		Inverse:        Text,
		InverseText:    Background,
		Scrollbar:      Scrollbar,
		Radius:         6,
		Spacing:        4,
		FontSize:       13,
		Font:           UIFont,
	}
	if light {
		t.AccentText = ui.RGB(255, 255, 255)
	} else {
		t.AccentText = ui.RGB(8, 14, 22)
		t.InactiveAccent = Accent
	}
	return t
}

// Code returns the editor theme for the current palette.
func Code(t *ui.Theme) *editor.Theme {
	ct := editor.DefaultTheme(t)
	if light {
		ct.Background = Background
		ct.Text = Text
		ct.LineNumber = Faint
		ct.ActiveLineNumber = Muted
		ct.ActiveLine = ui.Hex("#f3f6fa")
		ct.Selection = ui.Hex("#cfe1ff")
		ct.InactiveSelection = ui.Hex("#e6ecf5")
		ct.Cursor = ui.Hex("#1f2430")
		ct.IndentGuide = ui.Hex("#e8ecf2")
		ct.ActiveIndentGuide = ui.Hex("#d3dae5")
		ct.WrapGuide = ui.Hex("#f0f3f7")
		ct.Added, ct.Deleted, ct.Modified = Added, Deleted, Modified
		ct.Match = ui.Hex("#dbeafe")
		ct.CurrentMatch = ui.Hex("#bcd7ff")
		ct.Occurrence = ui.Hex("#e3edff")
	} else {
		ct.Background = Background
		ct.Text = Text
		ct.LineNumber = Faint
		ct.ActiveLineNumber = Muted
		ct.ActiveLine = ui.Hex("#161b22")
		ct.Selection = ui.Hex("#233a58")
		ct.InactiveSelection = ui.Hex("#1c2735")
		ct.Cursor = ui.Hex("#d7e2f0")
		ct.IndentGuide = ui.Hex("#20262f")
		ct.ActiveIndentGuide = ui.Hex("#2c3440")
		ct.WrapGuide = ui.Hex("#1e242c")
		ct.Added, ct.Deleted, ct.Modified = Added, Deleted, Modified
		ct.Match = ui.Hex("#2d3a26")
		ct.CurrentMatch = ui.Hex("#43552f")
		ct.Occurrence = ui.Hex("#243244")
	}
	return ct
}

// EditorFont is the font the code is set in.
func EditorFont() ui.Font {
	return ui.Font{Family: MonoFont, Size: 13, LineHeight: 1.5}
}
