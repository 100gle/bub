package block

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui/richtext"
)

// para returns a paragraph of one run with style.
func para(text string, style richtext.ParagraphStyle) richtext.Paragraph {
	return richtext.Paragraph{Style: style, Runs: []richtext.Run{{Text: text}}}
}

// textOf makes a text of paragraphs, with their styles and IDs.
func textOf(paras ...richtext.Paragraph) richtext.Text {
	if len(paras) == 0 {
		return richtext.NewText("")
	}
	return richtext.TextOf(paras...)
}

// docOf returns a document of s, its paragraphs unnamed.
func docOf(s string) *richtext.Document {
	return richtext.NewDocument(richtext.NewText(s))
}

// identify names the document's paragraphs p1, p2, ... as the app does.
func identify(doc *richtext.Document) *richtext.Document {
	n := 0
	doc.IdentifyParagraphs(func() string {
		n++
		return "p" + strconv.Itoa(n)
	})
	return doc
}

// textIn returns the plain text of a document.
func textIn(doc *richtext.Document) string { return doc.Text().String() }

// idsIn returns the IDs of a document's blocks, in order.
func idsIn(doc *richtext.Document) []string {
	var ids []string
	for _, b := range List(doc.Text()) {
		ids = append(ids, b.ID)
	}
	return ids
}

// textInBlocks returns the plain text of a document's blocks, in order.
func textsIn(doc *richtext.Document) []string {
	var texts []string
	for _, b := range List(doc.Text()) {
		texts = append(texts, b.Text)
	}
	return texts
}

