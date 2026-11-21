package main

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/bubbuild/bub/desktop/internal/appdata"
	"github.com/bubbuild/bub/desktop/internal/workspace"
)

// The sidebar is the quiet left column of the window: the workspace's pages,
// the saved AI conversations, and the way into settings. It draws on the
// theme's surface so the page canvas stays the brightest thing on screen.

// sidebarState is what the sidebar keeps between frames: the search query, the
// row whose overflow menu is open, and the page a modal is renaming or
// deleting. It lives in ui.Local on the sidebar's root element, never in
// package state.
type sidebarState struct {
	// The search filtering the pages and the chats.
	query string

	// The overflow menu of one page row.
	menuFor  string
	menuOpen bool

	// The rename modal: the page and the title being edited.
	renameFor  string
	renameText string
	renameOpen bool

	// The delete confirmation: the page and how the sidebar names it.
	deleteFor  string
	deleteName string
	deleteOpen bool
}

// sidebar builds the whole left column: the search, the pages, the chats, and
// the footer with settings.
func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	col := ui.Column(c).Fill().Background(t.Surface)
	st := ui.Local(col, "sidebar", func() sidebarState { return sidebarState{} })
	if st == nil {
		// No element to own the state (the view was built without a frame):
		// draw the sidebar read-only rather than crash.
		st = &sidebarState{}
	}
	// The query is matched trimmed and lowercased, so what the field shows and
	// what the lists match are the same.
	query := strings.ToLower(strings.TrimSpace(st.query))
	filtering := query != ""
	pages, chats := a.pages, a.chatList
	if filtering {
		pages = filterPages(a.pages, query)
		chats = filterChats(a.chatList, query)
	}
	col.Children(func() {
		// The field sits above the scrolling lists, in the same gutter they
		// use, on the canvas color so it reads as paper on the surface. The
		// ⌘K hint sits inside the field at its end, the way a search field
		// teaches its shortcut without spending a column beside it.
		ui.Box(c).FillWidth().Padding(tokens.Space2, tokens.Space2).Children(func() {
			ui.SearchField(c, &st.query).Key("sidebar-search").FillWidth().
				Label("Search").Placeholder("Search").
				Background(t.Background).Radius(tokens.RadiusSM).
				Bind(&a.searchField)
			if st.query == "" {
				ui.Row(c).Absolute().Right(16).Top(12).IgnoreInput(true).AccessHidden(true).Children(func() {
					keycap(c, "⌘K").IgnoreInput(true)
				})
			}
		})
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Padding(10, 8).Gap(2).Children(func() {
				// A search shows only the sections with matches, or one quiet
				// line in place of both captions.
				if filtering && len(pages) == 0 && len(chats) == 0 {
					a.noMatches(c)
					return
				}
				showPages := !filtering || len(pages) > 0
				showChats := !filtering || len(chats) > 0
				if showPages {
					a.pagesSection(c, st, pages, filtering)
				}
				if showPages && showChats {
					ui.Box(c).Height(12)
				}
				if showChats {
					a.chatsSection(c, chats)
				}
			})
		})
		a.sidebarFooter(c)
	})
	a.sidebarDialogs(c, st)
}

// pagesSection is the workspace tree under its small heading.
func (a *app) pagesSection(c *ui.Context, st *sidebarState, nodes []*workspace.Node, filtering bool) {
	ui.Column(c).FillWidth().Gap(1).Children(func() {
		ui.Row(c).Height(26).Padding(0, 8).AlignItems(ui.Center).Children(func() {
			sectionCaption(c, "Pages")
			ui.Spacer(c)
			if iconButton(c, "pages-new", icoPlus, "New page", headerIcon, false, true, true) {
				a.newPage()
			}
		})
		a.pageTree(c, nodes, 0, st, filtering)
	})
}

