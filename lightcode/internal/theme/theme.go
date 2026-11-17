// Package theme holds the design tokens of 轻码 IDE: sizes and colors from
// the 16 boards, turned into a *ui.Theme and an *editor.Theme.
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
	AIDockW    = 380
	TerminalH  = 200
	WindowMinW = 960
	WindowMinH = 600
	WindowW    = 1280
	WindowH    = 820
)

// Colors of the dark theme, from the "主窗（深色）" board.
var (
	Background = ui.Hex("#0f1216")
	Panel      = ui.Hex("#14181e")
	PanelDeep  = ui.Hex("#11151a")
	Border     = ui.Hex("#242a33")
	BorderSoft = ui.Hex("#1c222a")
	Text       = ui.Hex("#e6e9ef")
	Muted      = ui.Hex("#8b95a5")
	Faint      = ui.Hex("#5d6675")
	Accent     = ui.Hex("#5b9cff")
	AccentSoft = ui.Hex("#1e2b3d")
	AI         = ui.Hex("#a78bfa")
	AISoft     = ui.Hex("#2a2340")
	Hover      = ui.Hex("#1a2028")
	Active     = ui.Hex("#1f2630")
	Danger     = ui.Hex("#f87171")
	Warning    = ui.Hex("#fbbf24")
	Success    = ui.Hex("#4ade80")
	// WarningSoft fills the banner of a file changed on disk, its line
	// the border above the editor.
	WarningSoft = ui.RGBA(251, 191, 36, 0.10)
	WarningLine = ui.RGBA(251, 191, 36, 0.32)
	Added       = ui.Hex("#3fb950")
	Deleted     = ui.Hex("#f85149")
	Modified    = ui.Hex("#d29922")
)

// Fonts: Inter for the chrome, JetBrains Mono for the code; both fall back
// to the system's families when not installed.
const (
	UIFont   = "Inter"
	MonoFont = "JetBrains Mono"
)

// Dark returns the window theme of the dark boards.
func Dark() *ui.Theme {
	return &ui.Theme{
		Dark:        true,
		Background:  Background,
		Surface:     Panel,
		Border:      Border,
		Text:        Text,
		TextMuted:   Muted,
		Accent:      Accent,
		AccentText:  ui.RGB(8, 14, 22),
		Danger:      Danger,
		Warning:     Warning,
		Success:     Success,
		Selection:   AccentSoft,
		Focus:       Accent,
		Inverse:     Text,
		InverseText: Background,
		Scrollbar:   ui.Hex("#3a4352"),
		Radius:      6,
		Spacing:     4,
		FontSize:    13,
		Font:        UIFont,
	}
}

// Code returns the editor theme: our background and text, GitHub's token
// colors, and the diff colors of the boards.
func Code(t *ui.Theme) *editor.Theme {
	ct := editor.DefaultTheme(t)
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
	ct.Added = Added
	ct.Deleted = Deleted
	ct.Modified = Modified
	ct.Match = ui.Hex("#2d3a26")
	ct.CurrentMatch = ui.Hex("#43552f")
	ct.Occurrence = ui.Hex("#243244")
	return ct
}

// EditorFont is the font the code is set in.
func EditorFont() ui.Font {
	return ui.Font{Family: MonoFont, Size: 13, LineHeight: 1.5}
}
