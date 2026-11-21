//go:build e2e

// This file opts Bub Desktop into MyGo's automation plugin, in builds made
// with the e2e tag only, so an end-to-end test runner or an agent can drive a
// running window: read the native UI, press and type, choose menu items, and
// capture it as a PNG.
//
//	cd desktop
//	go build -tags e2e -o build/bub-desktop .
//	MYGO_AUTOMATION=/tmp/bub-desktop.sock ./build/bub-desktop
//	mygo-automation open --socket /tmp/bub-desktop.sock
//	mygo-automation snapshot -i --full
//	mygo-automation screenshot shot.png
//
// The plugin listens only when MYGO_AUTOMATION names a socket path, and it is
// left out of release builds. The elements worth driving carry stable keys:
//
//	editor, block-plus, block-drag, drop-<n>
//	slash-query, slash-<n>
//	ai-accept, ai-dismiss, ai-regenerate, ai-stop, ai-instruction
//	outline-toggle, outline-<blockID>
//	chat-input, chat-send, chat-stop, chat-new, chat-history,
//	chat-suggest-<n>, chat-insert-<n>, chat-copy-<n>, chat-regen-<n>,
//	chat-<conversationID>, composer-menu, composer-item-<n>
//	chat-back, chat-page-new, chat-page-suggest-<n>, chat-add-context,
//	chat-ref-chip-<n>, chat-ref-remove-<n>, chat-att-chip-<n>, chat-picture-<n>,
//	chat-config-<id>
//	ask-bub-fab, sidebar-toggle, sidebar-search, pages-new, page-<rel>,
//	folder-<rel>, page-actions, page-menu-chat, page-menu-rename,
//	page-menu-delete, rename-title,
//	rename-cancel, rename-save, delete-cancel, delete-confirm,
//	chats-new, settings-back, workspace-choose, theme,
//	editor-width-minus, editor-width-plus, editor-size-minus, editor-size-plus,
//	ai-neighbors, ai-outline, agent-command, autosave, agent-reconnect
//
// Actions live in the menu bar. The automation "menu" method takes a label
// path (the application menu is "Bub Desktop", the AI menu is "Bub"):
//
//	{"method":"menu","params":{"window":1,"path":["File","New Page"]}}
//	{"method":"menu","params":{"window":1,"path":["Bub","Rewrite Block"]}}
//	{"method":"menu","params":{"window":1,"path":["View","Settings…"]}}
//
// Menu labels: File: New Page, New AI Chat, Save, Close; View: Toggle Sidebar,
// Toggle AI Chat, Toggle Contents, Settings…; Bub: Rewrite Block, Shorten
// Block, Expand Block, Continue Block, Translate Block, Ask Bub…, Chat with
// Bub, Reconnect to Bub.
package main

import (
	// Registers the automation plugin with the MyGo app.
	_ "github.com/egoist/mygo/plugins/automation"
)