// sectionCaption is a section's muted caption: regular weight and a little
// tracking, so it groups without becoming bold or loud.
func sectionCaption(c *ui.Context, label string) {
	t := c.Theme()
	ui.Text(c, label).FontSize(tokens.FontCaption).FontWeight(tokens.WeightRegular).
		LetterSpacing(0.2).TextColor(t.TextMuted).SingleLine()
}

// noMatches is the one quiet line of a search that matches nothing, in place of
// the two section captions.
func (a *app) noMatches(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(tokens.RowHeight).Padding(0, 8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "No matches").FontSize(tokens.FontCaption).TextColor(t.TextMuted).SingleLine()
	})
}

// filterPages prunes the tree to the pages whose name holds query: a folder
// stays when its own name matches or a page below it does, and only its
// matching descendants come with it. It copies the nodes it keeps, so the
// saved tree is never touched.
func filterPages(nodes []*workspace.Node, query string) []*workspace.Node {
	var kept []*workspace.Node
	for _, node := range nodes {
		if !node.IsDir {
			if matchesQuery(node.Name, query) {
				kept = append(kept, node)
			}
			continue
		}
		children := filterPages(node.Children, query)
		if len(children) == 0 && !matchesQuery(node.Name, query) {
			continue
		}
		pruned := *node
		pruned.Children = children
		kept = append(kept, &pruned)
	}
	return kept
}

// filterChats keeps the conversations the query names.
func filterChats(convs []*appdata.Conversation, query string) []*appdata.Conversation {
	var kept []*appdata.Conversation
	for _, conv := range convs {
		if matchesQuery(conversationTitle(conv), query) {
			kept = append(kept, conv)
		}
	}
	return kept
}

// matchesQuery reports whether a title holds query, ignoring case; the caller
// passes it trimmed and lowercased.
func matchesQuery(title, query string) bool {
	return strings.Contains(strings.ToLower(title), query)
}

// pageTree draws the pages below a folder, one level of indentation per depth.
func (a *app) pageTree(c *ui.Context, nodes []*workspace.Node, depth int, st *sidebarState, filtering bool) {
	for _, node := range nodes {
		node := node
		if node.IsDir {
			a.folderRow(c, node, depth, filtering)
			// A search opens every folder on screen so the matches below it
			// show, without changing the user's saved open/closed state.
			if filtering || a.openFolders[node.Rel] {
				a.pageTree(c, node.Children, depth+1, st, filtering)
			}
			continue
		}
		a.pageRow(c, node, depth, st)
	}
}

// folderRow is one folder of the tree: a chevron that opens it and its name.
// While a search filters, it shows open without owning that state.
func (a *app) folderRow(c *ui.Context, node *workspace.Node, depth int, filtering bool) {
	t := c.Theme()
	open := filtering || a.openFolders[node.Rel]
	row := ui.Row(c).Key("folder-"+node.Rel).Height(30).Padding(0, 8).
		PaddingStart(8 + 14*float32(depth)).Radius(tokens.RadiusSM).Gap(7).
		AlignItems(ui.Center).Focusable()
	if c.Preferences().Hover() && row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	if row.Clicked() {
		a.toggleFolder(node.Rel)
	}
	row.Children(func() {
		ic := icoChevron
		if open {
			ic = icoChevronDown
		}
		ui.Icon(c, ic).Size(12, 12).TextColor(t.TextMuted)
		ui.Icon(c, icoFolder).Size(15, 15).TextColor(t.TextMuted)
		ui.Text(c, node.Name).FontSize(13).TextColor(t.Text).SingleLine().Grow(1)
	})
	row.Label(node.Name)
}

