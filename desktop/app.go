package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/richtext"

	"github.com/bubbuild/bub/desktop/internal/acp"
	"github.com/bubbuild/bub/desktop/internal/appdata"
	"github.com/bubbuild/bub/desktop/internal/block"
	"github.com/bubbuild/bub/desktop/internal/workspace"
)

const (
	appTitle         = "Bub"
	appDisplayName   = "Bub Desktop"
	appVersion       = "0.2.0"
	clientName       = "bub-desktop"
	handshakeTimeout = 20 * time.Second
	windowWidth      = 1400
	windowHeight     = 880
	minWindowWidth   = 760
	minWindowHeight  = 520
	untitledName     = "Untitled"
	autosaveDelay    = 900 * time.Millisecond
)

// viewMode is what the window's main area shows.
type viewMode int

const (
	viewPage viewMode = iota
	viewSettings
	viewChat
)

// railTab is the panel open on the right of the page.
type railTab int

const (
	railChat railTab = iota
	railOutline
)

// app is the window's state: the workspace, the open page, the AI rail and the
// connection to `bub acp`. The view builds from it on the main thread; the
// agent's goroutine changes it only through dispatch.
type app struct {
	win *mygo.Window

	// The workspace on disk: pages, settings and the saved AI conversations.
	ws           *workspace.Workspace
	chats        *appdata.Chats
	settings     appdata.Settings
	settingsPath string
	pages        []*workspace.Node
	openFolders  map[string]bool
	chatList     []*appdata.Conversation
	current      string // relative path of the open page, "" for none
	mode         viewMode

	// Layout of the window's chrome.
	sidebarWidth float32
	sidebarShown bool
	railShown    bool
	railTab      railTab

	// The open page.
	doc    *richtext.Document
	path   string
	dirty  bool
	idSeq  atomic.Int64
	editor ui.Handle
	// editorPadLeft is where the page's text starts in the main area, and
	// editorTextWidth its measure, so the editor can fill the area and keep
	// its scroll bar at the window's edge.
	editorPadLeft   float32
	editorTextWidth float32
	// codeLangs is the fence language of each code block, keyed by the first
	// paragraph's ID, which richtext does not keep; codeLangsRev rises when
	// it changes, so the editor draws code again.
	codeLangs    map[string]string
	codeLangsRev int
	// codeMenu is the block whose language menu is open, "" when none is.
	codeMenu string
	// codeCopied is the block whose copy was the last one, so its button can
	// say so until the pointer leaves it.
	codeCopied string
	// codeHi is the code highlighting of the frame being built,
	// codeSpansKey what it was built for, and codeSpansVer the editor's
	// SpansVersion.
	codeHi       *codeHi
	codeSpansKey codeSpansKey
	codeSpansVer int
	// chatFocus asks the chat page's composer for the focus in the next frame.
	chatFocus bool
	// composer is the chat message field's handle: focus queries on it ignore
	// whether the window itself is key, so the palette opens as soon as the
	// field owns the app's focus.
	composer ui.Handle
	// editorArea is the writing column around the editor, gutter included,
	// so the block chrome stays under the pointer while it is clicked.
	editorArea ui.Handle
	// searchField is the sidebar's search handle, so ⌘K can reach it even
	// while the sidebar is hidden.
	searchField ui.Handle
	focusIn     bool // focus the editor in the next frame
	saveSeq     int
	saving      bool

	// Block chrome: the hovered block, the slash menu and a block drag.
	hoverBlock int
	slash      *slashState
	drag       *dragState

	// Block AI: at most one suggestion, one ask, one turn in flight.
	suggestion        *suggestionState
	ask               *askState
	pendingAction     *actionState
	retryAfterConnect *actionState
	aiGen             int

	// AI chat: the conversation in the rail and the ACP session it uses,
	// with the configuration options each session advertised.
	chat         *chatState
	chatSessions map[string]string
	chatConfigs  map[string][]acp.ConfigOption

	// pictures are attached image files decoded once, keyed by path, so the
	// composer thumbnails and the transcript previews do not reread them.
	pictures map[string]ui.ImageSource

	// commands are what the agent advertised for "/", and commandPrefix the
	// agent's own command leader, which the composer never shows.
	commands      []acp.AvailableCommand
	commandPrefix string

	// Connection to `bub acp`. gen rises whenever a connection is replaced, so
	// callbacks from an old one do nothing.
	proc       *acp.Proc
	client     *acp.Client
	gen        int
	connecting bool
	sessionID  string
	connectErr string
	// agentImages is what the connected agent advertised: whether inline image
	// prompt blocks are accepted (Bub advertises them).
	agentImages bool

	// Native close asks about unsaved changes; these guard the prompt and the
	// second, programmatic close.
	allowClose    bool
	closingPrompt bool
	closed        bool

	// uiReady is set once the window is created and the app is running, so
	// failures before that are logged instead of shown in a dialog.
	uiReady bool

	// Hooks tests use in place of native dialogs and window state.
	dialog        func(title, message string, retry func())
	chooseUnsaved func(then func(choice int))
	onEdited      func(bool)
	onPageOpened  func(rel string)
	onSaved       func()
	// codeCopyHook, when set, takes a copy instead of the clipboard.
	codeCopyHook func(string)
	// codeShowAll forces the code block tools out when true, as tests drive
	// them without a pointer.
	codeShowAll bool
}

