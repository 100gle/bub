package main

import (
	"strings"
	"testing"

	"github.com/bubbuild/bub/desktop/internal/acp"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

func TestPromptPerAction(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "# Notes\n\nIntro line.\n\nFast pages feel more trustworthy.\n\nLast line.\n")
	blocks := a.blocks()
	if len(blocks) != 4 {
		t.Fatalf("blocks = %d, want 4", len(blocks))
	}
	target := blocks[2]
	base := a.actionFor(actionRewrite, "", target)

	tests := []struct {
		name     string
		kind     actionKind
		instruct string
		wants    []string
		notWant  []string
	}{
		{
			name: "rewrite", kind: actionRewrite,
			wants: []string{
				`Markdown document titled "Notes"`,
				"Rewrite the paragraph below",
				"Fast pages feel more trustworthy.",
				"Preceding paragraph: Intro line.",
				"Following paragraph: Last line.",
				"Document outline:",
				"replacement paragraph only",
				"no code fences",
			},
		},
		{name: "shorten", kind: actionShorten, wants: []string{"Shorten the paragraph below by about 30%", "Fast pages"}},
		{name: "expand", kind: actionExpand, wants: []string{"Expand the paragraph below", "Fast pages"}},
		{name: "translate", kind: actionTranslate, wants: []string{"Translate the paragraph below into English", "Fast pages"}},
		{name: "ask", kind: actionAsk, instruct: "Make it warmer and shorter.", wants: []string{"Instruction: Make it warmer and shorter.", "Fast pages"}},
		{
			name: "continue", kind: actionContinue,
			wants:   []string{"Continue the document after the paragraph below", "next paragraph only", "Preceding paragraph:"},
			notWant: []string{"Following paragraph:", "replacement paragraph only"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			act := base
			act.kind = tc.kind
			act.instruction = tc.instruct
			prompt := buildActionPrompt(act)
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

func TestPromptHonoursContextSettings(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "# Notes\n\nIntro line.\n\nTarget.\n\nNext line.\n")
	a.settings.AIContextNeighbors = false
	a.settings.AIContextOutline = false
	target := a.blocks()[2]
	prompt := buildActionPrompt(a.actionFor(actionRewrite, "", target))
	if strings.Contains(prompt, "Preceding paragraph:") || strings.Contains(prompt, "Following paragraph:") {
		t.Errorf("neighbour context was sent while disabled:\n%s", prompt)
	}
	if strings.Contains(prompt, "Document outline:") {
		t.Errorf("outline context was sent while disabled:\n%s", prompt)
	}
}

func TestCleanSuggestion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Hello world.", "Hello world."},
		{"  Hello world.  \n", "Hello world."},
		{"```\nHello world.\n```", "Hello world."},
		{"```markdown\nHello world.\n```", "Hello world."},
		{"```md\nHello", "Hello"},
		{`"Hello world."`, "Hello world."},
		{"“Hello world.”", "Hello world."},
		{"```", ""},
		{"\n\n", ""},
	}
	for _, c := range cases {
		if got := cleanSuggestion(c.in); got != c.want {
			t.Errorf("cleanSuggestion(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBlockLookupByParagraphID(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "One.\n\nTwo.\n\nThree.\n")
	second := a.blocks()[1]
	if second.Text != "Two." {
		t.Fatalf("second block = %q", second.Text)
	}
	if got := a.blockByID(second.ID); got == nil || got.Text != "Two." {
		t.Fatalf("blockByID(%q) = %+v", second.ID, got)
	}
	a.doc.Edit(func(e *richtext.Edit) {
		e.ReplaceString(0, 0, "Above.\n\n", richtext.Attributes{})
	})
	moved := a.blockByID(second.ID)
	if moved == nil || moved.Text != "Two." {
		t.Fatalf("after an edit, blockByID(%q) = %+v", second.ID, moved)
	}
	if moved.Index == second.Index {
		t.Errorf("the paragraph did not move: index %d", moved.Index)
	}
}

func TestApplySuggestionIsOneUndoStep(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "# Title\n\nFast pages feel more trustworthy.\n\nEnd.\n")
	before := a.doc.Text().PlainText()
	target := a.blocks()[1]
	if target.Text != "Fast pages feel more trustworthy." {
		t.Fatalf("target = %q", target.Text)
	}
	a.suggestion = &suggestionState{
		action: actionState{kind: actionRewrite, target: target},
		status: suggestReady,
		text:   "Fast pages earn trust.",
	}
	if !a.applySuggestion() {
		t.Fatal("applySuggestion refused a fresh suggestion")
	}
	after := a.doc.Text().PlainText()
	if !strings.Contains(after, "Fast pages earn trust.") || strings.Contains(after, "feel more trustworthy") {
		t.Errorf("after applying, text = %q", after)
	}
	if !a.dirty {
		t.Error("accepting a suggestion did not mark the document dirty")
	}
	if a.suggestion != nil || a.ask != nil {
		t.Error("the suggestion did not clear after accepting")
	}
	if id := a.blocks()[1].Style.ID; id != target.ID {
		t.Errorf("paragraph ID = %q, want %q kept", id, target.ID)
	}
	if !a.doc.CanUndo() {
		t.Fatal("accepting is not undoable")
	}
	a.doc.Undo()
	if got := a.doc.Text().PlainText(); got != before {
		t.Errorf("one undo left %q, want the original %q", got, before)
	}
}

func TestApplySuggestionKeepsParagraphStyle(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "# Old heading\n\nA paragraph.\n")
	heading := a.blocks()[0]
	if heading.Style.Heading != 1 {
		t.Fatalf("first block is not a heading: %+v", heading.Style)
	}
	a.suggestion = &suggestionState{
		action: actionState{kind: actionRewrite, target: heading},
		status: suggestReady,
		text:   "New heading",
	}
	if !a.applySuggestion() {
		t.Fatal("applySuggestion refused the heading")
	}
	after := a.blocks()[0]
	if after.Style.Heading != 1 || after.Text != "New heading" {
		t.Errorf("after = %+v, want a level-1 heading with the new text", after)
	}
}

func TestApplySuggestionRefusesStaleTarget(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "First paragraph stays.\n\nSecond paragraph changes.\n")
	target := a.blocks()[1]
	a.suggestion = &suggestionState{
		action: actionState{kind: actionRewrite, target: target},
		status: suggestReady,
		text:   "A rewrite.",
	}
	a.doc.Edit(func(e *richtext.Edit) {
		e.ReplaceString(target.Start, target.End, "Something else entirely.", richtext.Attributes{})
	})
	if !a.suggestionStale() {
		t.Error("suggestionStale did not notice the edit")
	}
	if a.applySuggestion() {
		t.Fatal("applySuggestion accepted a stale target")
	}
	if got := a.doc.Text().PlainText(); strings.Contains(got, "A rewrite.") {
		t.Errorf("text = %q, the stale suggestion was written", got)
	}
}

func TestApplySuggestionRefusesVanishedTarget(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "Only paragraph.\n")
	target := a.blocks()[0]
	a.suggestion = &suggestionState{
		action: actionState{kind: actionRewrite, target: target},
		status: suggestReady,
		text:   "A rewrite.",
	}
	a.setDocument(richtext.ParseMarkdown("A whole other document.\n"), "")
	if a.applySuggestion() {
		t.Fatal("applySuggestion accepted a target from another document")
	}
}

