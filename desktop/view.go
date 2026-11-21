package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

// railWidth is the width of the chat or contents rail.
const railWidth = 288

// view is the window: the sidebar of pages and AI chats, the page in the
// middle, and the rail of chat or contents on the right. The page is the
// document; everything else is quiet chrome around it.
func (a *app) view(c *ui.Context) {
	a.applyTheme(c)
	a.handleShortcuts(c)

	ui.Row(c).Fill().Background(c.Theme().Background).Children(func() {
		if a.sidebarShown {
			a.sidebarWidth = clamp(a.sidebarWidth, 208, 340)
			ui.Split(c, &a.sidebarWidth, func() { a.sidebar(c) }, func() { a.mainArea(c) }).Fill()
			return
		}
		a.mainArea(c)
	})
}

// mainArea is the page or the settings, with the rail when it fits.
func (a *app) mainArea(c *ui.Context) {
	width, _ := c.Size()
	showRail := a.railShown && width >= 1040
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Grow(1).FillHeight().MinWidth(0).Children(func() {
			a.header(c)
			switch a.mode {
			case viewSettings:
				a.settingsView(c)
			case viewChat:
				a.chatPage(c)
			default:
				a.pageView(c)
			}
		})
		if showRail {
			a.rail(c)
		}
	})
}

// header is the page's name in the workspace and the few actions the window
// has: the sidebar, contents, and whether the page is saved.
func (a *app) header(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(tokens.HeaderHeight).Padding(0, tokens.Space4).Gap(tokens.Space2).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		label := "Hide sidebar"
		if !a.sidebarShown {
			label = "Show sidebar"
		}
		if iconButton(c, "sidebar-toggle", icoSidebar, label, chromeIcon, false, true, true) {
			a.toggleSidebar()
		}
		if a.mode == viewSettings {
			if iconButton(c, "settings-back", icoChevronLeft, "Back to page", chromeIcon, false, true, true) {
				a.closeSettings()
			}
			ui.Text(c, "Settings").FontSize(tokens.FontBody).FontWeight(tokens.WeightStrong).TextColor(t.Text).SingleLine()
			ui.Spacer(c)
			return
		}
		if a.mode == viewChat {
			if iconButton(c, "chat-back", icoChevronLeft, "Back to page", chromeIcon, false, true, true) {
				a.closeChat()
			}
			ui.Text(c, chatTitle(a.chat)).FontSize(tokens.FontBody).
				FontWeight(tokens.WeightStrong).TextColor(t.Text).SingleLine().MaxWidth(280)
			// The header names the context the conversation reads; the chat
			// is not tied to a page, so there is no page to show otherwise.
			if refs := a.chatReferenceSummary(a.chat); refs != "" {
				ui.Icon(c, icoChevron).Size(12, 12).TextColor(t.TextMuted)
				ui.Text(c, refs).FontSize(tokens.FontBody).TextColor(t.TextMuted).
					SingleLine().MaxWidth(220)
			}
			ui.Spacer(c)
			if iconButton(c, "chat-page-new", icoPlus, "New chat", chromeIcon, false, true, true) {
				a.newChat()
			}
			return
		}
		a.breadcrumbs(c)
		ui.Spacer(c)
		a.saveStatus(c)
		// The rail's own tabs switch once it is open; the header keeps the
		// one way in while it is closed.
		if a.current != "" && !a.railShown {
			if iconButton(c, "outline-toggle", icoOutline, "Contents", chromeIcon, false, true, true) {
				a.toggleRail(railOutline)
			}
		}
	})
}

// breadcrumbs names the open page inside the workspace.
func (a *app) breadcrumbs(c *ui.Context) {
	t := c.Theme()
	root := filepath.Base(a.settings.Workspace)
	if root == "" || root == "." || root == "/" {
		root = "Bub"
	}
	ui.Text(c, root).FontSize(tokens.FontBody).TextColor(t.TextMuted).SingleLine()
	if dir := a.currentDir(); dir != "" {
		parts := strings.Split(dir, "/")
		if len(parts) > 2 {
			parts = parts[len(parts)-2:]
			parts[0] = "…"
		}
		for _, part := range parts {
			ui.Icon(c, icoChevron).Size(12, 12).TextColor(t.TextMuted)
			ui.Text(c, part).FontSize(tokens.FontBody).TextColor(t.TextMuted).SingleLine()
		}
	}
	ui.Icon(c, icoChevron).Size(12, 12).TextColor(t.TextMuted)
	title := a.documentTitle()
	if title == "" {
		title = untitledName
	}
	ui.Text(c, title).FontSize(tokens.FontBody).FontWeight(tokens.WeightStrong).TextColor(t.Text).SingleLine().MaxWidth(320)
}

