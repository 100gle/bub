package main

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/richtext"

	"github.com/bubbuild/bub/desktop/internal/block"
)

// --- Block chrome ---------------------------------------------------------

// The writing column's room around the text, in DIPs: the gutter takes the
// left, the right balances it, and the top keeps the title off the header.
const (
	editorPadX = 56
	editorPadY = 24
)

// slashState is the "/" menu: the block it will restyle, the "/query" range
// to take out when a type is chosen, and the search field's text and choice.
type slashState struct {
	blockID string
	start   int // rune offset of the "/" that opened the menu, or -1 from the gutter
	end     int // rune offset just after the query
	query   string
	sel     int
}

// dragState is the block being dragged, from its index.
type dragState struct {
	from int
	id   string
}

// onEdit runs after the writer changes the page: mark it edited, save soon,
// and notice a "/" that opens the block menu.
func (a *app) onEdit() {
	if !a.dirty {
		a.setDirty(true)
	}
	a.scheduleSave()
	a.detectSlash()
	a.updateTitle()
}

// detectSlash opens the block menu when the caret's block begins with "/",
// the gesture Notion taught people. The text stays in the page until a type
// is chosen, so dismissing the menu takes nothing away.
func (a *app) detectSlash() {
	if a.slash != nil {
		return
	}
	_, caret := a.doc.Selection()
	state, ok := block.SlashAt(a.doc.Text(), caret)
	if !ok {
		return
	}
	a.slash = &slashState{
		blockID: state.BlockID,
		start:   state.Start,
		end:     state.End,
		query:   state.Query,
	}
}

// openSlashAt opens the block menu for a block from the gutter, without text.
func (a *app) openSlashAt(b block.Block) {
	a.slash = &slashState{blockID: b.ID, start: -1, end: -1}
}

// closeSlash dismisses the block menu and gives the editor its focus back.
func (a *app) closeSlash() {
	a.slash = nil
	a.focusIn = true
}

// moveSlashSelection moves the highlighted command.
func (a *app) moveSlashSelection(delta int, n int) {
	if a.slash == nil || n == 0 {
		return
	}
	a.slash.sel = (a.slash.sel + delta + n) % n
}

// applySlash restyles the target block, as one undo step, taking the
// "/query" text out with the same step.
func (a *app) applySlash(cmd block.Command) {
	s := a.slash
	if s == nil {
		return
	}
	a.slash = nil
	if !block.Apply(a.doc, s.blockID, cmd.Kind, s.start, s.end) {
		a.focusIn = true
		return
	}
	a.doc.IdentifyParagraphs(a.newParagraphID)
	a.setDirty(true)
	a.scheduleSave()
	a.updateTitle()
	a.focusIn = true
}

// activeBlockIndex is the block the gutter serves: the one under the pointer,
// or else the caret's.
func (a *app) activeBlockIndex(c *ui.Context, blocks []block.Block, boxes []richtext.ParagraphBox) int {
	if len(blocks) == 0 {
		return -1
	}
	if _, y, over := c.Resolve(a.editorArea).PointerPosition(); over {
		for _, box := range boxes {
			if y >= box.Top-6 && y <= box.Top+box.Height+12 {
				for _, b := range blocks {
					if b.Index == box.Index {
						return b.Index
					}
				}
			}
		}
	}
	_, caret := a.doc.Selection()
	if b := a.blockAt(caret); b != nil {
		return b.Index
	}
	return -1
}

// codeBlockSpan is a code block in view: its paragraphs and the box of its
// first and last paragraph.
type codeBlockSpan struct {
	group       []block.Block
	first, last *richtext.ParagraphBox
}

// codeBlocksInView lists the code blocks whose every paragraph is on screen.
// A half-scrolled block is left out: framing it would put rounded corners in
// the middle of its panel.
func codeBlocksInView(blocks []block.Block, boxes []richtext.ParagraphBox) []codeBlockSpan {
	var out []codeBlockSpan
	for i := 0; i < len(blocks); {
		if !blocks[i].Style.CodeBlock {
			i++
			continue
		}
		start := i
		for i < len(blocks) && blocks[i].Style.CodeBlock {
			i++
		}
		group, end := blocks[start:i], i-1
		var first, last *richtext.ParagraphBox
		for k := range boxes {
			b := &boxes[k]
			if b.Index < blocks[start].Index || b.Index > blocks[end].Index {
				continue
			}
			if first == nil {
				first = b
			}
			last = b
		}
		if first == nil || first.Index != blocks[start].Index || last.Index != blocks[end].Index {
			continue
		}
		out = append(out, codeBlockSpan{group: group, first: first, last: last})
	}
	return out
}

