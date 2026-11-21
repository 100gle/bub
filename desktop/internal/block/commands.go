package block

import (
	"strings"
)

// CommandKind is what a slash command makes a block: the block formats the
// editor offers, each with the Markdown trigger that types it.
type CommandKind uint8

// The block formats the slash menu offers.
const (
	// CommandText is a body block: no heading, list, quote or code.
	CommandText CommandKind = iota
	// CommandHeading1, CommandHeading2 and CommandHeading3 are headings of
	// level 1, 2 and 3.
	CommandHeading1
	CommandHeading2
	CommandHeading3
	// CommandBulletedList is a bulleted list item.
	CommandBulletedList
	// CommandNumberedList is a numbered list item.
	CommandNumberedList
	// CommandTodoList is a checklist item, not yet checked.
	CommandTodoList
	// CommandQuote is a quotation.
	CommandQuote
	// CommandCodeBlock is a block of code.
	CommandCodeBlock
)

// Command is one entry of the slash menu: the format it applies, the label
// the menu shows, the Markdown trigger that types it and what typing
// after the slash filters it by.
type Command struct {
	// Kind is the format Apply gives the block.
	Kind CommandKind
	// Label is what the menu shows.
	Label string
	// Hint is the Markdown trigger that types the format: "#", "-", "1.",
	// "[]", ">" or "</>". CommandText has none.
	Hint string
	// Keywords are the words that filter the entry besides its label.
	Keywords []string
}

// catalog is the slash menu, in the order it shows its entries.
var catalog = []Command{
	{
		Kind:     CommandText,
		Label:    "Text",
		Hint:     "",
		Keywords: []string{"text", "plain", "paragraph", "body", "p"},
	},
	{
		Kind:     CommandHeading1,
		Label:    "Heading 1",
		Hint:     "#",
		Keywords: []string{"heading", "h1", "title", "heading 1", "large"},
	},
	{
		Kind:     CommandHeading2,
		Label:    "Heading 2",
		Hint:     "##",
		Keywords: []string{"heading", "h2", "subtitle", "heading 2", "medium"},
	},
	{
		Kind:     CommandHeading3,
		Label:    "Heading 3",
		Hint:     "###",
		Keywords: []string{"heading", "h3", "heading 3", "small"},
	},
	{
		Kind:     CommandBulletedList,
		Label:    "Bulleted list",
		Hint:     "-",
		Keywords: []string{"bulleted", "bullet", "list", "unordered", "ul"},
	},
	{
		Kind:     CommandNumberedList,
		Label:    "Numbered list",
		Hint:     "1.",
		Keywords: []string{"numbered", "number", "ordered", "list", "ol"},
	},
	{
		Kind:     CommandTodoList,
		Label:    "To-do list",
		Hint:     "[]",
		Keywords: []string{"todo", "to-do", "task", "check", "checkbox", "checklist", "list"},
	},
	{
		Kind:     CommandQuote,
		Label:    "Quote",
		Hint:     ">",
		Keywords: []string{"quote", "blockquote", "quotation"},
	},
	{
		Kind:     CommandCodeBlock,
		Label:    "Code block",
		Hint:     "</>",
		Keywords: []string{"code", "codeblock", "pre", "snippet", "monospace"},
	},
}

// Commands returns the slash menu's entries, in the order the menu shows
// them. The caller must not change them: the Keywords slices are the
// catalog's own.
func Commands() []Command {
	out := make([]Command, len(catalog))
	copy(out, catalog)
	return out
}

// CommandFor returns the catalog entry of kind, and whether there is one.
func CommandFor(kind CommandKind) (Command, bool) {
	for _, c := range catalog {
		if c.Kind == kind {
			return c, true
		}
	}
	return Command{}, false
}

// FilterCommands returns the entries of the slash menu that hold query in
// their label or their keywords, ignoring case and the spaces around it:
// typing "head" offers the headings, "list" the lists. The entries whose
// label holds it come first, as there the match shows, in the catalog's
// order, and the empty query offers them all that way. The caller must not
// change the entries, as with Commands.
func FilterCommands(query string) []Command {
	q := strings.ToLower(strings.TrimSpace(query))
	byLabel := make([]Command, 0, len(catalog))
	byKeyword := make([]Command, 0, len(catalog))
	for _, c := range catalog {
		switch {
		case q == "":
			byLabel = append(byLabel, c)
		case strings.Contains(strings.ToLower(c.Label), q):
			byLabel = append(byLabel, c)
		case commandHoldsKeyword(c, q):
			byKeyword = append(byKeyword, c)
		}
	}
	return append(byLabel, byKeyword...)
}

// commandHoldsKeyword reports whether one of c's keywords holds the
// lowercased query q.
func commandHoldsKeyword(c Command, q string) bool {
	for _, k := range c.Keywords {
		if strings.Contains(strings.ToLower(k), q) {
			return true
		}
	}
	return false
}
