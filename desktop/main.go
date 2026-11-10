// Command bub-desktop is Bub Desktop: a native chat window, drawn by MyGo,
// that talks to a `bub acp` subprocess over the Agent Client Protocol v1.
//
// The window owns the conversation and nothing else: the agent runs the turns,
// its tools act on the workspace, and the window shows what it streams. The two
// halves share no code, only ACP over the process's standard input and output,
// so anything that speaks ACP could take either end.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/bubbuild/bub/desktop/internal/acp"
)

const (
	appName          = "Bub Desktop"
	appVersion       = "0.1.0"
	clientName       = "bub-desktop"
	handshakeTimeout = 20 * time.Second
	windowWidth      = 820
	windowHeight     = 690
	minWindowWidth   = 720
	minWindowHeight  = 560
)

// logoPNG is the Bub mark, copied from the website's todo agent example. The
// same file is the app icon mygo.json names by convention.
//
//go:embed resources/icon.png
var logoPNG []byte

// The zinc palette of the website's Bub examples: a light page, white panels
// and hairline borders.
var (
	colorBackground = ui.Hex("#f4f4f5")
	colorPanel      = ui.Hex("#ffffff")
	colorSunken     = ui.Hex("#f4f4f5")
	colorFooter     = ui.Hex("#fafafa")
	colorBorder     = ui.Hex("#e4e4e7")
	colorBorderSoft = ui.RGBA(24, 24, 27, 0.10)
	colorText       = ui.Hex("#18181b")
	colorMuted      = ui.Hex("#71717a")
	colorFaint      = ui.Hex("#a1a1aa")
	colorInk        = ui.Hex("#18181b")
	colorOnInk      = ui.Hex("#ffffff")
	colorDanger     = ui.Hex("#dc2626")
	colorDangerBg   = ui.Hex("#fef2f2")
	colorDangerLine = ui.Hex("#fecaca")
	colorSuccess    = ui.Hex("#16a34a")
	colorWarning    = ui.Hex("#d97706")
)

// suggestions fill the composer and send on a click, as the website example's
// chips do.
var suggestions = []string{
	"Explain this repo",
	"What can you do?",
	"Summarize recent git commits",
}

// status is where the connection to the agent stands; a turn in progress is
// separate, busy.
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

type itemKind uint8

const (
	itemUser itemKind = iota
	itemAssistant
	itemTool
	itemStatus
)

// item is one row of the transcript: a user message, an assistant message
// (markdown, streamed while it comes), a tool call or, while a turn runs, the
// activity pill.
type item struct {
	id      int
	kind    itemKind
	text    string
	thought string
	done    bool
	tool    *toolState
}

type toolState struct {
	id     string
	title  string
	status string
	input  string
	output string
}

// app is the window's state. The view builds the interface from it on the main
// thread; the agent's goroutine changes it only through win.Update.
type app struct {
	win   *mygo.Window
	theme *ui.Theme
	md    markdown.Options
	logo  ui.ImageSource

	// logoLoaded makes the mark decode once.
	logoLoaded bool

	// Connection to `bub acp`. gen rises every time a connection is
	// replaced, so callbacks of the old one do nothing.
	proc       *acp.Proc
	client     *acp.Client
	gen        int
	connecting bool

	sessionID    string
	agentName    string
	agentVersion string
	agentCommand string
	model        string

	status  status
	busy    bool
	errText string
	// activity is what the turn is doing, as the pill says: a running tool.
	activity string

	messages []*item
	input    string
	nextID   int

	list     ui.ListState
	composer ui.Handle
}

func newApp() *app {
	theme := newTheme()
	// The footer names the command the window spawned the agent with.
	command := strings.Join(acp.ConfigFromEnv().Command, " ")
	if command == "" {
		command = acp.DefaultCommand
	}
	return &app{
		theme:        theme,
		md:           markdown.Options{Theme: markdown.DefaultTheme(theme)},
		status:       statusConnecting,
		agentCommand: command,
		model:        os.Getenv("BUB_MODEL"),
	}
}