// codeBlockPaddingEm is the room inside a code block, in ems: horizontal, then
// vertical. editorLook hands it to richtext, which paints the panel, and
// codeBlockPad lays the frame and the tools out from it, so the two agree.
var codeBlockPaddingEm = [2]float32{1.25, 2.0}

// codeBlockPad is how far the panel reaches past its first and last line, in
// DIPs: the horizontal inset of the frame, and the band above and below the
// code the tools sit in.
func (a *app) codeBlockPad() float32 {
	size := float32(a.settings.EditorFontSize)
	if size < 12 || size > 24 {
		size = 16
	}
	return size * codeBlockPaddingEm[1]
}

// codeBlockFrames draws a hairline around each code block in view. richtext
// paints the block's panel but no border, and without one the block reads as a
// gray patch rather than as code. The frame is painted over the editor and
// ignores input, so it frames the panel without taking a click from the text.
func (a *app) codeBlockFrames(c *ui.Context, blocks []block.Block, boxes []richtext.ParagraphBox) {
	if a.mode != viewPage || len(boxes) == 0 || a.editorTextWidth <= 0 {
		return
	}
	t := c.Theme()
	pad := a.codeBlockPad()
	for _, span := range codeBlocksInView(blocks, boxes) {
		ui.Box(c).Absolute().Left(a.editorPadLeft).Top(span.first.Top-pad).
			Width(a.editorTextWidth).Height(span.last.Top+span.last.Height-span.first.Top+2*pad).
			Radius(tokens.RadiusMD).Border(1, t.Border).IgnoreInput(true)
	}
}

// blockChrome draws the gutter beside the block at the pointer or caret: the
// plus that opens the block menu and the grip that drags the block. While a
// block is dragged, drop zones between the visible blocks take it.
func (a *app) blockChrome(c *ui.Context, blocks []block.Block, boxes []richtext.ParagraphBox) {
	if len(blocks) == 0 || a.mode != viewPage {
		return
	}
	t := c.Theme()
	active := a.activeBlockIndex(c, blocks, boxes)
	// Keep the dragged block's handle alive while the pointer is away from
	// the editor, so the drag can end cleanly.
	if a.drag != nil {
		active = a.drag.from
	}
	var box *richtext.ParagraphBox
	for i := range boxes {
		if boxes[i].Index == active {
			box = &boxes[i]
			break
		}
	}
	dragging := a.drag != nil
	dropped := false

	// Drop zones while a block is dragged: before each visible block and after
	// the last one, so a block can go anywhere on the page.
	if dragging {
		for i, b := range blocks {
			if len(boxes) == 0 {
				break
			}
			// A code block is several paragraphs in the text, but one block
			// to the writer: dropping between its lines would split it.
			if i > 0 && i < len(blocks) && blocks[i-1].Style.CodeBlock && b.Style.CodeBlock {
				continue
			}
			if !boxVisible(boxes, b.Index) {
				continue
			}
			top := float32(-1000)
			for _, bx := range boxes {
				if bx.Index == b.Index {
					top = bx.Top - 3
					break
				}
			}
			if a.dropZone(c, top, 14, i) {
				dropped = true
			}
		}
		last := boxes[len(boxes)-1]
		lastTop := last.Top + last.Height + 3
		// The rest of the editor below the last block is one long target, so
		// letting go in the empty page appends the block instead of missing.
		height := float32(14)
		if h := c.Resolve(a.editor).Bounds().H; h > 0 {
			if rest := h - lastTop; rest > height {
				height = rest
			}
		}
		if a.dropZone(c, lastTop, height, len(blocks)) {
			dropped = true
		}
	}

	if active < 0 || box == nil {
		if a.drag != nil && !dropped {
			a.drag = nil
		}
		return
	}
	// Notion-like: chrome shows at the caret and at the pointer, never for
	// every block at once.
	focused := a.editor.Focused(c)
	_, _, over := c.Resolve(a.editorArea).PointerPosition()
	if !focused && !over && !dragging {
		return
	}
	top := box.Top + (box.Height-24)/2
	if top < 2 {
		top = 2
	}

	plus := ui.Box(c).Key("block-plus").Absolute().Left(a.editorPadLeft-48).Top(top).Size(20, 24).
		Radius(tokens.RadiusXS).AlignItems(ui.Center).Justify(ui.Center).Focusable()
	if plus.Hovered() {
		plus.Background(t.SurfaceHover)
	}
	if plus.Clicked() {
		if b := a.blockByID(blockIDAt(blocks, active)); b != nil {
			a.openSlashAt(*b)
		}
	}
	plus.Children(func() {
		ui.Icon(c, icoPlus).Size(14, 14).TextColor(t.TextMuted)
	})
	plus.Label("Add block")

	drag := ui.Box(c).Key("block-drag").Absolute().Left(a.editorPadLeft-28).Top(top).Size(20, 24).
		Radius(tokens.RadiusXS).AlignItems(ui.Center).Justify(ui.Center).Focusable()
	if drag.Hovered() || dragging {
		drag.Background(t.SurfaceHover)
	}
	dragDrag := blockDrag{index: active, id: blockIDAt(blocks, active)}
	drag = drag.Drag(dragDrag)
	if drag.Dragging() {
		if a.drag == nil && dragDrag.id != "" {
			a.drag = &dragState{from: active, id: dragDrag.id}
		}
	} else if a.drag != nil && !dropped {
		a.drag = nil
	}
	drag.Children(func() {
		ui.Icon(c, icoGrip).Size(14, 14).TextColor(t.TextMuted)
	})
	drag.Label("Drag block")

	_ = dragging
}

