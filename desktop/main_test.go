package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/bubbuild/bub/desktop/internal/acp"
)

// newTestApp is the app as tests drive it: no window, no agent process, and
// markdown that shows at once so a frame shows a whole reply without waiting
// on the tester's clock.
func newTestApp() *app {
	a := newApp()
	a.md.Reveal = &markdown.Reveal{Unit: markdown.AtOnce}
	return a
}

func TestTypingAndEnterAddsUserMessage(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.view, 820, 690)
	tt.Type("How do I run the tests?")
	tt.Key(0, ui.KeyEnter)

	if len(a.messages) == 0 {
		t.Fatalf("no messages after Enter: texts %q", tt.Texts())
	}
	first := a.messages[0]
	if first.kind != itemUser || first.text != "How do I run the tests?" {
		t.Errorf("first message = %+v, want the user's text", first)
	}
	if !tt.HasText("How do I run the tests?") {
		t.Errorf("the user's message is not drawn: %q", tt.Texts())
	}
}

func TestSendButtonAddsUserMessage(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.view, 820, 690)
	tt.Type("Hello Bub")
	if err := tt.Click("Send"); err != nil {
		t.Fatalf("clicking Send: %v", err)
	}
	if len(a.messages) == 0 || a.messages[0].kind != itemUser {
		t.Fatalf("messages = %+v, want the user's message", a.messages)
	}
	if !tt.HasText("Hello Bub") {
		t.Errorf("the user's message is not drawn: %q", tt.Texts())
	}
}

func TestStreamingAssistantRendersMarkdown(t *testing.T) {
	a := newTestApp()
	a.messages = []*item{{id: 1, kind: itemAssistant, text: "The answer is **42**, computed live."}}
	tt := ui.NewTester(a.view, 820, 690)
	tt.Frame()
	if !tt.HasText("The answer is") || !tt.HasText("42") {
		t.Errorf("the streaming reply is not drawn: %q", tt.Texts())
	}
}

func TestDoneAssistantUsesMarkdownView(t *testing.T) {
	a := newTestApp()
	a.messages = []*item{{id: 1, kind: itemAssistant, done: true, text: "Done: `go test ./...` passes."}}
	tt := ui.NewTester(a.view, 820, 690)
	tt.Frame()
	if !tt.HasText("Done:") || !tt.HasText("go test ./...") {
		t.Errorf("the finished reply is not drawn: %q", tt.Texts())
	}
}

func TestToolCallRowShowsTitleAndStatus(t *testing.T) {
	a := newTestApp()
	a.messages = []*item{{
		id:   1,
		kind: itemTool,
		tool: &toolState{
			id:     "call-1",
			title:  "read README.md",
			status: acp.ToolStatusCompleted,
			input:  `{"path":"README.md"}`,
		},
	}}
	tt := ui.NewTester(a.view, 820, 690)
	tt.Frame()
	if !tt.HasText("read README.md") {
		t.Errorf("the tool title is not drawn: %q", tt.Texts())
	}
	if !tt.HasText("completed") {
		t.Errorf("the tool status is not drawn: %q", tt.Texts())
	}
}

func TestErrorBannerShowsRetry(t *testing.T) {
	a := newTestApp()
	a.status = statusOffline
	a.errText = "The agent could not start: exec: \"bub\": executable file not found"
	tt := ui.NewTester(a.view, 820, 690)
	tt.Frame()
	if !tt.HasText("could not start") {
		t.Errorf("the error is not drawn: %q", tt.Texts())
	}
	if !tt.HasText("Retry") {
		t.Errorf("no Retry button: %q", tt.Texts())
	}
	if !tt.HasText("offline") {
		t.Errorf("the status chip does not say offline: %q", tt.Texts())
	}
}

func TestSendDisabledWhenEmptyAndStopWhileBusy(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.view, 820, 690)
	if info, ok := tt.AccessByRole(ui.RoleButton, "Send"); !ok || !info.Disabled {
		t.Errorf("Send while empty: %+v found=%v, want a disabled button", info, ok)
	}
	a.busy = true
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleButton, "Stop"); !ok {
		t.Errorf("no Stop button while a turn runs: %q", tt.Texts())
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, "Send"); ok {
		t.Error("Send is still drawn while a turn runs")
	}
	if !tt.HasText("running") {
		t.Errorf("the status chip does not say running: %q", tt.Texts())
	}
}

