package block

import (
	"strings"

	"github.com/egoist/mygo/ui/richtext"
)

// Apply turns the block named blockID into kind, as one step of undo: it
// resets the paragraph's heading, list, quote and code formats to those of
// kind, which leaves its alignment, indentation and ID alone, and removes
// the slash command the menu was opened with — the runes from slashStart
// up to slashEnd — when slashStart is not negative.
//
// It reports whether the block exists and kind is one of the catalog's:
// an unknown block or kind changes nothing.
func Apply(doc *richtext.Document, blockID string, kind CommandKind, slashStart, slashEnd int) bool {
	if doc == nil {
		return false
	}
	set, ok := styleSetter(kind)
	if !ok {
		return false
	}
	applied := false
	doc.Edit(func(e *richtext.Edit) {
		b, ok := ByID(e.Text(), blockID)
		if !ok {
			return
		}
		applied = true
		// Format first: removing the command shortens the paragraph, but
		// formatting it does not move it.
		e.FormatParagraphs(b.Start, b.End, set)
		if slashStart >= 0 && slashEnd > slashStart {
			e.ReplaceString(slashStart, slashEnd, "", richtext.Attributes{})
		}
	})
	return applied
}

// styleSetter returns the change to a paragraph's style that makes it kind.
func styleSetter(kind CommandKind) (func(*richtext.ParagraphStyle), bool) {
	switch kind {
	case CommandText:
		return func(p *richtext.ParagraphStyle) {
			p.Heading, p.List, p.Checked, p.Quote, p.CodeBlock = 0, richtext.NoList, false, false, false
		}, true
	case CommandHeading1, CommandHeading2, CommandHeading3:
		level := int(kind-CommandHeading1) + 1
		return func(p *richtext.ParagraphStyle) {
			p.Heading, p.List, p.Checked, p.Quote, p.CodeBlock = level, richtext.NoList, false, false, false
		}, true
	case CommandBulletedList:
		return func(p *richtext.ParagraphStyle) {
			p.Heading, p.List, p.Checked, p.Quote, p.CodeBlock = 0, richtext.Bullets, false, false, false
		}, true
	case CommandNumberedList:
		return func(p *richtext.ParagraphStyle) {
			p.Heading, p.List, p.Checked, p.Quote, p.CodeBlock = 0, richtext.Numbers, false, false, false
		}, true
	case CommandTodoList:
		return func(p *richtext.ParagraphStyle) {
			p.Heading, p.List, p.Checked, p.Quote, p.CodeBlock = 0, richtext.Tasks, false, false, false
		}, true
	case CommandQuote:
		return func(p *richtext.ParagraphStyle) {
			p.Heading, p.List, p.Checked, p.Quote, p.CodeBlock = 0, richtext.NoList, false, true, false
		}, true
	case CommandCodeBlock:
		return func(p *richtext.ParagraphStyle) {
			p.Heading, p.List, p.Checked, p.Quote, p.CodeBlock = 0, richtext.NoList, false, false, true
		}, true
	}
	return nil, false
}

// Move reorders the document's blocks as one step of undo: the block at
// index from goes to where to is among the blocks as they are before the
// move, so that 0 puts it first and the block count appends it.
//
// Blocks keep their styles, their runs' attributes and their IDs, which
// stay with their text. A move that is out of range, or that would leave
// the order as it is, reports false and changes nothing.
func Move(doc *richtext.Document, from, to int) bool {
	if doc == nil {
		return false
	}
	text := doc.Text()
	n := text.ParagraphCount()
	if from < 0 || from >= n || to < 0 || to > n || to == from || to == from+1 {
		return false
	}
	paras := make([]richtext.Paragraph, 0, n)
	for _, p := range text.Paragraphs() {
		paras = append(paras, p)
	}
	moved := paras[from]
	rest := make([]richtext.Paragraph, 0, n-1)
	rest = append(rest, paras[:from]...)
	rest = append(rest, paras[from+1:]...)
	at := to
	if at > from {
		at--
	}
	order := make([]richtext.Paragraph, 0, n)
	order = append(order, rest[:at]...)
	order = append(order, moved)
	order = append(order, rest[at:]...)
	movedText := richtext.TextOf(order...)
	doc.Edit(func(e *richtext.Edit) {
		e.Replace(0, e.Text().Len(), movedText)
		// Replace takes the IDs and styles of the paragraphs it makes from
		// the text around the change, never from the text it puts there,
		// so put each block's own back.
		blocks := List(e.Text())
		if len(blocks) != len(order) {
			return
		}
		for i, b := range blocks {
			style := order[i].Style
			e.FormatParagraphs(b.Start, b.Start, func(p *richtext.ParagraphStyle) { *p = style })
		}
	})
	return true
}