// blockDrag is the value a block handle drags.
type blockDrag struct {
	index int
	id    string
}

// dropZone is one boundary between blocks: it shows a line while a block is
// over it and moves the block there when it is dropped.
func (a *app) dropZone(c *ui.Context, top, height float32, insert int) bool {
	t := c.Theme()
	if height < 12 {
		height = 12
	}
	zone := ui.Row(c).Key("drop-" + itoa(insert)).Absolute().Left(8).Right(8).Top(top).Height(height).
		AlignItems(ui.Center)
	if _, over := ui.DragOver[blockDrag](zone); over {
		zone.Children(func() {
			ui.Box(c).Height(2).FillWidth().Radius(1).Background(t.Accent)
		})
	}
	if d, ok := ui.Drop[blockDrag](zone); ok {
		a.moveBlock(d.index, insert)
		a.drag = nil
		return true
	}
	return false
}

// itoa names an element key from an index.
func itoa(n int) string { return strconv.Itoa(n) }

// moveBlock moves a block before the block at insert, as one undo step, and
// keeps its ID and style.
func (a *app) moveBlock(from, insert int) {
	if !block.Move(a.doc, from, insert) {
		return
	}
	a.doc.IdentifyParagraphs(a.newParagraphID)
	a.setDirty(true)
	a.scheduleSave()
	a.updateTitle()
}

// blockIDAt is the ID of the block with an index.
func blockIDAt(blocks []block.Block, index int) string {
	for _, b := range blocks {
		if b.Index == index {
			return b.ID
		}
	}
	return ""
}

// Box helpers for the visible paragraph boxes.
func boxVisible(boxes []richtext.ParagraphBox, index int) bool {
	for _, box := range boxes {
		if box.Index == index {
			return true
		}
	}
	return false
}

// outline lists the page's headings, in order.
func (a *app) outline() []block.OutlineEntry {
	return block.Outline(a.doc.Text())
}

// focusTitle selects the first heading of a new page, so typing names it.
func (a *app) focusTitle() {
	blocks := a.blocks()
	if len(blocks) == 0 {
		return
	}
	title := blocks[0]
	if title.Style.Heading == 0 {
		return
	}
	a.doc.SetSelection(title.Start, title.End)
	a.focusIn = true
}

