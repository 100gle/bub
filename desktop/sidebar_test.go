package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/bubbuild/bub/desktop/internal/appdata"
	"github.com/bubbuild/bub/desktop/internal/workspace"
)

// newShellApp is the app on a fresh temporary workspace, without a window or
// an agent. The workspace's settings path is inside it, so a change never
// writes to the real user's configuration.
func newShellApp(t *testing.T) (*app, *workspace.Workspace) {
	t.Helper()
	dir := t.TempDir()
	ws, err := workspace.Open(dir)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	a := newApp()
	a.ws = ws
	a.settings.Workspace = dir
	a.settingsPath = filepath.Join(dir, "settings.json")
	a.dialog = func(title, message string, retry func()) {}
	return a, ws
}

// saveChat writes a conversation into a chats store under dir.
func saveChat(t *testing.T, dir string, conv *appdata.Conversation) {
	t.Helper()
	chats := &appdata.Chats{Dir: dir}
	if err := chats.Save(conv); err != nil {
		t.Fatalf("save conversation: %v", err)
	}
}

func TestSidebarEmptyWorkspace(t *testing.T) {
	a, _ := newShellApp(t)
	a.reloadPages()
	if len(a.pages) != 0 {
		t.Fatalf("a fresh workspace has %d pages, want none", len(a.pages))
	}

	tt := ui.NewTester(a.sidebar, 280, 720)
	tt.Frame()
	for _, want := range []string{
		"Search",
		"Pages",
		"Chats",
		"Settings",
	} {
		if !tt.HasText(want) {
			t.Errorf("the empty sidebar does not show %q: %q", want, tt.Texts())
		}
	}
	if tt.HasText("AI Chats") {
		t.Errorf("the sidebar still says AI Chats: %q", tt.Texts())
	}
	if _, ok := tt.Find("⌘,"); !ok {
		t.Errorf("the settings shortcut has no keycap badge: %q", tt.Texts())
	}
	// The empty workspace has no paragraph and no second button: the section
	// header's icon is the one way to start a page.
	if tt.HasText("No pages yet") {
		t.Errorf("the empty state is still in the sidebar: %q", tt.Texts())
	}
	var newPageButtons int
	for _, info := range tt.AccessAll() {
		if info.Role == ui.RoleButton && info.Name == "New page" {
			newPageButtons++
		}
	}
	if newPageButtons != 1 {
		t.Errorf("the sidebar has %d New page buttons, want the header icon alone", newPageButtons)
	}
	// The window names the app; the sidebar does not repeat it, and the
	// workspace name lives in the header's breadcrumb.
	if tt.HasText("Bub") || tt.HasText(filepath.Base(a.settings.Workspace)) {
		t.Errorf("the sidebar repeats the app or workspace name: %q", tt.Texts())
	}

	// The header's icon starts the workspace.
	if err := tt.Click("New page"); err != nil {
		t.Fatalf("click the New page icon: %v", err)
	}
	if a.current == "" {
		t.Fatal("New page did not open a page")
	}
	tt.Frame()
	if !tt.HasText("Untitled") {
		t.Errorf("the new page is not in the tree: %q", tt.Texts())
	}
}

