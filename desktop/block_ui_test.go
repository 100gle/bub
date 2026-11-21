package main

import (
	"strings"
	"testing"

	"github.com/bubbuild/bub/desktop/internal/block"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

func TestSlashOpensAndAppliesABlockType(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "# Title\n\n/\n")
	a.doc.SetSelection(a.doc.Text().Len(), a.doc.Text().Len())
	a.detectSlash()
	if a.slash == nil {
		t.Fatal("/ did not open the block menu")
	}
	cmd, ok := block.CommandFor(block.CommandHeading2)
	if !ok {
		t.Fatal("no Heading 2 command")
	}
	a.applySlash(cmd)
	if a.slash != nil {
		t.Error("the block menu stayed open after choosing")
	}
	last := a.blocks()[len(a.blocks())-1]
	if last.Style.Heading != 2 {
		t.Errorf("block style = %+v, want a level-2 heading", last.Style)
	}
	if strings.TrimSpace(last.Text) != "" {
		t.Errorf("the slash text stayed: %q", last.Text)
	}
	if !a.dirty {
		t.Error("changing a block type did not mark the page dirty")
	}
}

func TestSlashQueryFiltersCommands(t *testing.T) {
	matches := block.FilterCommands("code")
	if len(matches) == 0 || matches[0].Kind != block.CommandCodeBlock {
		t.Errorf("filtering for code = %+v", matches)
	}
	if got := len(block.FilterCommands("")); got < 9 {
		t.Errorf("empty query returned %d commands, want all", got)
	}
}

func TestMoveBlockIsOneUndoStep(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "Alpha.\n\nBravo.\n\nCharlie.\n")
	before := a.doc.Text().PlainText()
	ids := []string{a.blocks()[0].ID, a.blocks()[1].ID, a.blocks()[2].ID}
	a.moveBlock(0, 3) // move Alpha after Charlie
	plain := a.doc.Text().PlainText()
	if !strings.HasPrefix(plain, "Bravo.\nCharlie.\nAlpha.") {
		t.Errorf("after moving: %q", plain)
	}
	got := []string{a.blocks()[0].ID, a.blocks()[1].ID, a.blocks()[2].ID}
	if got[0] != ids[1] || got[1] != ids[2] || got[2] != ids[0] {
		t.Errorf("IDs = %v, want %v reordered", got, ids)
	}
	if !a.doc.CanUndo() {
		t.Fatal("moving a block is not undoable")
	}
	a.doc.Undo()
	if got := a.doc.Text().PlainText(); got != before {
		t.Errorf("one undo left %q, want %q", got, before)
	}
}

func TestOutlineListsHeadings(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "# Title\n\nBody.\n\n## Section\n\n### Detail\n")
	entries := a.outline()
	if len(entries) != 3 {
		t.Fatalf("outline = %+v, want 3 entries", entries)
	}
	if entries[0].Text != "Title" || entries[0].Level != 1 {
		t.Errorf("first entry = %+v", entries[0])
	}
	if entries[2].Level != 3 {
		t.Errorf("third entry level = %d, want 3", entries[2].Level)
	}
	if entries[0].Start != 0 {
		t.Errorf("first heading start = %d, want 0", entries[0].Start)
	}
}

// --- View tests, without a window ----------------------------------------

