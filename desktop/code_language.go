package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/richtext"

	"github.com/bubbuild/bub/desktop/internal/block"
	md "github.com/bubbuild/bub/desktop/internal/markdown"
)

// --- The app's code languages ---------------------------------------------

// codeBlockGroups groups the paragraphs of bs into code blocks: a block of
// code is a run of consecutive code paragraphs, as Markdown writes one fenced
// block.
func codeBlockGroups(bs []block.Block) [][]block.Block {
	var groups [][]block.Block
	for i := 0; i < len(bs); {
		if !bs[i].Style.CodeBlock {
			i++
			continue
		}
		j := i + 1
		for j < len(bs) && bs[j].Style.CodeBlock {
			j++
		}
		groups = append(groups, bs[i:j])
		i = j
	}
	return groups
}

// setMarkdown shows markdown as the open page, saved at path, remembering the
// language of each code block's fence, which richtext parses away. Paragraphs
// of a block after its first are keyed by the first's ID, as Markdown writes
// them back as one fenced block.
func (a *app) setMarkdown(source, path string) {
	langs := md.FenceLanguages(source)
	a.setDocument(richtext.ParseMarkdown(source), path)
	a.codeLangs = map[string]string{}
	for i, group := range codeBlockGroups(a.blocks()) {
		if i < len(langs) && langs[i] != "" {
			a.codeLangs[group[0].ID] = langs[i]
		}
	}
	a.codeLangsRev++
}

// forgetCodeLanguages clears the languages remembered for the open page, as
// another page or another text takes its place.
func (a *app) forgetCodeLanguages() {
	a.codeLangs = map[string]string{}
	a.codeLangsRev++
}

// pageCodeLanguages returns the language of each code block of bs, in order,
// as remembered, and "" for a block the app does not know the language of.
func (a *app) pageCodeLanguages(bs []block.Block) []string {
	groups := codeBlockGroups(bs)
	langs := make([]string, len(groups))
	for i, group := range groups {
		langs[i] = a.codeLangs[group[0].ID]
	}
	return langs
}

// markdownText returns the open page as Markdown with the languages of its
// code fences, which richtext's Markdown leaves out, as the app is what
// remembers them.
func (a *app) markdownText() string {
	return md.ApplyFenceLanguages(a.doc.Text().Markdown(), a.pageCodeLanguages(a.blocks()))
}

// --- Choosing a code block's language -------------------------------------

// codeBlockOf returns the code block holding the paragraph named id: the
// paragraphs of one block share the first paragraph's ID as their key.
func (a *app) codeBlockOf(id string) (block.Block, []block.Block, bool) {
	bs := a.blocks()
	for _, group := range codeBlockGroups(bs) {
		if group[0].ID == id {
			return group[0], group, true
		}
	}
	return block.Block{}, nil, false
}

// codeLanguage returns the language remembered for the code block named id,
// "" when it names none.
func (a *app) codeLanguage(id string) string {
	return a.codeLangs[id]
}

// setCodeLanguage gives the code block named id language, "" to name none.
// The language is what the fence is written with when the page saves, and what
// the editor highlights the code as; codeLangsRev rises so the next frame
// tokenizes the block again.
func (a *app) setCodeLanguage(id, language string) {
	if id == "" {
		return
	}
	language = strings.TrimSpace(strings.ToLower(language))
	if a.codeLangs == nil {
		a.codeLangs = map[string]string{}
	}
	if a.codeLangs[id] == language {
		return
	}
	if language == "" {
		delete(a.codeLangs, id)
	} else {
		a.codeLangs[id] = language
	}
	a.codeLangsRev++
	// The page's Markdown changed with the fence, so it is edited.
	a.setDirty(true)
	a.scheduleSave()
}

// codeText returns the code of the block whose paragraphs are group, one
// paragraph per line, as the block reads.
func codeText(group []block.Block) string {
	lines := make([]string, len(group))
	for i, b := range group {
		lines[i] = b.Text
	}
	return strings.Join(lines, "\n")
}

// --- Code highlighting ----------------------------------------------------
//
// The editor's Spans hook colors a paragraph as it lays it out. The app gives
// it the code paragraphs' languages, which richtext does not keep, and the
// state each paragraph starts in, which a tokenizer carries from line to line
// as a block of code has a comment or a string spread over several of them.

