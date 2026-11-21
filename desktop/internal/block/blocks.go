// Package block is the block engine of the editor: it reads a
// richtext.Text as the blocks the app shows chrome for, finds the block
// and the slash command at a caret, and makes the structural edits the
// chrome asks for (move, insert, delete) as steps of undo.
//
// A block is one paragraph: what richtext keeps as a paragraph is what the
// app keeps as a block, and a block's ID is its ParagraphStyle.ID, which
// the document names (Document.IdentifyParagraphs) and edits carry along
// with the paragraph's text. Offsets here count runes, as in richtext, and
// never bytes.
//
// Nothing in the package panics on an empty document: every empty text
// has the one empty paragraph that every editor keeps, which List returns
// as the one empty block.
package block

import (
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/richtext"
)

// Block is one paragraph of a document as the editor's block: where it is
// in the text, what it holds and how it looks.
type Block struct {
	// ID is the block's stable name, the paragraph's ParagraphStyle.ID,
	// and is "" until the document names its paragraphs.
	ID string
	// Index is the block's place among the document's blocks.
	Index int
	// Start and End are the rune offsets where the block's paragraph
	// starts and ends, End before its newline.
	Start, End int
	// Text is the paragraph's plain text, its runs' texts joined.
	Text string
	// Style is the paragraph's style: heading, list, quote, code and the
	// app's ID.
	Style richtext.ParagraphStyle
}

// List returns the blocks of text, in order: one for each paragraph, the
// empty paragraph of an empty text included.
func List(text richtext.Text) []Block {
	var blocks []Block
	index := 0
	for start, p := range text.Paragraphs() {
		s := paragraphText(p)
		blocks = append(blocks, Block{
			ID:    p.Style.ID,
			Index: index,
			Start: start,
			End:   start + utf8.RuneCountInString(s),
			Text:  s,
			Style: p.Style,
		})
		index++
	}
	return blocks
}

// paragraphText returns the plain text of p.
func paragraphText(p richtext.Paragraph) string {
	if len(p.Runs) == 1 {
		return p.Runs[0].Text
	}
	var b strings.Builder
	for _, r := range p.Runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

// At returns the block that holds the caret, the paragraph whose runes
// around caret it is: the newline ending a paragraph belongs to it, so a
// caret at the end of a block is that block's. caret is clamped to the
// text, so a caret past the end belongs to the last block.
func At(text richtext.Text, caret int) (Block, bool) {
	caret = max(0, min(caret, text.Len()))
	for _, b := range List(text) {
		if caret >= b.Start && caret <= b.End {
			return b, true
		}
	}
	return Block{}, false
}

// ByID returns the block named id. The empty ID names no block, as the
// paragraphs of a document not asked to name its own have none.
func ByID(text richtext.Text, id string) (Block, bool) {
	if id == "" {
		return Block{}, false
	}
	for _, b := range List(text) {
		if b.ID == id {
			return b, true
		}
	}
	return Block{}, false
}

// OutlineEntry is one heading of a document, as a table of contents or the
// AI's context shows it.
type OutlineEntry struct {
	// BlockID is the heading block's ID.
	BlockID string
	// Index is the block's place among the document's blocks.
	Index int
	// Start is the rune offset where the heading's paragraph starts.
	Start int
	// Level is the heading's level, from 1.
	Level int
	// Text is the heading's plain text.
	Text string
}

// Outline returns the headings of text in order: the blocks with a heading
// level and text that is not only spaces. Everything else is left out, as
// an outline lists what it can lead to.
func Outline(text richtext.Text) []OutlineEntry {
	var entries []OutlineEntry
	for _, b := range List(text) {
		if b.Style.Heading <= 0 || strings.TrimSpace(b.Text) == "" {
			continue
		}
		entries = append(entries, OutlineEntry{
			BlockID: b.ID,
			Index:   b.Index,
			Start:   b.Start,
			Level:   b.Style.Heading,
			Text:    b.Text,
		})
	}
	return entries
}

// SlashState is the slash command a caret is at: the menu the app opens
// and the range Apply takes away once one of its entries is chosen.
type SlashState struct {
	// Query is the text between the slash and the caret, as typed.
	Query string
	// Start and End are the rune offsets of the whole "/query" range in
	// the paragraph: Start is the slash's, End the caret's.
	Start, End int
	// BlockID is the ID of the block holding the command.
	BlockID string
}

// SlashAt reports whether the caret is in a block whose text starts with
// the slash that opens the menu, and returns the command typed so far.
// The caret must be in the paragraph after the slash: at its start it is
// before the slash, where no command is. Start and End are the rune
// offsets of the slash and of the caret, so Apply removes the typed
// command but not the text after the caret.
func SlashAt(text richtext.Text, caret int) (SlashState, bool) {
	caret = max(0, min(caret, text.Len()))
	start, end := text.ParagraphRange(caret)
	if caret <= start {
		return SlashState{}, false
	}
	runes := []rune(text.Slice(start, end).String())
	if len(runes) == 0 || runes[0] != '/' {
		return SlashState{}, false
	}
	return SlashState{
		Query:   string(runes[1 : caret-start]),
		Start:   start,
		End:     caret,
		BlockID: text.ParagraphStyleAt(start).ID,
	}, true
}