// newApp makes the app's state without touching the disk, so tests can drive
// it without a workspace or a window.
func newApp() *app {
	a := &app{
		settings:     appdata.DefaultSettings(),
		openFolders:  map[string]bool{},
		sidebarWidth: 240,
		sidebarShown: true,
		railShown:    false,
		railTab:      railChat,
		hoverBlock:   -1,
		chatSessions: map[string]string{},
		chatConfigs:  map[string][]acp.ConfigOption{},
		pictures:     map[string]ui.ImageSource{},
		codeLangs:    map[string]string{},
	}
	a.doc = richtext.NewDocument(richtext.NewText(""))
	a.doc.IdentifyParagraphs(a.newParagraphID)
	return a
}

// start loads the settings and the workspace and opens a page. It runs on the
// main thread as the window appears.
func (a *app) start() {
	dir, err := appdata.Dir()
	if err != nil {
		log.Printf("bub-desktop: %v", err)
	}
	a.settingsPath = filepath.Join(dir, "settings.json")
	s, err := appdata.LoadSettings(a.settingsPath)
	if err != nil {
		log.Printf("bub-desktop: settings: %v", err)
	}
	a.settings = s
	applyThemeSource(a.settings.Theme)
	if a.settings.ShowOutline {
		a.railShown = true
		a.railTab = railOutline
	}
	a.openWorkspace(a.settings.Workspace)
	if chats, err := appdata.OpenChats(); err != nil {
		log.Printf("bub-desktop: chats: %v", err)
	} else {
		a.chats = chats
	}
	a.reloadPages()
	a.reloadChats()
	if a.current == "" {
		first := firstPage(a.pages)
		if first == "" {
			first = a.seedWorkspace()
		}
		if first != "" {
			a.openPage(first)
		}
	}
}

// openWorkspace switches the workspace folder, keeping the current page when
// it is in the new folder.
func (a *app) openWorkspace(root string) {
	if strings.TrimSpace(root) == "" {
		root, _ = workspace.DefaultRoot()
	}
	ws, err := workspace.Open(root)
	if err != nil {
		a.notifyError("Could not open the workspace", err.Error(), nil)
		return
	}
	a.ws = ws
	a.settings.Workspace = root
}

// reloadPages refreshes the sidebar tree from disk.
func (a *app) reloadPages() {
	if a.ws == nil {
		return
	}
	pages, err := a.ws.Tree()
	if err != nil {
		a.notifyError("Could not read the workspace", err.Error(), nil)
		return
	}
	a.pages = pages
}

