package main

import "github.com/egoist/mygo/ui"

// The window's design tokens live in this one file: the palette of each
// appearance, the radii, the type scale, and the measures every surface builds
// from. A change here moves everywhere, so a taste decision is made once
// instead of at a dozen call sites.
//
// The radii are a scale, not a set of one-offs: RadiusXS for keycaps and
// chips, RadiusSM for rows, icon buttons and small controls, RadiusMD for
// fields and popovers, RadiusLG for panels, RadiusXL for the largest floating
// surfaces, and RadiusPill for a capsule. The scale is generous — 6 to 20
// DIPs — so every surface reads as a soft, modern object rather than a box.
// To round the whole window further or tighten it, change the scale below;
// ui.Theme.Radius (the radius built-in controls use) follows RadiusSM.
type designTokens struct {
	// Radii, in DIPs.
	RadiusXS, RadiusSM, RadiusMD, RadiusLG, RadiusXL, RadiusPill float32

	// Spacing, in DIPs: the 4 DIP unit and the steps that build on it.
	Space1, Space2, Space3, Space4, Space5, Space6 float32

	// Type sizes, in DIPs. The font family stays the system's: an empty
	// family lets Core Text resolve SF Pro with its optical sizes.
	FontMicro, FontCaption, FontControl, FontBody, FontPage float32

	// Font weights. Three steps only: regular is body and labels, medium is a
	// chosen row or a small control, strong is for titles alone. A label is
	// never strong.
	WeightRegular, WeightMedium, WeightStrong int

	// Control measures, in DIPs: the header band, a row or a control, and a
	// list row.
	HeaderHeight, ControlHeight, RowHeight float32

	// Tints over the accent for a selected row, as An Alpha; dark windows need
	// a little more of it to read the same.
	TintSelected, TintSelectedDark float32
}

// tokens are the window's measurements. Tune them here, not in the views.
var tokens = designTokens{
	RadiusXS:   6,
	RadiusSM:   8,
	RadiusMD:   12,
	RadiusLG:   16,
	RadiusXL:   20,
	RadiusPill: 999,

	Space1: 4,
	Space2: 8,
	Space3: 12,
	Space4: 16,
	Space5: 24,
	Space6: 32,

	FontMicro:   10.5,
	FontCaption: 12,
	FontControl: 12.5,
	FontBody:    13,
	FontPage:    16,

	WeightRegular: 400,
	WeightMedium:  500,
	WeightStrong:  600,

	HeaderHeight:  48,
	ControlHeight: 30,
	RowHeight:     32,

	TintSelected:     0.12,
	TintSelectedDark: 0.20,
}

// palette is the semantic color of one appearance. Views ask for a role, never
// for a hex value, so the light and dark worlds stay the same design.
type palette struct {
	Background     ui.Color
	Surface        ui.Color
	SurfaceHover   ui.Color
	SurfacePressed ui.Color
	Border         ui.Color
	Text           ui.Color
	TextMuted      ui.Color
	Accent         ui.Color
	AccentHover    ui.Color
	AccentPressed  ui.Color
	AccentText     ui.Color
	Selection      ui.Color
	Focus          ui.Color
}

// lightPalette is the light window: white paper, a cool gray-blue chrome, and
// Bub's sky blue only where something is chosen or alive. The neutrals carry a
// whisper of the accent's hue, so the window reads as one material; the accent
// is a deep sky blue (4.9:1 on white) rather than a pale one, so white text and
// icons on it stay legible.
func lightPalette() palette {
	return palette{
		Background:     ui.Hex("#ffffff"),
		Surface:        ui.Hex("#f4f8fc"),
		SurfaceHover:   ui.Hex("#e9f1f8"),
		SurfacePressed: ui.Hex("#dce8f3"),
		Border:         ui.Hex("#e0e8f0"),
		Text:           ui.Hex("#0e1922"),
		TextMuted:      ui.Hex("#5f7183"),
		Accent:         ui.Hex("#0077b6"),
		AccentHover:    ui.Hex("#016aa1"),
		AccentPressed:  ui.Hex("#005c8b"),
		AccentText:     ui.Hex("#ffffff"),
		Selection:      ui.Hex("#0077b6").Alpha(0.16),
		Focus:          ui.Hex("#0077b6").Alpha(0.45),
	}
}

// darkPalette is the dark window, the same roles in a dim room: the sky steps
// up to its vivid light tone and the text on it turns navy, as a lit surface
// wants, while the chrome keeps the accent's blue in its near-blacks.
func darkPalette() palette {
	return palette{
		Background:     ui.Hex("#0b1117"),
		Surface:        ui.Hex("#121b23"),
		SurfaceHover:   ui.Hex("#182531"),
		SurfacePressed: ui.Hex("#203243"),
		Border:         ui.Hex("#25384a"),
		Text:           ui.Hex("#e8f0f7"),
		TextMuted:      ui.Hex("#8ea3b6"),
		Accent:         ui.Hex("#38bdf8"),
		AccentHover:    ui.Hex("#5ac8fa"),
		AccentPressed:  ui.Hex("#1fa9e8"),
		AccentText:     ui.Hex("#082f49"),
		Selection:      ui.Hex("#38bdf8").Alpha(0.30),
		Focus:          ui.Hex("#38bdf8").Alpha(0.55),
	}
}

// paletteFor is the palette of an appearance.
func paletteFor(dark bool) palette {
	if dark {
		return darkPalette()
	}
	return lightPalette()
}
