package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/bubbuild/bub/desktop/internal/appdata"
)

// TestMarkdownLook pins the answer look to the tokens: a real code panel and a
// heading ramp that still reads inside the narrow rail.
func TestMarkdownLook(t *testing.T) {
	a := newApp()
	var look *markdown.Theme
	tt := ui.NewTester(func(c *ui.Context) {
		a.applyTheme(c)
		look = a.markdownLook(c, false)
		ui.Text(c, "look")
	}, 320, 240)
	tt.Frame()

	if look == nil {
		t.Fatal("the frame made no Markdown theme")
	}
	if look.CodeBlockBackground == (ui.Color{}) {
		t.Error("the code block has no background")
	}
	if look.CodeBlockBorder == (ui.Color{}) {
		t.Error("the code block has no border")
	}
	if got, want := look.CodeBlockRadius, tokens.RadiusMD; got != want {
		t.Errorf("code block radius = %v, want %v", got, want)
	}
	if got, want := look.FontSize, tokens.FontControl; got != want {
		t.Errorf("body size = %v, want %v", got, want)
	}
	for i, h := range look.Headings {
		if got, want := h.Weight, tokens.WeightStrong; got != want {
			t.Errorf("heading %d weight = %d, want %d", i+1, got, want)
		}
	}
}

// TestChatMessageRendersCodeBlock checks that an answer goes through the full
// Markdown code block in the transcript: the fence's language, the copy
// action, and the code itself all come from the library's read-only view, as
// the page's editor cannot show them.
func TestChatMessageRendersCodeBlock(t *testing.T) {
	a := newApp()
	a.chat = &chatState{conv: appdata.Conversation{
		ID: "code-chat",
		Messages: []appdata.Message{
			{Role: appdata.RoleUser, Text: "Show me a Go program"},
			{Role: appdata.RoleAssistant, Text: "Here is a program:\n\n```go\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```\n"},
		},
	}}
	tt := ui.NewTester(func(c *ui.Context) {
		a.applyTheme(c)
		ui.Column(c).Width(288).FillHeight().Children(func() {
			a.chatMessages(c)
		})
	}, 288, 720)
	tt.Frame()

	if a.chat.md.Theme == nil {
		t.Fatal("the transcript did not theme its Markdown")
	}
	if got, want := a.chat.md.Theme.CodeBlockRadius, tokens.RadiusMD; got != want {
		t.Errorf("code block radius = %v, want %v", got, want)
	}
	for _, want := range []string{"Code, go", "go", "Copy code", "func main() {", `fmt.Println("hi")`} {
		if !tt.HasText(want) {
			t.Errorf("the rendered answer misses %q: %q", want, tt.Texts())
		}
	}
}