// reloadChats refreshes the sidebar's saved AI conversations.
func (a *app) reloadChats() {
	if a.chats == nil {
		return
	}
	list, err := a.chats.List()
	if err != nil {
		log.Printf("bub-desktop: chats: %v", err)
		return
	}
	a.chatList = list
}

// firstPage is the first page of a tree, depth first.
func firstPage(nodes []*workspace.Node) string {
	for _, n := range nodes {
		if !n.IsDir {
			return n.Rel
		}
		if p := firstPage(n.Children); p != "" {
			return p
		}
	}
	return ""
}

// seedWorkspace writes a small welcome page into an empty workspace so the
// first window shows the editor rather than an empty sidebar.
func (a *app) seedWorkspace() string {
	if a.ws == nil {
		return ""
	}
	rel, err := a.ws.CreatePage("", "Welcome")
	if err != nil {
		log.Printf("bub-desktop: welcome page: %v", err)
		return ""
	}
	const welcome = `# Welcome

This is your workspace: a folder of Markdown pages with an AI writing partner beside them.

## Write in blocks

Type **/** in an empty block for headings, lists, quotes and code blocks. The handle left of the block drags it somewhere else.

## Ask Bub

Select a block, then choose an action in the **Bub** menu or press **⇧⌘R**. Bub suggests a rewrite and nothing enters the page until you accept it.

## Keep it

Every page is a Markdown file in your workspace, so you can open it anywhere.

` + "```go\nfunc main() {\n    fmt.Println(\"Hello, Bub\")\n}\n```\n"
	if err := a.ws.WritePage(rel, welcome); err != nil {
		log.Printf("bub-desktop: welcome page: %v", err)
	}
	a.reloadPages()
	return rel
}

// --- Pages ----------------------------------------------------------------

// openPage shows a page of the workspace, saving the one open first.
func (a *app) openPage(rel string) {
	if a.ws == nil || rel == "" {
		return
	}
	a.savePage()
	markdown, err := a.ws.ReadPage(rel)
	if err != nil {
		a.notifyError("Could not open the page", err.Error(), nil)
		return
	}
	a.current = rel
	a.mode = viewPage
	a.path, _ = a.ws.Abs(rel)
	a.setMarkdown(markdown, a.path)
	a.focusIn = true
	a.hoverBlock = -1
	a.slash = nil
	a.drag = nil
	if a.onPageOpened != nil {
		a.onPageOpened(rel)
	}
}

// newPage creates a page in the folder of the open page and shows it.
func (a *app) newPage() {
	a.newPageIn(a.currentDir())
}

// newPageIn creates a page in a workspace folder and opens it.
func (a *app) newPageIn(dirRel string) {
	if a.ws == nil {
		return
	}
	a.savePage()
	rel, err := a.ws.CreatePage(dirRel, "Untitled")
	if err != nil {
		a.notifyError("Could not create the page", err.Error(), nil)
		return
	}
	a.reloadPages()
	a.openPage(rel)
	a.focusTitle()
}

// currentDir is the folder holding the open page, "" at the top.
func (a *app) currentDir() string {
	if a.current == "" {
		return ""
	}
	dir := filepath.ToSlash(filepath.Dir(a.current))
	if dir == "." {
		return ""
	}
	return dir
}

// deletePage removes a page or folder from the workspace.
func (a *app) deletePage(rel string) {
	if a.ws == nil || rel == "" {
		return
	}
	if err := a.ws.Delete(rel); err != nil {
		a.notifyError("Could not delete", err.Error(), nil)
		return
	}
	deletedOpen := a.current == rel || strings.HasPrefix(a.current, rel+"/")
	a.reloadPages()
	if deletedOpen {
		a.current = ""
		a.path = ""
		a.setDocument(richtext.NewText(""), "")
		if first := firstPage(a.pages); first != "" {
			a.openPage(first)
		}
	}
}

