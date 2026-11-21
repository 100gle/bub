package main

import (
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/bubbuild/bub/desktop/internal/acp"
	"github.com/bubbuild/bub/desktop/internal/appdata"
	"github.com/bubbuild/bub/desktop/internal/prompt"
)

// newChatPageApp is the app with a page open and one saved chat about it.
func newChatPageApp(t *testing.T) (*app, string) {
	t.Helper()
	a, ws := newShellApp(t)
	rel, err := ws.CreatePage("", "The quiet web")
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	a.reloadPages()
	a.current = rel
	load(t, a, "# The quiet web\n\nFor a decade the web grew by adding things.\n")
	chatsDir := filepath.Join(t.TempDir(), "chats")
	saveChat(t, chatsDir, &appdata.Conversation{
		ID:         "chat-tone",
		Title:      "Tighten the intro",
		References: []string{rel},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Messages: []appdata.Message{
			{Role: appdata.RoleUser, Text: "Tighten the intro", At: time.Now()},
		},
	})
	a.chats = &appdata.Chats{Dir: chatsDir}
	a.reloadChats()
	return a, rel
}

// TestChatRowOpensTheChatPage checks that a sidebar conversation opens the
// page-sized chat, not the rail, and that its header goes back to the page.
func TestChatRowOpensTheChatPage(t *testing.T) {
	a, rel := newChatPageApp(t)
	tt := ui.NewTester(a.sidebar, 300, 700)
	tt.Frame()
	if err := tt.Click("Tighten the intro"); err != nil {
		t.Fatalf("click chat: %v", err)
	}
	tt.Frame()
	if a.mode != viewChat {
		t.Fatalf("mode = %v, want the chat page", a.mode)
	}
	if a.railShown {
		t.Error("the chat page also opened the rail")
	}

	page := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1100, 800)
	page.Frame()
	for _, want := range []string{"Tighten the intro", "The quiet web", "Tighten the intro"} {
		if !page.HasText(want) {
			t.Errorf("the chat page misses %q: %q", want, page.Texts())
		}
	}
	if _, ok := page.AccessByRole(ui.RoleTextField, "Message Bub"); !ok {
		t.Errorf("the chat page has no composer: %q", page.Texts())
	}
	// Opening a chat puts the caret in the composer, ready to type.
	page.Type("Draft")
	page.Frame()
	if a.chat == nil || a.chat.input != "Draft" {
		t.Errorf("typing after opening a chat went nowhere: input = %q", a.chat.input)
	}
	if err := page.Click("Back to page"); err != nil {
		t.Fatalf("click back: %v", err)
	}
	if a.mode != viewPage {
		t.Errorf("mode after Back = %v, want the page", a.mode)
	}
	if a.current != rel {
		t.Errorf("current = %q, want %q", a.current, rel)
	}
}

// TestChatPageNewChatButton checks that the sidebar's New chat opens the chat
// page on a fresh conversation.
func TestChatPageNewChatButton(t *testing.T) {
	a, _ := newChatPageApp(t)
	tt := ui.NewTester(a.sidebar, 300, 700)
	tt.Frame()
	if err := tt.Click("New chat"); err != nil {
		t.Fatalf("click New chat: %v", err)
	}
	if a.mode != viewChat {
		t.Fatalf("mode = %v, want the chat page", a.mode)
	}
	if a.chat == nil || len(a.chat.conv.Messages) != 0 {
		t.Fatalf("new chat = %+v, want an empty conversation", a.chat)
	}
}