func TestList(t *testing.T) {
	bold := richtext.Attributes{Weight: 700}
	tests := []struct {
		name string
		text richtext.Text
		want []Block
	}{
		{
			name: "empty text",
			text: richtext.NewText(""),
			want: []Block{{Index: 0, Start: 0, End: 0}},
		},
		{
			name: "zero text",
			text: richtext.Text{},
			want: []Block{{Index: 0, Start: 0, End: 0}},
		},
		{
			name: "one block",
			text: richtext.NewText("hello"),
			want: []Block{{Index: 0, Start: 0, End: 5, Text: "hello"}},
		},
		{
			name: "two blocks",
			text: richtext.NewText("a\nbc"),
			want: []Block{
				{Index: 0, Start: 0, End: 1, Text: "a"},
				{Index: 1, Start: 2, End: 4, Text: "bc"},
			},
		},
		{
			name: "trailing newline makes an empty block",
			text: richtext.NewText("a\n"),
			want: []Block{
				{Index: 0, Start: 0, End: 1, Text: "a"},
				{Index: 1, Start: 2, End: 2, Text: ""},
			},
		},
		{
			name: "offsets count runes",
			text: richtext.NewText("héllo\n日本語"),
			want: []Block{
				{Index: 0, Start: 0, End: 5, Text: "héllo"},
				{Index: 1, Start: 6, End: 9, Text: "日本語"},
			},
		},
		{
			name: "runs join into plain text",
			text: textOf(richtext.Paragraph{Runs: []richtext.Run{
				{Text: "Bo", Attributes: bold},
				{Text: "ld"},
			}}),
			want: []Block{{Index: 0, Start: 0, End: 4, Text: "Bold"}},
		},
		{
			name: "styles and IDs",
			text: textOf(
				para("Title", richtext.ParagraphStyle{Heading: 1, ID: "h"}),
				para("quote", richtext.ParagraphStyle{Quote: true, ID: "q"}),
				para("item", richtext.ParagraphStyle{List: richtext.Bullets, Indent: 2, ID: "l"}),
			),
			want: []Block{
				{ID: "h", Index: 0, Start: 0, End: 5, Text: "Title", Style: richtext.ParagraphStyle{Heading: 1, ID: "h"}},
				{ID: "q", Index: 1, Start: 6, End: 11, Text: "quote", Style: richtext.ParagraphStyle{Quote: true, ID: "q"}},
				{ID: "l", Index: 2, Start: 12, End: 16, Text: "item", Style: richtext.ParagraphStyle{List: richtext.Bullets, Indent: 2, ID: "l"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := List(tt.text)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("List() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestAt(t *testing.T) {
	tests := []struct {
		name   string
		text   richtext.Text
		caret  int
		want   int // index of the block the caret is in
		wantOK bool
	}{
		{name: "empty text", text: richtext.NewText(""), caret: 0, want: 0, wantOK: true},
		{name: "start", text: richtext.NewText("ab\ncd"), caret: 0, want: 0, wantOK: true},
		{name: "inside first", text: richtext.NewText("ab\ncd"), caret: 1, want: 0, wantOK: true},
		{name: "at the newline of the first", text: richtext.NewText("ab\ncd"), caret: 2, want: 0, wantOK: true},
		{name: "start of the second", text: richtext.NewText("ab\ncd"), caret: 3, want: 1, wantOK: true},
		{name: "end of the text", text: richtext.NewText("ab\ncd"), caret: 5, want: 1, wantOK: true},
		{name: "past the end", text: richtext.NewText("ab\ncd"), caret: 99, want: 1, wantOK: true},
		{name: "before the start", text: richtext.NewText("ab\ncd"), caret: -3, want: 0, wantOK: true},
		{
			name:  "empty block in the middle",
			text:  richtext.NewText("a\n\nb"),
			caret: 2, want: 1, wantOK: true,
		},
		{
			name:  "trailing empty block",
			text:  richtext.NewText("a\n"),
			caret: 2, want: 1, wantOK: true,
		},
		{
			name:  "runes",
			text:  richtext.NewText("日本\n語"),
			caret: 2, want: 0, wantOK: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := At(tt.text, tt.caret)
			if ok != tt.wantOK {
				t.Fatalf("At(%d) ok = %v, want %v", tt.caret, ok, tt.wantOK)
			}
			blocks := List(tt.text)
			if !reflect.DeepEqual(got, blocks[tt.want]) {
				t.Fatalf("At(%d) = %#v, want %#v", tt.caret, got, blocks[tt.want])
			}
		})
	}
}

func TestByID(t *testing.T) {
	text := textOf(
		para("a", richtext.ParagraphStyle{ID: "one"}),
		para("b", richtext.ParagraphStyle{Heading: 2, ID: "two"}),
	)
	tests := []struct {
		name   string
		text   richtext.Text
		id     string
		want   int
		wantOK bool
	}{
		{name: "first", text: text, id: "one", want: 0, wantOK: true},
		{name: "second", text: text, id: "two", want: 1, wantOK: true},
		{name: "unknown", text: text, id: "three", wantOK: false},
		{name: "empty ID names nothing", text: text, id: "", wantOK: false},
		{name: "unnamed paragraphs", text: richtext.NewText("a\nb"), id: "p1", wantOK: false},
		{
			name: "first of equal IDs",
			text: textOf(
				para("a", richtext.ParagraphStyle{ID: "x"}),
				para("b", richtext.ParagraphStyle{ID: "x"}),
			),
			id: "x", want: 0, wantOK: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ByID(tt.text, tt.id)
			if ok != tt.wantOK {
				t.Fatalf("ByID(%q) ok = %v, want %v", tt.id, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if want := List(tt.text)[tt.want]; !reflect.DeepEqual(got, want) {
				t.Fatalf("ByID(%q) = %#v, want %#v", tt.id, got, want)
			}
		})
	}
}

func TestOutline(t *testing.T) {
	text := textOf(
		para("Intro", richtext.ParagraphStyle{Heading: 1, ID: "a"}),
		para("body", richtext.ParagraphStyle{ID: "b"}),
		para("Part", richtext.ParagraphStyle{Heading: 2, ID: "c"}),
		para("", richtext.ParagraphStyle{Heading: 1, ID: "d"}),
		para("   ", richtext.ParagraphStyle{Heading: 2, ID: "e"}),
		para("Detail", richtext.ParagraphStyle{Heading: 3, ID: "f"}),
	)
	want := []OutlineEntry{
		{BlockID: "a", Index: 0, Start: 0, Level: 1, Text: "Intro"},
		{BlockID: "c", Index: 2, Start: 11, Level: 2, Text: "Part"},
		{BlockID: "f", Index: 5, Start: 21, Level: 3, Text: "Detail"},
	}
	got := Outline(text)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Outline() = %#v, want %#v", got, want)
	}
}

func TestOutlineEmpty(t *testing.T) {
	for _, text := range []richtext.Text{richtext.NewText(""), richtext.Text{}} {
		if got := Outline(text); len(got) != 0 {
			t.Fatalf("Outline(empty) = %#v, want no entries", got)
		}
	}
}

func TestSlashAt(t *testing.T) {
	tests := []struct {
		name   string
		text   richtext.Text
		caret  int
		want   SlashState
		wantOK bool
	}{
		{
			name: "just the slash",
			text: richtext.NewText("/"), caret: 1,
			want:   SlashState{Query: "", Start: 0, End: 1},
			wantOK: true,
		},
		{
			name: "slash and a word",
			text: richtext.NewText("/head"), caret: 5,
			want:   SlashState{Query: "head", Start: 0, End: 5},
			wantOK: true,
		},
		{
			name: "caret inside the word",
			text: richtext.NewText("/head"), caret: 3,
			want:   SlashState{Query: "he", Start: 0, End: 3},
			wantOK: true,
		},
		{
			name: "text after the caret is not the query",
			text: richtext.NewText("/head rest"), caret: 5,
			want:   SlashState{Query: "head", Start: 0, End: 5},
			wantOK: true,
		},
		{
			name: "caret at the slash",
			text: richtext.NewText("/head"), caret: 0,
			wantOK: false,
		},
		{
			name: "no slash",
			text: richtext.NewText("head"), caret: 4,
			wantOK: false,
		},
		{
			name: "slash not at the paragraph start",
			text: richtext.NewText(" /head"), caret: 6,
			wantOK: false,
		},
		{
			name: "second block",
			text: richtext.NewText("a\n/quote"), caret: 99,
			want:   SlashState{Query: "quote", Start: 2, End: 8},
			wantOK: true,
		},
		{
			name: "caret in the block before the slash",
			text: richtext.NewText("a\n/quote"), caret: 1,
			wantOK: false,
		},
		{
			name: "empty trailing block",
			text: richtext.NewText("a\n"), caret: 2,
			wantOK: false,
		},
		{
			name: "empty text",
			text: richtext.NewText(""), caret: 0,
			wantOK: false,
		},
		{
			name: "negative caret",
			text: richtext.NewText("/head"), caret: -1,
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SlashAt(tt.text, tt.caret)
			if ok != tt.wantOK {
				t.Fatalf("SlashAt(%d) ok = %v, want %v", tt.caret, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SlashAt(%d) = %#v, want %#v", tt.caret, got, tt.want)
			}
		})
	}
}

func TestSlashAtNamesTheBlock(t *testing.T) {
	doc := identify(docOf("first\n/second"))
	got, ok := SlashAt(doc.Text(), doc.Text().Len())
	if !ok {
		t.Fatal("SlashAt() = false, want true")
	}
	if want := "p2"; got.BlockID != want {
		t.Fatalf("SlashAt() block ID = %q, want %q", got.BlockID, want)
	}
}

func TestEmptyDocumentsDoNotPanic(t *testing.T) {
	for _, text := range []string{"", "a"} {
		text := text
		t.Run("text "+text, func(t *testing.T) {
			doc := docOf(text)
			for i := -3; i <= 3; i++ {
				At(doc.Text(), i)
				SlashAt(doc.Text(), i)
				ByID(doc.Text(), "")
				Outline(doc.Text())
				Move(doc, i, 0)
				Move(doc, 0, i)
				InsertAfter(doc, i, "new", richtext.ParagraphStyle{})
				Delete(doc, i)
				Apply(doc, "", CommandText, i, i+1)
			}
		})
	}
}