// InsertAfter inserts a block with the given text and style after the
// block at index, or as the document's first block for -1, as one step of
// undo. The text may hold newlines: each line is a block, all of the same
// style, and its trailing newlines are dropped. The blocks are named as
// the document names new paragraphs: style.ID is not kept.
//
// It reports false, changing nothing, when index is not a block's or -1.
func InsertAfter(doc *richtext.Document, index int, text string, style richtext.ParagraphStyle) bool {
	if doc == nil {
		return false
	}
	inserted := false
	doc.Edit(func(e *richtext.Edit) {
		t := e.Text()
		blocks := List(t)
		if index < -1 || index >= len(blocks) {
			return
		}
		inserted = true
		lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
		body := strings.Join(lines, "\n")
		// A block after another starts after that block's newline; one at
		// the end of the text brings its own newline along.
		atEnd := index >= 0 && index == len(blocks)-1
		pos := 0
		switch {
		case index == -1:
			pos = 0
		case atEnd:
			pos = t.Len()
		default:
			pos = blocks[index+1].Start
		}
		if atEnd {
			body = "\n" + body
		} else {
			body += "\n"
		}
		e.ReplaceString(pos, pos, body, richtext.Attributes{})
		after := List(e.Text())
		k := len(lines)
		newAt := 0
		if index >= 0 {
			newAt = index + 1
		}
		// Name the new blocks as the document names new paragraphs: their
		// style is the one asked for, without an ID.
		for i := newAt; i < newAt+k && i < len(after); i++ {
			at := after[i].Start
			e.FormatParagraphs(at, at, func(p *richtext.ParagraphStyle) {
				*p = style
				p.ID = ""
			})
		}
		// The blocks that were there keep their styles and IDs, which
		// Replace follows over the text, not with the text it puts there.
		if len(after) != len(blocks)+k {
			return
		}
		for j, b := range blocks {
			at := j
			if j > index {
				at = j + k
			}
			st := b.Style
			e.FormatParagraphs(after[at].Start, after[at].Start, func(p *richtext.ParagraphStyle) { *p = st })
		}
	})
	return inserted
}

// Delete removes the block at index as one step of undo, with its newline.
// The only block of a document is emptied instead of removed, called back
// to a plain block with its ID, as an editor always keeps one block.
//
// It reports false, changing nothing, when index is not a block's.
func Delete(doc *richtext.Document, index int) bool {
	if doc == nil {
		return false
	}
	deleted := false
	doc.Edit(func(e *richtext.Edit) {
		t := e.Text()
		blocks := List(t)
		if index < 0 || index >= len(blocks) {
			return
		}
		deleted = true
		// The blocks left keep their styles and IDs. A document keeps one
		// block: removing the only one empties it instead.
		keep := make([]richtext.ParagraphStyle, 0, len(blocks))
		if len(blocks) == 1 {
			keep = append(keep, richtext.ParagraphStyle{ID: blocks[0].ID})
		} else {
			for j, b := range blocks {
				if j != index {
					keep = append(keep, b.Style)
				}
			}
		}
		switch {
		case len(blocks) == 1:
			e.ReplaceString(blocks[0].Start, blocks[0].End, "", richtext.Attributes{})
		case index < len(blocks)-1:
			e.ReplaceString(blocks[index].Start, blocks[index+1].Start, "", richtext.Attributes{})
		default:
			e.ReplaceString(blocks[index].Start-1, t.Len(), "", richtext.Attributes{})
		}
		after := List(e.Text())
		if len(after) != len(keep) {
			return
		}
		for i, st := range keep {
			e.FormatParagraphs(after[i].Start, after[i].Start, func(p *richtext.ParagraphStyle) { *p = st })
		}
	})
	return deleted
}