// TestChatPageComposerEnterAndShiftEnter checks the page composer's keys:
// Enter sends the draft, Shift+Enter keeps a line in it.
func TestChatPageComposerEnterAndShiftEnter(t *testing.T) {
	a, _ := newChatPageApp(t)
	a.mode = viewChat
	a.chatFocus = true
	a.chat = &chatState{conv: appdata.NewConversation()}
	a.chat.scroll.Y = 1 << 30

	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1100, 800)
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleTextField, "Message Bub"); !ok {
		t.Fatalf("no page composer: %q", tt.Texts())
	}
	tt.Type("Line one")
	tt.Frame()
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Type("Line two")
	tt.Frame()
	if len(a.chat.conv.Messages) != 0 {
		t.Fatalf("Shift+Enter sent the draft: %+v", a.chat.conv.Messages)
	}
	if !strings.Contains(a.chat.input, "\n") || !strings.Contains(a.chat.input, "Line two") {
		t.Fatalf("the draft after Shift+Enter = %q, want two lines", a.chat.input)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if len(a.chat.conv.Messages) == 0 {
		t.Fatal("Enter did not send the draft")
	}
	if got := a.chat.conv.Messages[0]; got.Role != appdata.RoleUser || got.Text != "Line one\nLine two" {
		t.Errorf("sent message = %+v, want the writer's two lines", got)
	}
	if a.chat.input != "" {
		t.Errorf("the composer kept %q after sending", a.chat.input)
	}
}

// TestChatPageRendersMarkdown checks the page transcript uses the Markdown
// renderer, code block and all.
func TestChatPageRendersMarkdown(t *testing.T) {
	a, _ := newChatPageApp(t)
	a.mode = viewChat
	a.chat = &chatState{conv: appdata.Conversation{
		ID: "chat-code",
		Messages: []appdata.Message{
			{Role: appdata.RoleUser, Text: "Show me a program", At: time.Now()},
			{Role: appdata.RoleAssistant, Text: "Here it is:\n\n```go\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```\n", At: time.Now()},
		},
	}}
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1100, 800)
	tt.Frame()
	for _, want := range []string{"Code, go", "Copy code", "func main() {"} {
		if !tt.HasText(want) {
			t.Errorf("the chat page misses %q: %q", want, tt.Texts())
		}
	}
	if a.chat.md.Theme == nil {
		t.Fatal("the chat page did not theme its Markdown")
	}
	if got, want := a.chat.md.Theme.CodeBlockRadius, tokens.RadiusMD; got != want {
		t.Errorf("code block radius = %v, want %v", got, want)
	}
}

// TestChatPageShowsAgentConfigOptions checks that the composer shows the
// session options the agent advertises, as the model and thinking selectors.
func TestChatPageShowsAgentConfigOptions(t *testing.T) {
	a, _ := newChatPageApp(t)
	a.mode = viewChat
	a.chat = &chatState{
		conv:      appdata.Conversation{ID: "chat-config"},
		sessionID: "sess-1",
		configOptions: []acp.ConfigOption{
			{ID: "model", Name: "Model", Kind: acp.ConfigKindSelect, Current: "sonnet",
				Choices: []acp.ConfigChoice{{Value: "sonnet", Name: "Sonnet"}, {Value: "opus", Name: "Opus"}}},
			{ID: "thought", Name: "Think", Kind: acp.ConfigKindSelect, Current: "high",
				Choices: []acp.ConfigChoice{{Value: "auto", Name: "Auto"}, {Value: "high", Name: "High"}}},
		},
	}
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1280, 860)
	tt.Frame()
	for _, want := range []string{"Model · Sonnet", "Think · High"} {
		if !tt.HasText(want) {
			t.Errorf("the composer misses the agent option %q: %q", want, tt.Texts())
		}
	}
}

// TestChatPromptBlocksCarryAttachments checks the ACP prompt: the words, then
// each file as a resource link, without reading the file into the prompt.
func TestChatPromptBlocksCarryAttachments(t *testing.T) {
	blocks := chatPromptBlocks("Read this brief", []string{"/tmp/report.pdf"}, false)
	if len(blocks) != 2 {
		t.Fatalf("prompt has %d blocks, want text plus one link", len(blocks))
	}
	if blocks[0].Text == nil || blocks[0].Text.Text != "Read this brief" {
		t.Errorf("first block = %+v, want the instruction", blocks[0])
	}
	link := blocks[1].ResourceLink
	if link == nil {
		t.Fatalf("second block is not a resource link: %+v", blocks[1])
	}
	if link.Name != "report.pdf" || link.Uri != "file:///tmp/report.pdf" {
		t.Errorf("resource link = %+v, want report.pdf at its file URI", link)
	}
}