func TestContinueInsertsAParagraph(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "First paragraph.\n\nSecond paragraph.\n")
	before := a.doc.Text().PlainText()
	target := a.blocks()[1]
	a.suggestion = &suggestionState{
		action: actionState{kind: actionContinue, target: target},
		status: suggestReady,
		text:   "A third paragraph.",
	}
	if !a.applySuggestion() {
		t.Fatal("applySuggestion refused a continuation")
	}
	plain := a.doc.Text().PlainText()
	if !strings.Contains(plain, "Second paragraph.\nA third paragraph.") {
		t.Errorf("text = %q, want the new paragraph after the second", plain)
	}
	if len(a.blocks()) != 3 || a.blocks()[2].ID == "" {
		t.Errorf("blocks = %+v, want three with an ID on the new one", a.blocks())
	}
	a.doc.Undo()
	if got := a.doc.Text().PlainText(); got != before {
		t.Errorf("one undo left %q, want %q", got, before)
	}
}

func TestApplyUpdateStreamsSuggestionOnly(t *testing.T) {
	a, _ := newTestApp(t)
	target := a.blocks()[0]
	a.suggestion = &suggestionState{
		action: actionState{kind: actionRewrite, target: target},
		status: suggestThinking,
	}
	a.applyUpdate(acp.Update{Type: acp.UpdateAgentMessageChunk, Text: "Hello "})
	a.applyUpdate(acp.Update{Type: acp.UpdateAgentMessageChunk, Text: "world."})
	if a.suggestion.text != "Hello world." {
		t.Errorf("suggestion text = %q, want Hello world.", a.suggestion.text)
	}
	if a.suggestion.status != suggestStreaming {
		t.Errorf("status = %v, want streaming", a.suggestion.status)
	}
	// Thoughts and tool calls have no document, no popover and no console here.
	a.applyUpdate(acp.Update{Type: acp.UpdateAgentThoughtChunk, Text: "thinking"})
	a.applyUpdate(acp.Update{Type: acp.UpdateToolCall, ToolCall: &acp.ToolCall{Title: "read", Status: acp.ToolStatusInProgress}})
	if got := strings.TrimSpace(a.doc.Text().PlainText()); got != "" {
		t.Errorf("document = %q, want it untouched", got)
	}
}