// newTheme is the light zinc look of the website examples: no heavy chrome,
// rounded surfaces, one dark accent.
func newTheme() *ui.Theme {
	t := *ui.LightTheme()
	t.Background = colorBackground
	t.Surface = colorPanel
	t.SurfaceHover = colorFooter
	t.SurfacePressed = colorSunken
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
	t.Selection = ui.Hex("#d4d4d8")
	t.Focus = ui.Hex("#a1a1aa")
	t.Inverse = colorInk
	t.InverseText = colorOnInk
	t.Scrollbar = ui.Hex("#d4d4d8")
	t.Radius = 12
	t.Spacing = 4
	t.FontSize = 14
	return &t
}

// view builds the window: a white panel with the header, the transcript and
// the composer.
func (a *app) view(c *ui.Context) {
	c.SetTheme(a.theme)
	a.ensureLogo()
	t := c.Theme()
	rows := a.rows()
	a.list.Key = func(i int) any { return rows[i].id }
	a.list.FollowEnd = true
	a.list.FollowAnimation = &ui.Smooth

	ui.Column(c).Fill().Background(colorBackground).Padding(18).Children(func() {
		ui.Column(c).Fill().Radius(24).Background(colorPanel).
			Border(1, colorBorderSoft).
			Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.03)).
			Shadow(0, 18, 40, -18, ui.RGBA(24, 24, 27, 0.18)).
			Children(func() {
				a.header(c, t)
				if a.errText != "" {
					a.errorBanner(c)
				}
				a.transcript(c, t, rows)
				a.composerRow(c)
			})
	})
}

func (a *app) header(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(64).Padding(0, 20).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(colorBorderSoft).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			a.logoMark(c, 26)
			ui.Text(c, appName).FontSize(15).FontWeight(600).LetterSpacing(-0.2).TextColor(t.Text)
		})
		ui.Spacer(c)
		// The connection chip: a dot and the state of the agent.
		ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(5, 11).Radius(999).
			Background(colorPanel).Border(1, colorBorder).Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Background(a.statusColor())
			ui.Text(c, a.statusLabel()).Key("status").FontSize(11).FontWeight(500).TextColor(colorMuted)
		})
	})
}

// logoMark shows the Bub icon at size DIPs, or its initial before the image
// decodes.
func (a *app) logoMark(c *ui.Context, size float32) {
	if a.logo != nil {
		ui.Image(c, a.logo).Size(size, size).Radius(size * 0.25)
		return
	}
	ui.Box(c).Size(size, size).Radius(size*0.25).Background(colorPanel).
		Border(1, colorBorder).Center().Children(func() {
		ui.Text(c, "B").FontSize(size * 0.6).Bold().TextColor(colorText)
	})
}

func (a *app) errorBanner(c *ui.Context) {
	ui.Row(c).Key("error").Gap(10).AlignItems(ui.Center).
		Margin(12, 18, 0, 18).Padding(8, 12).Radius(14).
		Background(colorDangerBg).Border(1, colorDangerLine).Children(func() {
		ui.Text(c, "!").FontSize(12).FontWeight(700).TextColor(colorDanger)
		ui.Text(c, a.errText).Grow(1).FontSize(12).LineHeight(1.45).TextColor(colorDanger)
		if ui.Button(c, "Retry").Key("retry").FontSize(11).Radius(999).Padding(4, 12).
			Background(colorPanel).Border(1, colorDangerLine).TextColor(colorDanger).Clicked() {
			a.reconnect()
		}
	})
}

func (a *app) transcript(c *ui.Context, t *ui.Theme, rows []*item) {
	if len(rows) == 0 {
		ui.Box(c).Grow(1).Center().Children(func() {
			a.emptyState(c, t)
		})
		return
	}
	ui.List(c, &a.list, len(rows), func(i int) {
		a.row(c, t, rows[i])
	}).Grow(1).Padding(24, 24).Gap(18).Justify(ui.End)
}

