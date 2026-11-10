package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

// view builds the window: a quiet header, the writing column with its block
// gutter and AI surfaces, and a footer of keys. There is no chat.
func (a *app) view(c *ui.Context) {
	c.SetTheme(a.theme)
	t := c.Theme()

	if c.Shortcut(ui.Cmd, ui.KeyN) {
		a.newAsync()
	}
	if c.Shortcut(ui.Cmd, ui.KeyO) {
		a.openAsync()
	}
	if c.Shortcut(ui.Cmd, ui.KeyS) {
		a.saveAsync()
	}
	// The AI surfaces take Escape, ⌘Return and ⌘R wherever the focus is, so
	// they work while a card button has it too.
	if a.suggestion != nil {
		c.OnShortcut(ui.Cmd, ui.KeyEnter, func() { a.applySuggestion() })
		c.OnShortcut(ui.Cmd, ui.KeyR, func() { a.regenerate() })
	}
	if a.suggestion != nil || a.action != nil {
		c.OnShortcut(0, ui.KeyEscape, func() { a.dismissSuggestion() })
	}

	blocks := a.blocks()
	boxes := a.doc.VisibleParagraphs() // the last frame's boxes, as the gutter needs
	layout := a.aiLayout(c, blocks, boxes)

	ui.Column(c).Fill().Background(colorBackground).Padding(16).Children(func() {
		ui.Column(c).Fill().Radius(20).Background(colorPanel).Border(1, colorBorderSoft).
			Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.03)).
			Shadow(0, 18, 40, -18, ui.RGBA(24, 24, 27, 0.18)).
			Children(func() {
				a.header(c, t)
				if a.errText != "" {
					a.errorBanner(c)
				}
				a.middle(c, t, blocks, boxes, layout)
				if layout.visible && !layout.anchored {
					a.aiDock(c)
				}
			})
		a.footer(c)
	})
}

// header names the document and its state, and holds the file actions and the
// agent chip.
func (a *app) header(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(58).Padding(0, 22).AlignItems(ui.Center).Gap(9).
		BorderWidth(0, 0, 1, 0).BorderColor(colorBorderSoft).Children(func() {
		ui.Text(c, "✦").FontSize(13).TextColor(colorViolet)
		ui.Text(c, "Bub").FontSize(13).FontWeight(650).LetterSpacing(-0.1).TextColor(colorText)
		ui.Box(c).Size(1, 15).Background(colorBorder).Margin(3, 5)
		ui.Text(c, a.documentName()).FontSize(13).FontWeight(550).TextColor(colorText)
		if a.dirty {
			ui.Text(c, "•").FontSize(16).TextColor(colorViolet).Margin(0, 0, 0, -2)
		}
		ui.Spacer(c)
		if a.activity != "" {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Margin(0, 10, 0, 0).Children(func() {
				ui.Spinner(c).FontSize(11)
				ui.Text(c, a.activity).FontSize(11).TextColor(colorViolet).MaxLines(1)
			})
		}
		ui.Text(c, a.wordLabel()).FontSize(11).TextColor(colorFaint).Margin(0, 10, 0, 2)
		a.fileButton(c, "New", "file-new", a.newAsync)
		a.fileButton(c, "Open", "file-open", a.openAsync)
		a.fileButton(c, "Save", "file-save", a.saveAsync)
		ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(5, 10).Radius(999).
			Background(colorPanel).Border(1, colorBorder).Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Background(a.statusColor())
			ui.Text(c, a.statusLabel()).Key("status").FontSize(11).FontWeight(500).TextColor(colorMuted)
		})
	})
}

// fileButton is a small header button for New, Open and Save.
func (a *app) fileButton(c *ui.Context, label, key string, action func()) {
	if ui.Button(c, label).Key(key).FontSize(11).Radius(8).Padding(3, 10).
		Background(colorPanel).Border(1, colorBorder).TextColor(colorText).Clicked() {
		action()
	}
}

// errorBanner is the red row that appears when the agent cannot run: the
// writing surface stays usable, and Retry starts the agent again.
func (a *app) errorBanner(c *ui.Context) {
	ui.Row(c).Key("error").Gap(10).AlignItems(ui.Center).
		Margin(10, 18, 0, 18).Padding(7, 12).Radius(12).
		Background(colorDangerBg).Border(1, colorDangerLine).Children(func() {
		ui.Text(c, "!").FontSize(12).FontWeight(700).TextColor(colorDanger)
		ui.Text(c, a.errText).Grow(1).FontSize(12).LineHeight(1.4).TextColor(colorDanger)
		if ui.Button(c, "Retry").Key("retry").FontSize(11).Radius(999).Padding(3, 11).
			Background(colorPanel).Border(1, colorDangerLine).TextColor(colorDanger).Clicked() {
			a.reconnect()
		}
	})
}