// pageRow is one page of the tree: the open page sits on the canvas color in a
// heavier weight, so the selection is not color alone. Its overflow menu shows
// while the row is hovered or holds the focus.
func (a *app) pageRow(c *ui.Context, node *workspace.Node, depth int, st *sidebarState) {
	t := c.Theme()
	selected := a.current == node.Rel
	menuHere := st.menuOpen && st.menuFor == node.Rel
	row := ui.Row(c).Key("page-"+node.Rel).Height(30).Padding(0, 8).
		PaddingStart(8 + 14*float32(depth)).Radius(tokens.RadiusSM).Gap(7).
		AlignItems(ui.Center).Focusable()
	hover := c.Preferences().Hover() && row.Hovered()
	switch {
	case selected:
		row.Background(selectedFace(t))
	case hover:
		row.Background(t.SurfaceHover)
	}
	if row.Clicked() {
		a.openPage(node.Rel)
	}
	showMenu := hover || row.FocusWithin() || menuHere
	row.Children(func() {
		ui.Icon(c, icoFile).Size(15, 15).TextColor(t.TextMuted)
		title := ui.Text(c, node.Name).FontSize(13).TextColor(t.Text).SingleLine().Grow(1)
		if selected {
			title.FontWeight(500)
		}
		if !showMenu {
			return
		}
		a.pageRowMenu(c, node, st)
	})
	row.Label(node.Name)
}