// emptyState is what the window shows before the first message: a mark, a
// question and chips that start a conversation.
func (a *app) emptyState(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Center().MaxWidth(440).Gap(10).Padding(0, 24).Children(func() {
		ui.Box(c).Size(52, 52).Radius(18).Background(colorPanel).Border(1, colorBorder).
			Center().Shadow(0, 8, 24, -12, ui.RGBA(24, 24, 27, 0.15)).Children(func() {
			a.logoMark(c, 30)
		})
		ui.Text(c, "What are you working on?").FontSize(20).FontWeight(650).
			LetterSpacing(-0.3).TextColor(t.Text).Margin(8, 0, 0, 0)
		ui.Text(c, "Ask Bub anything about this workspace. It can read, edit and run things here.").
			FontSize(13).LineHeight(1.5).TextColor(colorMuted).TextAlign(ui.Center)
		ui.Row(c).Wrap().Justify(ui.Center).Gap(8).Margin(14, 0, 0, 0).Children(func() {
			for i, s := range suggestions {
				if ui.Button(c, s).Key(fmt.Sprintf("suggest-%d", i)).FontSize(12).
					Radius(999).Padding(6, 14).Background(colorPanel).Border(1, colorBorder).
					TextColor(colorText).Clicked() {
					a.input = s
					a.send()
				}
			}
		})
	})
}

// row builds one transcript row.
func (a *app) row(c *ui.Context, t *ui.Theme, m *item) {
	switch m.kind {
	case itemUser:
		ui.Column(c).AlignItems(ui.End).Gap(6).Children(func() {
			ui.Text(c, "You").FontSize(11).FontWeight(500).TextColor(colorFaint).Padding(0, 4)
			ui.Text(c, m.text).Selectable().FontSize(14).LineHeight(1.55).
				TextColor(t.Text).MaxWidthPercent(80).TextAlign(ui.End)
		})
	case itemAssistant:
		if m.text == "" && m.thought == "" && m.done {
			return // nothing came of the turn: no empty bubble
		}
		ui.Column(c).AlignItems(ui.Start).Gap(6).Children(func() {
			ui.Text(c, "Agent").FontSize(11).FontWeight(500).TextColor(colorFaint).Padding(0, 4)
			ui.Box(c).Background(colorSunken).Radius(22).Padding(10, 14).MaxWidthPercent(88).Children(func() {
				if m.thought != "" {
					ui.Text(c, firstLine(m.thought)).FontSize(12).LineHeight(1.45).
						TextColor(colorFaint).MaxLines(3).Margin(0, 0, 6, 0)
				}
				switch {
				case m.text != "" && m.done:
					markdown.View(c, m.text, &a.md)
				case m.text != "":
					markdown.Stream(c, m.text, &a.md)
				case !m.done:
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Spinner(c).FontSize(12)
						ui.Text(c, "Thinking…").FontSize(13).TextColor(colorMuted)
					})
				}
			})
		})
	case itemTool:
		a.toolRow(c, m.tool)
	case itemStatus:
		ui.Row(c).AlignItems(ui.Start).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(5, 12).Radius(999).
				Background(colorPanel).Border(1, colorBorder).Children(func() {
				ui.Spinner(c).FontSize(11)
				ui.Text(c, a.activity).FontSize(11).TextColor(colorMuted)
			})
		})
	}
}

// toolRow is a tool call as a compact row: title, status and a preview of its
// arguments or output.
func (a *app) toolRow(c *ui.Context, tool *toolState) {
	if tool == nil {
		return
	}
	statusText, statusColor := toolStatusStyle(tool.status)
	ui.Column(c).AlignItems(ui.Start).Gap(4).Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Padding(6, 11).Radius(14).
			Background(colorPanel).Border(1, colorBorder).Children(func() {
			ui.Text(c, toolGlyph(tool.status)).FontSize(10).TextColor(statusColor)
			ui.Text(c, toolTitle(tool)).FontSize(12).FontWeight(500).TextColor(colorText).MaxLines(1)
			ui.Text(c, statusText).FontSize(11).TextColor(colorMuted)
		})
		if preview := toolPreview(tool); preview != "" {
			ui.Text(c, preview).FontSize(11).LineHeight(1.4).TextColor(colorFaint).
				MaxLines(2).Padding(0, 6)
		}
	})
}

