package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"

	"github.com/bubbuild/bub/desktop/internal/block"
	md "github.com/bubbuild/bub/desktop/internal/markdown"
	"github.com/bubbuild/bub/desktop/internal/workspace"
)

func TestFenceLanguageRoundTripThroughMarkdown(t *testing.T) {
	const page = "# Code\n\n```go\nfunc main() {\n\tprintln(\"hi\")\n}\n```\n"
	a := newApp()
	a.setMarkdown(page, "")
	// richtext's Markdown drops the language: the app puts it back.
	if got := md.FenceLanguages(a.doc.Text().Markdown()); !slices.Equal(got, []string{""}) {
		t.Fatalf("richtext's Markdown scanned as %q, want a bare fence", got)
	}
	saved := a.markdownText()
	if scanned := md.FenceLanguages(saved); !slices.Equal(scanned, []string{"go"}) {
		t.Errorf("a saved page scanned as %q, want [go]:\n%s", scanned, saved)
	}
	// Reading it back keeps the whole document.
	if got, want := a.doc.Text().Markdown(), richtextMarkdownOf(saved); got != want {
		t.Errorf("the saved page reads back as %q, want %q", got, want)
	}
}

// richtextMarkdownOf parses markdown and returns it as richtext writes it, so
// that a page written with a fence language can be compared with the document
// it came from.
func richtextMarkdownOf(markdown string) string {
	return richtext.ParseMarkdown(markdown).Markdown()
}

// --- A page on disk -------------------------------------------------------

// firstCodeBlock returns the first paragraph of the first code block of bs.
func firstCodeBlock(t *testing.T, bs []block.Block) block.Block {
	t.Helper()
	for _, b := range bs {
		if b.Style.CodeBlock {
			return b
		}
	}
	t.Fatal("the page has no code block")
	return block.Block{}
}

// showCodeTools forces the code block tools out, as a pointer over a block
// would in the window: the tests drive the tools without a pointer.
func showCodeTools(t *testing.T, a *app) {
	t.Helper()
	a.codeShowAll = true
}

func TestPageCodeFenceLanguageSurvivesSave(t *testing.T) {
	root := t.TempDir()
	ws, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := ws.CreatePage("", "Code")
	if err != nil {
		t.Fatal(err)
	}
	const page = "# Code\n\n```go\nfunc main() {\n\tprintln(\"hi\")\n}\n```\n"
	if err := ws.WritePage(rel, page); err != nil {
		t.Fatal(err)
	}

	a := newApp()
	a.ws = ws
	a.settings.Workspace = root
	a.openPage(rel)
	if a.current != rel {
		t.Fatalf("opened %q, want %q", a.current, rel)
	}
	// The language is keyed by the first paragraph of the block.
	first := firstCodeBlock(t, a.blocks())
	if got := a.codeLangs[first.ID]; got != "go" {
		t.Fatalf("the code language is %q, want go (codeLangs %v)", got, a.codeLangs)
	}
	if got := a.markdownText(); !strings.Contains(got, "```go\n") {
		t.Fatalf("the page writes no fence language:\n%s", got)
	}

	// An edit, saved, keeps the fence language: this is what autosave writes.
	a.doc.SetSelection(0, 0)
	a.doc.InsertText("Intro.\n\n")
	a.setDirty(true)
	a.savePage()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "```go\n") {
		t.Fatalf("the saved page lost the fence language:\n%s", data)
	}
	if !strings.Contains(string(data), "Intro.") {
		t.Fatalf("the saved page lost the edit:\n%s", data)
	}

	// Reopening the page reads the language back.
	a.openPage(rel)
	if got := a.codeLangs[firstCodeBlock(t, a.blocks()).ID]; got != "go" {
		t.Fatalf("reopened with language %q, want go", got)
	}

	// A rename rereads the page too, and keeps the languages with it.
	a.renamePage(rel, "Renamed")
	if a.current == rel {
		t.Fatal("the rename left the old page open")
	}
	if got := a.codeLangs[firstCodeBlock(t, a.blocks()).ID]; got != "go" {
		t.Fatalf("the renamed page has language %q, want go", got)
	}
	if got := a.markdownText(); !strings.Contains(got, "```go\n") {
		t.Fatalf("the renamed page writes no fence language:\n%s", got)
	}
}

// --- Highlighting in the editor -------------------------------------------

