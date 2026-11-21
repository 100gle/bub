# Bub Desktop — design notes

Status: the MVP described here is built in `desktop/`. [EDITOR_PLAN.md](EDITOR_PLAN.md)
is the earlier proposal that started it (standby bar, voice and multi-window are
still future work); the mocks in `design/` are from that proposal.

## The idea

A page is a document, and the agent belongs to the block you are writing, not to
a transcript. Two surfaces, one model:

- **Block AI** at the caret: the smallest possible request — rewrite, shorten,
  expand, continue, translate, or ask — answered into a popover, applied only
  when the writer accepts.
- **A chat rail** for everything that is not one block: "how does this argument
  read", "what should follow this page". The rail reads the pages and files the
  writer attaches to the conversation, and its answers can be inserted.

The window stays quiet: no formatting toolbar, no status bar, no chat by
default. Menus carry the actions, the sidebar carries the workspace, and the
rail is opt-in.

## Window anatomy

```
sidebar 240 DIPs (resizable) │ page, centered at 720 DIPs │ rail 288 DIPs
```

- **Sidebar** — surface color, hairline-free against the canvas. A search
  field (`⌘K`) over the pages tree (folders + pages) and Chats (saved conversations),
  with nothing else in the body: no empty-state paragraph and no second New page
  button, since the section header already carries it. Footer with Settings and
  its `⌘,` shortcut. A query filters both lists, keeps folder rows only for
  matches, and opens folders while filtering without changing their saved
  state.
- **Header** — the sidebar toggle, the workspace breadcrumb, a save state only
  while a page is being written or holds unsaved edits, and the Contents button
  while the rail is closed. The only chrome above the page.
- **Writing column** — `EditorWidth` from settings (720 default) plus at least
  a 56 DIP gutter on each side; the gutter holds the block handle and the `/`
  button. The editor fills the main area and centers the measure inside itself,
  so the page's scroll bar sits at the window's edge rather than in the middle
  of the content.
- **Ask Bub button** — a floating pill in the page's bottom-right corner. It
  opens the chat rail and is gone while the rail is open, so the rail's tabs are
  the only switch then and the window never shows two controls for one action.
- **Rail** — `Chat` or `Contents`, switched in its tabs; closes with the close
  button. Hidden below 1040 DIPs so the page keeps its measure. The chat rail is
  the quick companion while writing; the same conversation opens full-size from
  the sidebar.
- **Chat page** — a saved conversation opens from the sidebar into the main
  area at a 680 DIP measure. The writer's turns sit as quiet bubbles at the end
  of the measure, answers are Markdown at page size, and the composer grows
  with the draft: Enter sends, Shift+Enter keeps a line, the draft survives a
  switch to the rail, and a streaming answer offers Stop. The composer is one
  rounded card whose field owns a row and whose toolbar owns another — a plus
  attaches files, `@` mentions a workspace page and carries it along as an ACP
  resource link, `/` runs a command or expands a skill, and the toolbar's
  round Send and the session options the agent advertises — its model, its
  thinking level, and whatever else it names — share one centered line. Answers
  are actions a chat has: Copy, Regenerate; only where the page sits beside
  the conversation (the rail) can an answer also enter the page. This is ACP's
  session config options extension: `session/new` carries the options, and
  `session/set_config_option` applies a choice to that session's later turns.
  An agent that does not implement the extension simply shows no selectors.
  The commands and skills `/` lists come from the agent's
  `available_commands_update`; an agent that advertises none shows a quiet
  note instead. The connection itself belongs
  to Settings, so the composer says nothing about it. `⌘J` still opens the rail;
  the sidebar and the rail show the same conversation.
- **Settings** — one centered column of inset groups. A group is its muted
  caption over a hairline-bordered card on the page's own color — grouping
  without the gray fill that turned the page into a stack of tinted boxes;
  clear space separates the groups, rows are separated by hairlines inset from
  the card's edge, and each row is its name, its description under the name,
  and the control right-aligned in a fixed measure. A row that needs a long
  field (the agent command) takes the card's full width under its label.

