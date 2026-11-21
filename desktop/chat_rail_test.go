package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/bubbuild/bub/desktop/internal/acp"
	"github.com/bubbuild/bub/desktop/internal/appdata"
)

func TestChatPanelEmptyState(t *testing.T) {
	a := newApp()
	a.chat = &chatState{}
	tt := ui.NewTester(a.chatPanel, 288, 720)
	tt.Frame()
	for _, want := range []string{
		"What are you working on?",
		"Make this clearer",
		"Find the strongest line",
		"Continue this page",
	} {
		if !tt.HasText(want) {
			t.Errorf("the empty panel misses %q: %q", want, tt.Texts())
		}
	}
	// A new chat reads nothing, so the panel does not claim to see a page.
	if tt.HasText("Ask Bub about this page") {
		t.Errorf("the empty panel claims a page it does not read: %q", tt.Texts())
	}
	// The panel says what it can do; no secondary line explains the page.
	if tt.HasText("No page context") || tt.HasText("Bub can read") {
		t.Errorf("the empty panel still explains itself: %q", tt.Texts())
	}
	// The three suggestions are buttons the writer can press without a window.
	for _, label := range chatSuggestions {
		if _, ok := tt.AccessByRole(ui.RoleButton, label); !ok {
			t.Errorf("%q is not a button: %q", label, tt.Texts())
		}
	}
}

// TestChatPanelEmptyStateNamesReferences checks that the empty state speaks
// about the pages the writer attached, not about whichever page is open.
func TestChatPanelEmptyStateNamesReferences(t *testing.T) {
	a := newApp()
	a.chat = &chatState{conv: appdata.Conversation{
		References: []string{"essays/The Quiet Web.md"},
	}}
	tt := ui.NewTester(a.chatPanel, 288, 720)
	tt.Frame()
	if !tt.HasText("Ask Bub about The Quiet Web") {
		t.Errorf("the empty panel misses its reference: %q", tt.Texts())
	}
}

func TestChatPanelShowsConversation(t *testing.T) {
	a := newApp()
	a.chat = &chatState{conv: appdata.Conversation{
		ID: "conv-1",
		Messages: []appdata.Message{
			{Role: appdata.RoleUser, Text: "Make this clearer"},
			{Role: appdata.RoleAssistant, Text: "The argument is sharper without the second sentence."},
		},
	}}
	tt := ui.NewTester(a.chatPanel, 288, 720)
	tt.Frame()
	for _, want := range []string{
		"Make this clearer",
		"The argument is sharper without the second sentence.",
		"Insert",
		"Copy",
	} {
		if !tt.HasText(want) {
			t.Errorf("the panel misses %q: %q", want, tt.Texts())
		}
	}
	// The user's bubble is its own face; no role label repeats it.
	if tt.HasText("YOU") {
		t.Errorf("the writer's message still has a role label: %q", tt.Texts())
	}
}

func TestChatSendWithoutAgentShowsInlineError(t *testing.T) {
	a := newApp()
	a.uiReady = true // a native notice is shown only once the window exists
	alerts := 0
	a.dialog = func(title, message string, retry func()) { alerts++ }
	a.chat = &chatState{}
	tt := ui.NewTester(a.chatPanel, 288, 760)
	tt.Frame()
	if err := tt.Click("Make this clearer"); err != nil {
		t.Fatalf("click the suggestion: %v", err)
	}
	tt.Frame()
	if got := len(a.chat.conv.Messages); got != 1 {
		t.Fatalf("messages = %d, want the writer's one", got)
	}
	if a.chat.conv.Messages[0].Role != appdata.RoleUser || a.chat.conv.Messages[0].Text != "Make this clearer" {
		t.Errorf("the message sent = %+v", a.chat.conv.Messages[0])
	}
	if a.chat.streaming || a.chat.inFlight {
		t.Errorf("a turn is still in flight without an agent")
	}
	if !tt.HasText("Bub is not connected") {
		t.Errorf("no inline error: %q", tt.Texts())
	}
	if !tt.HasText("Retry") {
		t.Errorf("no inline Retry: %q", tt.Texts())
	}
	if alerts != 1 {
		t.Errorf("native alerts = %d, want one reconnect notice", alerts)
	}
}

func TestChatComposerSendButton(t *testing.T) {
	a := newApp()
	a.uiReady = true
	alerts := 0
	a.dialog = func(title, message string, retry func()) { alerts++ }
	a.chat = &chatState{input: "Find the strongest line"}
	tt := ui.NewTester(a.chatPanel, 288, 760)
	tt.Frame()
	if err := tt.Click("Send"); err != nil {
		t.Fatalf("click Send: %v", err)
	}
	tt.Frame()
	if got := len(a.chat.conv.Messages); got != 1 {
		t.Fatalf("messages = %d, want the writer's one", got)
	}
	if a.chat.conv.Messages[0].Role != appdata.RoleUser || a.chat.conv.Messages[0].Text != "Find the strongest line" {
		t.Errorf("the message sent = %+v", a.chat.conv.Messages[0])
	}
	if a.chat.input != "" {
		t.Errorf("the composer kept %q after sending", a.chat.input)
	}
	if alerts != 1 {
		t.Errorf("native alerts = %d, want one reconnect notice", alerts)
	}
}