func TestTypingSlashOpensAndFiltersTheBlockMenu(t *testing.T) {
	a, _ := newTestApp(t)
	a.setDocument(richtext.TextOf(
		richtext.Paragraph{Style: richtext.ParagraphStyle{Heading: 1}, Runs: []richtext.Run{{Text: "Title"}}},
		richtext.Paragraph{},
	), "")
	tt := ui.NewTester(a.view, 1180, 820)
	tt.Frame()
	a.doc.SetSelection(a.doc.Text().Len(), a.doc.Text().Len())
	tt.Frame()
	tt.Type("/")
	if a.slash == nil {
		t.Fatalf("typing / did not open the block menu: %q", a.doc.Text().PlainText())
	}
	tt.Frame()
	if !tt.HasText("Heading 1") || !tt.HasText("Code block") {
		t.Errorf("the block menu is missing types: %q", tt.Texts())
	}
	tt.Type("code")
	tt.Frame()
	if !tt.HasText("Code block") {
		t.Errorf("filtering for code lost the code block: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.slash != nil {
		t.Error("Enter did not choose a block type")
	}
	blocks := a.blocks()
	last := blocks[len(blocks)-1]
	if !last.Style.CodeBlock {
		t.Errorf("last block style = %+v, want a code block", last.Style)
	}
}

func TestDraggingABlockReordersIt(t *testing.T) {
	a, _ := newTestApp(t)
	a.sidebarShown = false
	a.setDocument(richtext.TextOf(
		richtext.Paragraph{Style: richtext.ParagraphStyle{Heading: 1}, Runs: []richtext.Run{{Text: "Title"}}},
		richtext.Paragraph{Runs: []richtext.Run{{Text: "Alpha."}}},
		richtext.Paragraph{Runs: []richtext.Run{{Text: "Bravo."}}},
		richtext.Paragraph{Runs: []richtext.Run{{Text: "Charlie."}}},
	), "")
	before := a.doc.Text().PlainText()
	tt := ui.NewTester(a.view, 1180, 820)
	tt.Frame()
	ed, ok := tt.Find("Page")
	if !ok {
		t.Fatal("no editor")
	}
	// Hover the first block so its gutter handles show.
	tt.Move(ed.X+ed.W/2, ed.Y+10)
	tt.Frame()
	grip, ok := tt.Find("Drag block")
	if !ok {
		t.Fatalf("no drag handle after hovering: %q", tt.Texts())
	}
	cx, cy := grip.X+grip.W/2, grip.Y+grip.H/2
	tt.Press(cx, cy)
	tt.Move(cx, cy+12)
	tt.Frame()
	// Drag well below the last paragraph and let go.
	tt.Move(cx, ed.Y+ed.H-40)
	tt.Frame()
	tt.Move(cx, ed.Y+ed.H-40)
	tt.Frame()
	tt.Release(cx, ed.Y+ed.H-40)
	tt.Frame()
	after := a.doc.Text().PlainText()
	if after == before {
		t.Fatalf("the block did not move: %q", after)
	}
	if !a.doc.CanUndo() {
		t.Fatal("the move is not undoable")
	}
	a.doc.Undo()
	if got := a.doc.Text().PlainText(); got != before {
		t.Errorf("one undo left %q, want %q", got, before)
	}
}

func TestGutterPlusOpensTheBlockMenuByPointer(t *testing.T) {
	a, _ := newTestApp(t)
	a.sidebarShown = false
	load(t, a, "# Title\n\nAlpha.\n")
	tt := ui.NewTester(a.view, 1180, 820)
	tt.Frame()
	ed, _ := tt.Find("Page")
	tt.Move(ed.X+ed.W/2, ed.Y+10)
	tt.Frame()
	plus, ok := tt.Find("Add block")
	if !ok {
		t.Fatalf("no plus button: %q", tt.Texts())
	}
	cx, cy := plus.X+plus.W/2, plus.Y+plus.H/2
	tt.Press(cx, cy)
	tt.Release(cx, cy)
	tt.Frame()
	if a.slash == nil {
		t.Fatalf("a pointer press on the gutter plus did not open the block menu")
	}
}

func TestGutterMenuRestylesTheHoveredBlock(t *testing.T) {
	a, _ := newTestApp(t)
	a.sidebarShown = false
	a.setDocument(richtext.TextOf(
		richtext.Paragraph{Style: richtext.ParagraphStyle{Heading: 1}, Runs: []richtext.Run{{Text: "Title"}}},
		richtext.Paragraph{Runs: []richtext.Run{{Text: "Alpha."}}},
		richtext.Paragraph{Runs: []richtext.Run{{Text: "Bravo."}}},
	), "")
	tt := ui.NewTester(a.view, 1180, 820)
	tt.Frame()
	ed, _ := tt.Find("Page")
	// Hover the second block.
	tt.Move(ed.X+ed.W/2, ed.Y+40)
	tt.Frame()
	plus, ok := tt.Find("Add block")
	if !ok {
		t.Fatalf("no plus: %q", tt.Texts())
	}
	tt.Press(plus.X+plus.W/2, plus.Y+plus.H/2)
	tt.Release(plus.X+plus.W/2, plus.Y+plus.H/2)
	tt.Frame()
	if a.slash == nil {
		t.Fatal("plus did not open the menu")
	}
	target := a.slash.blockID
	h2, ok := tt.Find("Heading 2")
	if !ok {
		t.Fatalf("no Heading 2 row: %q", tt.Texts())
	}
	tt.Press(h2.X+h2.W/2, h2.Y+h2.H/2)
	tt.Release(h2.X+h2.W/2, h2.Y+h2.H/2)
	tt.Frame()
	for _, b := range a.blocks() {
		t.Logf("%q heading=%d list=%d quote=%v code=%v id=%s", b.Text, b.Style.Heading, b.Style.List, b.Style.Quote, b.Style.CodeBlock, b.ID)
	}
	if a.slash != nil {
		t.Errorf("the menu stayed open")
	}
	found := false
	for _, b := range a.blocks() {
		if b.Style.Heading == 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("no block became a level-2 heading (target %q)", target)
	}
}