Visual language: SF Pro, the system font — the theme leaves the family empty so
Core Text resolves it with its optical sizes and tracking — at Apple's 13 DIP
control size. System light/dark themes with Bub's sky-blue accent (`#0077b6`
light, `#38bdf8` dark). Every geometric decision comes from `tokens.go`: a radius
scale (6, 8, 12, 16, 20 DIP, and a pill) instead of one-off corners, a 4 DIP
spacing unit, regular 12 px group captions and 12–13 px chrome type, 16 px page
type at 1.6 line height, and 1.8-stroke icons. Weights stay regular for rows and
labels, medium for the chosen row, and semibold only for titles; nothing is bold
for emphasis. Hairlines carry structure, and floating surfaces get one soft
shadow and the canvas color rather than a gray fill. The chosen row or tab takes
a 10% accent tint (18% in dark), so color and weight carry the selection
together, as a macOS sidebar's does. Shortcuts render as keycap badges: one per
key in the settings legend, one compact badge in the sidebar footer. Every
custom control is `Focusable` and keeps the theme's focus ring.

Copy is chrome too. A label names the control beside it, and a dialog says only
what the action does; nothing repeats what the layout already shows. A saved
page has no "Saved" badge, a conversation needs no "YOU" over the writer's own
bubble, the Contents rail needs no "On this page" heading, the settings page
needs no subtitle under the title, and a preference that reads clearly from its
control gets no description sentence.

## The block model

`richtext` paragraphs are the blocks; the app adds chrome, not a text engine.

- `blocks.List` reads the paragraphs into blocks with rune offsets and stable IDs
  from `Document.IdentifyParagraphs`.
- **`/` menu**: when a paragraph begins with `/`, the typed query becomes the
  menu's filter. Choosing a type calls `blocks.Apply`, which restyles the
  paragraph and removes the `/query` in the same undo step. The menu also opens
  from the gutter `+`.
- **Fence languages** are not part of `richtext`'s paragraph style: the app
  reads them from the Markdown on open, keys them by the first paragraph's ID,
  writes them back on save, and feeds them to the editor's syntax highlighting.
- **Drag**: the gutter handle drags a `blockDrag` value; boundary zones accept it
  and call `blocks.Move`, which rebuilds the paragraph list and re-applies styles
  and IDs as one undo step. The area under the last block appends; boundaries
  inside a code block are omitted.
- **Contents** is `blocks.Outline`: level, text and rune offset per heading.
  MyGo has no scroll-to-paragraph, so choosing one sets the caret and the editor
  reveals it.
- **Context menu** is `EditorOptions.Menu` (`blockContextMenu` in
  `context_menu.go`): the system's Cut, Copy, Paste and Select All, then a **Bub**
  submenu of the block actions. The target is the paragraph holding the
  selection's start, so the agent operates on what the writer selected.

Offsets are rune offsets throughout. Block chrome lives in app state and never
enters the text.

## The AI contract

Both surfaces are one ACP `session/prompt`. The difference is the output
contract and the review surface.

**Block action**

- Input: instruction, page title, outline, target block, neighbours (configurable).
- Output contract: the replacement paragraph only — no fences, no quotes, no
  explanation. The client strips wrappers and refuses empty answers.
- Review: popover under the block with Accept / Dismiss / Regenerate; Accept is
  one `Document.Edit` by paragraph ID and refuses a stale target. One action at
  a time; a new one cancels and queues.

**Chat**

- Context: the pages the conversation references and the files attached to the
  turn, both shown as removable chips above the composer field. A new chat
  references nothing; the open page becomes context only when the writer
  attaches it (composer's plus, a page's **Chat about this page**, or `@`).
- Input: instruction; every referenced page as an ACP `resource_link` block,
  and every attached file either as an inline `image` block (base64, when the
  agent advertises the image capability) or, for any other file, as a
  `resource_link` block; for the referenced page that is open, its title, up to
  12 headings, caret block and neighbours when the settings allow; and a
  system line asking for concise Markdown. Pictures attached to the turn
  render in the writer's bubble and open in the system viewer on click.