// --- "/" menu and popovers ------------------------------------------------

// slashMenu is the block menu: a search field, the matching block types, and
// a quiet hint of the key that applies them.
func (a *app) slashMenu(c *ui.Context, blocks []block.Block, boxes []richtext.ParagraphBox) {
	s := a.slash
	if s == nil {
		return
	}
	t := c.Theme()
	matches := block.FilterCommands(s.query)
	if s.sel >= len(matches) {
		s.sel = 0
	}
	width := float32(300)
	height := 40 + 32*float32(len(matches)) + 10
	areaH := float32(480)
	if bounds := a.editor.Bounds(c); bounds.W > 0 {
		areaH = bounds.H
	}
	left := clamp(float32(0), 0, max(0, a.editorTextWidth-width-8)) + a.editorPadLeft
	top := float32(12)
	if b := a.blockByID(s.blockID); b != nil {
		if box, ok := boxForID(blocks, boxes, b.ID); ok {
			below := box.Top + box.Height + 6
			above := box.Top - height - 6
			switch {
			case below+height <= areaH-8:
				top = below
			case above >= 8:
				top = above
			default:
				top = clamp(below, 8, max(8, areaH-height-8))
			}
		}
	}
	top = clamp(top, 8, max(8, areaH-height-8))
	panel := ui.Column(c).Key("slash-menu").Absolute().Left(left).Top(top).Width(width).
		Padding(6).Radius(tokens.RadiusMD).Background(t.Background).Border(1, t.Border).
		Shadow(0, 10, 28, -10, ui.RGBA(0, 0, 0, 0.24)).Gap(4)
	panel.Children(func() {
		ui.TextInput(c.Key("slash-query"), &s.query).Placeholder("Filter blocks").Label("Filter blocks").
			FontSize(13).AutoFocus().Padding(4, 8).
			OnTextKey(func(k ui.TextKey) bool {
				switch k.Key {
				case ui.KeyUp:
					a.moveSlashSelection(-1, len(matches))
					return true
				case ui.KeyDown:
					a.moveSlashSelection(1, len(matches))
					return true
				case ui.KeyEnter:
					if len(matches) > 0 {
						a.applySlash(matches[s.sel])
					}
					return true
				case ui.KeyEscape:
					a.closeSlash()
					return true
				}
				return false
			})
		if len(matches) == 0 {
			ui.Text(c, "No blocks match that.").FontSize(tokens.FontCaption).TextColor(t.TextMuted).Padding(6, 8)
		}
		for i, cmd := range matches {
			i, cmd := i, cmd
			row := ui.Row(c).Key("slash-"+itoa(i)).Height(32).Padding(0, 8).Radius(tokens.RadiusSM).
				AlignItems(ui.Center).Gap(9).Focusable()
			if i == s.sel {
				row.Background(t.Accent.Alpha(0.12))
			} else if row.Hovered() {
				row.Background(t.SurfaceHover)
			}
			if row.Clicked() {
				a.applySlash(cmd)
			}
			row.Children(func() {
				ui.Box(c).Size(22, 22).Radius(tokens.RadiusXS).Background(t.Surface).AlignItems(ui.Center).
					Justify(ui.Center).Children(func() {
					hint := cmd.Hint
					if hint == "" {
						hint = "¶"
					}
					ui.Text(c, hint).FontSize(10).FontWeight(500).TextColor(t.TextMuted)
				})
				ui.Text(c, cmd.Label).FontSize(tokens.FontControl).TextColor(t.Text)
			})
			row.Label(cmd.Label)
		}
	})
	if panel.PressedOutside() {
		a.closeSlash()
	}
	// Escape closes the menu even when the field does not have the focus.
	c.OnShortcut(0, ui.KeyEscape, func() { a.closeSlash() })
}

// --- Suggestion popover ---------------------------------------------------

