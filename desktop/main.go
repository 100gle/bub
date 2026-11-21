// Command bub-desktop is Bub Desktop: a native Notion-like writing app, drawn
// by MyGo, where pages are blocks and the agent works on them.
//
// The window is a sidebar of Markdown pages and saved AI chats, a block
// editor in the middle, and a rail for the AI conversation or the page's
// contents. Block actions live in the "Bub" menu (Rewrite Block ⇧⌘R, Shorten,
// Expand, Continue, Translate, Ask Bub… ⇧⌘K); the agent streams a suggestion
// for the caret's block into a small popover, and nothing enters the page —
// or its undo history — until the writer accepts it.
//
// The agent is a separate `bub acp` process speaking the Agent Client Protocol
// over stdio: the window and the agent share no code, only ACP.
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	mygo.App.SetName(appDisplayName)
	setNativeAppName(appDisplayName)
	a := newApp()
	a.start()
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     appTitle,
			Width:     windowWidth,
			Height:    windowHeight,
			MinWidth:  minWindowWidth,
			MinHeight: minWindowHeight,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
		a.win.OnClosed(a.shutdown)
		a.win.OnClose(a.handleClose)
		mygo.App.SetMenu(newMenu(a))
		setNativeMenuTitle(appDisplayName)
		a.uiReady = true
		a.updateTitle()
		a.focusIn = true
		a.connect()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