// middle is the writing column: the gutter of block marks, the editor, and the
// anchored AI surfaces over it.
func (a *app) middle(c *ui.Context, t *ui.Theme, blocks []block, boxes []richtext.ParagraphBox, layout aiLayout) {
	ui.Box(c).Grow(1).Clip().Children(func() {
		ui.Column(c).Fill().AlignItems(ui.Center).Children(func() {
			ui.Column(c).WidthPercent(100).MaxWidth(880).FillHeight().Padding(18, 0).Children(func() {
				ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
					a.gutter(c, blocks, boxes, layout)
					ui.Box(c).Grow(1).FillHeight().Children(func() {
						editor := richtext.EditorBase(c.Key("editor"), a.doc, richtext.EditorOptions{
							Label:         "Document",
							Placeholder:   "Start writing. Press ⌘K to work on a block with Bub.",
							Font:          ui.Font{Size: 17, LineHeight: 1.6},
							Theme:         a.editorTheme,
							OnLinkCommand: a.openActionAtCaret,
						}).Fill().AutoFocus().Bind(&a.editor)
						if editor.Changed() {
							a.dirty = true
						}
						if layout.visible && layout.anchored {
							a.aiStack(c, layout)
						}
					})
				})
			})
		})
	})
}

// gutter is the quiet ✦ beside each visible paragraph: the block's action bar
// opens from it, as ⌘K opens it from the caret.
func (a *app) gutter(c *ui.Context, blocks []block, boxes []richtext.ParagraphBox, layout aiLayout) {
	coveredFrom := float32(0)
	if layout.visible && layout.anchored && a.suggestion != nil {
		// A suggestion paints a band over the lines under it: the marks of the
		// paragraphs it hides would float over it.
		coveredFrom = layout.top - 2
	}
	ui.Box(c).Width(44).Clip().Children(func() {
		for _, box := range boxes {
			if box.Index < 0 || box.Index >= len(blocks) {
				continue
			}
			if coveredFrom > 0 && box.Top >= coveredFrom {
				continue
			}
			blk := blocks[box.Index]
			active := a.action != nil && a.action.target.id == blk.id
			color := colorFaint
			if active {
				color = colorViolet
			}
			mark := ui.Button(c, "✦").Key(fmt.Sprintf("ai-block-%d", box.Index)).
				FontSize(12).Size(24, 24).Radius(12).BorderWidth(0).
				Background(ui.Color{}).TextColor(color).
				Absolute().Left(10).Top(box.Top - 3)
			if mark.Hovered() || active {
				mark.Background(colorVioletSoft)
			}
			if mark.Clicked() {
				a.openActionForBlock(blk)
			}
		}
	})
}

// aiLayout decides where the AI surfaces go: under the target paragraph when
// the last frame's paragraph boxes leave room for them there, otherwise docked
// above the footer.
type aiLayout struct {
	visible  bool
	anchored bool
	top      float32
}

func (a *app) aiLayout(c *ui.Context, blocks []block, boxes []richtext.ParagraphBox) aiLayout {
	targetID := ""
	switch {
	case a.suggestion != nil:
		targetID = a.suggestion.action.target.id
	case a.action != nil:
		targetID = a.action.target.id
	}
	if targetID == "" {
		return aiLayout{}
	}
	layout := aiLayout{visible: true}
	box, ok := boxForID(blocks, boxes, targetID)
	if !ok {
		return layout
	}
	areaHeight := a.editor.Bounds(c).H
	if areaHeight <= 0 {
		return layout
	}
	need := float32(0)
	if a.action != nil {
		need += a.actionBarHeight()
	}
	if a.suggestion != nil {
		if need > 0 {
			need += 8
		}
		need += a.cardHeight()
	}
	top := box.Top + box.Height + 8
	if top+need <= areaHeight-4 {
		layout.anchored = true
		layout.top = top
	}
	return layout
}

// aiStack places the action bar and the suggestion card over the editor, under
// the paragraph they belong to.
func (a *app) aiStack(c *ui.Context, layout aiLayout) {
	stack := ui.Column(c).Absolute().Left(0).Right(0).Top(layout.top - 4).
		AlignItems(ui.Start).Gap(8)
	if a.suggestion != nil {
		// The band is the document showing a suggestion in place: it covers
		// the lines under it rather than letting them peek around the card.
		stack.Padding(4, 0, 8, 0).Background(ui.Hex("#fbfaff")).
			BorderWidth(1, 0, 1, 0).BorderColor(ui.Hex("#eeebff"))
	}
	stack.Children(func() {
		if a.action != nil {
			a.actionBar(c)
		}
		if a.suggestion != nil {
			a.suggestionCard(c).WidthPercent(80)
		}
	})
}