func TestCodeSpansHighlight(t *testing.T) {
	// A paragraph between two fences: richtext keeps adjacent blocks of code
	// as consecutive code paragraphs, which save as one fenced block.
	const page = "# Code\n\n" +
		"```go\nfunc main() {\n\tprintln(\"hi\")\n}\n```\n\n" +
		"Between.\n\n" +
		"```\nplain text\n```\n\n" +
		"Between too.\n\n" +
		"```nonsense\nmystery\n```\n"
	a := newApp()
	a.sidebarShown = false
	a.setMarkdown(page, "")
	a.current = "code.md"
	a.path = "/tmp/code.md"

	bs := a.blocks()
	goIndex, plainIndex, unknownIndex := -1, -1, -1
	for _, b := range bs {
		switch b.Text {
		case "func main() {":
			goIndex = b.Index
		case "plain text":
			plainIndex = b.Index
		case "mystery":
			unknownIndex = b.Index
		}
	}
	if goIndex < 0 || plainIndex < 0 || unknownIndex < 0 {
		t.Fatalf("the page's blocks are %+v", bs)
	}

	// The frame prepares the highlighting the editor builds its spans with.
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	hi := a.codeHi
	if hi == nil {
		t.Fatal("the frame prepared no code highlighting")
	}

	// A Go line splits into its tokens, the keyword in a color of its own.
	spans := []ui.Span{{Text: bs[goIndex].Text}}
	got := a.codeSpans(goIndex, bs[goIndex].Style, spans)
	if len(got) < 2 {
		t.Fatalf("the Go line was not split into tokens: %q", spanTexts(got))
	}
	if joined := strings.Join(spanTexts(got), ""); joined != bs[goIndex].Text {
		t.Fatalf("the tokens read %q, want %q", joined, bs[goIndex].Text)
	}
	keyword := got[0]
	if keyword.Text != "func" {
		t.Fatalf("the first token is %q, want func", keyword.Text)
	}
	if keyword.Color == (ui.Color{}) || keyword.Color == hi.theme.Text {
		t.Errorf("the keyword is not colored apart from the text: %v", keyword.Color)
	}
	if len(got) > 1 && keyword.Color == got[len(got)-1].Color {
		t.Errorf("every token has the same color: %v", keyword.Color)
	}

	// A block whose fence names no language keeps the spans it had.
	plain := []ui.Span{{Text: "plain text"}}
	if unchanged := a.codeSpans(plainIndex, bs[plainIndex].Style, plain); !slices.Equal(unchanged, plain) {
		t.Errorf("a language-less block was colored: %+v", unchanged)
	}
	// So does a block of a language the editor does not know.
	mystery := []ui.Span{{Text: "mystery"}}
	if unchanged := a.codeSpans(unknownIndex, bs[unknownIndex].Style, mystery); !slices.Equal(unchanged, mystery) {
		t.Errorf("an unknown language was colored: %+v", unchanged)
	}
	// And a paragraph that is not code, even a colored one, is left alone.
	title := []ui.Span{{Text: "Code", Color: ui.RGB(1, 2, 3)}}
	if unchanged := a.codeSpans(0, bs[0].Style, title); !slices.Equal(unchanged, title) {
		t.Errorf("a heading was colored: %+v", unchanged)
	}

	// The editor's SpansVersion changes when what is drawn changes, and not
	// when the frame is drawn again as it was.
	version := a.codeSpansVer
	tt.Frame()
	if a.codeSpansVer != version {
		t.Errorf("SpansVersion changed with nothing changed: %d, then %d", version, a.codeSpansVer)
	}
	a.doc.SetSelection(0, 0)
	a.doc.InsertText("Edit ")
	tt.Frame()
	if a.codeSpansVer == version {
		t.Error("SpansVersion did not change after an edit")
	}
}

// spanTexts returns the text of each span.
func spanTexts(spans []ui.Span) []string {
	texts := make([]string, len(spans))
	for i, s := range spans {
		texts[i] = s.Text
	}
	return texts
}

// TestCodeBlockLanguageFromTheTools checks the tools on a code block the
// writer made in the window: a new block has no language and no highlighting
// until one is chosen, and choosing one highlights the code and writes the
// fence.
func TestCodeBlockLanguageFromTheTools(t *testing.T) {
	a := newApp()
	a.sidebarShown = false
	a.setMarkdown("# Code\n\nhello\n", "")
	a.current = "code.md"
	a.path = "/tmp/code.md"

	// The writer turns the paragraph into a code block with "/code".
	target := a.blocks()[1]
	a.slash = &slashState{blockID: target.ID, start: -1, end: -1}
	cmd, ok := block.CommandFor(block.CommandCodeBlock)
	if !ok {
		t.Fatal("no code block command")
	}
	a.applySlash(cmd)
	code := firstCodeBlock(t, a.blocks())
	if got := a.codeLanguage(code.ID); got != "" {
		t.Fatalf("a new code block has language %q, want none", got)
	}

	// No language, no highlighting.
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1100, 800)
	tt.Frame()
	if a.codeHi != nil && len(a.codeHi.paras) != 0 {
		t.Errorf("a language-less block was tokenized: %v", a.codeHi.paras)
	}

	// Choosing Go from the tools highlights it and writes the fence.
	before := a.codeLangsRev
	a.setCodeLanguage(code.ID, "Go")
	if a.codeLangsRev == before {
		t.Error("choosing a language did not ask the editor to draw again")
	}
	if got := a.codeLanguage(code.ID); got != "go" {
		t.Errorf("language = %q, want go", got)
	}
	tt.Frame()
	if a.codeHi == nil || len(a.codeHi.paras) == 0 {
		t.Fatal("the block was still not tokenized after choosing a language")
	}
	if got, want := a.markdownText(), "```go\nhello\n```"; !strings.Contains(got, want) {
		t.Errorf("saved Markdown = %q, want it to hold %q", got, want)
	}
	if !a.dirty {
		t.Error("choosing a language did not mark the page edited")
	}

	// Choosing none takes the language back off the fence.
	a.setCodeLanguage(code.ID, "")
	if got := a.codeLanguage(code.ID); got != "" {
		t.Errorf("language = %q, want none", got)
	}
	if got := a.markdownText(); strings.Contains(got, "```go") {
		t.Errorf("the fence kept its language: %q", got)
	}
}

