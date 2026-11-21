package main

import "github.com/egoist/mygo/ui"

// The app's icons are Lucide outlines, parsed once at start and drawn in the
// text color where they appear. One icon per meaning, one library across the
// window. The hairline stroke is a little lighter than Lucide's default, as
// chrome type is quieter than the page.
func icon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

// iconFilled is for glyphs that read as a solid mark, such as the grip and
// the stop square.
func iconFilled(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="currentColor" stroke="none">` + body + `</svg>`))
}

var (
	icoSparkle     = icon(`<path d="M12 3l1.9 5.6L19.5 10l-5.6 1.9L12 17.5l-1.9-5.6L4.5 10l5.6-1.4z"/><path d="M19 15.5l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8z"/>`)
	icoPlus        = icon(`<path d="M12 5v14"/><path d="M5 12h14"/>`)
	icoChevron     = icon(`<path d="m9 18 6-6-6-6"/>`)
	icoChevronDown = icon(`<path d="m6 9 6 6 6-6"/>`)
	icoChevronLeft = icon(`<path d="m15 18-6-6 6-6"/>`)
	icoFile        = icon(`<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M9 13h6"/><path d="M9 17h6"/>`)
	icoFolder      = icon(`<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>`)
	icoChat        = icon(`<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>`)
	icoSettings    = icon(`<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>`)
	icoOutline     = icon(`<path d="M8 6h13"/><path d="M8 12h13"/><path d="M8 18h13"/><path d="M3 6h.01"/><path d="M3 12h.01"/><path d="M3 18h.01"/>`)
	icoSidebar     = icon(`<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M9 3v18"/>`)
	icoGrip        = iconFilled(`<circle cx="9" cy="5" r="1.6"/><circle cx="9" cy="12" r="1.6"/><circle cx="9" cy="19" r="1.6"/><circle cx="15" cy="5" r="1.6"/><circle cx="15" cy="12" r="1.6"/><circle cx="15" cy="19" r="1.6"/>`)
	icoClose       = icon(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	icoMore        = iconFilled(`<circle cx="5" cy="12" r="1.7"/><circle cx="12" cy="12" r="1.7"/><circle cx="19" cy="12" r="1.7"/>`)
	icoTrash       = icon(`<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>`)
	icoRename      = icon(`<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>`)
	icoSearch      = icon(`<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>`)
	icoStop        = iconFilled(`<rect x="6" y="6" width="12" height="12" rx="2"/>`)
	icoSend        = icon(`<path d="M12 19V5"/><path d="m5 12 7-7 7 7"/>`)
	icoTerminal    = icon(`<path d="m4 17 6-6-6-6"/><path d="M12 19h8"/>`)
	icoRefresh     = icon(`<path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/>`)
	icoCopy        = icon(`<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>`)
	icoInsert      = icon(`<path d="M12 5v14"/><path d="m19 12-7 7-7-7"/>`)
	// icoModel names a choice among models, icoEffort the thinking level:
	// Lucide's boxes and brain-circuit, which read at control size.
	icoModel      = icon(`<path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"/><path d="m3.3 7 8.7 5 8.7-5"/><path d="M12 22V12"/>`)
	icoEffort     = icon(`<path d="M12 5a3 3 0 1 0-5.997.125 4 4 0 0 0-2.526 5.77 4 4 0 0 0 .556 6.588A4 4 0 1 0 12 18Z"/><path d="M9 13a4.5 4.5 0 0 0 3-4"/><path d="M6.003 5.125A3 3 0 0 0 6.401 6.5"/><path d="M3.477 10.896a4 4 0 0 1 .585-.396"/><path d="M6 18a4 4 0 0 1-1.967-.516"/><path d="M19.5 13a4.5 4.5 0 0 0-2.5-4"/><path d="M20.477 10.896a4 4 0 0 0-.585-.396"/><path d="M18 18a4 4 0 0 0 1.967-.516"/><path d="M15 18a3 3 0 0 0 .002-5.889"/>`)
	icoCheck      = icon(`<path d="M20 6 9 17l-5-5"/>`)
	icoFolderOpen = icon(`<path d="m6 14 1.5-3a2 2 0 0 1 1.8-1H21a1 1 0 0 1 .9 1.4l-1.7 4.1a2 2 0 0 1-1.9 1.2H4a1 1 0 0 1-.9-1.4l1.7-4.1A2 2 0 0 1 6.7 10H20"/><path d="M4 14V5a2 2 0 0 1 2-2h4l2 2h6a2 2 0 0 1 2 2v2"/>`)
)
