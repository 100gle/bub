# Bub Desktop

A native, Notion-like writing app for [Bub](../../README.md): pages are Markdown
blocks, the agent works on the block at the caret, and a quiet AI chat rail sits
beside the page for everything that does not fit a one-block rewrite. Written in
Go with the [MyGo](https://mygo.egoist.dev) UI toolkit and connected to a Bub
agent through the [Agent Client Protocol](https://agentclientprotocol.com)
(ACP) v1 over stdio.

![Bub Desktop: a chat page with the pages and files it reads as context chips above the composer, the conversation's Copy and Regenerate actions under an answer, and the round Send](screenshot.png)

## What is in the box

- **A page sidebar.** A search field (`⌘K`) over the workspace's Markdown pages, nested
  in folders, and the saved chats; a way into settings sits in the footer. Pages
  are created, renamed and deleted in place, and every page is a plain `.md` file.
  The search filters pages and chats together and opens folders while filtering.
- **A block editor.** Headings, paragraphs, lists, tasks, quotes and code blocks
  from `ui/richtext`, with Markdown shortcuts (`# `, `- `, `> `, `**bold**`, …),
  a `/` menu for block types, a gutter handle that drags a block to a new place,
  and a contents outline of the page's headings. A fenced block keeps its
  language through load and save and is syntax-highlighted in the editor.
- **A context menu on the block.** Right-click a selection: Cut, Copy, Paste and
  Select All, then a **Bub** submenu with the block actions on the selected
  paragraph. Control-click opens it on a Mac trackpad.
- **Block AI.** Rewrite, shorten, expand, continue, translate, or ask in your own
  words about the caret's block (`⇧⌘R`, `⇧⌘K`, the **Bub** menu, and the context
  menu). The agent's answer streams into a small popover; nothing enters the
  page, or its undo history, until you accept it.
- **A chat page and a chat rail.** A saved conversation opens from the sidebar
  into a full chat page: a page-scale transcript, right-aligned turns, Markdown
  answers, and a rounded composer with a plus to attach context, `@` to mention a
  page, `/` to run a command or expand a skill, and the session's model and
  thinking selectors when the agent advertises them.
  The rail remains the quick companion while writing, opened from the floating
  **Ask Bub** button (`⌘J`); both show the same conversation. A chat reads the
  pages and files the writer attaches to it, shown as chips above the composer:
  a new chat starts with no context, a page holds any number of chats, and a
  chat may read several pages. A referenced page that is open also sends its
  title, outline, caret block and neighbours. Answers are
  Markdown — including code blocks with their language, a Copy button and
  highlighting — and may be copied or regenerated; from the rail one can also
  be inserted into the page, which the conversation sits beside. Conversations
  are saved, with their references, and listed in the sidebar.
- **Settings.** Grouped preferences — workspace, appearance, editor, AI,
  shortcuts and about — as inset groups on the page color: a muted caption, a
  hairline-bordered card with hairlines between rows, and each row's control
  right-aligned. The card carries the grouping without a gray fill. The
  shortcuts legend shows each chord as keycap badges. Preferences persist in
  `settings.json` in the app's configuration directory.

## Running it

For development, use `mygo dev`, which watches the Go sources and rebuilds and
relaunches the app on every change, so there is no need to kill and restart it
by hand:

```sh
cd desktop
go tool mygo dev -sign -
```

`-sign -` signs the development app ad hoc. Without it, `mygo dev` signs with an
Apple Development identity, which keeps Accessibility, Screen Recording and
Keychain grants across rebuilds; that is the better default on a machine whose
identity works, but this one has a revoked Apple Development certificate next to
the valid one, and a bundle signed with the identity it picks dies on launch.
Signing ad hoc avoids that at the cost of those grants.

`mygo dev` restarts the app rather than preserving its state: the draft in the
composer is lost the way it is with any Go program. It runs a separate app,
"Bub Desktop Dev" at `.mygo/dev`, with its own data directory, so its window
size and settings start fresh once and do not disturb a `go run .` build. A
change that fails to compile leaves the running app alone and the next change
tries again. In the terminal, `r` rebuilds and restarts, and `q` quits.

To run a single build without watching:

```sh
cd desktop
go run .
```

The `bub acp` command comes from the [`bub-acp-server`](https://pypi.org/project/bub-acp-server/)
plugin, not from Bub itself. Install it once into the environment the app
launches:

```sh
bub install bub-acp-server@main
```

No environment is needed beyond that in a Bub checkout: if `BUB_ACP_CMD` is
unset, the app runs `bub acp` when `bub` is on `PATH`, and otherwise, when `uv`
is on `PATH` and the working directory (or a parent) holds `pyproject.toml`,
runs `uv run bub acp` from that checkout. The ACP session is rooted at the
workspace the app writes pages to, so the agent can read the same files.

The first run creates a workspace at `~/Bub` with a welcome page. To use another
folder, choose it in **Settings → Workspace**; or set the agent command and
workspace in the environment:

```sh
cd desktop
BUB_ACP_CMD="bub acp" go run .
BUB_ACP_CMD="uv run bub acp" BUB_ACP_WORKSPACE="$(cd .. && pwd)" go run .
```

Packaged builds use MyGo's own tools, which read `mygo.json`:

```sh
cd desktop
go tool mygo build   # bundle in build/
go tool mygo dev     # rebuild and relaunch on change
```

`go.mod` lists `cmd/mygo` as a tool, so `go tool mygo` runs the pinned version.

Go is not necessarily on `PATH` in a non-interactive shell; on a Homebrew setup
`export PATH=/usr/local/opt/go/bin:$PATH` first.

### Environment

| Variable | Meaning | Default |
| --- | --- | --- |
| `BUB_ACP_CMD` | The agent command line. Quotes group arguments. | resolved: `bub acp`, else `uv run bub acp` in a checkout, else `bub acp` |
| `BUB_ACP_WORKSPACE` | The agent's working directory and session root, when set. | the workspace folder from settings |
| `BUB_ACP_DOTENV` | The `.env` whose values the agent inherits. | the nearest `.env` above the workspace or the app's directory |
| `BUB_MODEL` | Read by the agent; the window itself shows no model name. | unset |
| `MYGO_AUTOMATION` | Socket path for the automation plugin, in `-tags e2e` builds only. | unset |

Settings override `BUB_ACP_WORKSPACE`: the agent process still starts where the
app was launched, so a checkout's `uv run bub acp` finds its project, while the
ACP session is rooted at the page workspace.

Because the agent runs in the workspace, it cannot find a checkout's `.env` on
its own and would fall back to a credential-less default model, which fails
every prompt with an authentication error. The app therefore passes the values
of the nearest `.env` (or `BUB_ACP_DOTENV`) to the agent, so a checkout's model
settings reach the session with no extra configuration.

## Keyboard

| Key | Action |
| --- | --- |
| `⌘N` | New page |
| `⇧⌘N` | New AI chat |
| `⌘S` | Save |
| `⌘K` | Search pages and chats |
| `⇧⌘R` | Rewrite block |
| `⇧⌘K` | Ask Bub about the block |
| `⌘J` | AI chat rail |
| `⇧⌘O` | Contents rail |
| `⌘\` | Sidebar |
| `⌘,` | Settings |
| `/` | Block menu, in an empty block |
| `@` | Mention a page to reference, in a chat composer |
| `/` | Commands and skills, in a chat composer |
| `⌘↩` | Accept a suggestion |
| `Esc` | Dismiss a suggestion, menu or popover |

`⌘` is the platform's command modifier (Ctrl on Linux and Windows).

## Layout

```
┌─────────────┬────────────────────────────────────────────┬─────────────┐
│ sidebar     │ breadcrumb · contents                      │ rail        │
│             │                                            │ Chat /      │
│ Search      │       ┌──────────────────────┐             │ Contents    │
│ Pages       │       │  Welcome             │             │             │
│  ├ folder   │       │  text, lists, code…  │             │ messages    │
│  └ page     │       └──────────────────────┘             │             │
│ Chats       │                                            │             │
│ Settings ⌘, │                         (Ask Bub)          │ composer    │
└─────────────┴────────────────────────────────────────────┴─────────────┘
```

The page measure is centered in the main area while the editor fills it, so the
page's scroll bar sits at the window edge. The sidebar is resizable; the rail is
fixed at 288 DIPs and hides itself in windows narrower than 1040 DIPs. It opens on Contents by default; close it from
its header or with `⇧⌘O`, and open it on Chat from the floating **Ask Bub**
button or with `⌘J`. The window uses SF Pro, the macOS system font, at Apple's
13 DIP control size; section captions are sentence case at regular weight and
only titles are semibold. Radii, spacing, type sizes and the light/dark palette
come from `tokens.go`, so a taste change is one edit rather than a hunt through
the views.

## Blocks

`ui/richtext` keeps the text as paragraphs; the app treats each paragraph as a
block with a stable ID (`Document.IdentifyParagraphs`). The app adds chrome, not
a new text model:

- The gutter beside the caret's or pointer's block holds `+` (open the block
  menu) and `⠿` (drag the block). Both are auto-hidden until the block is under
  the pointer or the caret is in it.
- The `/` menu appears when a block begins with `/` and takes the typed text out
  with the chosen style in one undo step. It also opens from the gutter `+`.
- Dragging shows a line at each boundary it can drop on; choosing one moves the
  block, preserving its style, inline formatting and ID, as one undo step. The
  area below the last block appends, and the app never drops between the lines
  of one code block.
- Contents lists the page's headings; choosing one moves the caret there.
- Right-clicking the text opens the system's Cut, Copy, Paste and Select All,
  then a **Bub** submenu of the block actions. The target is the paragraph
  holding the selection's start, so a selected paragraph is what the agent
  rewrites.
- Blocks are Markdown paragraphs, so a code block is several paragraphs with the
  code style (as in `richtext`); its lines stay one visual block.

## AI

**Block actions.** One `session/prompt` per action on the caret's block, or on
the paragraph a right-click selected. The prompt carries the page title, the
page's headings, the target block, and its neighbours (both configurable in
Settings). It asks for replacement text only;
the answer is trimmed of wrapping fences and quotes, streamed into a popover,
and one `session/cancel` stops it. **Accept** applies one `Document.Edit` — one
undo step, keeping the block's style and ID — and refuses a target that changed
under it. One action runs at a time; a new one cancels and queues behind the
last.

**Chat.** A saved conversation opens from the sidebar into the full chat page;
the floating **Ask Bub** button in the page's corner opens the rail instead
(`⌘J`), and gives way to the rail's own Chat and Contents tabs once it is open,
so no two controls say the same thing. Both surfaces show the same conversation
and its draft, with its own ACP session so context does not bleed between chats.
A chat reads only what the writer attaches to it, and the pages and files the
composer carries sit as chips above the field. A new chat starts empty; a page
holds any number of chats, and one chat may read several pages — the page row's
**Chat about this page** starts one with that page attached, and the composer's
plus attaches the open page or files. Every turn sends the referenced pages and
attached files — pictures as inline image blocks the agent can see, any other
file as an ACP `resource_link` block — and a sent picture renders in the
transcript, ready to be opened; the referenced page that is
currently open also contributes its title, up to 12 headings, the caret block
and its neighbours. The turn then asks for a concise Markdown answer. The
composer is one rounded card: the context chips, a growing text area whose
Enter sends and Shift+Enter keeps a line, a toolbar whose plus gives the chat
context and whose round Send or Stop ends the row, `@` to mention a workspace
page (which then rides along as a resource link) and `/` to run a
command or expand a skill the agent advertises through ACP's
`available_commands_update`. It also shows the session options the
agent advertises — its model, its thinking level, and any other
select option — which `session/set_config_option` applies to the conversation's
later turns. An agent that does not implement config options shows none; one
that advertises no commands shows a quiet note in the `/` palette. Answers render through `ui/markdown` with
streaming and carry a chat's actions — **Copy**, and **Regenerate** for the
latest answer; in the rail, where the page sits beside the conversation, an
answer may also be **Insert**ed into the page at the caret as one undo
step. One turn runs at a time and a message
sent meanwhile is queued; the conversation, with the pages it references, is
saved after each turn and listed in the sidebar.

Tool calls, thoughts and plans have no UI: this is an editor, not a console.
Failures show in the surface that failed — a native message with Retry for a
block action, an inline note with Retry for a chat turn.

## Data and files

```
~/Bub/                         the workspace (Settings → Workspace)
  Welcome.md                   a page: plain Markdown, title is its first heading
  essays/
    The Quiet Web.md
~/Library/Application Support/Bub Desktop/   (os.UserConfigDir()/Bub Desktop)
  settings.json                preferences
  chats/<id>.json              saved AI conversations
```

`settings.json` is written as preferences change. Pages are ordinary Markdown
files written atomically; there is no sidecar metadata. The page title is its
first heading, falling back to the file name. Renaming a page renames the file
and its heading; deleting removes the file or folder.

## Architecture

```
┌──────────────────────────────────┐    ACP v1, JSON-RPC 2.0, one per line   ┌───────────────────────────┐
│  Bub Desktop (Go, MyGo)          │  ── initialize, session/new ─────────► │  bub acp (Python)         │
│  native window                   │  ── session/set_config_option ───────► │  per-session model/think  │
│  pages, blocks, chat, settings   │  ── session/prompt (block or chat) ───► │  BubFramework turns       │
│  pages, blocks, chat, settings   │  ◄─ session/update: message chunks ──── │  model, tools, tape       │
│  internal/acp adapter            │  ◄─ session/prompt response ─────────── │                           │
└──────────────────────────────────┘  ── session/cancel ───────────────────► └───────────────────────────┘
        client                                                                agent
```

| Path | What it is |
| --- | --- |
| `main.go` | The app, window and agent wiring (`connect`, handshake, reconnect, notification routing, native error dialogs). |
| `app.go` | App state: workspace, pages, settings, autosave, connection, close prompt. |
| `view.go` | The shell: sidebar split, header, writing surface, floating Ask Bub button, rail, empty state. |
| `editor_ui.go` | Block chrome: gutter, `/` menu, drag and drop, contents, suggestion popover. |
| `context_menu.go` | The editor's right-click menu: edit items and the Bub submenu on the selected block. |
| `editor.go` | The block action engine: prompt building, suggestion streaming/accept/stale checks. |
| `sidebar.go` | Pages tree, chat list, search, footer, page menu, rename/delete dialogs. |
| `settings_view.go` | The settings page. |
| `tokens.go` | The design tokens: light/dark palette, radius scale, spacing, type sizes and weights. |
| `code_language.go` | Fence-language memory for the editor, Markdown save round-trip, and code syntax highlighting. |
| `theme.go` | Applies the tokens to the window and derives the editor and Markdown looks. |
| `ai_panel.go` | The chat rail and chat page: turns, streaming, saved conversations, insertion, the composer. |
| `ai_prompt.go` | The chat prompt and the context it reads. |
| `menu.go` | File, View, Bub menus and their actions. |
| `theme.go`, `icons.go` | Bub's theme and the Lucide-style icon set. |
| `internal/workspace/` | Markdown pages on disk: tree, read/write, create, rename, delete, slugs. |
| `internal/appdata/` | `settings.json` and saved conversations. |
| `internal/blocks/` | The block engine: blocks, slash detection, block types, moves, outline. |
| `internal/acp/` | The ACP adapter over the Go SDK: sessions, prompts, session config options, resource links, and the agent-command resolution. |

## Tests

```sh
cd desktop
go build ./...
go build -tags e2e ./...
go test ./...
go vet ./...
```

`internal/acp` tests need no process and no network: a fake agent answers over an
SDK loopback connection, a spawned `testdata/agent` covers the real process path,
and command resolution is tested with fake `bub`/`uv` executables and temporary
workspaces. `internal/workspace`, `internal/appdata` and `internal/blocks` test
the disk and text engines with temporary folders. The `main` tests drive the
action engine, block chrome (slash menu, drag, contents), page lifecycle,
settings, chat turns and the views with `ui.NewTester`; they open no window, show
no dialog and start no agent.

## Driving a running window

Builds made with `-tags e2e` add MyGo's automation plugin:

```sh
cd desktop
go build -tags e2e -o build/bub-desktop .
MYGO_AUTOMATION=/tmp/bub-desktop.sock ./build/bub-desktop &
mygo-automation open --socket /tmp/bub-desktop.sock
mygo-automation snapshot -i --full
mygo-automation screenshot shot.png
```

The elements worth driving carry stable keys:

| Key | Element |
| --- | --- |
| `editor` | The page editor. |
| `block-plus`, `block-drag` | The gutter's add and drag controls. |
| `slash-query`, `slash-<n>` | The block menu's filter and its rows. |
| `ai-accept`, `ai-dismiss`, `ai-regenerate`, `ai-stop` | The suggestion popover. |
| `ai-instruction` | The Ask Bub… input. |
| `outline-toggle`, `outline-<block>` | The header's contents button and outline rows. |
| `chat-input`, `chat-send`, `chat-stop`, `chat-new`, `chat-history` | The chat rail and the chat page composer. |
| `chat-back`, `chat-page-new`, `chat-page-suggest-<n>`, `chat-add-context`, `chat-config-<id>` | The chat page and its composer. |
| `ask-bub-fab` | The floating Ask Bub button in the page's corner. |
| `chat-suggest-<n>`, `chat-insert-<n>`, `chat-copy-<n>`, `chat-ref-chip-<n>`, `chat-att-chip-<n>`, `chat-picture-<n>` | Chat empty-state ideas, answer actions, the context chips above the composer, and sent pictures. |
| `sidebar-toggle`, `sidebar-search`, `pages-new`, `page-<rel>`, `folder-<rel>` | The sidebar and its search. |
| `page-actions`, `page-menu-chat`, `page-menu-rename`, `page-menu-delete`, `rename-title`, `delete-confirm` | Page chat, rename and delete. |
| `chats-new`, `chat-<id>` | The chat list. |
| `settings-back`, `workspace-choose`, `theme`, `editor-width-minus`, `editor-width-plus`, `ai-neighbors`, `ai-outline`, `agent-command`, `autosave`, `agent-reconnect` | Settings controls. |

Actions live in the menu bar; the automation `menu` method takes a label path:

```json
{"method":"menu","params":{"window":1,"path":["File","New Page"]}}
{"method":"menu","params":{"window":1,"path":["Bub","Rewrite Block"]}}
{"method":"menu","params":{"window":1,"path":["View","Settings…"]}}
```

Menu labels: **File** — New Page, New AI Chat, Save, Close; **View** — Toggle
Sidebar, Toggle AI Chat, Toggle Contents, Settings…; **Bub** — Rewrite Block,
Shorten Block, Expand Block, Continue Block, Translate Block, Ask Bub…, Chat
with Bub, Reconnect to Bub. The application menu is titled **Bub Desktop**, so
the AI menu stays **Bub**.

## Limitations

- **One window.** No tabs, no session restore, no crash recovery. Autosave is on
  by default; closing and switching pages ask before discarding unsaved work.
- **Markdown only.** Save writes Markdown, so formatting it cannot carry
  (colors, fonts, underline, alignment) is lost on save; block IDs live for the
  session and are not stored in the file.
- **One block at a time for block AI.** A selection spanning several paragraphs
  is not a target; the whole caret block is rewritten. Chat can answer about the
  whole page, but insertion is at the caret.
- **Code blocks are lines.** `richtext` stores each code line as a paragraph with
  the code style, so the contents outline and the drag boundaries treat the
  block's lines as blocks; the app refuses drops inside one code block.
- **TOC moves the caret.** MyGo has no scroll-to-paragraph call, so choosing a
  heading selects it and the editor reveals the caret.
- **No permissions UI.** `session/request_permission` is answered with the
  "cancelled" outcome, so a tool that needs consent does not run.
- **Chat is one turn at a time.** A message sent during an answer is queued after
  a cancel; images and file attachments are not sent.
- **No diff view.** Suggestions show the new text, not a word-level diff.
- **The rail hides in narrow windows**, below 1040 DIPs.
