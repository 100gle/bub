// Package markdown reads a Markdown document with goldmark, so Bub Desktop
// and the rest of the Go ecosystem agree on what CommonMark means: which lines
// are headings, which are fenced code, and which are indented code.
//
// It exists because richtext keeps the blocks of code of a page but not the
// language of their fences: a ParagraphStyle has CodeBlock alone, and
// richtext's Markdown writes a fence with no info string. The editor therefore
// remembers the language of each code block beside the document, keyed by the
// first paragraph's ID, and writes it back when it saves.
package markdown

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// parser is the CommonMark parser with the extensions the pages use: GFM for
// its tables and task lists, and the typographer for the quotes a writer types.
// Front matter is stripped before parsing; goldmark has no extension for it.
var parser = goldmark.New(
	goldmark.WithExtensions(extension.GFM, extension.Typographer),
)

// Fence is one fenced block of code of a Markdown document.
type Fence struct {
	// Lang is the info string's first word, "" when the fence names none.
	Lang string
}

// Fences returns the fenced blocks of code of markdown in order. Indented code
// blocks are not fenced and are left out, as CommonMark tells them apart from
// fences by their indentation.
func Fences(markdown string) []Fence {
	doc, source := parse(markdown)
	var out []Fence
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		f, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}
		out = append(out, Fence{Lang: infoLanguage(f, source)})
		return ast.WalkContinue, nil
	})
	return out
}

// FenceLanguages returns the language each fenced block of code of markdown
// names, in order: the info string's first word, "" for a fence that names
// none.
func FenceLanguages(markdown string) []string {
	fences := Fences(markdown)
	langs := make([]string, len(fences))
	for i, f := range fences {
		langs[i] = f.Lang
	}
	return langs
}

// Title returns the text of the first heading of markdown, or "" when it has
// none. Only the document's top-level headings count: a heading inside a
// quotation is a quotation's, not the page's. YAML front matter is skipped.
func Title(markdown string) string {
	doc, source := parse(markdown)
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		h, ok := n.(*ast.Heading)
		if !ok {
			continue
		}
		var b bytes.Buffer
		for i := 0; i < h.Lines().Len(); i++ {
			segment := h.Lines().At(i)
			b.Write(segment.Value(source))
		}
		return strings.TrimSpace(b.String())
	}
	return ""
}

// HasHeading reports whether markdown already has a top-level heading, which
// is what renaming a page rewrites rather than adds.
func HasHeading(markdown string) bool {
	doc, _ := parse(markdown)
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if _, ok := n.(*ast.Heading); ok {
			return true
		}
	}
	return false
}

// parse parses markdown, with any YAML front matter removed, and returns the
// document with the source its segments index into.
func parse(markdown string) (ast.Node, []byte) {
	markdown = strings.TrimPrefix(markdown, "\ufeff")
	markdown = stripFrontMatter(markdown)
	source := []byte(markdown)
	return parser.Parser().Parse(text.NewReader(source)), source
}

// stripFrontMatter removes a leading YAML front matter block, the lines from
// an opening --- to the next --- or ..., and the blank line after it.
func stripFrontMatter(markdown string) string {
	lines := strings.Split(markdown, "\n")
	if len(lines) == 0 || strings.TrimSuffix(lines[0], "\r") != "---" {
		return markdown
	}
	for i := 1; i < len(lines); i++ {
		switch strings.TrimSuffix(lines[i], "\r") {
		case "---", "...":
			return strings.Join(lines[i+1:], "\n")
		}
	}
	// An unterminated block is not front matter: the document is as written.
	return markdown
}

// infoLanguage is the first word of a fence's info string.
func infoLanguage(f *ast.FencedCodeBlock, source []byte) string {
	if f.Info == nil {
		return ""
	}
	return FirstWord(string(f.Info.Value(source)))
}

// FirstWord returns the first word of s, "" when it has none.
func FirstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}