// --- Block chrome ---------------------------------------------------------

func TestSuggestionPopoverView(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "Fast pages feel more trustworthy.\n")
	target := a.blocks()[0]
	a.suggestion = &suggestionState{
		action: actionState{kind: actionRewrite, target: target},
		status: suggestReady,
		text:   "Fast pages earn trust.",
	}
	tt := ui.NewTester(a.view, 1180, 820)
	tt.Frame()
	for _, text := range []string{"Fast pages earn trust.", "Accept", "Dismiss", "Regenerate"} {
		if !tt.HasText(text) {
			t.Errorf("the popover does not show %q: %q", text, tt.Texts())
		}
	}
	if info, ok := tt.AccessByRole(ui.RoleButton, "Accept"); !ok || info.Disabled {
		t.Errorf("Accept = %+v found=%v, want an enabled button", info, ok)
	}
}

func TestStreamingPopoverView(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "Fast pages feel more trustworthy.\n")
	target := a.blocks()[0]
	a.suggestion = &suggestionState{
		action: actionState{kind: actionRewrite, target: target},
		status: suggestStreaming,
		text:   "Fast pages earn",
	}
	tt := ui.NewTester(a.view, 1180, 820)
	tt.Frame()
	if !tt.HasText("Bub is writing…") {
		t.Errorf("no streaming label: %q", tt.Texts())
	}
	if !tt.HasText("Stop") {
		t.Errorf("no Stop action: %q", tt.Texts())
	}
}

func TestStaleSuggestionPopoverDisablesAccept(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "Fast pages feel more trustworthy.\n")
	target := a.blocks()[0]
	a.suggestion = &suggestionState{
		action: actionState{kind: actionRewrite, target: target},
		status: suggestReady,
		text:   "Fast pages earn trust.",
	}
	a.doc.Edit(func(e *richtext.Edit) {
		e.ReplaceString(target.Start, target.End, "The paragraph changed.", richtext.Attributes{})
	})
	tt := ui.NewTester(a.view, 1180, 820)
	tt.Frame()
	if info, ok := tt.AccessByRole(ui.RoleButton, "Accept"); !ok || !info.Disabled {
		t.Errorf("Accept = %+v found=%v, want a disabled button", info, ok)
	}
	if !tt.HasText("The block changed") {
		t.Errorf("no stale note: %q", tt.Texts())
	}
}

func TestAskPopoverView(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "A paragraph.\n")
	a.ask = &askState{target: a.blocks()[0]}
	tt := ui.NewTester(a.view, 1180, 820)
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleTextField, "Ask Bub"); !ok {
		t.Errorf("no ask text field: %q", tt.Texts())
	}
	tt.Type("Make it shorter")
	tt.Key(0, ui.KeyEnter)
	if a.ask != nil {
		t.Error("Enter did not run the ask")
	}
}

func TestShortcutAcceptsAndEscapeDismisses(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "Fast pages feel more trustworthy.\n")
	target := a.blocks()[0]
	a.suggestion = &suggestionState{
		action: actionState{kind: actionRewrite, target: target},
		status: suggestReady,
		text:   "Fast pages earn trust.",
	}
	tt := ui.NewTester(a.view, 1180, 820)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyEnter)
	if got := a.doc.Text().PlainText(); !strings.Contains(got, "Fast pages earn trust.") {
		t.Errorf("⌘↩ did not accept: document = %q", got)
	}

	a.suggestion = &suggestionState{
		action: actionState{kind: actionRewrite, target: a.blocks()[0]},
		status: suggestReady,
		text:   "Another rewrite.",
	}
	a.ask = &askState{target: a.blocks()[0]}
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	if a.suggestion != nil || a.ask != nil {
		t.Errorf("escape left suggestion=%v ask=%v", a.suggestion, a.ask)
	}
}

func TestInsertChatAnswerIntoThePage(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "# Title\n\nOne line.\n")
	a.doc.SetSelection(a.doc.Text().Len(), a.doc.Text().Len())
	before := a.doc.Text().PlainText()
	a.insertChatText("**A better** second line.")
	plain := a.doc.Text().PlainText()
	if !strings.Contains(plain, "A better second line.") {
		t.Errorf("the answer did not enter the page: %q", plain)
	}
	if !a.dirty {
		t.Error("inserting an answer did not mark the page dirty")
	}
	a.doc.Undo()
	if got := a.doc.Text().PlainText(); got != before {
		t.Errorf("undo left %q, want %q", got, before)
	}
}
