package block

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

// styleOf returns the style of the block named id.
func styleOf(t *testing.T, doc *richtext.Document, id string) richtext.ParagraphStyle {
	t.Helper()
	b, ok := ByID(doc.Text(), id)
	if !ok {
		t.Fatalf("block %q not found", id)
	}
	return b.Style
}

func TestApply(t *testing.T) {
	tests := []struct {
		name       string
		paras      []richtext.Paragraph
		blockID    string
		styleID    string // where to look for the style, blockID if empty
		kind       CommandKind
		slashStart int
		slashEnd   int
		wantOK     bool
		wantText   string
		wantStyle  richtext.ParagraphStyle
	}{
		{
			name:       "unknown block",
			paras:      []richtext.Paragraph{para("a", richtext.ParagraphStyle{ID: "b1"})},
			blockID:    "nope",
			styleID:    "b1",
			kind:       CommandHeading1,
			slashStart: -1,
			slashEnd:   -1,
			wantOK:     false,
			wantText:   "a",
			wantStyle:  richtext.ParagraphStyle{ID: "b1"},
		},
		{
			name:       "slash command becomes a heading",
			paras:      []richtext.Paragraph{para("/h", richtext.ParagraphStyle{ID: "b1"})},
			blockID:    "b1",
			kind:       CommandHeading1,
			slashStart: 0,
			slashEnd:   2,
			wantOK:     true,
			wantText:   "",
			wantStyle:  richtext.ParagraphStyle{Heading: 1, ID: "b1"},
		},
		{
			name: "bulleted list resets heading, quote and code",
			paras: []richtext.Paragraph{para("x", richtext.ParagraphStyle{
				Heading: 2, Quote: true, CodeBlock: true, ID: "b",
			})},
			blockID:    "b",
			kind:       CommandBulletedList,
			slashStart: -1,
			slashEnd:   -1,
			wantOK:     true,
			wantText:   "x",
			wantStyle:  richtext.ParagraphStyle{List: richtext.Bullets, ID: "b"},
		},
		{
			name: "text resets every block format",
			paras: []richtext.Paragraph{para("x", richtext.ParagraphStyle{
				Heading: 1, List: richtext.Bullets, Checked: true, Quote: true, CodeBlock: true, ID: "b",
			})},
			blockID:    "b",
			kind:       CommandText,
			slashStart: -1,
			slashEnd:   -1,
			wantOK:     true,
			wantText:   "x",
			wantStyle:  richtext.ParagraphStyle{ID: "b"},
		},
		{
			name:       "numbered list",
			paras:      []richtext.Paragraph{para("x", richtext.ParagraphStyle{ID: "b"})},
			blockID:    "b",
			kind:       CommandNumberedList,
			slashStart: -1,
			slashEnd:   -1,
			wantOK:     true,
			wantText:   "x",
			wantStyle:  richtext.ParagraphStyle{List: richtext.Numbers, ID: "b"},
		},
		{
			name:       "to-do unchecks and resets a heading",
			paras:      []richtext.Paragraph{para("x", richtext.ParagraphStyle{Heading: 3, List: richtext.Tasks, Checked: true, ID: "b"})},
			blockID:    "b",
			kind:       CommandTodoList,
			slashStart: -1,
			slashEnd:   -1,
			wantOK:     true,
			wantText:   "x",
			wantStyle:  richtext.ParagraphStyle{List: richtext.Tasks, ID: "b"},
		},
		{
			name:       "quote resets code",
			paras:      []richtext.Paragraph{para("x", richtext.ParagraphStyle{CodeBlock: true, ID: "b"})},
			blockID:    "b",
			kind:       CommandQuote,
			slashStart: -1,
			slashEnd:   -1,
			wantOK:     true,
			wantText:   "x",
			wantStyle:  richtext.ParagraphStyle{Quote: true, ID: "b"},
		},
		{
			name:       "code block resets heading and quote",
			paras:      []richtext.Paragraph{para("x", richtext.ParagraphStyle{Heading: 1, Quote: true, ID: "b"})},
			blockID:    "b",
			kind:       CommandCodeBlock,
			slashStart: -1,
			slashEnd:   -1,
			wantOK:     true,
			wantText:   "x",
			wantStyle:  richtext.ParagraphStyle{CodeBlock: true, ID: "b"},
		},
		{
			name:       "alignment and indent stay",
			paras:      []richtext.Paragraph{para("x", richtext.ParagraphStyle{Align: ui.Center, Indent: 2, ID: "b"})},
			blockID:    "b",
			kind:       CommandHeading2,
			slashStart: -1,
			slashEnd:   -1,
			wantOK:     true,
			wantText:   "x",
			wantStyle:  richtext.ParagraphStyle{Heading: 2, Align: ui.Center, Indent: 2, ID: "b"},
		},
		{
			name: "another block is left alone",
			paras: []richtext.Paragraph{
				para("a", richtext.ParagraphStyle{ID: "p1"}),
				para("/q", richtext.ParagraphStyle{ID: "p2"}),
			},
			blockID:    "p2",
			kind:       CommandQuote,
			slashStart: 2,
			slashEnd:   4,
			wantOK:     true,
			wantText:   "a\n",
			wantStyle:  richtext.ParagraphStyle{Quote: true, ID: "p2"},
		},
		{
			name:       "without a slash range only the style changes",
			paras:      []richtext.Paragraph{para("x", richtext.ParagraphStyle{ID: "b"})},
			blockID:    "b",
			kind:       CommandHeading3,
			slashStart: -1,
			slashEnd:   -1,
			wantOK:     true,
			wantText:   "x",
			wantStyle:  richtext.ParagraphStyle{Heading: 3, ID: "b"},
		},
		{
			name:       "the range ends at the caret",
			paras:      []richtext.Paragraph{para("/head rest", richtext.ParagraphStyle{ID: "b"})},
			blockID:    "b",
			kind:       CommandHeading1,
			slashStart: 0,
			slashEnd:   5,
			wantOK:     true,
			wantText:   " rest",
			wantStyle:  richtext.ParagraphStyle{Heading: 1, ID: "b"},
		},
		{
			name:       "unknown kind",
			paras:      []richtext.Paragraph{para("a", richtext.ParagraphStyle{ID: "b"})},
			blockID:    "b",
			kind:       CommandKind(99),
			slashStart: -1,
			slashEnd:   -1,
			wantOK:     false,
			wantText:   "a",
			wantStyle:  richtext.ParagraphStyle{ID: "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := richtext.NewDocument(textOf(tt.paras...))
			ok := Apply(doc, tt.blockID, tt.kind, tt.slashStart, tt.slashEnd)
			if ok != tt.wantOK {
				t.Fatalf("Apply() = %v, want %v", ok, tt.wantOK)
			}
			if got := textIn(doc); got != tt.wantText {
				t.Fatalf("text = %q, want %q", got, tt.wantText)
			}
			id := tt.blockID
			if tt.styleID != "" {
				id = tt.styleID
			}
			if got := styleOf(t, doc, id); !reflect.DeepEqual(got, tt.wantStyle) {
				t.Fatalf("style = %#v, want %#v", got, tt.wantStyle)
			}
		})
	}
}