// renamePage renames the open page's file and its title block.
func (a *app) renamePage(rel, title string) {
	if a.ws == nil || strings.TrimSpace(title) == "" {
		return
	}
	newRel, err := a.ws.Rename(rel, title)
	if err != nil {
		a.notifyError("Could not rename", err.Error(), nil)
		return
	}
	a.reloadPages()
	switch {
	case a.current == rel:
		a.current = newRel
		a.path, _ = a.ws.Abs(newRel)
		markdown, err := a.ws.ReadPage(newRel)
		if err == nil {
			a.setMarkdown(markdown, a.path)
		}
	case strings.HasPrefix(a.current, rel+"/"):
		a.current = newRel + strings.TrimPrefix(a.current, rel)
		a.path, _ = a.ws.Abs(a.current)
	}
}

// savePage writes the open page as Markdown when it changed.
func (a *app) savePage() {
	if a.ws == nil || a.current == "" || !a.dirty || a.saving {
		return
	}
	// The Markdown is written on the main thread: what the app remembers of
	// the page's code languages is the app's, not the writing goroutine's.
	markdown := a.markdownText()
	rel := a.current
	version := a.doc.Version()
	write := func() error { return a.ws.WritePage(rel, markdown) }
	settle := func(err error) {
		a.saving = false
		defer func() {
			if a.onSaved != nil {
				a.onSaved()
			}
		}()
		if err != nil {
			a.notifyError("Could not save the page", err.Error(), nil)
			return
		}
		if a.current == rel && a.doc.Version() == version {
			a.setDirty(false)
		}
	}
	if a.win == nil {
		// Windowless (tests): write inline, so state read after savePage is
		// the state the write left.
		settle(write())
		return
	}
	a.saving = true
	go func() {
		err := write()
		a.dispatch(func() { settle(err) })
	}()
}

// scheduleSave saves soon after the last edit, so the file on disk follows.
func (a *app) scheduleSave() {
	if !a.settings.Autosave || a.current == "" || a.ws == nil {
		return
	}
	a.saveSeq++
	seq := a.saveSeq
	time.AfterFunc(autosaveDelay, func() {
		a.dispatch(func() {
			if seq == a.saveSeq {
				a.savePage()
			}
		})
	})
}

// documentTitle is the first heading of the open page, as the sidebar and the
// window title name it.
func (a *app) documentTitle() string {
	if outline := block.Outline(a.doc.Text()); len(outline) > 0 {
		if t := strings.TrimSpace(outline[0].Text); t != "" {
			return t
		}
	}
	if a.path != "" {
		base := strings.TrimSuffix(filepath.Base(a.path), filepath.Ext(a.path))
		if base != "" {
			return base
		}
	}
	return untitledName
}

// updateTitle names the window after the open page.
func (a *app) updateTitle() {
	if a.win != nil {
		a.win.SetTitle(a.documentTitle() + " — Bub")
	}
}

// setDirty marks the page edited, as the close button's dot does.
func (a *app) setDirty(dirty bool) {
	a.dirty = dirty
	if a.onEdited != nil {
		a.onEdited(dirty)
	}
	if a.win != nil {
		a.win.SetDocumentEdited(dirty)
	}
}

// setDocument shows text as the open page, saved at path.
func (a *app) setDocument(text richtext.Text, path string) {
	a.doc.SetText(text)
	a.doc.IdentifyParagraphs(a.newParagraphID)
	a.doc.SetSelection(0, 0)
	a.path = path
	// The languages belonged to the page that was open: richtext keeps none.
	a.forgetCodeLanguages()
	a.closeAI()
	a.setDirty(false)
	a.focusIn = true
	a.updateTitle()
}

// --- Settings -------------------------------------------------------------

// showSettings opens the settings page.
func (a *app) showSettings() {
	a.savePage()
	a.mode = viewSettings
}