// popover places the suggestion or the Ask input near what it belongs to.
func (a *app) popover(c *ui.Context, blocks []block.Block, boxes []richtext.ParagraphBox) {
	if a.suggestion == nil && a.ask == nil {
		return
	}
	if a.suggestion != nil {
		c.OnShortcut(ui.Cmd, ui.KeyEnter, func() { a.applySuggestion() })
	}
	if a.suggestion != nil || a.ask != nil {
		c.OnShortcut(0, ui.KeyEscape, func() { a.dismissPopovers() })
	}
	areaHeight := float32(480)
	if bounds := a.editor.Bounds(c); bounds.W > 0 {
		areaHeight = bounds.H
	}
	areaWidth := a.editorTextWidth
	if areaWidth <= 0 {
		areaWidth = 640
	}
	width := float32(460)
	height := a.popoverHeight()
	top, left := float32(8), float32(0)

	targetID := ""
	if a.suggestion != nil {
		targetID = a.suggestion.action.target.ID
	}
	if a.ask != nil {
		targetID = a.ask.target.ID
	}
	if a.ask != nil {
		width = 380
	}
	if box, ok := boxForID(blocks, boxes, targetID); ok {
		below := box.Top + box.Height + 6
		above := box.Top - height - 6
		switch {
		case below+height <= areaHeight-8:
			top = below
		case above >= 8:
			top = above
		default:
			top = clamp(below, 8, max(8, areaHeight-height-8))
		}
	}
	top = clamp(top, 8, max(8, areaHeight-height-8))
	left = a.editorPadLeft + clamp(left, 0, max(0, areaWidth-width-8))
	if a.ask != nil {
		a.askPopover(c, top, left, width)
		return
	}
	a.suggestionPopover(c, top, left, width)
}

// suggestionPopover is the smallest review surface: the proposed text and
// three plain actions, or what the agent is doing and Stop.
func (a *app) suggestionPopover(c *ui.Context, top, left, width float32) {
	s := a.suggestion
	t := c.Theme()
	stale := a.suggestionStale()
	ui.Column(c).Absolute().Left(left).Top(top).Width(width).Padding(10, 12).Gap(8).
		Radius(tokens.RadiusMD).Background(t.Background).Border(1, t.Border).
		Shadow(0, 6, 18, -8, ui.RGBA(0, 0, 0, 0.18)).Children(func() {
		if s.status.busy() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Bub is writing…").FontSize(12).TextColor(t.TextMuted)
				ui.Spacer(c)
				if ui.Button(c, "Stop").Key("ai-stop").FontSize(12).BorderWidth(0).
					Background(ui.Color{}).TextColor(t.TextMuted).Clicked() {
					a.dismissPopovers()
				}
			})
			return
		}
		ui.Text(c, s.text).FontSize(15).LineHeight(1.55).TextColor(t.Text)
		if stale {
			ui.Text(c, "The block changed since this was written.").FontSize(11).TextColor(t.Warning)
		}
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Spacer(c)
			accept := ui.Button(c, "Accept").Key("ai-accept").FontSize(12).BorderWidth(0).
				Background(ui.Color{}).TextColor(t.Accent).FontWeight(500).
				Disabled(stale || strings.TrimSpace(s.text) == "")
			if accept.Clicked() {
				a.applySuggestion()
			}
			if ui.Button(c, "Dismiss").Key("ai-dismiss").FontSize(12).BorderWidth(0).
				Background(ui.Color{}).TextColor(t.TextMuted).Clicked() {
				a.dismissPopovers()
			}
			if ui.Button(c, "Regenerate").Key("ai-regenerate").FontSize(12).BorderWidth(0).
				Background(ui.Color{}).TextColor(t.TextMuted).Clicked() {
				a.regenerate()
			}
		})
	})
}

// askPopover is one line of instruction in the same minimal surface.
func (a *app) askPopover(c *ui.Context, top, left, width float32) {
	t := c.Theme()
	ui.Column(c).Absolute().Left(left).Top(top).Width(width).Padding(6, 8).
		Radius(tokens.RadiusMD).Background(t.Background).Border(1, t.Border).
		Shadow(0, 6, 18, -8, ui.RGBA(0, 0, 0, 0.18)).Children(func() {
		ui.TextInput(c.Key("ai-instruction"), &a.ask.instruction).
			Placeholder("Ask Bub…").Label("Ask Bub").FontSize(14).AutoFocus().
			OnTextKey(func(k ui.TextKey) bool {
				switch {
				case k.Key == ui.KeyEnter && k.Mods == 0:
					a.runAsk()
					return true
				case k.Key == ui.KeyEscape:
					a.closeAsk()
					return true
				}
				return false
			})
	})
}