func TestApplyIsOneStepOfUndo(t *testing.T) {
	doc := richtext.NewDocument(textOf(para("/h", richtext.ParagraphStyle{ID: "b1"})))
	if !Apply(doc, "b1", CommandHeading1, 0, 2) {
		t.Fatal("Apply() = false, want true")
	}
	if got := styleOf(t, doc, "b1"); got.Heading != 1 {
		t.Fatalf("heading = %d, want 1", got.Heading)
	}
	doc.Undo()
	if got := textIn(doc); got != "/h" {
		t.Fatalf("after undo text = %q, want %q", got, "/h")
	}
	if got := styleOf(t, doc, "b1"); got.Heading != 0 {
		t.Fatalf("after undo heading = %d, want 0", got.Heading)
	}
	if doc.CanUndo() {
		t.Fatal("Apply left more than one step of undo")
	}
}

func TestApplyNilDocument(t *testing.T) {
	if Apply(nil, "b", CommandText, -1, -1) {
		t.Fatal("Apply(nil) = true, want false")
	}
}

func TestMove(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		from, to int
		wantOK   bool
		wantText string
	}{
		{name: "down", text: "a\nb\nc", from: 0, to: 2, wantOK: true, wantText: "b\na\nc"},
		{name: "up", text: "a\nb\nc", from: 2, to: 0, wantOK: true, wantText: "c\na\nb"},
		{name: "to the end", text: "a\nb\nc", from: 0, to: 3, wantOK: true, wantText: "b\nc\na"},
		{name: "last to the end is no move", text: "a\nb\nc", from: 2, to: 3, wantOK: false, wantText: "a\nb\nc"},
		{name: "after itself is no move", text: "a\nb\nc", from: 0, to: 1, wantOK: false, wantText: "a\nb\nc"},
		{name: "before itself is no move", text: "a\nb\nc", from: 1, to: 1, wantOK: false, wantText: "a\nb\nc"},
		{name: "past the end", text: "a\nb\nc", from: 0, to: 4, wantOK: false, wantText: "a\nb\nc"},
		{name: "no such block", text: "a\nb\nc", from: 3, to: 0, wantOK: false, wantText: "a\nb\nc"},
		{name: "negative from", text: "a\nb\nc", from: -1, to: 0, wantOK: false, wantText: "a\nb\nc"},
		{name: "negative to", text: "a\nb\nc", from: 0, to: -1, wantOK: false, wantText: "a\nb\nc"},
		{name: "one block", text: "a", from: 0, to: 0, wantOK: false, wantText: "a"},
		{name: "one block to the end", text: "a", from: 0, to: 1, wantOK: false, wantText: "a"},
		{name: "empty text", text: "", from: 0, to: 1, wantOK: false, wantText: ""},
		{name: "two blocks", text: "a\nb", from: 0, to: 2, wantOK: true, wantText: "b\na"},
		{name: "trailing empty block", text: "a\n", from: 0, to: 2, wantOK: true, wantText: "\na"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := docOf(tt.text)
			ok := Move(doc, tt.from, tt.to)
			if ok != tt.wantOK {
				t.Fatalf("Move(%d, %d) = %v, want %v", tt.from, tt.to, ok, tt.wantOK)
			}
			if got := textIn(doc); got != tt.wantText {
				t.Fatalf("text = %q, want %q", got, tt.wantText)
			}
		})
	}
}