// closeSettings returns to the open page.
func (a *app) closeSettings() {
	a.mode = viewPage
	a.focusIn = true
}

// settingsChanged persists the settings and applies what they change.
func (a *app) settingsChanged() {
	if a.settings.Workspace != "" && (a.ws == nil || a.settings.Workspace != a.ws.Root) {
		a.savePage()
		a.openWorkspace(a.settings.Workspace)
		a.reloadPages()
		if first := firstPage(a.pages); first != "" {
			a.openPage(first)
		}
	}
	if a.uiReady {
		applyThemeSource(a.settings.Theme)
	}
	if a.settingsPath == "" {
		return
	}
	go func(path string, s appdata.Settings) {
		if err := appdata.SaveSettings(path, s); err != nil {
			log.Printf("bub-desktop: save settings: %v", err)
		}
	}(a.settingsPath, a.settings)
	a.updateTitle()
}

// applyThemeSource follows the system appearance or forces light or dark.
func applyThemeSource(source string) {
	switch source {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

// --- Chrome ---------------------------------------------------------------

// toggleSidebar shows or hides the sidebar.
func (a *app) toggleSidebar() { a.sidebarShown = !a.sidebarShown }

// toggleRail shows the right rail on a tab, or hides it when that tab shows.
// The rail is chrome around the page, so a chat page goes back to the page
// first.
func (a *app) toggleRail(tab railTab) {
	if a.mode == viewChat {
		a.mode = viewPage
	}
	if a.railShown && a.railTab == tab {
		a.railShown = false
	} else {
		a.railShown = true
		a.railTab = tab
	}
	if tab == railOutline {
		a.settings.ShowOutline = a.railShown && a.railTab == railOutline
		a.settingsChanged()
	}
}

// showRail opens the right rail on a tab, over the page.
func (a *app) showRail(tab railTab) {
	if a.mode == viewChat {
		a.mode = viewPage
	}
	a.railShown = true
	a.railTab = tab
}

// toggleFolder opens or closes a folder in the sidebar.
func (a *app) toggleFolder(rel string) {
	if a.openFolders == nil {
		a.openFolders = map[string]bool{}
	}
	a.openFolders[rel] = !a.openFolders[rel]
}

// --- Agent wiring ---------------------------------------------------------

// dispatch runs fn on the main thread when there is a window, and directly in
// tests, which have none.
func (a *app) dispatch(fn func()) {
	if a.closed {
		return
	}
	if a.win != nil {
		a.win.Update(fn)
		return
	}
	fn()
}

// connect spawns the agent and runs the handshake in the background.
func (a *app) connect() {
	if a.connecting {
		return
	}
	a.gen++
	gen := a.gen
	a.connecting = true
	a.connectErr = ""
	go func() {
		proc, client, sessionID, info, err := a.startAgent()
		a.dispatch(func() {
			a.connecting = false
			if a.gen != gen {
				if proc != nil {
					_ = proc.Close()
				}
				return
			}
			if err != nil {
				if proc != nil {
					_ = proc.Close()
				}
				a.connectErr = err.Error()
				return
			}
			a.proc, a.client, a.sessionID = proc, client, sessionID
			a.agentImages = info != nil && info.PromptImage
			a.connectErr = ""
			a.installHandlers(gen)
			if retry := a.retryAfterConnect; retry != nil {
				a.retryAfterConnect = nil
				a.startAction(*retry)
			}
		})
	}()
}

// startAgent spawns `bub acp` and speaks the handshake, off the main thread.
func (a *app) startAgent() (*acp.Proc, *acp.Client, string, *acp.InitializeResult, error) {
	// The agent process runs where the app was started, so a checkout's
	// `uv run bub acp` finds its project, while the ACP session is rooted at
	// the workspace whose pages the window edits.
	cfg := acp.ConfigFromEnv(a.settings.AgentCmd)
	sessionDir := a.settings.Workspace
	if sessionDir == "" {
		sessionDir = cfg.Workspace
	}
	if sessionDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, nil, "", nil, fmt.Errorf("could not find the workspace: %w", err)
		}
		sessionDir = wd
	}
	cwd, err := filepath.Abs(sessionDir)
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("workspace %q: %w", sessionDir, err)
	}
	proc, err := acp.Start(cfg)
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("could not start the agent (%s): %w", strings.Join(cfg.Command, " "), err)
	}
	proc.Client.SetLogger(log.New(os.Stderr, "acp: ", 0))

	ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
	defer cancel()
	info, err := proc.Client.Initialize(ctx, acp.Implementation{Name: clientName, Title: appTitle, Version: appVersion})
	if err != nil {
		stderr := proc.Stderr()
		_ = proc.Close()
		return nil, nil, "", nil, agentError("initialize", err, stderr)
	}
	session, err := proc.Client.NewSession(ctx, cwd)
	if err != nil {
		stderr := proc.Stderr()
		_ = proc.Close()
		return nil, nil, "", nil, agentError("open a session", err, stderr)
	}
	return proc, proc.Client, session.SessionID, info, nil
}