// TestChatDraftAttachmentsTravelWithTheTurn checks that a composer holding
// files sends them with the turn, clears them, and keeps them in the saved
// message.
func TestChatDraftAttachmentsTravelWithTheTurn(t *testing.T) {
	a, _ := newChatPageApp(t)
	// No store: the save is asynchronous, and a temp workspace must not be
	// removed under it.
	a.chats = nil
	a.chat = &chatState{conv: appdata.NewConversation()}
	a.chat.attachments = []string{"/tmp/report.pdf", "/tmp/notes.md"}
	a.sendChatDraft(a.chat, "Use these")
	if len(a.chat.attachments) != 0 {
		t.Errorf("the draft kept its attachments: %v", a.chat.attachments)
	}
	if len(a.chat.conv.Messages) != 1 {
		t.Fatalf("the turn has %d messages, want the writer's one", len(a.chat.conv.Messages))
	}
	msg := a.chat.conv.Messages[0]
	if msg.Text != "Use these" || len(msg.Attachments) != 2 {
		t.Errorf("user message = %+v, want the words and two files", msg)
	}
}

// TestChatPageComposerStaysInTheWindow checks that a long transcript does not
// push the composer below the window: it is pinned to the page's bottom.
func TestChatPageComposerStaysInTheWindow(t *testing.T) {
	a, _ := newChatPageApp(t)
	a.mode = viewChat
	a.chat = &chatState{conv: appdata.Conversation{ID: "chat-long"}}
	for i := 0; i < 12; i++ {
		a.chat.conv.Messages = append(a.chat.conv.Messages,
			appdata.Message{Role: appdata.RoleUser, Text: "Question", At: time.Now()},
			appdata.Message{Role: appdata.RoleAssistant, Text: strings.Repeat("word ", 40), At: time.Now()},
		)
	}
	const height = 800
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1100, height)
	tt.Frame()
	send, ok := tt.Access("Send")
	if !ok {
		t.Fatalf("no Send button: %q", tt.Texts())
	}
	if bottom := send.Bounds.Y + send.Bounds.H; bottom > height {
		t.Errorf("composer bottom = %v, below the %d DIP window", bottom, height)
	}
}

// TestNewChatDoesNotReferenceTheOpenPage checks the optimizer's first point: a
// new chat starts with no context at all, even while a page is open. The page
// is something the writer attaches, not something the app inherits.
func TestNewChatDoesNotReferenceTheOpenPage(t *testing.T) {
	a, _ := newChatPageApp(t)
	if a.current == "" {
		t.Fatal("the fixture has no open page")
	}
	a.newChat()
	if a.chat == nil {
		t.Fatal("New chat made no conversation")
	}
	if len(a.chat.conv.References) != 0 || a.chat.conv.Page != "" {
		t.Fatalf("a new chat inherited context: references=%v page=%q", a.chat.conv.References, a.chat.conv.Page)
	}
	if ctx := a.chatPromptContext(a.chat.conv.References); ctx.Title != "" || len(ctx.Refs) != 0 {
		t.Errorf("a new chat carries prompt context: %+v", ctx)
	}
	written := prompt.Chat("What should I write next?", a.chatPromptContext(a.chat.conv.References))
	for _, notWant := range []string{"Open page:", "Referenced pages:", "Page outline:"} {
		if strings.Contains(written, notWant) {
			t.Errorf("the prompt of a fresh chat carries %q:\n%s", notWant, written)
		}
	}
}

