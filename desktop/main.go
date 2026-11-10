// Command bub-desktop is Bub Desktop: a native Markdown editor, drawn by MyGo,
// where the agent edits blocks inline instead of chatting.
//
// The document is the conversation. `⌘K` on the caret's block, or the quiet ✦
// in the gutter, opens a small action bar (Rewrite, Shorten, Expand, Continue,
// Translate, Ask…); the agent streams a suggestion for that block, and nothing
// enters the document — or its undo history — until the writer accepts it.
//
// The agent is a separate `bub acp` process speaking the Agent Client Protocol
// over stdio: the window and the agent share no code, only ACP.
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
)

const (
	appName          = "Bub Desktop"
	appVersion       = "0.1.0"
	clientName       = "bub-desktop"
	handshakeTimeout = 20 * time.Second
	windowWidth      = 1040
	windowHeight     = 760
	minWindowWidth   = 760
	minWindowHeight  = 560
)

// The zinc surface of the website's Bub examples, with violet for the AI
// surfaces only: the gutter mark, the action bar and a suggestion.
var (
	colorBackground = ui.Hex("#f4f4f5")
	colorPanel      = ui.Hex("#ffffff")
	colorBorder     = ui.Hex("#e4e4e7")
	colorBorderSoft = ui.RGBA(24, 24, 27, 0.10)
	colorText       = ui.Hex("#18181b")
	colorMuted      = ui.Hex("#71717a")
	colorFaint      = ui.Hex("#a1a1aa")
	colorViolet     = ui.Hex("#7c3aed")
	colorVioletSoft = ui.Hex("#f5f3ff")
	colorVioletLine = ui.Hex("#ddd6fe")
	colorInk        = ui.Hex("#18181b")
	colorOnInk      = ui.Hex("#ffffff")
	colorDanger     = ui.Hex("#dc2626")
	colorDangerBg   = ui.Hex("#fef2f2")
	colorDangerLine = ui.Hex("#fecaca")
	colorSuccess    = ui.Hex("#16a34a")
	colorWarning    = ui.Hex("#d97706")
	colorFooter     = ui.Hex("#fafafa")
)

// status is where the connection to the agent stands.
type status int

const (
	statusConnecting status = iota
	statusReady
	statusOffline
)

func (s status) String() string {
	switch s {
	case statusConnecting:
		return "connecting"
	case statusReady:
		return "ready"
	default:
		return "offline"
	}
}

// app is the window's state: the document, the AI surface, and the connection
// to `bub acp`. The view builds from it on the main thread; the agent's
// goroutine changes it only through win.Update.
type app struct {
	win         *mygo.Window
	theme       *ui.Theme
	editorTheme *richtext.Theme
	editor      ui.Handle

	// The document. idSeq names paragraphs so block actions survive edits.
	doc          *richtext.Document
	path         string
	dirty        bool
	idSeq        atomic.Int64
	wordCount    int
	wordsVersion uint64

	// Block AI: one action bar, at most one suggestion, one turn in flight.
	action        *actionState
	suggestion    *suggestionState
	pendingAction *actionState
	aiGen         int
	askMode       bool
	askText       string
	activity      string

	// Connection to `bub acp`. gen rises whenever a connection is replaced,
	// so callbacks from an old one do nothing.
	proc       *acp.Proc
	client     *acp.Client
	gen        int
	connecting bool

	status       status
	sessionID    string
	agentName    string
	agentVersion string
	agentCommand string
	model        string
	errText      string
}

func newApp() *app {
	theme := newTheme()
	command := strings.Join(acp.ConfigFromEnv().Command, " ")
	if command == "" {
		command = acp.DefaultCommand
	}
	a := &app{
		theme:        theme,
		editorTheme:  newEditorTheme(theme),
		status:       statusConnecting,
		agentCommand: command,
		model:        os.Getenv("BUB_MODEL"),
	}
	a.doc = richtext.NewDocument(richtext.NewText(""))
	a.doc.IdentifyParagraphs(a.newParagraphID)
	return a
}

// newTheme is the light zinc look: white panels, hairline borders, one dark
// ink, and violet only where the agent speaks.
func newTheme() *ui.Theme {
	t := *ui.LightTheme()
	t.Background = colorBackground
	t.Surface = colorPanel
	t.SurfaceHover = colorFooter
	t.SurfacePressed = colorBackground
	t.Border = colorBorder
	t.Text = colorText
	t.TextMuted = colorMuted
	t.Accent = colorInk
	t.AccentHover = ui.Hex("#3f3f46")
	t.AccentPressed = ui.Hex("#27272a")
	t.AccentText = colorOnInk
	t.Danger = colorDanger
	t.Warning = colorWarning
	t.Success = colorSuccess
	t.Selection = colorVioletSoft
	t.Focus = colorViolet
	t.Inverse = colorInk
	t.InverseText = colorOnInk
	t.Scrollbar = ui.Hex("#d4d4d8")
	t.Radius = 10
	t.Spacing = 4
	t.FontSize = 14
	return &t
}