// aiDock shows the same surfaces in the flow above the footer, for when the
// target paragraph is too low on the page to hold them.
func (a *app) aiDock(c *ui.Context) {
	ui.Column(c).Padding(10, 20, 12, 20).Gap(8).
		BorderWidth(1, 0, 0, 0).BorderColor(colorBorderSoft).
		Background(colorFooter).Children(func() {
		if a.action != nil {
			a.actionBar(c)
		}
		if a.suggestion != nil {
			a.suggestionCard(c)
		}
	})
}

// actionBar is the block's actions: Rewrite, Shorten, Expand, Continue,
// Translate, Ask…, and the one-line ask input behind the last.
func (a *app) actionBar(c *ui.Context) ui.Element {
	return ui.Column(c).Radius(12).Background(colorPanel).Border(1, colorVioletLine).
		Shadow(0, 10, 26, -8, ui.RGBA(24, 24, 27, 0.16)).Children(func() {
		ui.Row(c).Gap(1).Padding(5).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "✦").FontSize(11).TextColor(colorViolet).Padding(0, 6)
			for _, act := range actionLabels {
				if ui.Button(c, act.label).Key("ai-"+string(act.kind)).FontSize(11.5).
					Radius(8).Padding(4, 9).BorderWidth(0).Background(ui.Color{}).
					TextColor(colorText).Clicked() {
					if act.kind == actionAsk {
						a.askMode = true
						a.askText = ""
					} else {
						a.chooseAction(act.kind, "")
					}
				}
			}
			ui.Spacer(c)
			ui.Text(c, "⌘K").FontSize(10).TextColor(colorFaint).Padding(0, 8)
		})
		if a.askMode {
			a.askRow(c)
		}
	})
}

// askRow is the one-line instruction input of "Ask…": type what the block
// should become, then Generate.
func (a *app) askRow(c *ui.Context) {
	ui.Row(c).Gap(6).Padding(0, 6, 6, 6).Children(func() {
		input := ui.TextInput(c.Key("ai-instruction"), &a.askText).
			Placeholder("Ask Bub to edit this paragraph…").
			FontSize(12).Grow(1).AutoFocus()
		submitted := input.Submitted()
		clicked := ui.Button(c, "Generate").Key("ai-generate").FontSize(11).
			Radius(8).Padding(4, 10).BorderWidth(0).
			Background(colorViolet).TextColor(colorOnInk).
			Disabled(strings.TrimSpace(a.askText) == "").
			Clicked()
		if strings.TrimSpace(a.askText) != "" && (submitted || clicked) {
			a.chooseAction(actionAsk, a.askText)
		}
	})
}

// suggestionCard is the agent's answer, streamed beside the block, never in
// the document until accepted.
func (a *app) suggestionCard(c *ui.Context) ui.Element {
	s := a.suggestion
	stale := a.suggestionStale()
	body := s.text
	if strings.TrimSpace(body) == "" {
		switch s.status {
		case suggestThinking:
			body = "Thinking…"
		case suggestFailed:
			body = ""
		}
	}
	return ui.Column(c).Radius(14).Background(colorPanel).Border(1, colorVioletLine).
		Shadow(0, 14, 34, -12, ui.RGBA(24, 24, 27, 0.20)).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(8, 10, 0, 12).Children(func() {
			ui.Text(c, "✦ "+suggestionLabel(s.action.kind)).FontSize(11).
				FontWeight(650).TextColor(colorViolet)
			if summary := a.suggestionSummary(s); summary != "" {
				ui.Text(c, "· "+summary).FontSize(10.5).TextColor(colorFaint)
			}
			ui.Spacer(c)
			if s.status.busy() {
				if ui.Button(c, "Stop").Key("ai-stop").FontSize(10.5).Radius(999).
					Padding(2, 9).Background(colorPanel).Border(1, colorBorder).
					TextColor(colorMuted).Clicked() {
					a.dismissSuggestion()
				}
			}
			accept := ui.Button(c, "Accept").Key("ai-accept").FontSize(11).Radius(999).
				Padding(3, 12).BorderWidth(0).
				Background(colorViolet).TextColor(colorOnInk).
				Disabled(s.status != suggestReady || stale || strings.TrimSpace(s.text) == "")
			if accept.Clicked() {
				a.applySuggestion()
			}
			if ui.Button(c, "Regenerate").Key("ai-regenerate").FontSize(11).Radius(999).
				Padding(3, 11).Background(colorPanel).Border(1, colorBorder).
				TextColor(colorText).Disabled(s.status.busy()).Clicked() {
				a.regenerate()
			}
			if ui.Button(c, "Dismiss").Key("ai-dismiss").FontSize(11).Radius(999).
				Padding(3, 11).Background(colorPanel).Border(1, colorBorder).
				TextColor(colorText).Clicked() {
				a.dismissSuggestion()
			}
		})
		if body != "" {
			muted := s.text == ""
			ui.Scroll(c).MaxHeight(208).Padding(2, 12).Children(func() {
				text := ui.Text(c, body).FontSize(15).LineHeight(1.6).TextColor(colorText)
				if muted {
					text.TextColor(colorMuted)
				}
			})
		}
		ui.Row(c).AlignItems(ui.Center).Padding(2, 12, 8, 12).Children(func() {
			switch {
			case stale:
				ui.Text(c, "This paragraph changed — regenerate or dismiss.").
					FontSize(10.5).TextColor(colorWarning)
			case s.status == suggestFailed && s.errText != "":
				ui.Text(c, s.errText).FontSize(10.5).TextColor(colorDanger).MaxLines(2)
			}
			ui.Spacer(c)
			ui.Text(c, "⌘↩ accept · esc dismiss").FontSize(10).TextColor(colorFaint)
		})
	})
}