func TestChatHistoryPopoverOpensSavedChat(t *testing.T) {
	chats := &appdata.Chats{Dir: t.TempDir()}
	saved := appdata.Conversation{
		ID:    "saved-1",
		Title: "Earlier chat",
		Messages: []appdata.Message{
			{Role: appdata.RoleUser, Text: "What is the argument?"},
			{Role: appdata.RoleAssistant, Text: "Subtraction as a virtue."},
		},
	}
	if err := chats.Save(&saved); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	a.chats = chats
	a.chatList = []*appdata.Conversation{&saved}
	a.chat = &chatState{conv: appdata.NewConversation(), history: true}
	tt := ui.NewTester(a.chatPanel, 288, 760)
	tt.Frame()
	if !tt.HasText("Earlier chat") {
		t.Fatalf("the popover misses the saved chat: %q", tt.Texts())
	}
	if err := tt.Click("Earlier chat"); err != nil {
		t.Fatalf("choose the saved chat: %v", err)
	}
	tt.Frame()
	if a.chat.conv.ID != "saved-1" {
		t.Fatalf("open chat = %q, want saved-1", a.chat.conv.ID)
	}
	if a.chat.history {
		t.Error("the popover stayed open after choosing a chat")
	}
	if !tt.HasText("Subtraction as a virtue.") {
		t.Errorf("the saved conversation does not show: %q", tt.Texts())
	}

	// The header's other button starts a fresh conversation.
	if err := tt.Click("New chat"); err != nil {
		t.Fatalf("start a new chat: %v", err)
	}
	tt.Frame()
	if len(a.chat.conv.Messages) != 0 || a.chat.conv.ID == "saved-1" {
		t.Errorf("New chat kept the old conversation: %+v", a.chat.conv)
	}
}

func TestChatStreamKeepsAndDropsAnswers(t *testing.T) {
	chatWith := func(role string, text string) *chatState {
		return &chatState{
			conv: appdata.Conversation{ID: "c1", Messages: []appdata.Message{
				{Role: appdata.RoleUser, Text: "Make this clearer"},
				{Role: role, Text: text},
			}},
			streaming: true,
			inFlight:  true,
			sessionID: "s1",
			gen:       7,
		}
	}

	// Streamed chunks land in the answer, which a finished turn keeps.
	a := newApp()
	a.chat = chatWith(appdata.RoleAssistant, "")
	a.applyUpdate(acp.Update{Type: acp.UpdateAgentMessageChunk, SessionID: "s1", Text: "Hello "})
	a.applyUpdate(acp.Update{Type: acp.UpdateAgentMessageChunk, SessionID: "s1", Text: "world."})
	if got := a.chat.conv.Messages[1].Text; got != "Hello world." {
		t.Fatalf("answer = %q, want Hello world.", got)
	}
	if !a.chat.streaming {
		t.Error("streaming ended while chunks were still arriving")
	}
	a.finishChat(7, acp.StopEndTurn, nil)
	if a.chat.streaming || a.chat.inFlight {
		t.Errorf("the turn is still open: %+v", a.chat)
	}
	if got := len(a.chat.conv.Messages); got != 2 || a.chat.conv.Messages[1].Text != "Hello world." {
		t.Errorf("the answer was not kept: %+v", a.chat.conv.Messages)
	}

	// A chunk from another session is not this conversation's.
	a.chat = chatWith(appdata.RoleAssistant, "")
	a.applyUpdate(acp.Update{Type: acp.UpdateAgentMessageChunk, SessionID: "other", Text: "no"})
	if got := a.chat.conv.Messages[1].Text; got != "" {
		t.Errorf("a chunk of another session landed: %q", got)
	}

	// An empty answer is dropped and the failure shows inline.
	a.chat = chatWith(appdata.RoleAssistant, "")
	a.finishChat(7, "", errors.New("the turn broke"))
	if a.chat.streaming || a.chat.inFlight {
		t.Errorf("a failed turn is still open: %+v", a.chat)
	}
	if len(a.chat.conv.Messages) != 1 {
		t.Errorf("the empty answer stayed: %+v", a.chat.conv.Messages)
	}
	if !strings.Contains(a.chat.err, "the turn broke") {
		t.Errorf("inline error = %q, want the failure", a.chat.err)
	}
	if a.chat.retry.instruction != "" || len(a.chat.retry.attachments) > 0 {
		t.Errorf("retry = %+v without a request", a.chat.retry)
	}

	// A callback from a turn the writer replaced does nothing.
	a.chat = chatWith(appdata.RoleAssistant, "Kept.")
	a.finishChat(6, acp.StopEndTurn, nil)
	if !a.chat.streaming || len(a.chat.conv.Messages) != 2 {
		t.Errorf("a stale finish changed the chat: %+v", a.chat)
	}
}

func TestChatSavesConversation(t *testing.T) {
	dir := t.TempDir()
	a := newApp()
	a.chats = &appdata.Chats{Dir: dir}
	a.chat = &chatState{}
	a.sendChatPrompt("Make this clearer")
	deadline := time.Now().Add(5 * time.Second)
	for {
		list, err := a.chats.List()
		if err == nil && len(list) == 1 && len(list[0].Messages) == 1 {
			conv := list[0]
			if conv.Title != "Make this clearer" {
				t.Errorf("saved title = %q", conv.Title)
			}
			if conv.Messages[0].Role != appdata.RoleUser {
				t.Errorf("saved message = %+v", conv.Messages[0])
			}
			if conv.Page != "" {
				t.Errorf("saved page = %q without one open", conv.Page)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the conversation was not saved: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