// newEditorTheme styles the writing surface: prose spacing, violet links and
// selection, headings that fit a page.
func newEditorTheme(t *ui.Theme) *richtext.Theme {
	rt := *richtext.DefaultTheme(t)
	rt.Link = colorViolet
	rt.Selection = colorVioletSoft
	rt.Caret = colorInk
	rt.ParagraphSpacing = 0.65
	rt.Headings[0].Size = 1.9
	rt.Headings[0].Top = 0
	return &rt
}

// --- Agent wiring ---------------------------------------------------------

// dispatch runs fn on the main thread when there is a window, and directly in
// tests, which have none.
func (a *app) dispatch(fn func()) {
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
	a.status = statusConnecting
	a.errText = ""
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
				a.status = statusOffline
				a.errText = err.Error()
				return
			}
			a.proc, a.client, a.sessionID = proc, client, sessionID
			a.status = statusReady
			a.errText = ""
			if info != nil && info.AgentInfo != nil {
				a.agentName = info.AgentInfo.Name
				a.agentVersion = info.AgentInfo.Version
			}
			a.installHandlers(gen)
		})
	}()
}

// startAgent spawns `bub acp` and speaks the handshake, off the main thread.
func (a *app) startAgent() (*acp.Proc, *acp.Client, string, *acp.InitializeResult, error) {
	cfg := acp.ConfigFromEnv()
	workspace := cfg.Workspace
	if workspace == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, nil, "", nil, fmt.Errorf("could not find the workspace: %w", err)
		}
		workspace = wd
	}
	cwd, err := filepath.Abs(workspace)
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("workspace %q: %w", workspace, err)
	}
	cfg.Workspace = workspace
	proc, err := acp.Start(cfg)
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("could not start the agent (%s): %w", strings.Join(cfg.Command, " "), err)
	}
	proc.Client.SetLogger(log.New(os.Stderr, "acp: ", 0))

	ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
	defer cancel()
	info, err := proc.Client.Initialize(ctx, acp.Implementation{Name: clientName, Title: appName, Version: appVersion})
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

// agentGone marks the window offline, keeps the writing surface usable, and
// fails the suggestion the agent will never finish.
func (a *app) agentGone(err error) {
	a.client = nil
	a.sessionID = ""
	a.status = statusOffline
	a.activity = ""
	if lost := a.proc; lost != nil {
		a.proc = nil
		go func() { _ = lost.Close() }()
	}
	if s := a.suggestion; s != nil && s.status.busy() {
		s.status = suggestFailed
		s.errText = "The agent stopped before the suggestion finished."
	}
	a.pendingAction = nil
	if err == nil || errors.Is(err, acp.ErrClosed) {
		a.errText = "The Bub agent stopped. Press Retry to start it again."
	} else {
		a.errText = "The Bub agent stopped: " + err.Error() + ". Press Retry to start it again."
	}
}

// reconnect replaces the agent process, keeping the document and suggestion.
func (a *app) reconnect() {
	a.gen++
	old := a.proc
	a.proc, a.client, a.sessionID = nil, nil, ""
	a.activity = ""
	a.errText = ""
	a.pendingAction = nil
	if s := a.suggestion; s != nil && s.status.busy() {
		s.status = suggestFailed
		s.errText = "The agent restarted before the suggestion finished."
	}
	if old != nil {
		go func() { _ = old.Close() }()
	}
	a.connect()
}

// shutdown stops the agent when the window closes. Close gives the agent a
// grace period to exit after its standard input closes, so it runs off the main
// thread: quitting the window should not wait on it.
func (a *app) shutdown() {
	a.gen++
	if a.proc != nil {
		proc := a.proc
		a.proc = nil
		go func() { _ = proc.Close() }()
	}
	a.client = nil
}

// --- Agent notifications --------------------------------------------------

// applyUpdate folds one session/update into the AI surface: text feeds the
// suggestion, tools and thoughts only set the small activity label. Nothing
// here creates a chat row, because there are none.
func (a *app) applyUpdate(u acp.Update) {
	switch u.Type {
	case acp.UpdateAgentMessageChunk:
		a.appendSuggestion(u.Text)
	case acp.UpdateAgentThoughtChunk:
		if s := a.suggestion; s != nil && s.status.busy() {
			a.activity = "Thinking…"
		}
	case acp.UpdateToolCall:
		if u.ToolCall != nil {
			title := u.ToolCall.Title
			if title == "" {
				title = u.ToolCall.Name
			}
			if title == "" {
				title = "a tool"
			}
			a.activity = "Running " + title + "…"
		}
	case acp.UpdateToolCallUpdate:
		if u.ToolCallUpdate != nil && u.ToolCallUpdate.Status != nil {
			switch *u.ToolCallUpdate.Status {
			case acp.ToolStatusCompleted, acp.ToolStatusFailed:
				a.activity = ""
			}
		}
	}
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

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     appName,
			Width:     windowWidth,
			Height:    windowHeight,
			MinWidth:  minWindowWidth,
			MinHeight: minWindowHeight,
			StateKey:  "main",
			Content:   ui.View(a.view),
		})
		a.win.OnClosed(a.shutdown)
		a.connect()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
