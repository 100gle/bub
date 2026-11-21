package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/bubbuild/bub/desktop/internal/acp"
	"github.com/bubbuild/bub/desktop/internal/appdata"
)

// TestMentionFilesResolveWorkspacePages checks that "@page" mentions become
// resource links: the workspace is searched for the named pages, and only
// pages the draft actually mentions are attached.
func TestMentionFilesResolveWorkspacePages(t *testing.T) {
	a, ws := newShellApp(t)
	relOne, err := ws.CreatePage("", "Alpha page")
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	relTwo, err := ws.CreatePage("", "Beta page")
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	a.reloadPages()
	absOne, err := a.ws.Abs(relOne)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	got := a.mentionFiles("Compare @" + relOne + " with @" + relTwo + ".")
	if len(got) != 2 {
		t.Fatalf("mentions = %v, want the two pages", got)
	}
	seen := map[string]bool{}
	for _, path := range got {
		seen[path] = true
	}
	if !seen[absOne] {
		t.Errorf("mentions missing %s: %v", absOne, got)
	}
	// A mention that names no page is left alone.
	if got := a.mentionFiles("see @missing.md now"); len(got) != 0 {
		t.Errorf("unknown mention attached %v", got)
	}
	// An email's "@" is not a page mention either.
	if got := a.mentionFiles("contact me at a@b.example"); len(got) != 0 {
		t.Errorf("an email's @ attached %v", got)
	}
}

// TestCommandRequestTranslatesSlashCommands checks the composer's "/" turn:
// an advertised command becomes the agent's command line, sent verbatim; a
// skill becomes its "$name" hint, which the agent expands while it answers.
func TestCommandRequestTranslatesSlashCommands(t *testing.T) {
	a := newApp()
	a.commands = []acp.AvailableCommand{
		{Name: "help", Description: "Show available commands"},
		{Name: "gh", Kind: commandKindSkill, Description: "Run a GitHub skill"},
	}

	req, ok := a.commandRequest("/help")
	if !ok {
		t.Fatal("/help is not a command")
	}
	if req.instruction != "/help" || req.prompt != "/help" {
		t.Errorf("command = %+v, want the display text and the agent's /help", req)
	}
	req, ok = a.commandRequest("/help now")
	if !ok || req.prompt != "/help now" {
		t.Errorf("command with args = %+v, want /help now", req)
	}

	req, ok = a.commandRequest("/gh ")
	if !ok {
		t.Fatal("/gh is not a command")
	}
	if req.instruction != "/gh" || req.hint != "$gh" || req.prompt != "" {
		t.Errorf("skill = %+v, want the /gh display and the $gh hint in a normal turn", req)
	}
	req, ok = a.commandRequest("/gh open the repo")
	if !ok || req.instruction != "/gh open the repo" || req.hint != "$gh open the repo" {
		t.Errorf("skill with words = %+v, want the display and the hint plus the request", req)
	}

	if _, ok := a.commandRequest("/no-such-command x"); ok {
		t.Error("an unadvertised command still became a command")
	}
	if _, ok := a.commandRequest("a plain question /help"); ok {
		t.Error("a slash inside prose became a command")
	}
}

// TestComposerPaletteMentionsAPage checks the "/" and "@" palettes: typing
// the trigger opens the menu, Enter inserts the row's text into the draft,
// and the menu closes.
func TestComposerPaletteMentionsAPage(t *testing.T) {
	a, _ := newChatPageApp(t) // a page "The quiet web.md" exists in the workspace
	a.mode = viewChat
	a.chatFocus = true
	a.chat = &chatState{conv: appdata.NewConversation()}
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1100, 800)
	tt.Frame()

	tt.Type("@")
	tt.Frame()
	if a.chat.menu == nil || a.chat.menu.kind != '@' {
		t.Fatalf("typing @ opened no mention palette: %+v", a.chat.menu)
	}
	if len(a.composerItems(a.chat)) == 0 {
		t.Fatal("the mention palette has no pages")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !strings.HasPrefix(a.chat.input, "@") {
		t.Errorf("picking a mention left %q", a.chat.input)
	}
	if a.chat.menu != nil {
		t.Error("the mention palette stayed open after picking")
	}
}

// TestComposerPaletteRunsACommand checks the "/" palette against the commands
// the agent advertised: typing filters them, Enter inserts "/name " so the
// writer can add arguments, and Escape dismisses without reopening.
func TestComposerPaletteRunsACommand(t *testing.T) {
	a, _ := newChatPageApp(t)
	a.mode = viewChat
	a.chatFocus = true
	a.chat = &chatState{conv: appdata.NewConversation()}
	a.commands = []acp.AvailableCommand{
		{Name: "help", Description: "Show available commands"},
		{Name: "model", Description: "Choose the model"},
	}
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1100, 800)
	tt.Frame()

	tt.Type("/mo")
	tt.Frame()
	if a.chat.menu == nil || a.chat.menu.kind != '/' {
		t.Fatalf("typing / opened no command palette: %+v", a.chat.menu)
	}
	items := a.composerItems(a.chat)
	if len(items) != 1 || items[0].title != "/model" {
		t.Fatalf("filtered commands = %+v, want only /model", items)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.chat.input != "/model " {
		t.Errorf("inserted %q, want \"/model \"", a.chat.input)
	}

	// Escape closes the palette and its trigger stays dismissed until the
	// caret leaves it.
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.chat.menu != nil {
		t.Fatal("Escape did not close the palette")
	}
	tt.Frame()
	if a.chat.menu != nil {
		t.Error("the dismissed trigger reopened the palette")
	}
}

// TestRegenerateRerunsTheLastTurn checks that Regenerate takes the newest
// finished answer away and runs its turn again, keeping the writer's message.
func TestRegenerateRerunsTheLastTurn(t *testing.T) {
	a := newApp()
	a.chat = &chatState{conv: appdata.Conversation{
		ID: "conv",
		Messages: []appdata.Message{
			{Role: appdata.RoleUser, Text: "Tighten this"},
			{Role: appdata.RoleAssistant, Text: "Sharper."},
		},
	}}
	a.regenerateChat(1)
	// No agent in a bare app: the rerun fails fast, but the old answer is
	// already gone and the writer's message is what a retry would repeat.
	if got := len(a.chat.conv.Messages); got != 1 {
		t.Fatalf("messages = %d, want the writer's one", got)
	}
	if a.chat.conv.Messages[0].Role != appdata.RoleUser || a.chat.conv.Messages[0].Text != "Tighten this" {
		t.Errorf("kept %+v, want the writer's message", a.chat.conv.Messages[0])
	}

	// Regenerate is only for the last finished answer, not an older one.
	a.chat = &chatState{conv: appdata.Conversation{
		Messages: []appdata.Message{
			{Role: appdata.RoleUser, Text: "A"},
			{Role: appdata.RoleAssistant, Text: "One"},
			{Role: appdata.RoleUser, Text: "B"},
			{Role: appdata.RoleAssistant, Text: "Two"},
		},
	}}
	a.regenerateChat(1)
	if n := len(a.chat.conv.Messages); n != 4 {
		t.Errorf("an older answer was regenerated: %d messages", n)
	}
}