func TestMoveKeepsStylesAndIDs(t *testing.T) {
	bold := richtext.Attributes{Weight: 700}
	doc := richtext.NewDocument(richtext.TextOf(
		para("A", richtext.ParagraphStyle{ID: "a"}),
		richtext.Paragraph{
			Style: richtext.ParagraphStyle{Heading: 2, ID: "b"},
			Runs:  []richtext.Run{{Text: "B", Attributes: bold}},
		},
		para("C", richtext.ParagraphStyle{Quote: true, ID: "c"}),
	))
	if !Move(doc, 0, 3) {
		t.Fatal("Move(0, 3) = false, want true")
	}
	if got := textIn(doc); got != "B\nC\nA" {
		t.Fatalf("text = %q, want %q", got, "B\nC\nA")
	}
	if want := []string{"b", "c", "a"}; !reflect.DeepEqual(idsIn(doc), want) {
		t.Fatalf("IDs = %v, want %v", idsIn(doc), want)
	}
	blocks := List(doc.Text())
	wantStyles := []richtext.ParagraphStyle{
		{Heading: 2, ID: "b"},
		{Quote: true, ID: "c"},
		{ID: "a"},
	}
	for i, want := range wantStyles {
		if !reflect.DeepEqual(blocks[i].Style, want) {
			t.Fatalf("block %d style = %#v, want %#v", i, blocks[i].Style, want)
		}
	}
	for _, p := range doc.Text().Paragraphs() {
		if p.Style.ID != "b" {
			continue
		}
		if len(p.Runs) != 1 || p.Runs[0].Attributes.Weight != 700 {
			t.Fatalf("the bold run was lost: %#v", p.Runs)
		}
	}

	doc.Undo()
	if got := textIn(doc); got != "A\nB\nC" {
		t.Fatalf("after undo text = %q, want %q", got, "A\nB\nC")
	}
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(idsIn(doc), want) {
		t.Fatalf("after undo IDs = %v, want %v", idsIn(doc), want)
	}
	if doc.CanUndo() {
		t.Fatal("Move left more than one step of undo")
	}
}

