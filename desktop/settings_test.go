package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/bubbuild/bub/desktop/internal/appdata"
)

func TestSettingsPage(t *testing.T) {
	a, _ := newShellApp(t)
	tt := ui.NewTester(a.settingsView, 960, 1800)
	tt.Frame()
	for _, want := range []string{
		"Workspace", "Appearance", "Editor", "AI", "Shortcuts", "About",
		"Location", "Choose…", a.settings.Workspace,
		"Theme", "System", "Light", "Dark",
		"Line width", "Font size",
		"Context from nearby blocks", "Page outline in context",
		"Agent command", "Autosave",
		"New page", "Ask Bub", "Reconnect",
		appVersion, a.settingsPath,
	} {
		if !tt.HasText(want) {
			t.Errorf("the settings page does not show %q: %q", want, tt.Texts())
		}
	}
	// Each shortcut renders as a keycap badge, with the whole chord as its
	// accessible name.
	for _, keys := range []string{"⌘N", "⌘K", "⇧⌘R", "⇧⌘K", "⌘,"} {
		if _, ok := tt.Find(keys); !ok {
			t.Errorf("the shortcuts group has no %q keycap: %q", keys, tt.Texts())
		}
	}
}

func TestSettingsControlsPersist(t *testing.T) {
	a, _ := newShellApp(t)
	tt := ui.NewTester(a.settingsView, 960, 1800)
	tt.Frame()

	// Appearance: a segment changes the persisted theme.
	if err := tt.Click("Dark"); err != nil {
		t.Fatalf("click Dark: %v", err)
	}
	if a.settings.Theme != appdata.ThemeDark {
		t.Errorf("theme = %q, want %q", a.settings.Theme, appdata.ThemeDark)
	}
	tt.Frame()
	if a.settings.Theme != appdata.ThemeDark {
		t.Errorf("theme did not stay dark: %q", a.settings.Theme)
	}

	// A switch changes its setting and does not flip back next frame.
	before := a.settings.Autosave
	if err := tt.Click("Toggle autosave"); err != nil {
		t.Fatalf("click autosave: %v", err)
	}
	if a.settings.Autosave == before {
		t.Errorf("autosave = %v, want it toggled", a.settings.Autosave)
	}
	tt.Frame()
	if a.settings.Autosave == before {
		t.Errorf("autosave flipped back to %v", a.settings.Autosave)
	}

	// The agent command is kept while it is typed and saved when the field
	// gives the focus back. The row's label and the field share a name, so
	// click the field itself rather than the label.
	field, ok := tt.AccessByRole(ui.RoleTextField, "Agent command")
	if !ok {
		t.Fatalf("no agent command field: %q", tt.Texts())
	}
	tt.ClickAt(field.Bounds.X+field.Bounds.W/2, field.Bounds.Y+field.Bounds.H/2)
	tt.Type("bub acp --workspace .")
	tt.Frame()
	if err := tt.Click("Toggle context from nearby blocks"); err != nil {
		t.Fatalf("click context switch: %v", err)
	}
	tt.Frame()
	if got, want := a.settings.AgentCmd, "bub acp --workspace ."; got != want {
		t.Errorf("agent command = %q, want %q", got, want)
	}
}