// composerRow is the input card: the text area, the Send or Stop button, and a
// footer with what the agent is and how to send.
func (a *app) composerRow(c *ui.Context) {
	ui.Column(c).Padding(14, 20, 16, 20).Gap(8).Children(func() {
		card := ui.Box(c).Radius(20).Background(colorPanel).
			Border(1, ui.Hex("#d4d4d8")).
			Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.04))
		var input ui.Element
		card.Children(func() {
			ui.Column(c).Children(func() {
				ui.Row(c).AlignItems(ui.End).Gap(8).Padding(8, 8, 8, 14).Children(func() {
					input = ui.TextAreaBase(c.Key("composer"), &a.input).
						Placeholder("Message the agent · Enter to send").
						Label("Message").
						Lines(1, 6).
						FontSize(14).
						Grow(1).
						AutoFocus().
						OnTextKey(func(k ui.TextKey) bool {
							if k.Key != ui.KeyEnter || k.Mods != 0 {
								return false // Shift+Enter breaks the line
							}
							if a.busy {
								return true // a turn runs: keep the text for later
							}
							a.sendText(k.Text())
							k.SetText("")
							return true
						})
					a.sendButton(c)
				})
				ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(6, 12).
					BorderWidth(1, 0, 0, 0).BorderColor(colorBorderSoft).
					Background(colorFooter).Children(func() {
					ui.Text(c, a.footerLabel()).FontSize(10).TextColor(colorFaint).MaxLines(1)
					ui.Spacer(c)
					ui.Text(c, "Enter to send · Shift+Enter for a new line").
						FontSize(10).TextColor(colorFaint)
				})
			})
		})
		if input.Focused() {
			card.Border(1, ui.Hex("#a1a1aa"))
		}
	})
}

// sendButton is the dark round button: Send, or Stop while a turn runs.
func (a *app) sendButton(c *ui.Context) {
	if a.busy {
		if ui.Button(c, "■").Key("stop").Label("Stop").Size(36, 36).Radius(18).
			Background(colorInk).TextColor(colorOnInk).BorderWidth(0).FontSize(11).
			Clicked() {
			a.stop()
		}
		return
	}
	if ui.Button(c, "↑").Key("send").Label("Send").Size(36, 36).Radius(18).
		Background(colorInk).TextColor(colorOnInk).BorderWidth(0).FontSize(16).
		Disabled(strings.TrimSpace(a.input) == "").
		Clicked() {
		a.send()
	}
}

// rows is what the transcript shows: the messages, with a tool call still
// running folded into the activity pill at the end rather than shown twice.
func (a *app) rows() []*item {
	rows := make([]*item, 0, len(a.messages)+1)
	for _, m := range a.messages {
		if m.kind == itemTool && m.tool != nil && running(m.tool.status) {
			continue
		}
		rows = append(rows, m)
	}
	if a.busy {
		rows = append(rows, &item{id: -1, kind: itemStatus})
	}
	return rows
}

// running reports whether a tool call still works: it is the activity pill
// then, not a row.
func running(status string) bool {
	return status == "" || status == acp.ToolStatusPending || status == acp.ToolStatusInProgress
}

func (a *app) statusLabel() string {
	if a.busy {
		return "running"
	}
	return a.status.String()
}

func (a *app) statusColor() ui.Color {
	switch {
	case a.status == statusOffline:
		return colorDanger
	case a.status == statusConnecting:
		return colorWarning
	case a.busy:
		return colorInk
	default:
		return colorSuccess
	}
}

func (a *app) footerLabel() string {
	parts := []string{a.agentCommand}
	if a.agentName != "" {
		parts = append(parts, a.agentName+" "+a.agentVersion)
	} else if a.model != "" {
		parts = append(parts, a.model)
	}
	return strings.Join(parts, " · ")
}

// ensureLogo decodes the embedded mark once.
func (a *app) ensureLogo() {
	if a.logoLoaded {
		return
	}
	a.logoLoaded = true
	if img, err := ui.DecodeImage(logoPNG); err == nil {
		a.logo = img
	}
}

// --- Agent wiring -------------------------------------------------------

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

// startAgent spawns `bub acp` and speaks the handshake. It runs off the main
// thread.
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

// installHandlers routes the agent's notifications to the window's state, on
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

// agentGone marks the window offline and keeps what it shows usable.
func (a *app) agentGone(err error) {
	a.client = nil
	a.sessionID = ""
	a.status = statusOffline
	a.busy = false
	a.activity = ""
	a.finishStreaming()
	a.dropEmptyReplies()
	if lost := a.proc; lost != nil {
		a.proc = nil
		go func() { _ = lost.Close() }()
	}
	if err == nil || errors.Is(err, acp.ErrClosed) {
		a.errText = "The Bub agent stopped. Press Retry to start it again."
	} else {
		a.errText = "The Bub agent stopped: " + err.Error() + ". Press Retry to start it again."
	}
}