// popoverHeight estimates the popover's height for placement.
func (a *app) popoverHeight() float32 {
	if a.ask != nil {
		return 46
	}
	s := a.suggestion
	if s == nil {
		return 0
	}
	if s.status.busy() {
		return 44
	}
	lines := float32(len([]rune(s.text)))/58 + 1
	body := lines * 23
	if body < 23 {
		body = 23
	}
	if body > 200 {
		body = 200
	}
	return 20 + body + 30
}

// boxForID finds the visible box of the paragraph with an ID.
func boxForID(blocks []block.Block, boxes []richtext.ParagraphBox, id string) (richtext.ParagraphBox, bool) {
	for _, b := range blocks {
		if b.ID != id {
			continue
		}
		for _, box := range boxes {
			if box.Index == b.Index {
				return box, true
			}
		}
		return richtext.ParagraphBox{}, false
	}
	return richtext.ParagraphBox{}, false
}

func clamp(v, lo, hi float32) float32 {
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// codeBlockTools draws each code block's tools in its top right corner: the
// language its fence names, which opens the languages this editor highlights,
// and a button that copies the block. They sit on the panel's own surface, so
// they read as part of the block rather than as chrome over it, and take the
// clicks the frame ignores.
func (a *app) codeBlockTools(c *ui.Context, blocks []block.Block, boxes []richtext.ParagraphBox) {
	if a.mode != viewPage || len(boxes) == 0 || a.editorTextWidth <= 0 {
		return
	}
	_, pointerY, over := c.Resolve(a.editorArea).PointerPosition()
	for _, span := range codeBlocksInView(blocks, boxes) {
		span := span
		id := span.group[0].ID
		pad := a.codeBlockPad()
		topEdge := span.first.Top - pad
		bottomEdge := span.last.Top + span.last.Height + pad
		hovered := over && pointerY >= topEdge && pointerY <= bottomEdge
		shown := hovered || a.codeMenu == id || a.codeShowAll
		// The band is a box, as the block gutter's buttons are: a row placed
		// absolutely would be offset by the blocks above it.
		band := ui.Box(c).Key("code-tools-" + id).Absolute().
			Left(a.editorPadLeft).Top(topEdge + (pad-codeToolHeight)/2).
			Width(a.editorTextWidth).Height(codeToolHeight).Focusable()
		if !shown {
			// An idle page of code reads as code: the tools are not in the
			// build at all, so nothing of them is drawn, announced or hit.
			continue
		}
		band.Children(func() {
			ui.Row(c).FillWidth().Height(codeToolHeight).AlignItems(ui.Center).Children(func() {
				a.codeLanguageButton(c, id)
				ui.Spacer(c)
				a.codeCopyButton(c, id, codeText(span.group))
			})
		})
	}
}

// codeToolHeight is the height of a code block's tools, in DIPs.
const codeToolHeight float32 = 20

// codeLanguageButton is the language a code block's fence names, and the way
// to choose another. Until one is chosen the button says "Code": the block
// highlights only once it has a language.
func (a *app) codeLanguageButton(c *ui.Context, id string) {
	t := c.Theme()
	lang := a.codeLanguage(id)
	label := lang
	if label == "" {
		label = "Plain text"
	}
	open := a.codeMenu == id
	btn := ui.Row(c).Key("code-lang-"+id).Height(24).Padding(0, 8).Radius(tokens.RadiusSM).
		Gap(4).AlignItems(ui.Center).Focusable()
	switch {
	case open:
		btn.Background(t.Accent.Alpha(0.14))
	case btn.Hovered():
		btn.Background(t.SurfaceHover)
	}
	clicked := btn.Clicked()
	// The language is the block's state, so it reads at the text color; it
	// takes the accent while its menu is open, or under the pointer.
	color := t.Text
	switch {
	case open:
		color = t.Accent
	case btn.Hovered():
		color = t.Accent
	}
	btn.Children(func() {
		ui.Text(c, label).FontSize(tokens.FontCaption).FontWeight(tokens.WeightMedium).
			TextColor(color).SingleLine()
		ui.Icon(c, icoChevronDown).Size(11, 11).TextColor(color)
	})
	btn.Label("Code language: " + label)
	if clicked {
		if open {
			a.codeMenu = ""
		} else {
			a.codeMenu = id
		}
	}
	a.codeLanguageMenu(c, id, btn)
}

// codeLanguageMenu lists the languages the editor highlights, under the
// button that opened it.
func (a *app) codeLanguageMenu(c *ui.Context, id string, anchor ui.Element) {
	open := a.codeMenu == id
	if !open {
		return
	}
	t := c.Theme()
	languages := editor.Languages()
	ui.PopoverBase(c, anchor, &open, func(panel ui.Element) {
		panel.Key("code-lang-menu").AttachTo(anchor, ui.AnchorTopEnd, ui.AnchorBottomEnd).
			Margin(0, 0, 4, 0).MinWidth(176).MaxHeight(320).
			Padding(4).Gap(1).Radius(tokens.RadiusMD).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 12, 32, -12, ui.RGBA(0, 0, 0, 0.24))
		panel.Children(func() {
			ui.Scroll(c).MaxHeight(312).Children(func() {
				ui.Column(c).Gap(1).Children(func() {
					current := a.codeLanguage(id)
					a.codeLanguageChoice(c, id, "Plain text", "", current == "")
					for _, l := range languages {
						name := strings.ToLower(l.Name)
						a.codeLanguageChoice(c, id, l.Name, name, current == name)
					}
				})
			})
		})
	})
	if !open {
		a.codeMenu = ""
	}
}