// footer is the standing key hint under the page.
func (a *app) footer(c *ui.Context) {
	ui.Row(c).Height(32).Padding(2, 6).AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Text(c, "⌘K").FontSize(10.5).TextColor(colorMuted)
		ui.Text(c, "edit block").FontSize(10.5).TextColor(colorFaint)
		ui.Text(c, "·").FontSize(10).TextColor(colorFaint)
		ui.Text(c, "⌘S").FontSize(10.5).TextColor(colorMuted)
		ui.Text(c, "save").FontSize(10.5).TextColor(colorFaint)
		ui.Text(c, "·").FontSize(10).TextColor(colorFaint)
		ui.Text(c, "⌘O").FontSize(10.5).TextColor(colorMuted)
		ui.Text(c, "open").FontSize(10.5).TextColor(colorFaint)
		ui.Text(c, "·").FontSize(10).TextColor(colorFaint)
		ui.Text(c, "⌘N").FontSize(10.5).TextColor(colorMuted)
		ui.Text(c, "new").FontSize(10.5).TextColor(colorFaint)
		ui.Spacer(c)
		ui.Text(c, a.documentPath()).FontSize(10).TextColor(colorFaint).MaxLines(1)
	})
}

// --- Small view helpers ---------------------------------------------------

func (a *app) statusLabel() string {
	if s := a.suggestion; s != nil && s.status.busy() {
		return "running"
	}
	return a.status.String()
}

func (a *app) statusColor() ui.Color {
	switch {
	case a.status == statusOffline:
		return colorDanger
	case a.status == statusConnecting:
		return colorWarning
	case a.suggestion != nil && a.suggestion.status.busy():
		return colorViolet
	default:
		return colorSuccess
	}
}

func (a *app) wordLabel() string {
	words := a.words()
	if words == 1 {
		return "1 word"
	}
	return fmt.Sprintf("%d words", words)
}

// suggestionLabel names the card for the action that made it.
func suggestionLabel(kind actionKind) string {
	switch kind {
	case actionShorten:
		return "Suggested shortening"
	case actionExpand:
		return "Suggested expansion"
	case actionContinue:
		return "Suggested continuation"
	case actionTranslate:
		return "Suggested translation"
	case actionAsk:
		return "Suggested edit"
	default:
		return "Suggested rewrite"
	}
}

// suggestionSummary is the card's small second label: what changed, or what
// the agent is doing.
func (a *app) suggestionSummary(s *suggestionState) string {
	if s.status.busy() {
		return s.status.label()
	}
	if s.status == suggestFailed {
		return "failed"
	}
	return changeSummary(s.action.target.text, s.text)
}

// boxForID finds the visible box of the paragraph with an ID.
func boxForID(blocks []block, boxes []richtext.ParagraphBox, id string) (richtext.ParagraphBox, bool) {
	for _, b := range blocks {
		if b.id != id {
			continue
		}
		for _, box := range boxes {
			if box.Index == b.index {
				return box, true
			}
		}
		return richtext.ParagraphBox{}, false
	}
	return richtext.ParagraphBox{}, false
}

// actionBarHeight estimates the action bar's height for placement.
func (a *app) actionBarHeight() float32 {
	if a.askMode {
		return 82
	}
	return 44
}

// cardHeight estimates the suggestion card's height for placement, from the
// text it streams.
func (a *app) cardHeight() float32 {
	s := a.suggestion
	if s == nil {
		return 0
	}
	lines := float32(len([]rune(s.text)))/72 + 1
	body := lines * 24
	if body < 24 {
		body = 24
	}
	if body > 208 {
		body = 208
	}
	height := 84 + body
	if s.status == suggestFailed && s.errText != "" {
		height += 20
	}
	return height
}