// saveStatus says what the file on disk has, without a toast or a dialog. A
// saved page says nothing: only a write in progress or an unsaved edit does.
func (a *app) saveStatus(c *ui.Context) {
	if a.current == "" || (!a.saving && !a.dirty) {
		return
	}
	t := c.Theme()
	label := "Edited"
	if a.saving {
		label = "Saving…"
	}
	ui.Text(c, label).FontSize(tokens.FontMicro).TextColor(t.TextMuted).SingleLine()
	ui.Box(c).Width(4)
}

// pageView is the writing surface: one calm measure of text, the page's
// blocks, and the block chrome that appears at the caret or the pointer. The
// editor spans the whole main area and centers the measure inside itself, so
// its scroll bar sits at the window's edge instead of in the middle of the
// content.
func (a *app) pageView(c *ui.Context) {
	if a.ws != nil && a.current == "" {
		a.emptyState(c)
		return
	}
	t := c.Theme()
	blocks := a.blocks()
	boxes := a.doc.VisibleParagraphs()
	// The measure is the editor's own padding: the page area around the text,
	// at least the gutter the block chrome needs.
	windowW, _ := c.Size()
	areaW := windowW
	if a.sidebarShown {
		areaW -= a.sidebarWidth
	}
	if a.railShown && windowW >= 1040 {
		areaW -= railWidth
	}
	if bounds := c.Resolve(a.editorArea).Bounds().W; bounds > 0 {
		areaW = bounds
	}
	a.editorPadLeft = max(float32(editorPadX), (areaW-float32(a.settings.EditorWidth))/2)
	a.editorTextWidth = max(areaW-2*a.editorPadLeft, 240)
	look := a.editorLook(c, a.editorPadLeft)
	// The editor asks for the code paragraphs' spans as it builds, so the
	// frame's highlighting is ready first.
	spansVersion := a.beginCodeFrame(c, blocks)
	ui.Column(c).FillHeight().FillWidth().Children(func() {
		ui.Box(c).Fill().Bind(&a.editorArea).Children(func() {
			editor := richtext.Editor(c.Key("editor"), a.doc, richtext.EditorOptions{
				Label:        "Page",
				Placeholder:  "Start writing, or type / for blocks…",
				Font:         ui.Font{Size: float32(a.settings.EditorFontSize), LineHeight: 1.6},
				Theme:        look,
				Menu:         a.blockContextMenu,
				Spans:        a.codeSpans,
				SpansVersion: spansVersion,
			}).Fill().Background(t.Background).BorderWidth(0, 0, 0, 0).Radius(0).
				MinHeight(0).FocusRing(false).AutoFocus().Bind(&a.editor)
			if a.focusIn {
				a.editor.Focus()
				a.focusIn = false
			}
			if editor.Changed() {
				a.onEdit()
			}
			a.blockChrome(c, blocks, boxes)
			a.codeBlockFrames(c, blocks, boxes)
			a.codeBlockTools(c, blocks, boxes)
			a.slashMenu(c, blocks, boxes)
			a.popover(c, blocks, boxes)
		})
		if !a.railShown {
			a.askBubFAB(c)
		}
	})
}

// emptyState is the window before a page is open: one way to start, and
// nothing else.
func (a *app) emptyState(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(14).Children(func() {
		ui.Box(c).Size(48, 48).Radius(tokens.RadiusXL).Background(t.Accent.Alpha(0.12)).AlignItems(ui.Center).Justify(ui.Center).
			Children(func() {
				ui.Icon(c, icoSparkle).Size(24, 24).TextColor(t.Accent)
			})
		ui.Text(c, "A quiet place to write").FontSize(tokens.FontPage + 1).FontWeight(tokens.WeightStrong).TextColor(t.Text)
		ui.Box(c).Height(6)
		if ui.PrimaryButton(c, "New page").Key("empty-new-page").Clicked() {
			a.newPage()
		}
	})
}