// reconnect replaces the agent process, keeping the transcript.
func (a *app) reconnect() {
	a.gen++
	old := a.proc
	a.proc, a.client, a.sessionID = nil, nil, ""
	a.busy = false
	a.activity = ""
	a.errText = ""
	a.finishStreaming()
	if old != nil {
		go func() { _ = old.Close() }()
	}
	a.connect()
}

// shutdown stops the agent when the window closes.
func (a *app) shutdown() {
	a.gen++
	if a.proc != nil {
		_ = a.proc.Close()
		a.proc = nil
	}
	a.client = nil
}

// --- Turn -----------------------------------------------------------------

// send sends what the composer holds.
func (a *app) send() {
	a.sendText(a.input)
}

func (a *app) sendText(text string) {
	text = strings.TrimSpace(text)
	if text == "" || a.busy {
		return
	}
	a.input = ""
	a.errText = ""
	a.messages = append(a.messages, &item{id: a.nextID, kind: itemUser, text: text})
	a.nextID++

	client, session := a.client, a.sessionID
	if client == nil || session == "" {
		a.errText = "Not connected to a Bub agent. Start one with BUB_ACP_CMD, or press Retry."
		return
	}
	reply := &item{id: a.nextID, kind: itemAssistant}
	a.nextID++
	a.messages = append(a.messages, reply)
	a.busy = true
	a.activity = ""
	gen := a.gen
	go func() {
		stop, err := client.Prompt(context.Background(), session, []acp.ContentBlock{acp.TextBlock(text)})
		a.dispatch(func() {
			if a.gen != gen {
				return
			}
			a.busy = false
			a.activity = ""
			a.finishStreaming()
			a.dropEmptyReplies()
			if err != nil && !errors.Is(err, acp.ErrClosed) {
				a.errText = "The turn failed: " + err.Error()
			}
			switch stop {
			case acp.StopMaxTokens:
				if a.errText == "" {
					a.errText = "The agent reached its token limit and stopped the turn."
				}
			case acp.StopRefusal:
				if a.errText == "" {
					a.errText = "The agent refused to continue this turn."
				}
			}
		})
	}()
}

// stop asks the agent to cancel the running turn.
func (a *app) stop() {
	client, session := a.client, a.sessionID
	if client == nil || session == "" || !a.busy {
		return
	}
	go func() {
		if err := client.Cancel(session); err != nil {
			a.dispatch(func() {
				if a.client != client {
					return
				}
				a.errText = "Could not stop the agent: " + err.Error()
			})
		}
	}()
}

// finishStreaming marks the messages still coming as done, so they render as
// finished Markdown.
func (a *app) finishStreaming() {
	for _, m := range a.messages {
		if m.kind == itemAssistant && !m.done {
			m.done = true
		}
	}
}

// dropEmptyReplies takes out assistant rows a turn left empty, as a failure or
// a cancel before any text does.
func (a *app) dropEmptyReplies() {
	kept := a.messages[:0]
	for _, m := range a.messages {
		if m.kind == itemAssistant && m.text == "" && m.thought == "" {
			continue
		}
		kept = append(kept, m)
	}
	a.messages = kept
}

// applyUpdate folds one session/update notification into the transcript. It
// runs on the main thread.
func (a *app) applyUpdate(u acp.Update) {
	switch u.Type {
	case acp.UpdateAgentMessageChunk:
		if u.Text != "" {
			m := a.openAssistant()
			m.text += u.Text
		}
	case acp.UpdateAgentThoughtChunk:
		if u.Text != "" {
			m := a.openAssistant()
			m.thought += u.Text
		}
	case acp.UpdateUserMessageChunk:
		// The window already shows what the user wrote.
	case acp.UpdateToolCall:
		if u.ToolCall != nil {
			a.applyToolCall(u.ToolCall)
		}
	case acp.UpdateToolCallUpdate:
		if u.ToolCallUpdate != nil {
			a.applyToolCallUpdate(u.ToolCallUpdate)
		}
	}
}

