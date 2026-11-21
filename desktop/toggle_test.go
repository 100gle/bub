package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/bubbuild/bub/desktop/internal/acp"
	"github.com/bubbuild/bub/desktop/internal/appdata"
)

// TestChatConfigToggleFlipsOnce checks a boolean option: the switch carries
// the option's name for assistive technology, and pressing it asks the agent
// for the change, which the app applies when the agent answers with the
// options it keeps.
//
// The agent here is a client with no connection: the click must reach
// setChatConfigBool, which reports the failed change through the app's usual
// error path. Turning the switch on in the view is internal/acp's own
// business, covered by its loopback tests.
func TestChatConfigToggleFlipsOnce(t *testing.T) {
	a, _ := newChatPageApp(t)
	a.mode = viewChat
	a.chat = &chatState{conv: appdata.Conversation{ID: "chat-bool"}, sessionID: "sess-1"}
	a.chat.configOptions = []acp.ConfigOption{
		{ID: "web", Name: "Search the web", Kind: acp.ConfigKindBoolean, Boolean: false},
	}
	var alerts []alert
	a.dialog = func(title, message string, retry func()) {
		alerts = append(alerts, alert{title: title, message: message, retry: retry})
	}
	// Non-nil, so the app lets the change through instead of dropping it
	// for want of a session; the calls it makes fail, as they would with no
	// agent.
	a.client = &acp.Client{}
	// Report the failed change as the window would, instead of to the log.
	a.uiReady = true

	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1280, 860)
	tt.Frame()
	if !tt.HasText("Search the web") {
		t.Fatalf("the toggle lost its name: %q", tt.Texts())
	}
	sw, ok := tt.AccessByRole(ui.RoleSwitch, "Search the web")
	if !ok {
		t.Fatalf("the toggle has no switch named after it: %q", tt.Texts())
	}
	tt.ClickAt(sw.Bounds.X+sw.Bounds.W/2, sw.Bounds.Y+sw.Bounds.H/2)
	tt.Frame()
	if len(alerts) == 0 || alerts[0].title != "Could not change this chat setting" {
		t.Fatalf("pressing the switch asked the agent for nothing: %+v", alerts)
	}
}
