package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/richtext"
)

func TestTitleAndEditedState(t *testing.T) {
	a, _ := newTestApp(t)
	var edited []bool
	a.onEdited = func(v bool) { edited = append(edited, v) }

	if a.documentTitle() != "Untitled" {
		t.Errorf("new document title = %q, want Untitled", a.documentTitle())
	}
	a.setDirty(true)
	if !a.dirty {
		t.Error("setDirty(true) did not mark it edited")
	}
	a.setDocument(richtext.ParseMarkdown("# A quiet title\n"), filepath.Join(t.TempDir(), "note.md"))
	if a.dirty {
		t.Error("an opened document arrived dirty")
	}
	if got := a.documentTitle(); got != "A quiet title" {
		t.Errorf("title = %q, want the first heading", got)
	}
	a.setDocument(richtext.NewText(""), "")
	if a.dirty {
		t.Error("a cleared document is still dirty")
	}
	if got := a.documentTitle(); got != "Untitled" {
		t.Errorf("after clearing: title = %q, want Untitled", got)
	}
	if len(edited) < 2 || !edited[0] || edited[len(edited)-1] {
		t.Errorf("edited states = %v, want true then false", edited)
	}
}

func TestPageLifecycleOnDisk(t *testing.T) {
	a, root := newWorkspaceApp(t)
	rel, err := a.ws.CreatePage("", "Notes")
	if err != nil {
		t.Fatal(err)
	}
	a.reloadPages()
	a.openPage(rel)
	if a.current != rel || a.path != filepath.Join(root, rel) {
		t.Fatalf("current = %q path = %q, want %q", a.current, a.path, filepath.Join(root, rel))
	}
	if got := a.documentTitle(); got != "Notes" {
		t.Errorf("title = %q, want Notes", got)
	}
	a.doc.SetSelection(a.doc.Text().Len(), a.doc.Text().Len())
	a.doc.InsertText("\nA line written by hand.")
	a.onEdit()
	if !a.dirty {
		t.Fatal("typing did not mark the page dirty")
	}
	saveAndWait(t, a)
	data, err := os.ReadFile(a.path)
	if err != nil || !strings.Contains(string(data), "A line written by hand.") {
		t.Fatalf("the page on disk = %q, %v", data, err)
	}
	if a.dirty {
		t.Error("a saved page is still dirty")
	}

	// Reopening another page and coming back reads what was written.
	other, err := a.ws.CreatePage("", "Other")
	if err != nil {
		t.Fatal(err)
	}
	a.reloadPages()
	a.openPage(other)
	a.openPage(rel)
	if got := a.doc.Text().PlainText(); !strings.Contains(got, "A line written by hand.") {
		t.Errorf("reopened text = %q", got)
	}
}

func TestNewPageCreatesAndOpens(t *testing.T) {
	a, _ := newWorkspaceApp(t)
	first, err := a.ws.CreatePage("", "First")
	if err != nil {
		t.Fatal(err)
	}
	a.reloadPages()
	a.openPage(first)
	a.newPage()
	if a.current == "" {
		t.Fatal("New page did not open a page")
	}
	if a.current == first {
		t.Fatal("New page opened the old page")
	}
	if got := a.documentTitle(); got != "Untitled" {
		t.Errorf("new page title = %q", got)
	}
	if firstPage(a.pages) == "" {
		t.Error("the sidebar tree is empty after creating a page")
	}
}

func TestRenameAndDeletePage(t *testing.T) {
	a, _ := newWorkspaceApp(t)
	rel, err := a.ws.CreatePage("", "Old name")
	if err != nil {
		t.Fatal(err)
	}
	a.reloadPages()
	a.openPage(rel)
	a.renamePage(rel, "New name")
	if a.current == rel {
		t.Error("rename left the old page path open")
	}
	if got := a.documentTitle(); got != "New name" {
		t.Errorf("renamed title = %q, want New name", got)
	}
	renamed := a.current
	a.deletePage(renamed)
	if _, err := os.Stat(filepath.Join(a.ws.Root, renamed)); !os.IsNotExist(err) {
		t.Errorf("delete left %q on disk", renamed)
	}
	if a.current == renamed {
		t.Error("delete left the deleted page open")
	}
}

func TestMarkdownSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	source := "# Title\n\nHello **world**, and _friends_.\n\n- One\n- Two\n"
	text := richtext.ParseMarkdown(source)
	if err := os.WriteFile(path, []byte(text.Markdown()), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# Title") || !strings.Contains(string(data), "**world**") {
		t.Errorf("saved Markdown = %q", data)
	}
	read := richtext.ParseMarkdown(string(data))
	if read.Markdown() != text.Markdown() {
		t.Errorf("round trip changed the Markdown:\n%q\n%q", text.Markdown(), read.Markdown())
	}
}