func TestInsertAfter(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		index    int
		insert   string
		style    richtext.ParagraphStyle
		wantOK   bool
		wantText string
		wantAt   int
	}{
		{name: "in the middle", text: "a\nb", index: 0, insert: "x", wantOK: true, wantText: "a\nx\nb", wantAt: 1},
		{name: "at the end", text: "a\nb", index: 1, insert: "x", wantOK: true, wantText: "a\nb\nx", wantAt: 2},
		{name: "at the start", text: "a\nb", index: -1, insert: "x", wantOK: true, wantText: "x\na\nb", wantAt: 0},
		{name: "in an empty text", text: "", index: 0, insert: "x", wantOK: true, wantText: "\nx", wantAt: 1},
		{name: "at the start of an empty text", text: "", index: -1, insert: "x", wantOK: true, wantText: "x\n", wantAt: 0},
		{name: "one block", text: "a", index: 0, insert: "x", wantOK: true, wantText: "a\nx", wantAt: 1},
		{name: "styled", text: "a", index: 0, insert: "H", style: richtext.ParagraphStyle{Heading: 1}, wantOK: true, wantText: "a\nH", wantAt: 1},
		{name: "several lines", text: "a", index: 0, insert: "x\ny", wantOK: true, wantText: "a\nx\ny", wantAt: 1},
		{name: "trailing newlines are dropped", text: "a", index: 0, insert: "x\n\n", wantOK: true, wantText: "a\nx", wantAt: 1},
		{name: "empty block", text: "a", index: 0, insert: "", wantOK: true, wantText: "a\n", wantAt: 1},
		{name: "past the end", text: "a\nb", index: 2, insert: "x", wantOK: false, wantText: "a\nb", wantAt: 0},
		{name: "before the start", text: "a\nb", index: -2, insert: "x", wantOK: false, wantText: "a\nb", wantAt: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := docOf(tt.text)
			ok := InsertAfter(doc, tt.index, tt.insert, tt.style)
			if ok != tt.wantOK {
				t.Fatalf("InsertAfter(%d) = %v, want %v", tt.index, ok, tt.wantOK)
			}
			if got := textIn(doc); got != tt.wantText {
				t.Fatalf("text = %q, want %q", got, tt.wantText)
			}
			if !tt.wantOK {
				return
			}
			blocks := List(doc.Text())
			got := blocks[tt.wantAt]
			want := tt.style
			want.ID = got.ID // the document names the new block
			if !reflect.DeepEqual(got.Style, want) {
				t.Fatalf("new block style = %#v, want %#v", got.Style, want)
			}
		})
	}
}

func TestInsertAfterNamesTheNewBlock(t *testing.T) {
	doc := identify(docOf("a"))
	if !InsertAfter(doc, 0, "x", richtext.ParagraphStyle{Heading: 1, ID: "p1"}) {
		t.Fatal("InsertAfter() = false, want true")
	}
	ids := idsIn(doc)
	if len(ids) != 2 {
		t.Fatalf("IDs = %v, want two blocks", ids)
	}
	if ids[0] == "" || ids[1] == "" {
		t.Fatalf("IDs = %v, want them named", ids)
	}
	if ids[0] == ids[1] {
		t.Fatalf("IDs = %v, want two different IDs", ids)
	}
	if got := styleOf(t, doc, ids[0]); got.ID != ids[0] {
		t.Fatalf("block %q has style ID %q", ids[0], got.ID)
	}
}