func TestSidebarPagesAndChats(t *testing.T) {
	a, ws := newShellApp(t)
	first, err := ws.CreatePage("", "First page")
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	second, err := ws.CreatePage("", "Second page")
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	nested, err := ws.CreatePage("notes", "Nested page")
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	a.reloadPages()
	a.current = first

	chatsDir := filepath.Join(t.TempDir(), "chats")
	saveChat(t, chatsDir, &appdata.Conversation{
		ID:        "chat-tone",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages: []appdata.Message{
			{Role: appdata.RoleUser, Text: "Make the intro shorter", At: time.Now()},
		},
	})
	a.chats = &appdata.Chats{Dir: chatsDir}
	a.reloadChats()
	if len(a.chatList) != 1 {
		t.Fatalf("chat list has %d conversations, want 1", len(a.chatList))
	}

	tt := ui.NewTester(a.sidebar, 300, 900)
	tt.Frame()
	for _, want := range []string{"First page", "Second page", "notes", "Make the intro shorter", "now"} {
		if !tt.HasText(want) {
			t.Errorf("the sidebar does not show %q: %q", want, tt.Texts())
		}
	}
	if tt.HasText("Nested page") {
		t.Errorf("a closed folder shows its pages: %q", tt.Texts())
	}

	// A page row opens the page.
	if err := tt.Click("Second page"); err != nil {
		t.Fatalf("click page: %v", err)
	}
	if a.current != second {
		t.Errorf("current = %q, want %q", a.current, second)
	}

	// A folder row opens and closes it.
	if err := tt.Click("notes"); err != nil {
		t.Fatalf("click folder: %v", err)
	}
	if !a.openFolders["notes"] {
		t.Fatal("clicking the folder did not open it")
	}
	tt.Frame()
	if !tt.HasText("Nested page") {
		t.Errorf("the open folder does not show its page: %q", tt.Texts())
	}

	// A chat row opens the conversation.
	if err := tt.Click("Make the intro shorter"); err != nil {
		t.Fatalf("click chat: %v", err)
	}
	if a.chat == nil || a.chat.conv.ID != "chat-tone" {
		t.Errorf("chat = %+v, want chat-tone", a.chat)
	}
	if got, want := nested, "notes/nested-page.md"; got != want {
		t.Errorf("nested page = %q, want %q", got, want)
	}
}

// focusSearch puts the caret in the sidebar's search field, checking that
// assistive technology sees it as a search field named Search.
func focusSearch(t *testing.T, tt *ui.Tester) {
	t.Helper()
	field, ok := tt.AccessByRole(ui.RoleTextField, "Search")
	if !ok {
		t.Fatalf("the sidebar has no search field: %q", tt.Texts())
	}
	if !field.Search {
		t.Errorf("the search field is a plain text field")
	}
	tt.ClickAt(field.Bounds.X+field.Bounds.W/2, field.Bounds.Y+field.Bounds.H/2)
	tt.Frame()
}

func TestSidebarSearchFilters(t *testing.T) {
	a, ws := newShellApp(t)
	first, err := ws.CreatePage("", "First page")
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	if _, err := ws.CreatePage("", "Second page"); err != nil {
		t.Fatalf("create page: %v", err)
	}
	if _, err := ws.CreatePage("notes", "Nested page"); err != nil {
		t.Fatalf("create page: %v", err)
	}
	a.reloadPages()
	a.current = first

	chatsDir := filepath.Join(t.TempDir(), "chats")
	saveChat(t, chatsDir, &appdata.Conversation{
		ID:        "chat-intro",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Messages: []appdata.Message{
			{Role: appdata.RoleUser, Text: "Make the intro shorter", At: time.Now()},
		},
	})
	a.chats = &appdata.Chats{Dir: chatsDir}
	a.reloadChats()

	tt := ui.NewTester(a.sidebar, 320, 900)
	tt.Frame()
	focusSearch(t, tt)

	// A lowercase query finds a capitalized name, and a page inside a closed
	// folder matches through the tree: the folder row stays and opens for the
	// search, while the other pages drop away.
	tt.Type("  nested  ")
	tt.Frame()
	if !tt.HasText("Nested page") {
		t.Errorf("the nested page does not match: %q", tt.Texts())
	}
	if !tt.HasText("notes") {
		t.Errorf("the matching page's folder row is missing: %q", tt.Texts())
	}
	if tt.HasText("First page") || tt.HasText("Second page") {
		t.Errorf("the search shows pages that do not match: %q", tt.Texts())
	}
	if tt.HasText("Chats") || tt.HasText("Make the intro shorter") {
		t.Errorf("the search shows the chats without a match: %q", tt.Texts())
	}
	if a.openFolders["notes"] {
		t.Errorf("filtering changed the folder's saved open state")
	}

	// Clearing restores the real tree, with the folder still closed.
	if err := tt.Click("Clear"); err != nil {
		t.Fatalf("clear the search: %v", err)
	}
	tt.Frame()
	if tt.HasText("Nested page") {
		t.Errorf("the closed folder shows its page after the search: %q", tt.Texts())
	}
	if !tt.HasText("First page") || !tt.HasText("Second page") {
		t.Errorf("clearing the search did not restore the pages: %q", tt.Texts())
	}

	// A chat title matches case-insensitively, and no page row comes along.
	tt.Type("INTRO")
	tt.Frame()
	if !tt.HasText("Make the intro shorter") {
		t.Errorf("the chat does not match: %q", tt.Texts())
	}
	if !tt.HasText("Chats") {
		t.Errorf("the Chats caption is missing from the matching section: %q", tt.Texts())
	}
	if tt.HasText("Pages") || tt.HasText("First page") {
		t.Errorf("the search shows the pages without a match: %q", tt.Texts())
	}

	// A query nothing matches replaces both captions with one quiet line.
	if err := tt.Click("Clear"); err != nil {
		t.Fatalf("clear the search: %v", err)
	}
	tt.Frame()
	tt.Type("zzz")
	tt.Frame()
	if !tt.HasText("No matches") {
		t.Errorf("the no-match state is missing: %q", tt.Texts())
	}
	for _, hidden := range []string{
		"Pages", "Chats", "First page", "Second page", "notes",
		"Make the intro shorter",
	} {
		if tt.HasText(hidden) {
			t.Errorf("the no-match state still shows %q: %q", hidden, tt.Texts())
		}
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, "New page"); ok {
		t.Errorf("the no-match state offers a New page button")
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, "New chat"); ok {
		t.Errorf("the no-match state offers a New chat button")
	}

	// Clearing the field brings the normal sidebar back.
	if err := tt.Click("Clear"); err != nil {
		t.Fatalf("clear the search: %v", err)
	}
	tt.Frame()
	for _, want := range []string{
		"Pages", "Chats", "First page", "Second page", "notes",
		"Make the intro shorter",
	} {
		if !tt.HasText(want) {
			t.Errorf("clearing the search does not restore %q: %q", want, tt.Texts())
		}
	}
	if tt.HasText("No matches") {
		t.Errorf("the no-match line stayed after the search was cleared: %q", tt.Texts())
	}
}

