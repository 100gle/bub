// Package prompt builds what a session/prompt gets when Bub writes for the
// writer: the block actions of the Bub menu, and the chat turns.
//
// The prompts are text alone, so they live apart from the window that shows
// them and can be read and tested without one. The context each takes is what
// the writer attached, gathered by the caller.
package prompt

import (
	"fmt"
	"strings"
)

// maxChatOutline is how many of the page's headings a chat prompt carries, so
// that a long page stays a little context rather than a document.
const maxChatOutline = 12

// ChatContext is the context a chat turn carries: the pages the conversation
// references, and, for the referenced page that is open, its title, the
// headings the settings allow, and the text around the caret.
type ChatContext struct {
	// Refs are the referenced pages, workspace-relative.
	Refs []string
	// Title is the open referenced page's title, "" when it is not open.
	Title string
	// Outline is that page's headings, "## Section", when enabled.
	Outline []string
	// Caret is the paragraph holding the caret, when it has text.
	Caret string
	// Prev is the paragraph before it, when enabled.
	Prev string
	// Next is the paragraph after it, when enabled.
	Next string
}

// Chat is what session/prompt gets for one turn: the writer's instruction and
// the context the writer attached, never the page that merely happens to be
// open. It asks for a concise Markdown answer.
func Chat(instruction string, ctx ChatContext) string {
	instruction = strings.TrimSpace(instruction)
	var b strings.Builder
	b.WriteString("You are Bub, the AI writing partner in the writer's workspace. Answer the writer directly, as a thoughtful editor.\n")
	if len(ctx.Refs) > 0 {
		b.WriteString("\nReferenced pages (also attached as resource links):\n")
		for _, rel := range ctx.Refs {
			rel = strings.TrimSpace(rel)
			if rel == "" {
				continue
			}
			fmt.Fprintf(&b, "- %s\n", rel)
		}
	}
	if ctx.Title == "" {
		if len(ctx.Refs) == 0 {
			b.WriteString("\nNo page is referenced, so answer from the conversation alone.\n")
		}
	} else {
		fmt.Fprintf(&b, "\nOpen page: %q\n", ctx.Title)
		if outline := trimChatOutline(ctx.Outline); len(outline) > 0 {
			b.WriteString("\nPage outline:\n")
			for _, heading := range outline {
				fmt.Fprintf(&b, "- %s\n", heading)
			}
		}
		if caret := strings.TrimSpace(ctx.Caret); caret != "" {
			fmt.Fprintf(&b, "\nWhere the caret is:\n%s\n", caret)
		}
		if prev := strings.TrimSpace(ctx.Prev); prev != "" {
			fmt.Fprintf(&b, "\nPreceding paragraph: %s\n", prev)
		}
		if next := strings.TrimSpace(ctx.Next); next != "" {
			fmt.Fprintf(&b, "\nFollowing paragraph: %s\n", next)
		}
	}
	fmt.Fprintf(&b, "\nThe writer asks:\n%s\n", instruction)
	b.WriteString("\nReply in concise Markdown. Do not repeat the page back to the writer, and do not restate what it already says: say only what helps with the page as it is now.\n")
	return b.String()
}

// trimChatOutline keeps the first maxChatOutline headings, dropping blanks.
func trimChatOutline(headings []string) []string {
	out := make([]string, 0, min(len(headings), maxChatOutline))
	for _, heading := range headings {
		if heading = strings.TrimSpace(heading); heading == "" {
			continue
		}
		out = append(out, heading)
		if len(out) == maxChatOutline {
			break
		}
	}
	return out
}