// TestChatReferencesTravelWithTurns checks the second point: a conversation
// reads exactly the pages the writer attached, as resource links every turn,
// and the open page is only enriched when it is one of them.
func TestChatReferencesTravelWithTurns(t *testing.T) {
	a, rel := newChatPageApp(t)
	a.chats = nil
	a.chat = &chatState{conv: appdata.NewConversation()}

	// While nothing is attached, the open page is not context.
	if ctx := a.chatPromptContext(a.chat.conv.References); ctx.Title != "" {
		t.Fatalf("the open page became context without a reference: %+v", ctx)
	}

	// Attaching the page gives every turn its file and its live details.
	a.addChatReference(a.chat, rel)
	abs, err := a.ws.Abs(rel)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	files := a.chatAttachmentsForTurn(a.chat.conv.References, []string{"/tmp/report.pdf"})
	if len(files) != 2 || files[0] != "/tmp/report.pdf" || files[1] != abs {
		t.Errorf("turn files = %v, want the attachment and the referenced page", files)
	}
	ctx := a.chatPromptContext(a.chat.conv.References)
	if ctx.Title != "The quiet web" {
		t.Errorf("open referenced page = %q, want its title", ctx.Title)
	}
	written := prompt.Chat("Tighten the intro", ctx)
	if !strings.Contains(written, "Referenced pages") || !strings.Contains(written, `Open page: "The quiet web"`) {
		t.Errorf("the prompt does not carry the reference:\n%s", written)
	}

	// A duplicate (a mention of the same page) is sent once.
	dup := a.chatAttachmentsForTurn(a.chat.conv.References, []string{abs})
	if len(dup) != 1 {
		t.Errorf("the referenced page was sent twice: %v", dup)
	}

	// Another page the writer opens is not context until it is referenced.
	other, err := a.ws.CreatePage("", "Beta notes")
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	a.reloadPages()
	a.openPage(other)
	ctx = a.chatPromptContext(a.chat.conv.References)
	if ctx.Title != "" {
		t.Errorf("an unreferenced page became context: %+v", ctx)
	}

	// Removing the reference takes it off the turns too.
	removeChatReference(a.chat, rel)
	if ctx := a.chatPromptContext(a.chat.conv.References); ctx.Title != "" || ctx.Refs != nil {
		t.Errorf("a removed reference still contributes context: %+v", ctx)
	}
}

// TestChatContextChipsSitAboveTheComposer checks the composer's hierarchy: the
// referenced pages and the attached files are chips above the field, as
// context and attachments, not controls in the toolbar.
func TestChatContextChipsSitAboveTheComposer(t *testing.T) {
	a, _ := newChatPageApp(t)
	a.mode = viewChat
	a.chat = &chatState{conv: appdata.NewConversation()}
	other, err := a.ws.CreatePage("", "Beta notes")
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	a.reloadPages()
	a.addChatReference(a.chat, other)
	a.chat.attachments = []string{"/tmp/report.pdf"}

	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1100, 800)
	tt.Frame()
	field, ok := tt.AccessByRole(ui.RoleTextField, "Message Bub")
	if !ok {
		t.Fatalf("no composer field: %q", tt.Texts())
	}
	for _, label := range []string{"Remove reference Beta notes", "Remove attachment report.pdf"} {
		remove, ok := tt.Access(label)
		if !ok {
			t.Fatalf("no %q chip: %q", label, tt.Texts())
		}
		if remove.Bounds.Y+remove.Bounds.H > field.Bounds.Y {
			t.Errorf("%q sits at y=%v, below the field at y=%v", label, remove.Bounds.Y, field.Bounds.Y)
		}
	}
	// The add-context control stays a control, in the toolbar under the field.
	add, ok := tt.Access("Add context")
	if !ok {
		t.Fatalf("no Add context control: %q", tt.Texts())
	}
	if add.Bounds.Y < field.Bounds.Y {
		t.Errorf("Add context sits above the field: y=%v field=%v", add.Bounds.Y, field.Bounds.Y)
	}
}

// TestAddContextReferencesTheOpenPage checks the one gesture that gives a chat
// context: the composer's + menu attaches the open page as a visible chip.
func TestAddContextReferencesTheOpenPage(t *testing.T) {
	a, rel := newChatPageApp(t)
	a.mode = viewChat
	a.chat = &chatState{conv: appdata.NewConversation()}
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1100, 800)
	tt.Frame()
	if err := tt.Click("Add context"); err != nil {
		t.Fatalf("click Add context: %v", err)
	}
	tt.Frame()
	if !tt.HasText("Reference this page") {
		t.Fatalf("the add-context menu did not open: %q", tt.Texts())
	}
	if err := tt.Click("Reference this page"); err != nil {
		t.Fatalf("click Reference this page: %v", err)
	}
	tt.Frame()
	if len(a.chat.conv.References) != 1 || a.chat.conv.References[0] != rel {
		t.Errorf("references = %v, want the open page %q", a.chat.conv.References, rel)
	}
	if a.chat.contextOpen {
		t.Error("the add-context menu stayed open after picking")
	}
	if !tt.HasText("The quiet web") {
		t.Errorf("the reference chip does not show: %q", tt.Texts())
	}
}

