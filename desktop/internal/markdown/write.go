package markdown

import (
	"strings"

	"github.com/yuin/goldmark/ast"
)

// ApplyFenceLanguages returns markdown with the language of each opening fence
// taken from langs, in order: the code, and the fences that close a block, are
// left as they were. A fence that already names a language keeps it, so
// applying the same languages again changes nothing.
//
// This is the one place the package edits Markdown rather than reading it: the
// page's fences come from richtext's Markdown with their info strings gone, and
// putting them back means rewriting those lines. goldmark parses and does not
// write back, so the scan here is hand-written; Fences, which reads the same
// documents, is goldmark's, and the two agree on what opens and closes a block.
func ApplyFenceLanguages(markdown string, langs []string) string {
	if len(langs) == 0 {
		return markdown
	}
	lines := strings.Split(markdown, "\n")
	open := false
	var marker byte
	width := 0
	next := 0 // the language the next opening fence takes
	for i, line := range lines {
		before, fence, rest, ok := fenceAt(line)
		if open {
			if ok && fence[0] == marker && len(fence) >= width && strings.TrimSpace(rest) == "" {
				open = false
			}
			continue
		}
		if !ok {
			continue
		}
		open, marker, width = true, fence[0], len(fence)
		if next >= len(langs) {
			continue
		}
		lang := langs[next]
		next++
		if lang == "" || FirstWord(rest) != "" {
			continue
		}
		lines[i] = before + fence + lang + rest
	}
	return strings.Join(lines, "\n")
}

// fenceAt reads a line as the opening of a fenced code block: before is what
// comes before its run of ` or ~, marker that run and rest what follows it. ok
// is false when the line is not a fence, as when it is indented four spaces or
// more, which is indented code.
func fenceAt(line string) (before, marker, rest string, ok bool) {
	s := strings.TrimSuffix(line, "\r")
	i := 0
	for {
		// Up to three spaces and a blockquote marker, as often as the line
		// nests quotations.
		j := i
		for n := 0; n < 3 && j < len(s) && s[j] == ' '; n++ {
			j++
		}
		if j >= len(s) || s[j] != '>' {
			break
		}
		j++
		if j < len(s) && s[j] == ' ' {
			j++
		}
		i = j
	}
	// Up to three spaces of indentation before the fence: four would be code.
	for n := 0; n < 3 && i < len(s) && s[i] == ' '; n++ {
		i++
	}
	if i >= len(s) || (s[i] != '`' && s[i] != '~') {
		return "", "", "", false
	}
	c := s[i]
	n := 0
	for i+n < len(s) && s[i+n] == c {
		n++
	}
	if n < 3 {
		return "", "", "", false
	}
	// A backtick fence's info string cannot hold a backtick: such a line is
	// the text of the block, not one of its fences.
	if c == '`' && strings.Contains(s[i+n:], "`") {
		return "", "", "", false
	}
	return s[:i], s[i : i+n], line[i+n:], true
}

// ReplaceTitle replaces the text of the first heading of markdown with title,
// keeping its level, or prepends "# title" plus a blank line when the document
// has no heading. A leading YAML front matter block is left untouched.
//
// goldmark finds the heading and reports its level, so a heading inside a
// fence or an indented block is not the one rewritten; the line itself is
// rewritten here, as goldmark parses and does not write back.
func ReplaceTitle(markdown, title string) string {
	source := strings.TrimPrefix(markdown, "\ufeff")
	line, level, style, ok := titleLine(source)
	if !ok {
		return withTitle(source, title)
	}
	lines := strings.Split(source, "\n")
	cr := ""
	if strings.HasSuffix(lines[line], "\r") {
		cr = "\r"
	}
	if style == setext {
		// The text is the line above the underline; the underline stays.
		lines[line] = title + cr
	} else {
		lines[line] = strings.Repeat("#", level) + " " + title + cr
	}
	return strings.Join(lines, "\n")
}

// withTitle prepends "# title" and a blank line, after the front matter when
// the document has one.
func withTitle(source, title string) string {
	lines := strings.Split(source, "\n")
	if start := frontMatterLines(lines); start > 0 {
		out := append([]string{}, lines[:start]...)
		out = append(out, "# "+title, "")
		out = append(out, lines[start:]...)
		return strings.Join(out, "\n")
	}
	return "# " + title + "\n\n" + source
}

// headingStyle is how the document wrote its first heading.
type headingStyle int

const (
	atx headingStyle = iota
	setext
)

// titleLine returns the line of the document's first heading, its level, how
// it is written, and whether there is one. The line is in source, front matter
// included.
func titleLine(source string) (line, level int, style headingStyle, ok bool) {
	body, skip := stripFrontMatterWithOffset(source)
	doc, parsed := parse(body)
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		h, isHeading := n.(*ast.Heading)
		if !isHeading || h.Lines().Len() == 0 {
			continue
		}
		segment := h.Lines().At(0)
		off := segment.Start
		for off > 0 && parsed[off-1] != '\n' {
			off--
		}
		line = strings.Count(body[:off], "\n") + skip
		// A setext heading writes its text on the line above its underline.
		style = atx
		if line+1 < len(strings.Split(source, "\n")) {
			next := strings.TrimSpace(strings.TrimSuffix(strings.Split(source, "\n")[line+1], "\r"))
			if next == "===" || next == "---" || (len(next) > 0 && allOf(next, '=')) || (len(next) > 0 && allOf(next, '-')) {
				style = setext
			}
		}
		return line, h.Level, style, true
	}
	return 0, 0, atx, false
}

// allOf reports whether s is every one of c.
func allOf(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != c {
			return false
		}
	}
	return true
}

// frontMatterLines is how many lines a leading YAML front matter block takes,
// 0 when the document does not start with one.
func frontMatterLines(lines []string) int {
	if len(lines) == 0 || strings.TrimSuffix(lines[0], "\r") != "---" {
		return 0
	}
	for i := 1; i < len(lines); i++ {
		switch strings.TrimSuffix(lines[i], "\r") {
		case "---", "...":
			return i + 1
		}
	}
	return 0
}

// stripFrontMatterWithOffset is stripFrontMatter, reporting how many lines it
// took off the front.
func stripFrontMatterWithOffset(markdown string) (string, int) {
	lines := strings.Split(markdown, "\n")
	if n := frontMatterLines(lines); n > 0 {
		return strings.Join(lines[n:], "\n"), n
	}
	return markdown, 0
}