func TestEmptyStateShowsSuggestions(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.view, 820, 690)
	tt.Frame()
	if !tt.HasText("What are you working on?") {
		t.Errorf("no empty state: %q", tt.Texts())
	}
	for _, s := range suggestions {
		if !tt.HasText(s) {
			t.Errorf("suggestion %q is not drawn", s)
		}
	}
}

func TestSuggestionChipSends(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.view, 820, 690)
	if err := tt.Click(suggestions[0]); err != nil {
		t.Fatalf("clicking a suggestion: %v", err)
	}
	if len(a.messages) == 0 || a.messages[0].kind != itemUser || a.messages[0].text != suggestions[0] {
		t.Fatalf("messages = %+v, want the suggestion as the user's message", a.messages)
	}
}

func TestApplyUpdateBuildsTheTranscript(t *testing.T) {
	a := newApp()
	a.applyUpdate(acp.Update{Type: acp.UpdateAgentMessageChunk, Text: "Hel"})
	a.applyUpdate(acp.Update{Type: acp.UpdateAgentThoughtChunk, Text: "considering\nmore"})
	a.applyUpdate(acp.Update{Type: acp.UpdateAgentMessageChunk, Text: "lo"})
	if len(a.messages) != 1 {
		t.Fatalf("messages = %+v, want one assistant message", a.messages)
	}
	reply := a.messages[0]
	if reply.kind != itemAssistant || reply.text != "Hello" {
		t.Errorf("reply = %+v, want Hello", reply)
	}
	if !strings.HasPrefix(reply.thought, "considering") {
		t.Errorf("thought = %q, want the thinking text", reply.thought)
	}

	a.applyUpdate(acp.Update{Type: acp.UpdateToolCall, ToolCall: &acp.ToolCall{
		ToolCallID: "call-1",
		Title:      "read README.md",
		Status:     acp.ToolStatusInProgress,
		RawInput:   []byte(`{"path": "README.md"}`),
	}})
	if !reply.done {
		t.Error("the text before a tool call is still streaming")
	}
	if len(a.messages) != 2 || a.messages[1].kind != itemTool {
		t.Fatalf("messages = %+v, want a tool row after the message", a.messages)
	}
	if !strings.Contains(a.activity, "read README.md") {
		t.Errorf("activity = %q, want the running tool", a.activity)
	}

	completed := acp.ToolStatusCompleted
	a.applyUpdate(acp.Update{Type: acp.UpdateToolCallUpdate, ToolCallUpdate: &acp.ToolCallUpdate{
		ToolCallID: "call-1",
		Status:     &completed,
		RawOutput:  []byte(`"README contents"`),
	}})
	tool := a.messages[1].tool
	if tool.status != acp.ToolStatusCompleted {
		t.Errorf("tool status = %q, want completed", tool.status)
	}
	if !strings.Contains(tool.output, "README contents") {
		t.Errorf("tool output = %q, want the raw output", tool.output)
	}
	if a.activity != "" {
		t.Errorf("activity = %q, want none after the tool finished", a.activity)
	}

	// A late update for a call the window never saw is shown, not dropped.
	a.applyUpdate(acp.Update{Type: acp.UpdateToolCallUpdate, ToolCallUpdate: &acp.ToolCallUpdate{
		ToolCallID: "call-2",
		Status:     &completed,
	}})
	if len(a.messages) != 3 || a.messages[2].tool.id != "call-2" {
		t.Errorf("messages = %+v, want a row for the unseen tool call", a.messages)
	}
}

func TestPromptWhileDisconnectedShowsBanner(t *testing.T) {
	a := newTestApp()
	tt := ui.NewTester(a.view, 820, 690)
	tt.Type("Anyone there?")
	tt.Key(0, ui.KeyEnter)
	if len(a.messages) != 1 || a.messages[0].kind != itemUser {
		t.Fatalf("messages = %+v, want the user's message kept", a.messages)
	}
	if !strings.Contains(a.errText, "Not connected") {
		t.Errorf("errText = %q, want a disconnect message", a.errText)
	}
	tt.Frame()
	if !tt.HasText("Not connected") {
		t.Errorf("the banner is not drawn: %q", tt.Texts())
	}
}