- Output: Markdown, rendered by `ui/markdown` and streamed with `Stream`.
  Answers use `markdownLook` in the rail and `pageMarkdownLook` on the chat page
  (`theme.go`): body size and heading ramp from the tokens, and code blocks as
  bordered panels at `RadiusMD` with the fence's language, a Copy button and
  syntax highlighting. The page body is not Markdown rendering but a `richtext`
  editor, whose code blocks get the same neutral panel, radius and hairline
  frame from `editorLook` and `codeBlockFrames`, and syntax highlighting from
  `EditorOptions.Spans`. `richtext` keeps no fence language, so the app
  remembers each block's language beside the document (keyed by the block's
  first paragraph ID): `setMarkdown` reads it, `markdownText` writes it back on
  save, and `codeSpans` tokenizes the block with `editor.LanguageNamed`. A code
  block typed fresh in the editor has no language until a saved fence names
  one.
- Review: answers are messages with a chat's actions — Copy, and Regenerate
  for the turn that just answered. Only in the rail, where the page sits
  beside the conversation, does an answer also offer Insert: one
  `Document.Insert` + one undo step. A page holds any number of conversations
  and a conversation may reference several pages; one ACP session per
  conversation (so context is isolated), one turn at a time with a pending
  queue.

Context is deliberately small: a block action never sends the whole page unless
the settings say to include the outline. The full page is never sent implicitly,
and a chat never inherits the page that merely happens to be open. The **Bub**
menu, the context menu and the `⇧⌘R` path all build the same action;
the target is the caret's block, or the paragraph a right-click selected.

## Persistence

- **Pages**: a folder of Markdown files (`~/Bub` by default), folders for
  nesting. No database, no sidecar. The title is the first heading; renaming
  renames the file and the heading together.
- **Settings**: `os.UserConfigDir()/Bub Desktop/settings.json`; written on every
  change (theme, width, font, context, agent command, autosave).
- **Chats**: one JSON file per conversation under `chats/`, saved after each
  turn; the sidebar list is sorted by `updatedAt`.
- Writes are atomic (temp + rename) and paths are resolved with `os.Root` so a
  page cannot escape the workspace.

## ACP and processes

The agent is a separate `bub acp` process. The app spawns it where it was
launched (so `uv run bub acp` resolves the checkout's environment) and roots the
session at the page workspace (so the agent sees the same Markdown files).
`session/new` is called once for block actions and once per chat conversation;
it is prefetched when a chat opens so the composer can show the agent's model
and thinking selectors, and `session/set_config_option` applies a choice to that
session's later turns. Notifications are routed by session id: block chunks go
to the suggestion, chat chunks to the conversation. `session/cancel` stops one
turn.

## Decisions and trade-offs

- **Markdown over a custom document format.** Git-friendly and portable, at the
  cost of losing rich formatting on save; block IDs are session-only.
- **`richtext` over a custom block engine.** Undo, Markdown shortcuts, IME,
  accessibility and code formatting come for free. The cost is that a code block
  is several paragraphs; the app works at block level and anchors UI to
  paragraph boxes, and the context menu replaces the editor's own menu to add
  the Bub submenu.
- **A pinned MyGo commit.** The module pins
  `v0.8.4-0.20261110073650-5ae20dc8a990`, which adds
  `EditorOptions.Menu` (the context menu above), `Document.CaretRect` and
  `EditorOptions.OnKey`. Being a commit rather than a release is the cost of
  those hooks; the app still builds against it alone.
- **One window, one page.** Simpler state and fewer failure modes for the MVP;
  multi-window and session restore are future work.
- **A rail rather than a chat window.** It keeps the page and the conversation
  visible together; it stays out of the way when closed and remembers its tab.
- **Cyan accent only for selection, AI and focus.** The chrome is neutral, so
  AI surfaces read as the one active thing in the window.
- **The floating Ask Bub button gives way to the rail.** One entry point at a
  time: the button closes when the rail opens, and the rail's tabs take over.

## Open questions

- A faster model path for Continue and a diff for suggestions (word-level, not
  just the rewritten text).
- Multi-block selection as a target for block AI.
- Page search, backlinks and a graph.
- Drag-and-drop of pages into folders (currently the folders come from disk).
- Session restore and a document-per-window mode.
- A caret-anchored block menu and ghost completion, now that the pinned MyGo has
  `Document.CaretRect` and `EditorOptions.OnKey`.
