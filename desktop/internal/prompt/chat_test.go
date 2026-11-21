package prompt

import (
	"fmt"
	"strings"
	"testing"
)

func TestChat(t *testing.T) {
	page := ChatContext{
		Refs:    []string{"essays/The Quiet Web.md"},
		Title:   "The Quiet Web",
		Outline: []string{"# The Quiet Web", "## Write in blocks", "## Ask Bub"},
		Caret:   "Fast pages feel more trustworthy.",
		Prev:    "Intro line.",
		Next:    "Last line.",
	}
	tests := []struct {
		name    string
		ask     string
		ctx     ChatContext
		wants   []string
		notWant []string
	}{
		{
			name: "page context",
			ask:  "Make this clearer",
			ctx:  page,
			wants: []string{
				"Referenced pages (also attached as resource links):",
				"- essays/The Quiet Web.md",
				`Open page: "The Quiet Web"`,
				"Page outline:",
				"- ## Write in blocks",
				"Where the caret is:",
				"Fast pages feel more trustworthy.",
				"Preceding paragraph: Intro line.",
				"Following paragraph: Last line.",
				"The writer asks:",
				"Make this clearer",
				"concise Markdown",
				"Do not repeat the page",
			},
			notWant: []string{"No page is referenced"},
		},
		{
			name: "no page",
			ask:  "What should I write next?",
			ctx:  ChatContext{},
			wants: []string{
				"No page is referenced",
				"What should I write next?",
				"concise Markdown",
				"Do not repeat the page",
			},
			notWant: []string{
				"Referenced pages:",
				"Open page:",
				"Page outline:",
				"Where the caret is:",
				"Preceding paragraph:",
				"Following paragraph:",
			},
		},
		{
			name: "references only",
			ask:  "Compare the two pages",
			ctx:  ChatContext{Refs: []string{"essays/alpha.md", "essays/beta.md"}},
			wants: []string{
				"Referenced pages (also attached as resource links):",
				"- essays/alpha.md",
				"- essays/beta.md",
				"Compare the two pages",
				"Do not repeat the page",
			},
			notWant: []string{"No page is referenced", "Open page:", "Page Outline:", "Where the caret is:"},
		},
		{
			name: "title only",
			ask:  "Summarize the argument",
			ctx:  ChatContext{Refs: []string{"essays/The Quiet Web.md"}, Title: "The Quiet Web"},
			wants: []string{
				`Open page: "The Quiet Web"`,
				"Summarize the argument",
				"Do not repeat the page",
			},
			notWant: []string{"Page Outline:", "Where the caret is:", "Preceding paragraph:", "Following paragraph:"},
		},
		{
			name: "neighbours disabled",
			ask:  "Find the strongest line",
			ctx:  ChatContext{Refs: []string{"essays/The Quiet Web.md"}, Title: "The Quiet Web", Caret: "Fast pages.", Outline: []string{"# The Quiet Web"}},
			wants: []string{
				"Find the strongest line",
				"Page outline:",
				"Where the caret is:",
				"Fast pages.",
			},
			notWant: []string{"Preceding paragraph:", "Following paragraph:"},
		},
		{
			name:    "instruction trimmed",
			ask:     "  Continue this page\n\n",
			ctx:     ChatContext{},
			wants:   []string{"Continue this page"},
			notWant: []string{"  Continue this page"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prompt := Chat(tc.ask, tc.ctx)
			for _, want := range tc.wants {
				if !strings.Contains(prompt, want) {
					t.Errorf("prompt misses %q:\n%s", want, prompt)
				}
			}
			for _, notWant := range tc.notWant {
				if strings.Contains(prompt, notWant) {
					t.Errorf("prompt should not have %q:\n%s", notWant, prompt)
				}
			}
		})
	}
}

func TestChatTrimsOutline(t *testing.T) {
	var headings []string
	for i := 1; i <= 15; i++ {
		headings = append(headings, fmt.Sprintf("## Section %d", i))
	}
	prompt := Chat("Continue this page", ChatContext{Title: "Long page", Outline: headings})
	if !strings.Contains(prompt, "## Section 12") {
		t.Errorf("the twelfth heading is missing:\n%s", prompt)
	}
	if strings.Contains(prompt, "## Section 13") {
		t.Errorf("the outline was not trimmed to %d headings:\n%s", maxChatOutline, prompt)
	}

	// Blank headings are dropped rather than leaving empty list items.
	sparse := Chat("Continue", ChatContext{
		Title:   "Sparse",
		Outline: []string{"", "  ", "## One", ""},
	})
	if strings.Contains(sparse, "- \n") || !strings.Contains(sparse, "- ## One") {
		t.Errorf("blank headings were not dropped:\n%s", sparse)
	}

	// No headings at all leaves the outline out entirely.
	none := Chat("Continue", ChatContext{Title: "Sparse"})
	if strings.Contains(none, "Page outline:") {
		t.Errorf("an empty outline was sent:\n%s", none)
	}
}