func TestInsertAfterIsOneStepOfUndo(t *testing.T) {
	doc := identify(docOf("a\nb"))
	if !InsertAfter(doc, 0, "x", richtext.ParagraphStyle{Heading: 1}) {
		t.Fatal("InsertAfter() = false, want true")
	}
	doc.Undo()
	if got := textIn(doc); got != "a\nb" {
		t.Fatalf("after undo text = %q, want %q", got, "a\nb")
	}
	if want := []string{"p1", "p2"}; !reflect.DeepEqual(idsIn(doc), want) {
		t.Fatalf("after undo IDs = %v, want %v", idsIn(doc), want)
	}
	if doc.CanUndo() {
		t.Fatal("InsertAfter left more than one step of undo")
	}
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name     string
		paras    []richtext.Paragraph
		index    int
		wantOK   bool
		wantText string
	}{
		{
			name: "in the middle",
			paras: []richtext.Paragraph{
				para("a", richtext.ParagraphStyle{ID: "a"}),
				para("b", richtext.ParagraphStyle{ID: "b"}),
				para("c", richtext.ParagraphStyle{ID: "c"}),
			},
			index: 1, wantOK: true, wantText: "a\nc",
		},
		{
			name: "the first",
			paras: []richtext.Paragraph{
				para("a", richtext.ParagraphStyle{ID: "a"}),
				para("b", richtext.ParagraphStyle{ID: "b"}),
			},
			index: 0, wantOK: true, wantText: "b",
		},
		{
			name: "the last",
			paras: []richtext.Paragraph{
				para("a", richtext.ParagraphStyle{ID: "a"}),
				para("b", richtext.ParagraphStyle{ID: "b"}),
			},
			index: 1, wantOK: true, wantText: "a",
		},
		{
			name:  "the only block",
			paras: []richtext.Paragraph{para("a", richtext.ParagraphStyle{Heading: 2, ID: "a"})},
			index: 0, wantOK: true, wantText: "",
		},
		{
			name: "past the end", paras: []richtext.Paragraph{para("a", richtext.ParagraphStyle{ID: "a"})},
			index: 1, wantOK: false, wantText: "a",
		},
		{
			name: "negative index", paras: []richtext.Paragraph{para("a", richtext.ParagraphStyle{ID: "a"})},
			index: -1, wantOK: false, wantText: "a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := richtext.NewDocument(textOf(tt.paras...))
			ok := Delete(doc, tt.index)
			if ok != tt.wantOK {
				t.Fatalf("Delete(%d) = %v, want %v", tt.index, ok, tt.wantOK)
			}
			if got := textIn(doc); got != tt.wantText {
				t.Fatalf("text = %q, want %q", got, tt.wantText)
			}
		})
	}
}

func TestDeleteTheOnlyBlockKeepsItsID(t *testing.T) {
	doc := richtext.NewDocument(textOf(para("x", richtext.ParagraphStyle{Heading: 2, Quote: true, ID: "b"})))
	if !Delete(doc, 0) {
		t.Fatal("Delete(0) = false, want true")
	}
	blocks := List(doc.Text())
	if len(blocks) != 1 {
		t.Fatalf("blocks = %#v, want one", blocks)
	}
	if blocks[0].Text != "" {
		t.Fatalf("text = %q, want empty", blocks[0].Text)
	}
	if want := (richtext.ParagraphStyle{ID: "b"}); !reflect.DeepEqual(blocks[0].Style, want) {
		t.Fatalf("style = %#v, want %#v", blocks[0].Style, want)
	}
}

func TestDeleteKeepsTheOtherBlocks(t *testing.T) {
	doc := richtext.NewDocument(richtext.TextOf(
		para("A", richtext.ParagraphStyle{Heading: 1, ID: "a"}),
		para("B", richtext.ParagraphStyle{ID: "b"}),
		para("C", richtext.ParagraphStyle{Quote: true, ID: "c"}),
	))
	if !Delete(doc, 1) {
		t.Fatal("Delete(1) = false, want true")
	}
	if want := []string{"a", "c"}; !reflect.DeepEqual(idsIn(doc), want) {
		t.Fatalf("IDs = %v, want %v", idsIn(doc), want)
	}
	if got := styleOf(t, doc, "a"); got.Heading != 1 {
		t.Fatalf("heading block style = %#v, want heading 1", got)
	}
	if got := styleOf(t, doc, "c"); !got.Quote {
		t.Fatalf("quote block style = %#v, want a quote", got)
	}
}

func TestDeleteIsOneStepOfUndo(t *testing.T) {
	doc := identify(docOf("a\nb\nc"))
	if !Delete(doc, 1) {
		t.Fatal("Delete(1) = false, want true")
	}
	doc.Undo()
	if got := textIn(doc); got != "a\nb\nc" {
		t.Fatalf("after undo text = %q, want %q", got, "a\nb\nc")
	}
	if want := []string{"p1", "p2", "p3"}; !reflect.DeepEqual(idsIn(doc), want) {
		t.Fatalf("after undo IDs = %v, want %v", idsIn(doc), want)
	}
	if doc.CanUndo() {
		t.Fatal("Delete left more than one step of undo")
	}
}