// installHandlers routes the agent's notifications into the window's state, on
// the main thread, and notices when the connection ends.
func (a *app) installHandlers(gen int) {
	client := a.client
	client.SetUpdateHandler(func(u acp.Update) {
		a.dispatch(func() {
			if a.gen != gen {
				return
			}
			a.applyUpdate(u)
		})
	})
	client.SetCloseHandler(func(err error) {
		a.dispatch(func() {
			if a.gen != gen {
				return
			}
			a.agentGone(err)
		})
	})
}

// agentGone forgets the connection, closes the AI surfaces and says why, with a
// Retry that reconnects and reruns the action.
func (a *app) agentGone(err error) {
	a.client = nil
	a.sessionID = ""
	if lost := a.proc; lost != nil {
		a.proc = nil
		go func() { _ = lost.Close() }()
	}
	acted := a.closeAI()
	a.failChat("The agent stopped.")
	detail := "The Bub agent stopped."
	if err != nil && !errors.Is(err, acp.ErrClosed) {
		detail = "The Bub agent stopped: " + err.Error() + "."
	}
	a.notifyError("Bub stopped", detail, a.retryAfterReconnect(acted))
}

// reconnect replaces the agent process, keeping the pages and the chats.
func (a *app) reconnect() {
	a.gen++
	old := a.proc
	a.proc, a.client, a.sessionID = nil, nil, ""
	a.chatSessions = map[string]string{}
	if old != nil {
		go func() { _ = old.Close() }()
	}
	a.connect()
}

// retryAfterReconnect returns a Retry action: reconnect, then run act again.
func (a *app) retryAfterReconnect(act *actionState) func() {
	if act == nil {
		return nil
	}
	return func() {
		a.retryAfterConnect = act
		a.reconnect()
	}
}

// shutdown stops the agent when the window closes.
func (a *app) shutdown() {
	a.savePage()
	a.closed = true
	a.gen++
	if a.proc != nil {
		proc := a.proc
		a.proc = nil
		go func() { _ = proc.Close() }()
	}
	a.client = nil
}

// --- Agent notifications --------------------------------------------------

// applyUpdate folds one session/update into the AI surfaces: block suggestions
// and the chat turn of the session it belongs to.
func (a *app) applyUpdate(u acp.Update) {
	switch u.Type {
	case acp.UpdateAvailableCommands:
		// The agent advertises what "/" may run; every session of one agent
		// advertises the same list, so the composer reads the latest.
		a.commands = u.Commands
		if u.CommandPrefix != "" {
			a.commandPrefix = u.CommandPrefix
		}
		return
	case acp.UpdateAgentMessageChunk:
	default:
		return
	}
	// Chats open a session of their own; the block actions use the app's
	// first session. Until session/new answers, a chat turn's session id is
	// empty, so a chunk that is not the block session still belongs to it.
	if a.chat != nil && a.chat.streaming {
		switch {
		case a.chat.sessionID != "" && u.SessionID == a.chat.sessionID:
			a.appendChat(u.Text)
			return
		case a.chat.sessionID == "" && u.SessionID != "" && u.SessionID != a.sessionID:
			a.appendChat(u.Text)
			return
		}
	}
	a.appendSuggestion(u.Text)
}