// pageRowMenu is the small "…" at the end of a page row and the menu it opens.
func (a *app) pageRowMenu(c *ui.Context, node *workspace.Node, st *sidebarState) {
	t := c.Theme()
	dots := ui.Box(c).Key("page-actions").Size(20, 20).Radius(tokens.RadiusXS).Shrink(0).
		AlignItems(ui.Center).Justify(ui.Center).Focusable()
	if c.Preferences().Hover() && dots.Hovered() {
		dots.Background(t.SurfacePressed)
	}
	if dots.Clicked() {
		if st.menuOpen && st.menuFor == node.Rel {
			st.menuOpen = false
		} else {
			st.menuFor = node.Rel
			st.menuOpen = true
		}
	}
	dots.Children(func() {
		ui.Icon(c, icoMore).Size(14, 14).TextColor(t.TextMuted)
	})
	dots.Label("Actions for " + node.Name)
	if !st.menuOpen || st.menuFor != node.Rel {
		return
	}
	pop := ui.PopoverBase(c, dots, &st.menuOpen, func(panel ui.Element) {
		panel.AttachTo(dots, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(4, 0, 0, 0).
			MinWidth(168).Padding(4).Gap(1).Radius(tokens.RadiusMD).Background(t.Background).
			Border(1, t.Border).Shadow(0, 8, 20, -8, ui.RGBA(0, 0, 0, 0.18))
		a.pageMenu(c, node, st)
	})
	if pop.Dismissed() {
		st.menuFor = ""
	}
}

// pageMenu is the page's overflow menu: start a chat about it, rename it, or
// delete it.
func (a *app) pageMenu(c *ui.Context, node *workspace.Node, st *sidebarState) {
	t := c.Theme()
	chatAbout := ui.Row(c).Key("page-menu-chat").Height(30).Padding(0, 8).Radius(tokens.RadiusSM).
		Gap(8).AlignItems(ui.Center).Focusable()
	if c.Preferences().Hover() && chatAbout.Hovered() {
		chatAbout.Background(t.SurfaceHover)
	}
	if chatAbout.Clicked() {
		st.menuOpen = false
		st.menuFor = ""
		a.newChatAboutPage(node.Rel)
	}
	chatAbout.Children(func() {
		ui.Icon(c, icoChat).Size(14, 14).TextColor(t.TextMuted)
		ui.Text(c, "Chat about this page").FontSize(12.5).TextColor(t.Text).SingleLine()
	})
	chatAbout.Label("Chat about this page")

	ui.Box(c).Height(1).FillWidth().Background(t.Border).Margin(3, 0, 3, 0)

	rename := ui.Row(c).Key("page-menu-rename").Height(30).Padding(0, 8).Radius(tokens.RadiusSM).
		Gap(8).AlignItems(ui.Center).Focusable()
	if c.Preferences().Hover() && rename.Hovered() {
		rename.Background(t.SurfaceHover)
	}
	if rename.Clicked() {
		st.menuOpen = false
		st.menuFor = ""
		st.renameFor = node.Rel
		st.renameText = node.Name
		st.renameOpen = true
	}
	rename.Children(func() {
		ui.Icon(c, icoRename).Size(14, 14).TextColor(t.TextMuted)
		ui.Text(c, "Rename…").FontSize(12.5).TextColor(t.Text).SingleLine()
	})
	rename.Label("Rename…")

	remove := ui.Row(c).Key("page-menu-delete").Height(30).Padding(0, 8).Radius(tokens.RadiusSM).
		Gap(8).AlignItems(ui.Center).Focusable()
	if c.Preferences().Hover() && remove.Hovered() {
		remove.Background(t.Danger.Alpha(0.10))
	}
	if remove.Clicked() {
		st.menuOpen = false
		st.menuFor = ""
		st.deleteFor = node.Rel
		st.deleteName = node.Name
		st.deleteOpen = true
	}
	remove.Children(func() {
		ui.Icon(c, icoTrash).Size(14, 14).TextColor(t.Danger)
		ui.Text(c, "Delete").FontSize(12.5).TextColor(t.Danger).SingleLine()
	})
	remove.Label("Delete")
}

// chatsSection lists the saved AI conversations, newest first.
func (a *app) chatsSection(c *ui.Context, convs []*appdata.Conversation) {
	ui.Column(c).FillWidth().Gap(1).Children(func() {
		ui.Row(c).Height(26).Padding(0, 8).AlignItems(ui.Center).Children(func() {
			sectionCaption(c, "Chats")
			ui.Spacer(c)
			if iconButton(c, "chats-new", icoPlus, "New chat", headerIcon, false, true, true) {
				a.newChat()
			}
		})
		for _, conv := range convs {
			a.chatRow(c, conv)
		}
	})
}

// chatRow is one saved conversation; the selection is a face and a weight, not
// just a color.
func (a *app) chatRow(c *ui.Context, conv *appdata.Conversation) {
	t := c.Theme()
	selected := a.chat != nil && a.chat.conv.ID == conv.ID
	row := ui.Row(c).Key("chat-"+conv.ID).Height(30).Padding(0, 8).Radius(tokens.RadiusSM).Gap(7).
		AlignItems(ui.Center).Focusable()
	hover := c.Preferences().Hover() && row.Hovered()
	switch {
	case selected:
		row.Background(selectedFace(t))
	case hover:
		row.Background(t.SurfaceHover)
	}
	if row.Clicked() && a.chats != nil {
		a.openChat(conv.ID)
	}
	title := conversationTitle(conv)
	row.Children(func() {
		ui.Icon(c, icoChat).Size(15, 15).TextColor(t.TextMuted)
		label := ui.Text(c, title).FontSize(13).TextColor(t.Text).SingleLine().Grow(1)
		if selected {
			label.FontWeight(500)
		}
		if !selected {
			if when := shortTime(conv.UpdatedAt); when != "" {
				ui.Text(c, when).FontSize(11).TextColor(t.TextMuted).SingleLine()
			}
		}
	})
	row.Label(title)
}

// conversationTitle is how the sidebar names a conversation: its saved title,
// else the first thing the writer asked, else a fresh chat's name.
func conversationTitle(conv *appdata.Conversation) string {
	if conv == nil {
		return "New chat"
	}
	if title := strings.TrimSpace(conv.Title); title != "" {
		return title
	}
	for _, msg := range conv.Messages {
		if msg.Role != appdata.RoleUser {
			continue
		}
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			continue
		}
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = strings.TrimSpace(text[:i])
		}
		return truncate(text, 48)
	}
	return "New chat"
}