func TestNilDocuments(t *testing.T) {
	if Move(nil, 0, 1) {
		t.Error("Move(nil) = true, want false")
	}
	if InsertAfter(nil, 0, "x", richtext.ParagraphStyle{}) {
		t.Error("InsertAfter(nil) = true, want false")
	}
	if Delete(nil, 0) {
		t.Error("Delete(nil) = true, want false")
	}
}

func TestInsertAfterKeepsTheIDsOfTheOtherBlocks(t *testing.T) {
	doc := identify(docOf("a\n"))
	if !InsertAfter(doc, 1, "x", richtext.ParagraphStyle{}) {
		t.Fatal("InsertAfter(1) = false, want true")
	}
	if got := textIn(doc); got != "a\n\nx" {
		t.Fatalf("text = %q, want %q", got, "a\n\nx")
	}
	ids := idsIn(doc)
	if len(ids) != 3 {
		t.Fatalf("IDs = %v, want three blocks", ids)
	}
	// The empty block keeps its ID, and the new block is named anew: the
	// ID generator may have been asked more than once for it.
	if ids[0] != "p1" || ids[1] != "p2" {
		t.Fatalf("IDs = %v, want the first two kept", ids)
	}
	if ids[2] == "" || ids[2] == ids[0] || ids[2] == ids[1] {
		t.Fatalf("new block ID = %q, want a different name", ids[2])
	}
}

func TestDeleteKeepsTheIDsOfTheBlocksBeforeAnEmptyOne(t *testing.T) {
	doc := identify(docOf("a\n\nb"))
	if !Delete(doc, 2) {
		t.Fatal("Delete(2) = false, want true")
	}
	if got := textIn(doc); got != "a\n" {
		t.Fatalf("text = %q, want %q", got, "a\n")
	}
	if want := []string{"p1", "p2"}; !reflect.DeepEqual(idsIn(doc), want) {
		t.Fatalf("IDs = %v, want %v", idsIn(doc), want)
	}
}

// TestSlashMenuFlow walks the steps the app takes when a block is typed in
// as a slash command: find the menu, apply its entry, add a block after it
// and move that one up.
func TestSlashMenuFlow(t *testing.T) {
	doc := identify(docOf(""))
	// Type "/h" in the only block, as the editor does.
	doc.Edit(func(e *richtext.Edit) { e.ReplaceString(0, 0, "/h", richtext.Attributes{}) })

	state, ok := SlashAt(doc.Text(), 2)
	if !ok {
		t.Fatal("SlashAt() = false, want true")
	}
	if state.Query != "h" || state.BlockID != "p1" {
		t.Fatalf("SlashAt() = %#v, want query %q in block p1", state, "h")
	}
	commands := FilterCommands(state.Query)
	if len(commands) == 0 || commands[0].Kind != CommandHeading1 {
		t.Fatalf("FilterCommands(%q) = %v, want a heading first", state.Query, kindsOf(commands))
	}
	if !Apply(doc, state.BlockID, CommandHeading1, state.Start, state.End) {
		t.Fatal("Apply() = false, want true")
	}
	if got := textIn(doc); got != "" {
		t.Fatalf("text = %q, want empty", got)
	}
	if got := styleOf(t, doc, "p1"); got.Heading != 1 {
		t.Fatalf("heading = %d, want 1", got.Heading)
	}

	if !InsertAfter(doc, 0, "body", richtext.ParagraphStyle{}) {
		t.Fatal("InsertAfter(0) = false, want true")
	}
	if got := textIn(doc); got != "\nbody" {
		t.Fatalf("text = %q, want %q", got, "\nbody")
	}
	if !Move(doc, 1, 0) {
		t.Fatal("Move(1, 0) = false, want true")
	}
	if got := textIn(doc); got != "body\n" {
		t.Fatalf("text = %q, want %q", got, "body\n")
	}
	blocks := List(doc.Text())
	if blocks[0].Text != "body" || !reflect.DeepEqual(blocks[1].Style, richtext.ParagraphStyle{Heading: 1, ID: "p1"}) {
		t.Fatalf("blocks = %#v, want the body before the heading", blocks)
	}
}
