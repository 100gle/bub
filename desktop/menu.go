package main

import "github.com/egoist/mygo"

// newMenu is the application menu: the app, File, Edit, Format, View, Bub and
// Window menus. Everything the window can do with the keyboard is here.
func newMenu(a *app) *mygo.Menu {
	return mygo.NewMenu(menuTemplate(a))
}

// menuTemplate is the menu as a template, so tests can check labels,
// accelerators and roles without a window.
func menuTemplate(a *app) []*mygo.MenuItem {
	click := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) { fn() }
	}
	block := func(kind actionKind) func(*mygo.MenuItem, *mygo.Window) {
		return click(func() { a.menuAction(kind) })
	}
	return []*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "File", Submenu: []*mygo.MenuItem{
			{ID: "file-new", Label: "New Page", Accelerator: "CmdOrCtrl+N", Click: click(a.menuNew)},
			{ID: "file-new-chat", Label: "New AI Chat", Accelerator: "CmdOrCtrl+Shift+N", Click: click(a.menuNewChat)},
			mygo.Separator(),
			{ID: "file-save", Label: "Save", Accelerator: "CmdOrCtrl+S", Click: click(a.menuSave)},
			mygo.Separator(),
			{Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleFormatMenu},
		{Label: "View", Submenu: []*mygo.MenuItem{
			{ID: "view-search", Label: "Search", Accelerator: "CmdOrCtrl+K", Click: click(a.focusSearch)},
			{ID: "view-sidebar", Label: "Toggle Sidebar", Accelerator: "CmdOrCtrl+\\", Click: click(a.toggleSidebar)},
			{ID: "view-chat", Label: "Toggle AI Chat", Accelerator: "CmdOrCtrl+J", Click: click(func() { a.toggleRail(railChat) })},
			{ID: "view-outline", Label: "Toggle Contents", Accelerator: "CmdOrCtrl+Shift+O", Click: click(func() { a.toggleRail(railOutline) })},
			mygo.Separator(),
			{ID: "view-settings", Label: "Settings…", Accelerator: "CmdOrCtrl+,", Click: click(a.menuSettings)},
		}},
		{Label: "Bub", Submenu: []*mygo.MenuItem{
			{ID: "bub-rewrite", Label: "Rewrite Block", Accelerator: "CmdOrCtrl+Shift+R", Click: block(actionRewrite)},
			{ID: "bub-shorten", Label: "Shorten Block", Click: block(actionShorten)},
			{ID: "bub-expand", Label: "Expand Block", Click: block(actionExpand)},
			{ID: "bub-continue", Label: "Continue Block", Click: block(actionContinue)},
			{ID: "bub-translate", Label: "Translate Block", Click: block(actionTranslate)},
			mygo.Separator(),
			{ID: "bub-ask", Label: "Ask Bub…", Accelerator: "CmdOrCtrl+Shift+K", Click: click(a.menuAsk)},
			{ID: "bub-chat", Label: "Chat with Bub", Click: click(func() { a.showRail(railChat) })},
			mygo.Separator(),
			{ID: "bub-reconnect", Label: "Reconnect to Bub", Click: click(a.menuReconnect)},
		}},
		{Role: mygo.RoleWindowMenu},
	}
}

// menuNew starts a new page in the open page's folder. Discarding unsaved
// changes must not save them on the way, so the page is marked saved first.
func (a *app) menuNew() {
	a.confirmDiscard(func() {
		a.dirty = false
		a.newPage()
	})
}

// menuNewChat starts an empty AI conversation on the chat page.
func (a *app) menuNewChat() {
	a.newChat()
}

// menuSave writes the open page.
func (a *app) menuSave() {
	a.savePage()
}

// menuSettings opens the settings page.
func (a *app) menuSettings() {
	a.showSettings()
}