// codeSpansKey is what a frame's code highlighting depends on: the document's
// version, the appearance, and the languages the app remembers. The editor's
// SpansVersion rises when it changes, so the editor lays its paragraphs out
// again only when what they draw has changed.
type codeSpansKey struct {
	version uint64
	dark    bool
	langs   int
}

// codeHighlight is the highlighting of one code paragraph: the tokenizer of
// its block's language and the state the paragraph begins in.
type codeHighlight struct {
	tok   editor.Tokenizer
	state uint64
}

// codeHi is the code highlighting of the frame being built: the colors its
// tokens take and, by paragraph index, what highlights each code paragraph.
type codeHi struct {
	theme *editor.Theme
	paras map[int]codeHighlight
}

// beginCodeFrame prepares the frame's code highlighting and returns the
// editor's SpansVersion. It tokenizes nothing again until the document, the
// appearance or the remembered languages change, as a frame that changed none
// of them draws what the frame before it drew.
func (a *app) beginCodeFrame(c *ui.Context, bs []block.Block) int {
	key := codeSpansKey{version: a.doc.Version(), dark: c.Theme().Dark, langs: a.codeLangsRev}
	if key != a.codeSpansKey {
		a.codeSpansKey = key
		a.codeSpansVer++
		a.codeHi = a.codeHighlighting(c.Theme(), bs)
	}
	return a.codeSpansVer
}

// codeHighlighting tokenizes the code blocks of bs in their languages, each
// paragraph starting where the one before it ended.
func (a *app) codeHighlighting(theme *ui.Theme, bs []block.Block) *codeHi {
	hi := &codeHi{theme: editor.DefaultTheme(theme), paras: map[int]codeHighlight{}}
	for _, group := range codeBlockGroups(bs) {
		lang := a.codeLangs[group[0].ID]
		if lang == "" {
			continue
		}
		l := editor.LanguageNamed(lang)
		if l == nil || l.Tokenizer == nil {
			continue
		}
		var state uint64
		for _, b := range group {
			hi.paras[b.Index] = codeHighlight{tok: l.Tokenizer, state: state}
			_, state = l.Tokenizer.Tokenize(b.Text, state, nil)
		}
	}
	if len(hi.paras) == 0 {
		return nil
	}
	return hi
}

// codeSpans colors the spans a paragraph is drawn as when the paragraph is a
// block of code whose language the editor knows: it joins the spans' text,
// splits it into the language's tokens, and returns one span per token, in the
// token colors of the editor's theme. Any other paragraph, and one of a
// language the editor does not know, keeps the spans it had.
func (a *app) codeSpans(p int, style richtext.ParagraphStyle, spans []ui.Span) []ui.Span {
	hi := a.codeHi
	if hi == nil || !style.CodeBlock || len(spans) == 0 {
		return spans
	}
	para, ok := hi.paras[p]
	if !ok || para.tok == nil {
		return spans
	}
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	text := b.String()
	if text == "" {
		return spans
	}
	toks, _ := para.tok.Tokenize(text, para.state, nil)
	if len(toks) == 0 {
		return spans
	}
	// The paragraph's own span keeps the code block's font and size: a token
	// changes the color, and the italics or bold it is drawn with.
	base := spans[0]
	base.Text = ""
	out := make([]ui.Span, 0, len(toks)+1)
	start := 0
	for _, tok := range toks {
		// Token.End counts bytes of the line, as the spans' text does.
		end := min(tok.End, len(text))
		if end <= start {
			continue
		}
		out = append(out, hi.tokenSpan(base, text[start:end], tok.Kind))
		start = end
	}
	if start < len(text) {
		s := base
		s.Text = text[start:]
		s.Color = hi.theme.Text
		out = append(out, s)
	}
	if len(out) == 0 {
		return spans
	}
	return out
}

// tokenSpan returns text as the span of a token of kind: the base's font, size
// and decorations, the kind's color, and its italics or bold.
func (hi *codeHi) tokenSpan(base ui.Span, text string, kind editor.TokenKind) ui.Span {
	s := base
	s.Text = text
	s.Color = hi.theme.TokenColor(kind)
	style := hi.theme.Tokens[kind]
	if style.Italic {
		s.Italic = true
	}
	if style.Bold {
		s.Weight = tokens.WeightStrong
	}
	return s
}
