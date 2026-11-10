package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"

	"github.com/bubbuild/bub/desktop/internal/acp"
)

// newTestApp is the app without a window or an agent: the document and the
// action engine are driven directly.
func newTestApp(t *testing.T) *app {
	t.Helper()
	a := newApp()
	a.status = statusReady
	return a
}

// load shows Markdown as the open document.
func load(t *testing.T, a *app, markdown string) {
	t.Helper()
	a.setDocument(richtext.ParseMarkdown(markdown), "")
}

func TestNewDocumentIsEmpty(t *testing.T) {
	a := newTestApp(t)
	if got := strings.TrimSpace(a.doc.Text().PlainText()); got != "" {
		t.Errorf("new document holds %q, want empty", got)
	}
	if a.documentName() != "Untitled" {
		t.Errorf("name = %q, want Untitled", a.documentName())
	}
	if a.dirty {
		t.Error("a new document is dirty")
	}
	if a.words() != 0 {
		t.Errorf("words = %d, want 0", a.words())
	}
}

func TestPromptPerAction(t *testing.T) {
	a := newTestApp(t)
	load(t, a, "# Notes\n\nIntro line.\n\nFast pages feel more trustworthy.\n\nLast line.\n")
	blocks := a.blocks()
	if len(blocks) != 4 {
		t.Fatalf("blocks = %d, want 4", len(blocks))
	}
	target := blocks[2]
	prev, next := contextAround(blocks, target.index)
	base := actionState{target: target, title: a.documentName(), prev: prev, next: next}
	if target.text != "Fast pages feel more trustworthy." {
		t.Fatalf("target = %q", target.text)
	}

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
				`Markdown document titled "Untitled"`,
				"Rewrite the paragraph below",
				"Fast pages feel more trustworthy.",
				"Preceding paragraph: Intro line.",
				"Following paragraph: Last line.",
				"replacement paragraph only",
				"no code fences",
			},
		},
		{
			name: "shorten", kind: actionShorten,
			wants: []string{"Shorten the paragraph below by about 30%", "Fast pages"},
		},
		{
			name: "expand", kind: actionExpand,
			wants: []string{"Expand the paragraph below", "Fast pages"},
		},
		{
			name: "translate", kind: actionTranslate,
			wants: []string{"Translate the paragraph below into English", "Fast pages"},
		},
		{
			name: "ask", kind: actionAsk, instruct: "Make it warmer and shorter.",
			wants: []string{"Instruction: Make it warmer and shorter.", "Fast pages"},
		},
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

func TestCleanSuggestion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Hello world.", "Hello world."},
		{"  Hello world.  \n", "Hello world."},
		{"```\nHello world.\n```", "Hello world."},
		{"```markdown\nHello world.\n```", "Hello world."},
		{"```md\nHello", "Hello"}, // a fence still streaming
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