// --- Native dialogs -------------------------------------------------------

// notifyError tells the writer about a failure the way the platform does: a
// native message, with Retry when the action can be run again.
func (a *app) notifyError(title, message string, retry func()) {
	if !a.uiReady {
		log.Printf("bub-desktop: %s: %s", title, message)
		return
	}
	if a.dialog != nil {
		a.dialog(title, message, retry)
		return
	}
	go func() {
		buttons := []string{"OK"}
		if retry != nil {
			buttons = []string{"OK", "Retry"}
		}
		result, err := mygo.Dialog.Message(mygo.MessageOptions{
			Title:         title,
			Message:       message,
			Buttons:       buttons,
			DefaultButton: 0,
			CancelButton:  0,
		})
		if err != nil || retry == nil {
			return
		}
		if result.Button == 1 {
			a.dispatch(retry)
		}
	}()
}

// agentError describes a failed handshake, with the last of what the agent
// wrote to its standard error when it said nothing useful.
func agentError(what string, err error, stderr string) error {
	msg := fmt.Sprintf("The agent could not %s: %v", what, err)
	if tail := lastLine(stderr); tail != "" {
		msg += " — " + tail
	}
	return errors.New(msg)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// --- Close prompt ---------------------------------------------------------

// confirmDiscard asks before an action would throw away unsaved writing, with
// the platform's Save / Cancel / Discard message.
func (a *app) confirmDiscard(then func()) {
	if !a.dirty {
		then()
		return
	}
	a.askUnsaved(func(choice int) {
		switch choice {
		case 0: // Save, then continue.
			a.savePage()
			then()
		case 2: // Discard.
			then()
		}
	})
}

// askUnsaved asks about unsaved changes off the main thread. then runs on the
// main thread with the platform's button: 0 Save, 1 Cancel, 2 Discard.
func (a *app) askUnsaved(then func(choice int)) {
	if a.chooseUnsaved != nil {
		a.chooseUnsaved(then)
		return
	}
	go func() {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{
			Title:              "Discard changes?",
			Message:            "The page has unsaved changes.",
			Detail:             "Save them before continuing?",
			Buttons:            []string{"Save", "Cancel", "Discard"},
			DefaultButton:      0,
			CancelButton:       1,
			DestructiveButtons: []int{2},
		})
		if err != nil {
			a.dispatch(func() {
				a.notifyError("Could not ask about unsaved changes", err.Error(), nil)
				then(1)
			})
			return
		}
		a.dispatch(func() { then(result.Button) })
	}()
}

// handleClose asks before the close button, ⌘W or File > Close throws writing
// away; Cancel keeps the window open.
func (a *app) handleClose(e *mygo.CloseEvent) {
	if a.allowClose || !a.dirty {
		return
	}
	e.PreventDefault()
	a.askToClose()
}

// askToClose runs the Save / Cancel / Discard prompt once for the window.
func (a *app) askToClose() {
	if !a.dirty || a.allowClose || a.closingPrompt {
		return
	}
	a.closingPrompt = true
	a.askUnsaved(func(choice int) {
		a.closingPrompt = false
		switch choice {
		case 0: // Save, then close.
			a.savePage()
			a.closeWindow()
		case 2: // Discard and close.
			a.closeWindow()
		}
	})
}

// closeWindow closes after the prompt; allowClose keeps the second OnClose
// from asking again.
func (a *app) closeWindow() {
	a.allowClose = true
	if a.win != nil {
		a.win.Close()
	}
}