// sidebarFooter is the way into settings, with the shortcut the View menu also
// shows.
func (a *app) sidebarFooter(c *ui.Context) {
	t := c.Theme()
	active := a.mode == viewSettings
	ui.Column(c).FillWidth().Padding(8, 8).Gap(4).
		BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		row := ui.Row(c).FillWidth().Height(30).Padding(0, 8).Radius(tokens.RadiusSM).Gap(8).
			AlignItems(ui.Center).Focusable()
		switch {
		case active:
			row.Background(selectedFace(t))
		case c.Preferences().Hover() && row.Hovered():
			row.Background(t.SurfaceHover)
		}
		if row.Clicked() {
			a.showSettings()
		}
		row.Children(func() {
			ui.Icon(c, icoSettings).Size(15, 15).TextColor(t.TextMuted)
			label := ui.Text(c, "Settings").FontSize(13).TextColor(t.Text).SingleLine().Grow(1)
			if active {
				label.FontWeight(500)
			}
			keycap(c, "⌘,")
		})
		row.Label("Settings")
	})
}

// sidebarDialogs draws the rename and delete modals when they are open.
func (a *app) sidebarDialogs(c *ui.Context, st *sidebarState) {
	a.renamePageDialog(c, st)
	a.deletePageDialog(c, st)
}

// renamePageDialog edits a page's title in a modal, prefilled with the name the
// sidebar shows.
func (a *app) renamePageDialog(c *ui.Context, st *sidebarState) {
	if !st.renameOpen {
		return
	}
	t := c.Theme()
	ui.Modal(c, &st.renameOpen, func() {
		ui.Text(c, "Rename page").FontSize(13.5).FontWeight(600).TextColor(t.Text)
		// The field fits the window: a narrow window's dialog is narrower
		// than the reading measure.
		width, _ := c.Size()
		fieldWidth := min(float32(360), width-80)
		if fieldWidth < 160 {
			fieldWidth = 160
		}
		field := ui.TextInput(c.Key("rename-title"), &st.renameText).Width(fieldWidth).
			AutoFocus().Label("Page title").Placeholder("Page title")
		field.OnTextKey(func(k ui.TextKey) bool {
			switch {
			case k.Key == ui.KeyEnter && k.Mods == 0:
				a.commitRename(st)
				return true
			case k.Key == ui.KeyEscape:
				st.renameOpen = false
				return true
			}
			return false
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spacer(c)
			if ui.Button(c, "Cancel").Key("rename-cancel").Clicked() {
				st.renameOpen = false
			}
			save := ui.PrimaryButton(c, "Save").Key("rename-save").
				Disabled(strings.TrimSpace(st.renameText) == "")
			if save.Clicked() {
				a.commitRename(st)
			}
		})
	})
}

// commitRename renames the page the modal was opened for and closes it.
func (a *app) commitRename(st *sidebarState) {
	title := strings.TrimSpace(st.renameText)
	rel := st.renameFor
	st.renameOpen = false
	st.renameFor = ""
	if title == "" || rel == "" {
		return
	}
	a.renamePage(rel, title)
}

// deletePageDialog asks before a page leaves the workspace.
func (a *app) deletePageDialog(c *ui.Context, st *sidebarState) {
	if !st.deleteOpen {
		return
	}
	t := c.Theme()
	ui.Modal(c, &st.deleteOpen, func() {
		ui.Text(c, "Delete page?").FontSize(13.5).FontWeight(600).TextColor(t.Text)
		ui.Text(c, "“"+st.deleteName+"” will be deleted. This cannot be undone.").
			FontSize(12.5).TextColor(t.TextMuted).MaxWidth(340).LineHeight(1.5)
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spacer(c)
			if ui.Button(c, "Cancel").Key("delete-cancel").Clicked() {
				st.deleteOpen = false
			}
			remove := ui.Button(c, "Delete").Key("delete-confirm").TextColor(t.Danger).
				Background(t.Danger.Alpha(0.10))
			if remove.Clicked() {
				rel := st.deleteFor
				st.deleteOpen = false
				st.deleteFor = ""
				if rel != "" {
					a.deletePage(rel)
				}
			}
		})
	})
}
