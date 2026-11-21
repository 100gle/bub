package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"
)

// contextMenuView shows the app's editor alone in the window, with the
// context menu an app sets in EditorOptions.Menu, and counts its builds.
func contextMenuView(a *app, builds *int) func(c *ui.Context) {
	return func(c *ui.Context) {
		ui.Box(c).Fill().Padding(40).Children(func() {
			richtext.EditorBase(c.Key("editor"), a.doc, richtext.EditorOptions{
				Label: "Page",
				Menu: func(m *ui.Menu) {
					*builds++
					a.blockContextMenu(m)
				},
			}).Fill()
		})
	}
}

// contextMenuEditor frames the window and returns the editor's box.
func contextMenuEditor(t *testing.T, tt *ui.Tester) ui.Rect {
	t.Helper()
	tt.Frame()
	ed, ok := tt.Find("Page")
	if !ok {
		t.Fatalf("no editor showing Page: %q", tt.Texts())
	}
	return ed
}

// rightClickBlock right-clicks the text of the page's paragraph index, so
// that a right-click inside the selection keeps it.
func rightClickBlock(t *testing.T, tt *ui.Tester, a *app, ed ui.Rect, index int) {
	t.Helper()
	boxes := a.doc.VisibleParagraphs()
	if index >= len(boxes) {
		t.Fatalf("the editor shows %d paragraphs, want block %d", len(boxes), index)
	}
	b := boxes[index]
	tt.RightClickAt(ed.X+ed.W/2, ed.Y+b.Top+b.Height/2)
}

// hasMenuItem reports whether menu contains label.
func hasMenuItem(menu []string, label string) bool {
	for _, l := range menu {
		if l == label {
			return true
		}
	}
	return false
}

// TestBlockContextMenuAsksAboutTheSelectedBlock opens the editor's context
// menu on a selection in the second paragraph and asks Bub about it.
func TestBlockContextMenuAsksAboutTheSelectedBlock(t *testing.T) {
	a := newApp()
	a.setDocument(richtext.ParseMarkdown("Alpha.\n\nBravo.\n"), "")
	blocks := a.blocks()
	if len(blocks) < 2 {
		t.Fatalf("the page has %d blocks, want 2", len(blocks))
	}

	// The target is the block holding the selection's start.
	a.doc.SetSelection(blocks[1].Start+1, blocks[1].Start+3)
	if target := a.contextTarget(); target == nil || target.Index != 1 {
		t.Fatalf("contextTarget() = %+v, want block 1", target)
	}

	builds := 0
	tt := ui.NewTester(contextMenuView(a, &builds), 700, 400)
	ed := contextMenuEditor(t, tt)
	a.doc.SetSelection(blocks[1].Start, blocks[1].End)
	tt.Frame()

	rightClickBlock(t, tt, a, ed, 1)
	menu := tt.Menu()
	if !hasMenuItem(menu, "Bub") {
		t.Fatalf("the context menu has no Bub submenu: %q", menu)
	}
	// Replacing the editor's menu must not take the system's editing
	// commands away.
	for _, item := range []string{"Cut", "Copy", "Paste", "Select All"} {
		if !hasMenuItem(menu, item) {
			t.Errorf("the context menu lost the system's %q item: %q", item, menu)
		}
	}
	if err := tt.ChooseMenuItem("Bub", "Ask Bub…"); err != nil {
		t.Fatalf("choosing Ask Bub…: %v", err)
	}
	if builds < 2 {
		t.Errorf("the menu was built %d times, want the collect and the chosen run", builds)
	}
	if a.ask == nil {
		t.Fatal("Ask Bub… did not open the ask popover")
	}
	if a.ask.target.Index != 1 {
		t.Errorf("ask target = block %d, want the selected block 1", a.ask.target.Index)
	}
}

// TestBlockContextMenuTargetsTheCaretBlock opens the editor's context menu
// over a caret in the first paragraph and asks Bub about that block.
func TestBlockContextMenuTargetsTheCaretBlock(t *testing.T) {
	a := newApp()
	a.setDocument(richtext.ParseMarkdown("Alpha.\n\nBravo.\n"), "")
	blocks := a.blocks()
	if len(blocks) < 2 {
		t.Fatalf("the page has %d blocks, want 2", len(blocks))
	}

	builds := 0
	tt := ui.NewTester(contextMenuView(a, &builds), 700, 400)
	ed := contextMenuEditor(t, tt)
	a.doc.SetSelection(blocks[0].Start+1, blocks[0].Start+1)
	tt.Frame()

	rightClickBlock(t, tt, a, ed, 0)
	if !hasMenuItem(tt.Menu(), "Bub") {
		t.Fatalf("the context menu has no Bub submenu: %q", tt.Menu())
	}
	if err := tt.ChooseMenuItem("Bub", "Ask Bub…"); err != nil {
		t.Fatalf("choosing Ask Bub…: %v", err)
	}
	if a.ask == nil {
		t.Fatal("Ask Bub… did not open the ask popover")
	}
	if a.ask.target.Index != 0 {
		t.Errorf("ask target = block %d, want the caret's block 0", a.ask.target.Index)
	}
}

// TestBlockContextMenuDerivesTheTargetEachRun checks that a new selection
// between the menu's two runs reaches a new target.
func TestBlockContextMenuDerivesTheTargetEachRun(t *testing.T) {
	a := newApp()
	a.setDocument(richtext.ParseMarkdown("Alpha.\n\nBravo.\n"), "")
	blocks := a.blocks()
	if len(blocks) < 2 {
		t.Fatalf("the page has %d blocks, want 2", len(blocks))
	}
	a.doc.SetSelection(blocks[0].Start+1, blocks[0].Start+1)
	if target := a.contextTarget(); target == nil || target.Index != 0 {
		t.Fatalf("contextTarget() = %+v, want block 0", target)
	}
	a.doc.SetSelection(blocks[1].Start+1, blocks[1].Start+1)
	if target := a.contextTarget(); target == nil || target.Index != 1 {
		t.Fatalf("contextTarget() = %+v, want block 1 after the selection moved", target)
	}
}