func TestSidebarSettingsRow(t *testing.T) {
	a, _ := newShellApp(t)
	a.reloadPages()
	tt := ui.NewTester(a.sidebar, 280, 720)
	tt.Frame()
	if err := tt.Click("Settings"); err != nil {
		t.Fatalf("click Settings: %v", err)
	}
	if a.mode != viewSettings {
		t.Errorf("mode = %v, want settings", a.mode)
	}
}

// openPageMenu hovers a page row so its overflow button shows, then opens the
// menu under it.
func openPageMenu(t *testing.T, tt *ui.Tester, title string) {
	t.Helper()
	row, ok := tt.Find(title)
	if !ok {
		t.Fatalf("no page row %q: %q", title, tt.Texts())
	}
	tt.Move(row.X+row.W/2, row.Y+row.H/2)
	tt.Frame()
	actions, ok := tt.Find("Actions for " + title)
	if !ok {
		t.Fatalf("no overflow button for %q: %q", title, tt.Texts())
	}
	tt.ClickAt(actions.X+actions.W/2, actions.Y+actions.H/2)
	tt.Frame()
	if !tt.HasText("Rename…") || !tt.HasText("Delete") {
		t.Fatalf("the overflow menu did not open: %q", tt.Texts())
	}
}

func TestSidebarRenamePage(t *testing.T) {
	a, ws := newShellApp(t)
	rel, err := ws.CreatePage("", "Old title")
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	a.reloadPages()
	a.current = rel
	tt := ui.NewTester(a.sidebar, 320, 760)
	tt.Frame()
	openPageMenu(t, tt, "Old title")

	if err := tt.Click("Rename…"); err != nil {
		t.Fatalf("click Rename: %v", err)
	}
	tt.Frame()
	field, ok := tt.AccessByRole(ui.RoleTextField, "Page title")
	if !ok {
		t.Fatalf("no rename field: %q", tt.Texts())
	}
	if field.Value != "Old title" {
		t.Errorf("rename field = %q, want the page's title", field.Value)
	}
	tt.ClickAt(field.Bounds.X+field.Bounds.W/2, field.Bounds.Y+field.Bounds.H/2)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("New title")
	tt.Frame()
	if err := tt.Click("Save"); err != nil {
		t.Fatalf("click Save: %v", err)
	}
	tt.Frame()
	if tt.HasText("Rename page") {
		t.Errorf("the rename dialog stayed open: %q", tt.Texts())
	}
	if !tt.HasText("New title") {
		t.Errorf("the renamed page is not in the tree: %q", tt.Texts())
	}
	if a.current == rel {
		t.Errorf("current is still the old page %q", rel)
	}
	if _, err := ws.ReadPage(a.current); err != nil {
		t.Errorf("read the renamed page: %v", err)
	}
	if _, err := ws.ReadPage(rel); err == nil {
		t.Errorf("the old page %q still exists", rel)
	}
}

