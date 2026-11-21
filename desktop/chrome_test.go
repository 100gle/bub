package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestViewShowsSidebarHeaderAndEditor(t *testing.T) {
	a, _ := newTestApp(t)
	a.sidebarShown = false
	load(t, a, "Fast pages feel more trustworthy.\n")
	a.current = "note.md"
	a.path = "/tmp/note.md"
	tt := ui.NewTester(a.view, 1180, 820)
	tt.Frame()
	if !tt.HasText("Ask Bub") {
		t.Errorf("the window misses its AI button: %q", tt.Texts())
	}
	// A saved page says nothing; the layout is the state.
	if tt.HasText("Saved") {
		t.Errorf("a saved page still shows a save label: %q", tt.Texts())
	}
	if !tt.HasText("Page") {
		t.Errorf("no editor: %q", tt.Texts())
	}
	a.setDirty(true)
	tt.Frame()
	if !tt.HasText("Edited") {
		t.Errorf("an edited page shows no state: %q", tt.Texts())
	}
}

func TestFloatingAskBubOpensChat(t *testing.T) {
	a, _ := newTestApp(t)
	a.sidebarShown = false
	load(t, a, "Fast pages feel more trustworthy.\n")
	a.current = "note.md"
	a.path = "/tmp/note.md"
	tt := ui.NewTester(a.view, 1180, 820)
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleButton, "Ask Bub"); !ok {
		t.Fatalf("no floating Ask Bub button: %q", tt.Texts())
	}
	if err := tt.Click("Ask Bub"); err != nil {
		t.Fatalf("click the floating button: %v", err)
	}
	tt.Frame()
	if !a.railShown || a.railTab != railChat {
		t.Fatalf("the floating button did not open the chat rail: shown=%v tab=%v", a.railShown, a.railTab)
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, "Ask Bub"); ok {
		t.Errorf("the floating button stays while the rail is open: %q", tt.Texts())
	}
}

func TestHeaderTogglesTheSidebar(t *testing.T) {
	a, _ := newTestApp(t)
	a.sidebarShown = false
	load(t, a, "Fast pages feel more trustworthy.\n")
	a.current = "note.md"
	a.path = "/tmp/note.md"
	tt := ui.NewTester(a.view, 1180, 820)
	tt.Frame()
	if err := tt.Click("Show sidebar"); err != nil {
		t.Fatalf("click Show sidebar: %v", err)
	}
	tt.Frame()
	if !a.sidebarShown {
		t.Fatal("the header toggle did not show the sidebar")
	}
	if err := tt.Click("Hide sidebar"); err != nil {
		t.Fatalf("click Hide sidebar: %v", err)
	}
	tt.Frame()
	if a.sidebarShown {
		t.Fatal("the header toggle did not hide the sidebar")
	}
}

func TestTypingMarksTheDocumentDirty(t *testing.T) {
	a, _ := newTestApp(t)
	var edited []bool
	a.onEdited = func(v bool) { edited = append(edited, v) }
	tt := ui.NewTester(a.view, 1180, 820)
	tt.Frame()
	tt.Type("Hello")
	if !a.dirty {
		t.Error("typing did not mark the document dirty")
	}
	if got := a.doc.Text().PlainText(); !strings.Contains(got, "Hello") {
		t.Errorf("document = %q, want the typed text", got)
	}
	if len(edited) == 0 || !edited[len(edited)-1] {
		t.Errorf("edited states = %v, want true", edited)
	}
}

// --- Close prompt ---------------------------------------------------------

// unsavedResponder makes askUnsaved answer with a fixed button, recording how
// often it was asked.
func unsavedResponder(a *app, choice int) *int {
	asked := 0
	a.chooseUnsaved = func(then func(int)) {
		asked++
		then(choice)
	}
	return &asked
}