// askBubFAB is the floating way into the AI chat in the page's corner: the one
// accent object in the writing view, gone while the rail is open.
func (a *app) askBubFAB(c *ui.Context) {
	t := c.Theme()
	b := ui.Row(c).Key("ask-bub-fab").Absolute().Right(24).Bottom(20).Height(36).
		Padding(0, 14).Radius(tokens.RadiusPill).Gap(7).AlignItems(ui.Center).
		Background(t.Accent).Shadow(0, 8, 22, -10, ui.RGBA(0, 0, 0, 0.30)).Focusable()
	switch {
	case b.Pressed():
		b.Background(t.AccentPressed)
	case b.Hovered():
		b.Background(t.AccentHover)
	}
	if b.Clicked() {
		a.toggleRail(railChat)
	}
	b.Children(func() {
		ui.Icon(c, icoSparkle).Size(15, 15).TextColor(t.AccentText)
		ui.Text(c, "Ask Bub").FontSize(tokens.FontBody).FontWeight(tokens.WeightMedium).TextColor(t.AccentText).SingleLine()
	})
	b.Label("Ask Bub")
}

// rail is the right side of the page: the AI chat or the page's contents.
func (a *app) rail(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(railWidth).FillHeight().Background(t.Surface).
		BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Row(c).Height(tokens.HeaderHeight).Padding(0, 10).Gap(2).AlignItems(ui.Center).Children(func() {
			a.railTabButton(c, "Chat", icoChat, railChat)
			a.railTabButton(c, "Contents", icoOutline, railOutline)
			ui.Spacer(c)
			if iconButton(c, "rail-close", icoClose, "Close panel", chromeIcon, false, true, true) {
				a.railShown = false
			}
		})
		if a.railTab == railOutline {
			a.outlinePanel(c)
			return
		}
		a.chatPanel(c)
	})
}

// railTabButton is one tab in the rail's header.
func (a *app) railTabButton(c *ui.Context, label string, ic *ui.SVG, tab railTab) {
	t := c.Theme()
	active := a.railTab == tab
	b := ui.Row(c).Height(tokens.ControlHeight).Padding(0, 9).Radius(tokens.RadiusSM).Gap(6).AlignItems(ui.Center).
		Focusable()
	switch {
	case active:
		b.Background(selectedFace(t))
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	if b.Clicked() {
		a.railTab = tab
	}
	b.Children(func() {
		color := t.TextMuted
		if active {
			color = t.Text
		}
		ui.Icon(c, ic).Size(14, 14).TextColor(color)
		ui.Text(c, label).FontSize(tokens.FontControl).FontWeight(tokens.WeightMedium).TextColor(color).SingleLine()
	})
	b.Label(label)
}

// outlinePanel lists the page's headings and moves the caret to one.
func (a *app) outlinePanel(c *ui.Context) {
	t := c.Theme()
	entries := a.outline()
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).Padding(12, 8).Gap(1).Children(func() {
			if len(entries) == 0 {
				ui.Text(c, "No headings yet. Type # to make one.").FontSize(tokens.FontCaption).TextColor(t.TextMuted).
					Padding(4, 4).LineHeight(1.5)
				return
			}
			for _, entry := range entries {
				entry := entry
				row := ui.Row(c).Key("outline-"+entry.BlockID).Height(28).Padding(0, 4).
					PaddingStart(4 + 12*float32(entry.Level-1)).Radius(tokens.RadiusSM).AlignItems(ui.Center).Gap(6).
					Focusable()
				if row.Hovered() {
					row.Background(t.SurfaceHover)
				}
				if row.Clicked() {
					a.doc.SetSelection(entry.Start, entry.Start)
					a.focusIn = true
				}
				row.Children(func() {
					ui.Text(c, entry.Text).FontSize(tokens.FontControl).TextColor(t.Text).SingleLine()
				})
				row.Label(entry.Text)
			}
		})
	})
}

// --- helpers --------------------------------------------------------------

