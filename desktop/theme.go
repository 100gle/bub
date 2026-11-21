package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/richtext"
)

// applyTheme gives the window Bub's own take on the system theme: a calm
// neutral chrome, a sky-blue accent for the AI and the current page, and the
// token radii and spacing everywhere. The light or dark base still follows the
// system, or the settings' appearance.
func (a *app) applyTheme(c *ui.Context) {
	t := *c.Theme()
	p := paletteFor(t.Dark)
	t.Background = p.Background
	t.Surface = p.Surface
	t.SurfaceHover = p.SurfaceHover
	t.SurfacePressed = p.SurfacePressed
	t.Border = p.Border
	t.Text = p.Text
	t.TextMuted = p.TextMuted
	t.Accent = p.Accent
	t.AccentHover = p.AccentHover
	t.AccentPressed = p.AccentPressed
	t.AccentText = p.AccentText
	t.Selection = p.Selection
	t.Focus = p.Focus
	// Built-in controls take the small radius; the views use the rest of the
	// scale in tokens.go.
	t.Radius = tokens.RadiusSM
	t.Spacing = tokens.Space1
	// The family stays empty so Core Text resolves the system font — SF Pro on
	// macOS, with its optical sizes and tracking — and 13 DIPs is Apple's
	// standard control size.
	t.Font = ""
	t.FontSize = tokens.FontBody
	c.SetTheme(&t)
}

// editorLook is the editor's own colors and measure, derived from the window
// theme so the writing column matches the chrome.
func (a *app) editorLook(c *ui.Context, padLeft float32) *richtext.Theme {
	t := c.Theme()
	fontSize := float32(a.settings.EditorFontSize)
	if fontSize < 12 || fontSize > 24 {
		fontSize = 16
	}
	look := richtext.DefaultTheme(t)
	look.Padding = [2]float32{padLeft, 24}
	look.Caret = t.Accent
	look.Selection = t.Selection
	look.Link = t.Accent
	look.QuoteBar = t.Accent.Alpha(0.35)
	// A code block is a quiet neutral panel, not a second surface color: it
	// should read as code without competing with the page. `richtext` paints
	// the block itself, so the panel is its background and radius only.
	look.CodeBlockBackground = t.Surface
	look.CodeBlockRadius = tokens.RadiusMD / fontSize
	// Padding around the code, in ems. codeBlockPad reads the same numbers
	// back, so the hairline the app draws around the panel lands on the
	// panel's own edge.
	look.CodeBlockPadding = codeBlockPaddingEm
	// SF Pro Display semibold for headings, as Apple's own documents set
	// them: presence from size and weight together, never from bold alone.
	for i := range look.Headings {
		look.Headings[i].Weight = tokens.WeightStrong
	}
	look.Headings[0].Size = 1.9
	look.Headings[0].Top = 0.1
	look.Headings[1].Top = 0.9
	look.ParagraphSpacing = 0.35
	return look
}

// markdownLook is how a chat answer reads: the app's body sizes and a real
// code block — a bordered panel with the language above the code and
// highlighting from the editor's token theme — rather than a bare gray run.
// The rail uses the narrow measure; the chat page passes wide for a larger
// answer whose headings carry the hierarchy across a 680 DIP column.
func (a *app) markdownLook(c *ui.Context, wide bool) *markdown.Theme {
	t := c.Theme()
	look := markdown.DefaultTheme(t)
	look.StrongWeight = tokens.WeightStrong
	look.LinkWeight = tokens.WeightRegular
	// A chat surface is the rail's surface color or the page's canvas, so a
	// code panel on the canvas color reads as a distinct sheet.
	look.CodeBlockBackground = t.Background
	look.CodeBlockBorder = t.Border
	look.CodeBlockRadius = tokens.RadiusMD
	look.CodeSize = 0.88
	sizes := [6]float32{1.4, 1.2, 1.08, 1, 0.96, 0.92}
	tops := [6]float32{1.1, 0.95, 0.8, 0.7, 0.6, 0.6}
	if wide {
		look.FontSize = tokens.FontBody + 1
		look.LineHeight = 1.65
		look.CodeLineHeight = 1.5
		look.CodeBlockPadding = 1
		sizes = [6]float32{1.75, 1.45, 1.22, 1.06, 1, 0.95}
		tops = [6]float32{1.35, 1.15, 0.95, 0.8, 0.7, 0.65}
	} else {
		look.FontSize = tokens.FontControl
		look.LineHeight = 1.55
		look.CodeLineHeight = 1.45
		look.CodeBlockPadding = 0.9
	}
	for i := range look.Headings {
		look.Headings[i].Size = sizes[i]
		look.Headings[i].Weight = tokens.WeightStrong
		look.Headings[i].Top = tops[i]
		look.Headings[i].Bottom = 0.35
	}
	return look
}

// selectedFace is behind the row or tab that is chosen: a whisper of the
// accent, so the choice reads by color and weight together, as a sidebar's
// selection does.
func selectedFace(t *ui.Theme) ui.Color {
	alpha := tokens.TintSelected
	if t.Dark {
		alpha = tokens.TintSelectedDark
	}
	return t.Accent.Alpha(alpha)
}