func TestCodeTextJoinsTheBlocksParagraphs(t *testing.T) {
	a := newApp()
	a.setMarkdown("```go\nfunc main() {\n\treturn 1\n}\n```\n", "")
	code := firstCodeBlock(t, a.blocks())
	_, group, ok := a.codeBlockOf(code.ID)
	if !ok {
		t.Fatal("the block was not found by its ID")
	}
	want := "func main() {\n\treturn 1\n}"
	if got := codeText(group); got != want {
		t.Errorf("codeText = %q, want %q", got, want)
	}
}

// TestCodeBlockToolsRender checks the tools the writer sees on a code block:
// its language and a copy button, and the languages the menu offers.
func TestCodeBlockToolsRender(t *testing.T) {
	a := newApp()
	a.sidebarShown = false
	a.setMarkdown("# Code\n\n```go\nfunc main() {}\n```\n", "")
	a.current = "code.md"
	a.path = "/tmp/code.md"

	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1100, 800)
	tt.Frame()
	code := firstCodeBlock(t, a.blocks())

	// The tools sit out of the way until the block is reached for: idle, no
	// language, no copy button is shown.
	if tt.HasText("Copy") {
		t.Errorf("the copy button shows while the block is idle: %q", tt.Texts())
	}
	showCodeTools(t, a)
	tt.Frame()
	if !tt.HasText("go") {
		t.Errorf("the block does not show its language on hover: %q", tt.Texts())
	}
	if !tt.HasText("Copy") {
		t.Errorf("on hover the block has no copy button: %q", tt.Texts())
	}

	// The language opens a menu of the languages the editor highlights.
	if err := tt.Click("Code language: go"); err != nil {
		t.Fatalf("click the language: %v", err)
	}
	tt.Frame()
	if a.codeMenu != code.ID {
		t.Fatalf("the menu did not open for %q (open: %q)", code.ID, a.codeMenu)
	}
	if !tt.HasText("Rust") || !tt.HasText("Python") || !tt.HasText("Plain text") {
		t.Fatalf("the menu lists no languages: %q", tt.Texts())
	}

	// Choosing one sets it on the block and closes the menu.
	if err := tt.Click("Rust"); err != nil {
		t.Fatalf("click Rust: %v", err)
	}
	tt.Frame()
	if got := a.codeLanguage(code.ID); got != "rust" {
		t.Errorf("language = %q, want rust", got)
	}
	if a.codeMenu != "" {
		t.Error("the menu stayed open after a choice")
	}
}

// TestCodeCopyTakesTheCode checks the copy button copies the block as code —
// every line, indentation kept — and confirms itself on the button.
func TestCodeCopyTakesTheCode(t *testing.T) {
	a := newApp()
	a.sidebarShown = false
	a.setMarkdown("# Code\n\n```go\nfunc main() {\n\treturn 1\n}\n```\n", "")
	a.current = "code.md"
	a.path = "/tmp/code.md"

	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1100, 800)
	tt.Frame()
	var copied string
	a.codeCopyHook = func(s string) { copied = s }
	showCodeTools(t, a)
	tt.Frame()
	if err := tt.Click("Copy"); err != nil {
		t.Fatalf("click copy: %v", err)
	}
	tt.Frame()
	want := "func main() {\n\treturn 1\n}"
	if copied != want {
		t.Errorf("copied %q, want %q", copied, want)
	}
	if !tt.HasText("Copied") {
		t.Errorf("the button did not confirm the copy: %q", tt.Texts())
	}
}

// TestCodeCopyConfirmsOncePerVisit checks the copy button says "Copied" while
// the pointer stays on it, and starts from "Copy" when the pointer comes back.
func TestCodeCopyConfirmsOncePerVisit(t *testing.T) {
	a := newApp()
	a.sidebarShown = false
	a.setMarkdown("```go\nx := 1\n```\n", "")
	a.current = "code.md"
	a.path = "/tmp/code.md"

	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1100, 600)
	tt.Frame()
	showCodeTools(t, a)
	tt.Frame()
	if err := tt.Click("Copy"); err != nil {
		t.Fatalf("click copy: %v", err)
	}
	tt.Frame()
	if !tt.HasText("Copied") {
		t.Fatalf("the button did not confirm the copy: %q", tt.Texts())
	}
	rect, ok := tt.Find("Copied")
	if !ok {
		t.Fatal("no Copied label to move away from")
	}
	// The pointer leaves, as to another part of the page.
	tt.Move(rect.X-300, rect.Y+120)
	tt.Frame()
	if !tt.HasText("Copy") || tt.HasText("Copied") {
		t.Errorf("the button did not go back to Copy: %q", tt.Texts())
	}
}
