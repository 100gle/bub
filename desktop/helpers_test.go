package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/richtext"

	"github.com/bubbuild/bub/desktop/internal/workspace"
)

// alert records what the app would have shown as a native dialog.
type alert struct {
	title, message string
	retry          func()
}

// newTestApp is the app without a window or a workspace: the editor, the
// popovers and the action engine are driven directly, and dialogs are
// recorded instead of shown.
func newTestApp(t *testing.T) (*app, *[]alert) {
	t.Helper()
	a := newApp()
	a.railShown = false
	var alerts []alert
	a.dialog = func(title, message string, retry func()) {
		alerts = append(alerts, alert{title: title, message: message, retry: retry})
	}
	return a, &alerts
}

// newWorkspaceApp is the app over a temporary workspace on disk.
func newWorkspaceApp(t *testing.T) (*app, string) {
	t.Helper()
	root := t.TempDir()
	ws, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := newTestApp(t)
	a.ws = ws
	a.settings.Workspace = root
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.reloadPages()
	return a, root
}

// load shows Markdown as the open document.
func load(t *testing.T, a *app, markdown string) {
	t.Helper()
	a.setDocument(richtext.ParseMarkdown(markdown), "")
}

// saveAndWait saves the open page and waits for the asynchronous write, so
// the test does not read app state from another goroutine.
func saveAndWait(t *testing.T, a *app) {
	t.Helper()
	done := make(chan struct{}, 1)
	a.onSaved = func() { done <- struct{}{} }
	a.savePage()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the page to save")
	}
	a.onSaved = nil
}