func TestSidebarDeletePage(t *testing.T) {
	a, ws := newShellApp(t)
	rel, err := ws.CreatePage("", "Disposable page")
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	a.reloadPages()
	a.current = rel
	tt := ui.NewTester(a.sidebar, 320, 760)
	tt.Frame()

	// Cancel keeps the page.
	openPageMenu(t, tt, "Disposable page")
	if err := tt.Click("Delete"); err != nil {
		t.Fatalf("click Delete: %v", err)
	}
	tt.Frame()
	if !tt.HasText("Delete page?") {
		t.Fatalf("no delete confirmation: %q", tt.Texts())
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatalf("click Cancel: %v", err)
	}
	tt.Frame()
	if _, err := ws.ReadPage(rel); err != nil {
		t.Errorf("cancel deleted the page: %v", err)
	}
	if a.current != rel {
		t.Errorf("cancel closed the page: current = %q", a.current)
	}

	// Confirming removes it.
	openPageMenu(t, tt, "Disposable page")
	if err := tt.Click("Delete"); err != nil {
		t.Fatalf("click Delete: %v", err)
	}
	tt.Frame()
	if err := tt.Click("Delete"); err != nil {
		t.Fatalf("confirm Delete: %v", err)
	}
	tt.Frame()
	if _, err := ws.ReadPage(rel); err == nil {
		t.Errorf("the page %q still exists after delete", rel)
	}
	if tt.HasText("Disposable page") {
		t.Errorf("the deleted page is still in the tree: %q", tt.Texts())
	}
}

// TestSearchShortcutFocusesTheField checks the ⌘K way into search: it opens
// the sidebar when it is hidden and puts the caret in the search field.
func TestSearchShortcutFocusesTheField(t *testing.T) {
	a, _ := newShellApp(t)
	a.sidebarShown = false
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1100, 800)
	tt.Frame()
	a.focusSearch()
	tt.Frame()
	tt.Frame()
	if !a.sidebarShown {
		t.Error("⌘K left the sidebar hidden")
	}
	if !tt.Focused("Search") {
		t.Error("⌘K did not focus the search field")
	}
}

// TestSidebarChatsAboutAPage checks the explicit page-scoped way into a chat:
// the page row's menu starts a conversation that references that page, so a
// chat is tied to a page only when the writer says so.
func TestSidebarChatsAboutAPage(t *testing.T) {
	a, ws := newShellApp(t)
	rel, err := ws.CreatePage("", "A page to chat about")
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	a.reloadPages()
	tt := ui.NewTester(a.sidebar, 320, 760)
	tt.Frame()
	openPageMenu(t, tt, "A page to chat about")

	if err := tt.Click("Chat about this page"); err != nil {
		t.Fatalf("click Chat about this page: %v", err)
	}
	tt.Frame()
	if a.mode != viewChat {
		t.Errorf("mode = %v, want the chat page", a.mode)
	}
	if a.chat == nil {
		t.Fatal("no conversation")
	}
	if len(a.chat.conv.References) != 1 || a.chat.conv.References[0] != rel {
		t.Errorf("references = %v, want the page %q", a.chat.conv.References, rel)
	}
	if a.chat.conv.Page != "" {
		t.Errorf("the chat is still bound to one page: %q", a.chat.conv.Page)
	}
}