// TestChatPageEmptyOffersTheOpenPage checks that the empty state offers the
// open page as context in one press, so discovery does not require the menu.
func TestChatPageEmptyOffersTheOpenPage(t *testing.T) {
	a, rel := newChatPageApp(t)
	a.mode = viewChat
	a.chat = &chatState{conv: appdata.NewConversation()}
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1100, 800)
	tt.Frame()
	if !tt.HasText("Add this page as context") {
		t.Fatalf("the empty state offers no page context: %q", tt.Texts())
	}
	if err := tt.Click("Add this page as context"); err != nil {
		t.Fatalf("click Add this page as context: %v", err)
	}
	tt.Frame()
	if len(a.chat.conv.References) != 1 || a.chat.conv.References[0] != rel {
		t.Errorf("references = %v, want the open page %q", a.chat.conv.References, rel)
	}
	if tt.HasText("Add this page as context") {
		t.Errorf("the offer stayed after the page was attached: %q", tt.Texts())
	}
}

// tinyPNG writes an 8x8 PNG into dir and returns its path, a picture the
// transcript and the composer can decode.
func tinyPNG(t *testing.T, dir, name string) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 30), B: 128, A: 255})
		}
	}
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestChatPromptBlocksAttachImages checks the ACP prompt for pictures: an
// image rides as an inline image block with its base64 data, and any other
// file keeps its resource link.
func TestChatPromptBlocksAttachImages(t *testing.T) {
	shot := tinyPNG(t, t.TempDir(), "shot.png")
	blocks := chatPromptBlocks("Look at this", []string{shot, "/tmp/notes.pdf"}, true)
	if len(blocks) != 3 {
		t.Fatalf("prompt has %d blocks, want text plus image plus link", len(blocks))
	}
	img := blocks[1].Image
	if img == nil {
		t.Fatalf("second block is not an image: %+v", blocks[1])
	}
	if img.MimeType != "image/png" || img.Data == "" {
		t.Errorf("image block = %+v, want base64 image/png data", img)
	}
	if _, err := base64.StdEncoding.DecodeString(img.Data); err != nil {
		t.Errorf("image data is not base64: %v", err)
	}
	if link := blocks[2].ResourceLink; link == nil || link.Name != "notes.pdf" {
		t.Errorf("third block = %+v, want the notes.pdf link", blocks[2])
	}

	// A file that only looks like an image falls back to a resource link.
	not := filepath.Join(t.TempDir(), "fake.png")
	if err := os.WriteFile(not, []byte("not a picture"), 0o600); err != nil {
		t.Fatal(err)
	}
	blocks = chatPromptBlocks("Look", []string{not}, true)
	if len(blocks) != 2 || blocks[1].ResourceLink == nil {
		t.Errorf("an undecodable image did not fall back to a link: %+v", blocks)
	}

	// An agent that does not advertise images gets the file as a link.
	blocks = chatPromptBlocks("Look", []string{shot}, false)
	if len(blocks) != 2 || blocks[1].ResourceLink == nil || blocks[1].Image != nil {
		t.Errorf("an image outside the agent's capability did not become a link: %+v", blocks[1])
	}
}

// TestChatImagesRenderInTranscript checks that a sent picture shows in the
// transcript as something the writer can open, not only as a file chip.
func TestChatImagesRenderInTranscript(t *testing.T) {
	a, _ := newChatPageApp(t)
	a.chats = nil
	a.mode = viewChat
	shot := tinyPNG(t, t.TempDir(), "shot.png")
	a.chat = &chatState{conv: appdata.Conversation{
		ID: "chat-img",
		Messages: []appdata.Message{
			{Role: appdata.RoleUser, Text: "Here is the shot", Attachments: []string{shot}},
			{Role: appdata.RoleAssistant, Text: "The hand is out of focus."},
		},
	}}
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1100, 800)
	tt.Frame()
	if _, ok := tt.Access("Open shot.png"); !ok {
		t.Errorf("the picture has no open affordance: %q", tt.Texts())
	}
	if a.picture(shot) == nil {
		t.Error("the picture did not decode")
	}
}