// iconStyle is how an icon button looks where it sits: square buttons in the
// chrome, a compact one in a section header, and a labelled one under an
// answer in the transcript.
type iconStyle struct {
	// Size is a square button's side; Height is a labelled button's.
	Size, Height float32
	// Glyph is the side of the icon.
	Glyph float32
	// Padded is the horizontal padding of a labelled button, Radius its
	// corner; a square button always uses RadiusSM.
	Padded, Radius float32
	// ShowLabel shows the label beside the glyph, which squares do not.
	ShowLabel bool
	// HoverRead asks for the hover face only after asking the platform
	// whether it hovers, as a section header's quiet button does.
	HoverRead bool
}

var (
	// chromeIcon is the window chrome's square icon button.
	chromeIcon = iconStyle{Size: tokens.ControlHeight, Glyph: 16}
	// railIcon is the rail header's, a little smaller, and in the accent
	// while its popover is open.
	railIcon = iconStyle{Size: 26, Glyph: 14}
	// headerIcon is a sidebar section header's compact one.
	headerIcon = iconStyle{Size: 22, Glyph: 14, HoverRead: true}
	// answerIcon is a transcript action: a glyph and its label, kept built
	// but out of the way until the answer is hovered.
	answerIcon = iconStyle{Height: 22, Glyph: 12, Padded: 6, Radius: tokens.RadiusXS, ShowLabel: true}
)

// iconButton draws one icon button of style. A square button reports a click;
// a labelled one only while it is shown. enabled false keeps a square button
// from acting, dimmed, as the rail's do.
func iconButton(c *ui.Context, key any, ic *ui.SVG, label string, style iconStyle, active, enabled, visible bool) bool {
	t := c.Theme()
	var b ui.Element
	if style.ShowLabel {
		b = ui.Row(c).Key(key).Height(style.Height).Padding(0, style.Padded).
			Radius(style.Radius).Gap(4).AlignItems(ui.Center).Focusable()
	} else {
		b = ui.Box(c).Key(key).Size(style.Size, style.Size).Radius(tokens.RadiusSM).
			AlignItems(ui.Center).Justify(ui.Center).Focusable()
		if !enabled {
			b.Disabled(true)
		}
	}
	switch {
	case !enabled:
		// Disabled above, and never in the accent.
	case active:
		b.Background(t.Accent.Alpha(0.14))
	case style.HoverRead && !c.Preferences().Hover():
		// Hover is read on request only: the quiet header button.
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	if style.ShowLabel && !visible {
		b.Opacity(0)
		b.IgnoreInput(true)
	}
	clicked := b.Clicked()
	color := t.TextMuted
	if active && enabled && !style.ShowLabel {
		color = t.Accent
	}
	b.Children(func() {
		ui.Icon(c, ic).Size(style.Glyph, style.Glyph).TextColor(color)
		if style.ShowLabel {
			ui.Text(c, label).FontSize(tokens.FontMicro).TextColor(color)
		}
	})
	b.Label(label)
	return clicked && enabled && (visible || !style.ShowLabel)
}

// keycap shows a shortcut as a row of small key badges, as a shortcut legend
// does: one badge per key, the platform's glyphs as the menu writes them.
func keycap(c *ui.Context, keys string) ui.Element {
	t := c.Theme()
	return ui.Row(c).Gap(3).AlignItems(ui.Center).Label(keys).Children(func() {
		for _, key := range keys {
			ui.Box(c).Height(19).Padding(0, 5).Radius(tokens.RadiusXS).Background(t.Background).
				Border(1, t.Border).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Text(c, string(key)).FontSize(tokens.FontMicro).FontWeight(tokens.WeightMedium).TextColor(t.TextMuted).
					SingleLine()
			})
		}
	})
}

// shortTime names when a chat was last used, as a sidebar says it.
func shortTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

// handleShortcuts sets the window-wide keys the menus also offer.
func (a *app) handleShortcuts(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyK, func() { a.focusSearch() })
	c.OnShortcut(ui.Cmd, ui.KeyJ, func() { a.toggleRail(railChat) })
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyO, func() { a.toggleRail(railOutline) })
	c.OnShortcut(ui.Cmd, ui.KeyBackslash, func() { a.toggleSidebar() })
	c.OnShortcut(ui.Cmd, ui.KeyComma, func() { a.showSettings() })
}

// focusSearch opens the sidebar when it is hidden and puts the caret in its
// search field: the ⌘K every page list has.
func (a *app) focusSearch() {
	if !a.sidebarShown {
		a.sidebarShown = true
	}
	a.searchField.Focus()
}
