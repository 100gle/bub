package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClosePromptCancelKeepsWindowOpen(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "Unsaved words.\n")
	a.setDirty(true)
	asked := unsavedResponder(a, 1) // Cancel
	a.askToClose()
	if *asked != 1 {
		t.Errorf("asked %d times, want once", *asked)
	}
	if a.allowClose {
		t.Error("Cancel let the window close")
	}
	if !a.dirty {
		t.Error("Cancel cleared the edited state")
	}
	if a.closingPrompt {
		t.Error("the close prompt stayed open after Cancel")
	}
}

func TestClosePromptDiscardCloses(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "Unsaved words.\n")
	a.setDirty(true)
	unsavedResponder(a, 2) // Discard
	a.askToClose()
	if !a.allowClose {
		t.Error("Discard did not let the window close")
	}
	if a.closingPrompt {
		t.Error("the close prompt stayed open after Discard")
	}
}

func TestClosePromptSaveWritesThenCloses(t *testing.T) {
	a, _ := newWorkspaceApp(t)
	rel, err := a.ws.CreatePage("", "Closing")
	if err != nil {
		t.Fatal(err)
	}
	a.reloadPages()
	a.openPage(rel)
	a.doc.SetSelection(a.doc.Text().Len(), a.doc.Text().Len())
	a.doc.InsertText("\nUnsaved words.")
	a.onEdit()
	done := make(chan struct{}, 1)
	a.onSaved = func() { done <- struct{}{} }
	unsavedResponder(a, 0) // Save
	a.askToClose()
	if !a.allowClose {
		t.Error("Save on close did not close the window")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Save on close to write")
	}
	data, err := os.ReadFile(a.path)
	if err != nil || !strings.Contains(string(data), "Unsaved words.") {
		t.Fatalf("saved Markdown = %q, %v", data, err)
	}
	if a.dirty {
		t.Error("a saved document is still dirty")
	}
}

func TestClosePromptAskedOnce(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "Unsaved words.\n")
	a.setDirty(true)
	asked := 0
	var answer func(int)
	a.chooseUnsaved = func(then func(int)) {
		asked++
		answer = then
	}
	a.askToClose()
	a.askToClose() // a second close request while the prompt is up
	if asked != 1 {
		t.Fatalf("asked %d times, want once", asked)
	}
	answer(1) // Cancel
	if a.allowClose || a.closingPrompt {
		t.Errorf("after Cancel: allowClose=%v closingPrompt=%v", a.allowClose, a.closingPrompt)
	}
}

func TestFileNewAsksBeforeDiscarding(t *testing.T) {
	a, _ := newWorkspaceApp(t)
	rel, err := a.ws.CreatePage("", "Precious")
	if err != nil {
		t.Fatal(err)
	}
	a.reloadPages()
	a.openPage(rel)
	a.doc.SetSelection(a.doc.Text().Len(), a.doc.Text().Len())
	a.doc.InsertText("\nPrecious words.")
	a.onEdit()
	unsavedResponder(a, 1) // Cancel
	template := menuTemplate(a)
	template[1].Submenu[0].Click(template[1].Submenu[0], nil) // File > New Page
	if a.current != rel {
		t.Errorf("New Page switched pages after Cancel (current %q)", a.current)
	}
	if got := a.doc.Text().PlainText(); !strings.Contains(got, "Precious words.") {
		t.Errorf("New Page discarded text after Cancel: %q", got)
	}

	oldPath := filepath.Join(a.ws.Root, rel)
	unsavedResponder(a, 2) // Discard
	template = menuTemplate(a)
	template[1].Submenu[0].Click(template[1].Submenu[0], nil)
	if a.current == rel {
		t.Errorf("New Page kept the old page open after Discard")
	}
	if data, err := os.ReadFile(oldPath); err == nil && strings.Contains(string(data), "Precious words.") {
		t.Errorf("Discard wrote the discarded words to disk: %q", data)
	}
}

func TestCleanCloseAsksNothing(t *testing.T) {
	a, _ := newTestApp(t)
	load(t, a, "Saved already.\n")
	asked := unsavedResponder(a, 2)
	a.askToClose()
	if *asked != 0 {
		t.Errorf("asked %d times without unsaved changes", *asked)
	}
	if a.allowClose {
		t.Error("a clean close was marked handled")
	}
}