// codeLanguageChoice is one row of the language menu.
func (a *app) codeLanguageChoice(c *ui.Context, id, label, value string, chosen bool) {
	t := c.Theme()
	row := ui.Row(c).Key("code-lang-pick-"+value).Height(28).Padding(0, 8).
		Radius(tokens.RadiusSM).Gap(8).AlignItems(ui.Center).Focusable()
	switch {
	case chosen:
		row.Background(t.Accent.Alpha(0.12))
	case row.Hovered():
		row.Background(t.SurfaceHover)
	}
	clicked := row.Clicked()
	row.Children(func() {
		ui.Text(c, label).FontSize(tokens.FontControl).TextColor(t.Text).SingleLine().Grow(1)
		if chosen {
			ui.Icon(c, icoCheck).Size(12, 12).TextColor(t.Accent)
		}
	})
	row.Label(label)
	if clicked {
		a.setCodeLanguage(id, value)
		a.codeMenu = ""
	}
}

// codeCopyButton copies the whole block, as code and never as rendered text.
// It says "Copied" for as long as the pointer stays on it, so the writer sees
// the copy without a notification.
func (a *app) codeCopyButton(c *ui.Context, id, code string) {
	t := c.Theme()
	btn := ui.Row(c).Key("code-copy-"+id).Height(24).Padding(0, 8).Radius(tokens.RadiusSM).
		Gap(4).AlignItems(ui.Center).Focusable()
	copied := a.codeCopied == id
	if copied && !btn.Hovered() {
		// The pointer left the button: the next visit starts from "Copy".
		a.codeCopied = ""
		copied = false
	}
	label := "Copy"
	if copied {
		label = "Copied"
	}
	if btn.Hovered() {
		btn.Background(t.SurfaceHover)
	}
	color := t.TextMuted
	if copied {
		color = t.Accent
	}
	clicked := btn.Clicked()
	btn.Children(func() {
		ic := icoCopy
		if copied {
			ic = icoCheck
		}
		ui.Icon(c, ic).Size(12, 12).TextColor(color)
		ui.Text(c, label).FontSize(tokens.FontCaption).TextColor(color).SingleLine()
	})
	btn.Label("Copy code")
	if clicked {
		if a.codeCopyHook != nil {
			a.codeCopyHook(code)
		} else {
			c.WriteClipboard(code)
		}
		a.codeCopied = id
	}
}