func TestApplySuggestionIsOneUndoStep(t *testing.T) {
	a := newTestApp(t)
	load(t, a, "# Title\n\nFast pages feel more trustworthy.\n\nEnd.\n")
	before := a.doc.Text().PlainText()
	target := a.blocks()[1]
	if target.text != "Fast pages feel more trustworthy." {
		t.Fatalf("target = %q", target.text)
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
	if a.suggestion != nil || a.action != nil {
		t.Error("the suggestion did not clear after accepting")
	}
	if id := a.blocks()[1].style.ID; id != target.id {
		t.Errorf("paragraph ID = %q, want %q kept", id, target.id)
	}
	if !a.doc.CanUndo() {
		t.Fatal("accepting is not undoable")
	}
	a.doc.Undo()
	if got := a.doc.Text().PlainText(); got != before {
		t.Errorf("one undo left %q, want the original %q", got, before)
	}
}

func TestBlockLookupByParagraphID(t *testing.T) {
	a := newTestApp(t)
	load(t, a, "One.\n\nTwo.\n\nThree.\n")
	second := a.blocks()[1]
	if second.text != "Two." {
		t.Fatalf("second block = %q", second.text)
	}
	if got := a.blockByID(second.id); got == nil || got.text != "Two." {
		t.Fatalf("blockByID(%q) = %+v", second.id, got)
	}
	// A paragraph inserted above moves the index but not the ID.
	a.doc.Edit(func(e *richtext.Edit) {
		e.ReplaceString(0, 0, "Above.\n\n", richtext.Attributes{})
	})
	moved := a.blockByID(second.id)
	if moved == nil || moved.text != "Two." {
		t.Fatalf("after an edit, blockByID(%q) = %+v", second.id, moved)
	}
	if moved.index == second.index {
		t.Errorf("the paragraph did not move: index %d", moved.index)
	}
}

func TestApplySuggestionKeepsParagraphStyle(t *testing.T) {
	a := newTestApp(t)
	load(t, a, "# Old heading\n\nA paragraph.\n")
	heading := a.blocks()[0]
	if heading.style.Heading != 1 {
		t.Fatalf("first block is not a heading: %+v", heading.style)
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
	if after.style.Heading != 1 {
		t.Errorf("heading level = %d, want 1 kept", after.style.Heading)
	}
	if after.text != "New heading" {
		t.Errorf("heading text = %q", after.text)
	}
}

func TestApplySuggestionRefusesStaleTarget(t *testing.T) {
	a := newTestApp(t)
	load(t, a, "First paragraph stays.\n\nSecond paragraph changes.\n")
	target := a.blocks()[1]
	a.suggestion = &suggestionState{
		action: actionState{kind: actionRewrite, target: target},
		status: suggestReady,
		text:   "A rewrite.",
	}
	// The writer edits the paragraph while the suggestion is there.
	a.doc.Edit(func(e *richtext.Edit) {
		e.ReplaceString(target.start, target.end, "Something else entirely.", richtext.Attributes{})
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
	if !a.suggestion.wasStale {
		t.Error("the suggestion was not marked stale")
	}
}

func TestApplySuggestionRefusesVanishedTarget(t *testing.T) {
	a := newTestApp(t)
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
	a := newTestApp(t)
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
	if len(a.blocks()) != 3 {
		t.Errorf("blocks = %d, want 3", len(a.blocks()))
	}
	if a.blocks()[2].style.ID == "" {
		t.Error("the inserted paragraph has no ID")
	}
	a.doc.Undo()
	if got := a.doc.Text().PlainText(); got != before {
		t.Errorf("one undo left %q, want %q", got, before)
	}
}

func TestMarkdownLoadSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	source := "# Title\n\nHello **world**, and _friends_.\n\n- One\n- Two\n"
	text := richtext.ParseMarkdown(source)
	if err := writeMarkdownFile(path, text); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# Title") || !strings.Contains(string(data), "**world**") {
		t.Errorf("saved Markdown = %q", data)
	}
	read, err := readMarkdownFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if read.Markdown() != text.Markdown() {
		t.Errorf("round trip changed the Markdown:\n%q\n%q", text.Markdown(), read.Markdown())
	}

	a := newTestApp(t)
	a.setDocument(read, path)
	if a.documentName() != "note.md" {
		t.Errorf("name = %q, want note.md", a.documentName())
	}
	if a.dirty {
		t.Error("an opened document is dirty")
	}
}

func TestApplyUpdateStreamsSuggestionOnly(t *testing.T) {
	a := newTestApp(t)
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
	a.applyUpdate(acp.Update{Type: acp.UpdateToolCall, ToolCall: &acp.ToolCall{Title: "read README.md", Status: acp.ToolStatusInProgress}})
	if !strings.Contains(a.activity, "read README.md") {
		t.Errorf("activity = %q, want the running tool", a.activity)
	}
	a.applyUpdate(acp.Update{Type: acp.UpdateToolCallUpdate, ToolCallUpdate: &acp.ToolCallUpdate{Status: strPtr(acp.ToolStatusCompleted)}})
	if a.activity != "" {
		t.Errorf("activity = %q, want none after the tool finished", a.activity)
	}
	// Tool calls and thoughts never touch the document.
	if got := strings.TrimSpace(a.doc.Text().PlainText()); got != "" {
		t.Errorf("document = %q, want it untouched", got)
	}
}

func TestActionOfflineShowsBanner(t *testing.T) {
	a := newTestApp(t)
	a.client = nil
	a.action = &actionState{target: a.blocks()[0], title: "Untitled"}
	a.chooseAction(actionRewrite, "")
	if !strings.Contains(a.errText, "Not connected") {
		t.Errorf("errText = %q, want a connection message", a.errText)
	}
	if a.suggestion != nil {
		t.Error("a suggestion started without an agent")
	}
}

func strPtr(s string) *string { return &s }

// --- View tests, without a window ----------------------------------------

func TestEditorViewShowsChrome(t *testing.T) {
	a := newTestApp(t)
	load(t, a, "# The Quiet Web\n\nFast pages feel more trustworthy.\n")
	tt := ui.NewTester(a.view, 1040, 760)
	tt.Frame()
	for _, text := range []string{"Bub", "Untitled", "words", "edit block", "save", "open", "new", "ready"} {
		if !tt.HasText(text) {
			t.Errorf("the window does not show %q: %q", text, tt.Texts())
		}
	}
}

func TestActionBarView(t *testing.T) {
	a := newTestApp(t)
	load(t, a, "A paragraph.\n")
	a.action = &actionState{target: a.blocks()[0], title: "Untitled"}
	tt := ui.NewTester(a.view, 1040, 760)
	tt.Frame()
	for _, label := range []string{"Rewrite", "Shorten", "Expand", "Continue", "Translate", "Ask…"} {
		if !tt.HasText(label) {
			t.Errorf("the action bar does not show %q: %q", label, tt.Texts())
		}
	}
}

func TestSuggestionCardView(t *testing.T) {
	a := newTestApp(t)
	load(t, a, "Fast pages feel more trustworthy.\n")
	target := a.blocks()[0]
	a.suggestion = &suggestionState{
		action: actionState{kind: actionRewrite, target: target},
		status: suggestReady,
		text:   "Fast pages earn trust.",
	}
	tt := ui.NewTester(a.view, 1040, 760)
	tt.Frame()
	for _, text := range []string{"Suggested rewrite", "Fast pages earn trust.", "Accept", "Regenerate", "Dismiss"} {
		if !tt.HasText(text) {
			t.Errorf("the card does not show %q: %q", text, tt.Texts())
		}
	}
	if info, ok := tt.AccessByRole(ui.RoleButton, "Accept"); !ok || info.Disabled {
		t.Errorf("Accept = %+v found=%v, want an enabled button", info, ok)
	}
}

func TestStaleSuggestionCardDisablesAccept(t *testing.T) {
	a := newTestApp(t)
	load(t, a, "Fast pages feel more trustworthy.\n")
	target := a.blocks()[0]
	a.suggestion = &suggestionState{
		action: actionState{kind: actionRewrite, target: target},
		status: suggestReady,
		text:   "Fast pages earn trust.",
	}
	a.doc.Edit(func(e *richtext.Edit) {
		e.ReplaceString(target.start, target.end, "The paragraph changed.", richtext.Attributes{})
	})
	tt := ui.NewTester(a.view, 1040, 760)
	tt.Frame()
	if info, ok := tt.AccessByRole(ui.RoleButton, "Accept"); !ok || !info.Disabled {
		t.Errorf("Accept = %+v found=%v, want a disabled button", info, ok)
	}
	if !tt.HasText("This paragraph changed") {
		t.Errorf("no stale note: %q", tt.Texts())
	}
}

func TestShortcutAcceptsSuggestion(t *testing.T) {
	a := newTestApp(t)
	load(t, a, "Fast pages feel more trustworthy.\n")
	target := a.blocks()[0]
	a.suggestion = &suggestionState{
		action: actionState{kind: actionRewrite, target: target},
		status: suggestReady,
		text:   "Fast pages earn trust.",
	}
	tt := ui.NewTester(a.view, 1040, 760)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyEnter)
	if got := a.doc.Text().PlainText(); !strings.Contains(got, "Fast pages earn trust.") {
		t.Errorf("⌘↩ did not accept: document = %q", got)
	}
}

func TestEscapeDismissesSuggestion(t *testing.T) {
	a := newTestApp(t)
	load(t, a, "A paragraph.\n")
	target := a.blocks()[0]
	a.suggestion = &suggestionState{
		action: actionState{kind: actionRewrite, target: target},
		status: suggestReady,
		text:   "A rewrite.",
	}
	a.action = &actionState{target: target, title: "Untitled"}
	tt := ui.NewTester(a.view, 1040, 760)
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	if a.suggestion != nil || a.action != nil {
		t.Errorf("escape left suggestion=%v action=%v", a.suggestion, a.action)
	}
}

func TestOfflineBannerAndActivityView(t *testing.T) {
	a := newTestApp(t)
	a.status = statusOffline
	a.errText = "The Bub agent stopped. Press Retry to start it again."
	a.activity = "Running read README.md…"
	tt := ui.NewTester(a.view, 1040, 760)
	tt.Frame()
	for _, text := range []string{"offline", "Retry", "Running read README.md"} {
		if !tt.HasText(text) {
			t.Errorf("the window does not show %q: %q", text, tt.Texts())
		}
	}
}

func TestCmdKOpensActionForTheCaretBlock(t *testing.T) {
	a := newTestApp(t)
	load(t, a, "A paragraph worth editing.\n")
	tt := ui.NewTester(a.view, 1040, 760)
	tt.Frame()
	tt.Type("One more sentence. ")
	tt.Key(ui.Cmd, ui.KeyK)
	if a.action == nil {
		t.Fatalf("⌘K opened no action bar: %q", tt.Texts())
	}
	if !strings.Contains(a.action.target.text, "One more sentence.") {
		t.Errorf("action target = %q, want the caret's block", a.action.target.text)
	}
}

func TestTypingMarksTheDocumentDirty(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1040, 760)
	tt.Frame()
	tt.Type("Hello")
	if !a.dirty {
		t.Error("typing did not mark the document dirty")
	}
	if got := a.doc.Text().PlainText(); !strings.Contains(got, "Hello") {
		t.Errorf("document = %q, want the typed text", got)
	}
}

func TestSaveShortcutWritesMarkdown(t *testing.T) {
	a := newTestApp(t)
	load(t, a, "# Title\n\nSaved by the shortcut.\n")
	path := filepath.Join(t.TempDir(), "saved.md")
	a.path = path
	a.dirty = true
	tt := ui.NewTester(a.view, 1040, 760)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyS)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("⌘S wrote no file")
		}
		time.Sleep(time.Millisecond)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# Title") || !strings.Contains(string(data), "Saved by the shortcut.") {
		t.Errorf("saved Markdown = %q", data)
	}
	if a.dirty {
		t.Error("the document is still dirty after a save")
	}
}

func TestEditorShowsNoChatChrome(t *testing.T) {
	a := newTestApp(t)
	tt := ui.NewTester(a.view, 1040, 760)
	tt.Frame()
	for _, chat := range []string{"Send", "Message the agent", "You", "Agent"} {
		if tt.HasText(chat) {
			t.Errorf("the editor shows chat chrome %q: %q", chat, tt.Texts())
		}
	}
}