// openAssistant returns the assistant message being streamed, starting one
// when the last row is something else.
func (a *app) openAssistant() *item {
	if n := len(a.messages); n > 0 {
		last := a.messages[n-1]
		if last.kind == itemAssistant && !last.done {
			return last
		}
	}
	m := &item{id: a.nextID, kind: itemAssistant}
	a.nextID++
	a.messages = append(a.messages, m)
	return m
}

// closeAssistant ends the message being streamed, as a tool call does once the
// text before it is complete.
func (a *app) closeAssistant() {
	if n := len(a.messages); n > 0 {
		last := a.messages[n-1]
		if last.kind == itemAssistant && !last.done {
			last.done = true
		}
	}
}

func (a *app) applyToolCall(call *acp.ToolCall) {
	a.closeAssistant()
	tool := a.findTool(call.ToolCallID)
	if tool == nil {
		tool = &toolState{id: call.ToolCallID}
		a.messages = append(a.messages, &item{id: a.nextID, kind: itemTool, tool: tool})
		a.nextID++
	}
	tool.title = call.Title
	tool.status = call.Status
	tool.input = previewJSON(call.RawInput)
	tool.output = previewJSON(call.RawOutput)
	a.setActivity(tool)
}

func (a *app) applyToolCallUpdate(update *acp.ToolCallUpdate) {
	tool := a.findTool(update.ToolCallID)
	if tool == nil {
		// An update for a call the window never saw: show it rather than
		// drop it, so a late result is not lost.
		tool = &toolState{id: update.ToolCallID}
		a.messages = append(a.messages, &item{id: a.nextID, kind: itemTool, tool: tool})
		a.nextID++
	}
	if update.Title != nil && *update.Title != "" {
		tool.title = *update.Title
	}
	if update.Status != nil {
		tool.status = *update.Status
	}
	if input := previewJSON(update.RawInput); input != "" {
		tool.input = input
	}
	if output := previewJSON(update.RawOutput); output != "" {
		tool.output = output
	}
	a.setActivity(tool)
}

// findTool looks a tool call up by its id.
func (a *app) findTool(id string) *toolState {
	if id == "" {
		return nil
	}
	for _, m := range a.messages {
		if m.kind == itemTool && m.tool != nil && m.tool.id == id {
			return m.tool
		}
	}
	return nil
}

// setActivity says what the pill shows: a tool running, or nothing once it
// finished.
func (a *app) setActivity(tool *toolState) {
	switch tool.status {
	case acp.ToolStatusPending, acp.ToolStatusInProgress:
		a.activity = "Running " + toolTitle(tool) + "…"
	default:
		a.activity = ""
	}
}

// --- Small helpers --------------------------------------------------------

func toolTitle(tool *toolState) string {
	switch {
	case tool.title != "":
		return tool.title
	case tool.id != "":
		return "tool " + tool.id
	default:
		return "tool call"
	}
}

func toolGlyph(status string) string {
	switch status {
	case acp.ToolStatusCompleted:
		return "✓"
	case acp.ToolStatusFailed:
		return "✕"
	case acp.ToolStatusInProgress:
		return "●"
	default:
		return "○"
	}
}

func toolStatusStyle(status string) (string, ui.Color) {
	switch status {
	case acp.ToolStatusCompleted:
		return "completed", colorSuccess
	case acp.ToolStatusFailed:
		return "failed", colorDanger
	case acp.ToolStatusInProgress:
		return "in progress", colorInk
	case "":
		return "pending", colorMuted
	default:
		return status, colorMuted
	}
}

// toolPreview is the arguments a running tool got, or what it returned.
func toolPreview(tool *toolState) string {
	if tool.output != "" {
		return tool.output
	}
	return tool.input
}

// previewJSON makes a one-line preview of a raw ACP value, unquoting a JSON
// string so a tool's output reads as text rather than as its encoding.
func previewJSON(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" || s == "{}" {
		return ""
	}
	if s[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			s = text
		}
	}
	s = strings.Join(strings.Fields(s), " ")
	const max = 180
	if len(s) > max {
		s = strings.TrimSpace(s[:max]) + "…"
	}
	return s
}

// firstLine trims a thought to its first line, for the small reasoning hint.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	const max = 160
	if len(s) > max {
		s = strings.TrimSpace(s[:max]) + "…"
	}
	return s
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
