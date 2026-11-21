package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/richtext"

	"github.com/bubbuild/bub/desktop/internal/acp"
	"github.com/bubbuild/bub/desktop/internal/appdata"
	"github.com/bubbuild/bub/desktop/internal/prompt"
	"github.com/bubbuild/bub/desktop/internal/workspace"
)

// chatSuggestions are the empty state's one-line ways into the conversation.
var chatSuggestions = []string{
	"Make this clearer",
	"Find the strongest line",
	"Continue this page",
}

// commandKindSkill is the ACP command kind the agent uses for a skill it can
// expand, as opposed to a command line it runs.
const commandKindSkill = "skill"

// chatState is the conversation in the rail and the one turn it is running.
//
// A turn streams its answer into the last message; gen identifies it, so a
// callback from a turn the writer replaced does nothing. One prompt is in
// flight at a time (inFlight): a message sent meanwhile is cancelled in and
// waits in pending, so an ACP session never runs two turns at once.
type chatState struct {
	conv        appdata.Conversation
	streaming   bool // chunks belong to the answer arriving now
	inFlight    bool // one prompt runs; what comes next waits in pending
	sessionID   string
	gen         int
	input       string
	attachments []string      // files for the next turn, by path
	err         string        // the turn's failure, shown inline with a Retry
	retry       chatRequest   // the turn an inline Retry sends again
	pending     []chatRequest // turns sent while one was in flight
	abort       chan struct{} // closed when the turn outlives the conversation

	saving      bool // a snapshot is being written
	savePending bool // ... and another is owed when it lands

	// contextOpen is the composer's add-context popover: the one place that
	// attaches a page or a file to the conversation.
	contextOpen bool

	scroll            ui.ScrollState     // where the transcript shows, kept across frames
	history           bool               // the recent-chats popover is open
	configOptions     []acp.ConfigOption // selectors the session advertised
	connectingSession bool               // a session is being created for the options
	md                markdown.Options

	// The composer's mention and command palette, and the trigger Escape
	// dismissed so the same token does not reopen it.
	menu        *composerMenu
	menuDismiss int // rune offset of the dismissed trigger, -1 for none

	// pendingEdit is a palette pick waiting to be applied to the composer
	// field on the next build, when the field's element is valid again.
	pendingEdit *composerEdit

	// copied is the answer action that just copied, for its check mark; it
	// clears itself after a moment.
	copied string
}

// composerEdit is one palette pick, applied to the composer's field during a
// build: ReplaceText with the choice, from the opened trigger to the caret.
type composerEdit struct {
	start, caret int
	insert       string
}

// composerMenu is the composer's open palette: "@" mentions the workspace's
// pages, "/" lists the commands and skills the agent advertised.
type composerMenu struct {
	kind  byte   // '@' or '/'
	start int    // rune offset of the trigger in the draft
	caret int    // rune offset of the caret when it opened
	query string // what was typed after the trigger
	sel   int    // the highlighted row
}

// composerItem is one row of the composer palette.
type composerItem struct {
	title  string // "@The quiet web" or "/model"
	detail string // the page's path, or what the command does
	insert string // what choosing the row puts in the draft
	cmd    acp.AvailableCommand
	icon   *ui.SVG
}

// chatRequest is one turn: what to send, and the files it carries.
type chatRequest struct {
	instruction string
	// prompt overrides what the agent receives, for a command, which travels
	// as the agent's own command line rather than as a page-context question.
	prompt string
	// hint is the "$name args" form of a picked skill, which the agent
	// expands; it rides inside the usual page-context question.
	hint        string
	attachments []string
}

// newChat starts an unsaved conversation and shows it on the chat page,
// stopping any turn the previous conversation was running.
func (a *app) newChat() {
	a.resetChat()
	a.mode = viewChat
	a.railShown = false
	a.chatFocus = true
}

// newChatInRail starts an unsaved conversation but keeps the rail, for the
// rail's own New chat button.
func (a *app) newChatInRail() {
	a.resetChat()
	a.railShown = true
}

// newChatAboutPage starts an unsaved conversation and attaches page as its
// first reference, for a page the writer asks about from its row.
func (a *app) newChatAboutPage(rel string) {
	a.resetChat()
	a.addChatReference(a.chat, rel)
	a.mode = viewChat
	a.railShown = false
	a.chatFocus = true
}

// resetChat starts an unsaved, empty conversation. It references no page: a
// new chat never inherits the page that happened to be open. The writer
// attaches the pages and files it reads in the composer.
func (a *app) resetChat() {
	a.detachChat()
	a.chat = &chatState{conv: appdata.NewConversation()}
	a.chat.scroll.Y = math.MaxFloat32
	a.railTab = railChat
}

// loadChat replaces the conversation in the rail with a saved one. It restores
// the conversation's references, not a page: opening a chat does not move the
// writer to a page it was once about.
func (a *app) loadChat(id string) bool {
	if a.chats == nil || strings.TrimSpace(id) == "" {
		return false
	}
	if a.chat != nil && a.chat.conv.ID == id {
		return true
	}
	conv, err := a.chats.Get(id)
	if err != nil {
		log.Printf("bub-desktop: chat %s: %v", id, err)
		return false
	}
	a.detachChat()
	a.chat = &chatState{conv: *conv, configOptions: a.chatConfigs[id]}
	a.chat.scroll.Y = math.MaxFloat32
	a.railTab = railChat
	return true
}

// ensureChatSession creates the ACP session a conversation uses before its
// first turn, so the composer can show the model and thinking options the
// agent advertises. It never blocks the frame; the options arrive when the
// session does.
func (a *app) ensureChatSession(chat *chatState) {
	if chat == nil || a.client == nil || chat.sessionID != "" || chat.connectingSession {
		return
	}
	convID := chat.conv.ID
	if convID == "" {
		return
	}
	if session := a.chatSessions[convID]; session != "" {
		chat.sessionID = session
		chat.configOptions = a.chatConfigs[convID]
		return
	}
	client := a.client
	cwd := a.chatWorkspaceDir()
	chat.connectingSession = true
	go func() {
		result, err := client.NewSession(context.Background(), cwd)
		a.dispatch(func() {
			chat.connectingSession = false
			if err != nil {
				log.Printf("bub-desktop: chat session: %v", err)
				return
			}
			a.chatSessions[convID] = result.SessionID
			a.chatConfigs[convID] = result.ConfigOptions
			if a.chat == chat {
				chat.sessionID = result.SessionID
				chat.configOptions = result.ConfigOptions
			}
		})
	}()
}

// openChat shows a saved conversation on the chat page.
func (a *app) openChat(id string) {
	if !a.loadChat(id) {
		return
	}
	a.mode = viewChat
	a.railShown = false
	a.chatFocus = true
}

// openChatInRail shows a saved conversation in the rail, as its recent-chats
// popover does.
func (a *app) openChatInRail(id string) {
	if !a.loadChat(id) {
		return
	}
	a.railShown = true
	a.railTab = railChat
}

// detachChat stops the conversation in the rail before another replaces it,
// keeping what the agent already wrote.
func (a *app) detachChat() {
	chat := a.chat
	if chat == nil || (!chat.inFlight && !chat.streaming) {
		return
	}
	a.cancelChat(chat)
	abortChatTurn(chat)
	chat.streaming = false
	chat.inFlight = false
	chat.pending = nil
	chat.gen = 0
	dropEmptyAssistant(chat)
	a.saveChat(chat)
}

// ensureChat returns the rail's conversation, starting an empty one when the
// rail has none yet. It references no page of its own.
func (a *app) ensureChat() *chatState {
	chat := a.chat
	if chat == nil {
		chat = &chatState{conv: appdata.NewConversation()}
		chat.scroll.Y = math.MaxFloat32
		a.chat = chat
	} else if chat.conv.ID == "" {
		chat.conv = appdata.NewConversation()
	}
	a.railTab = railChat
	return chat
}

// sendChat sends what the composer holds: its draft and its attachments.
func (a *app) sendChat() {
	chat := a.ensureChat()
	a.sendChatDraft(chat, strings.TrimSpace(chat.input))
}

// sendChatDraft takes the draft and attachments of the composer and starts a
// turn, clearing the composer. A draft with attachments and no words is a
// turn too: the files are the message.
func (a *app) sendChatDraft(chat *chatState, instruction string) {
	if chat == nil {
		return
	}
	instruction = strings.TrimSpace(instruction)
	attachments := append([]string(nil), chat.attachments...)
	chat.input = ""
	chat.attachments = nil
	chat.menu = nil
	chat.menuDismiss = -1
	if instruction == "" && len(attachments) == 0 {
		return
	}
	a.sendChatRequest(chat, a.newChatRequest(instruction, attachments))
}

// sendChatPrompt adds the writer's message and asks the agent for an answer,
// cancelling the turn that still runs. It carries no attachments of its own,
// and still resolves the draft's "@" mentions like any other turn.
func (a *app) sendChatPrompt(instruction string) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return
	}
	a.sendChatRequest(a.ensureChat(), a.newChatRequest(instruction, nil))
}

// newChatRequest turns the composer's draft into the turn to send. A draft
// that opens with an advertised "/command" becomes the agent's command line —
// or, for a skill, a hint the agent expands — and never the page-context
// wrapper. Otherwise the draft's "@page" mentions ride along as resource
// links, so a reference works whether the writer attached it or typed it.
func (a *app) newChatRequest(instruction string, attachments []string) chatRequest {
	instruction = strings.TrimSpace(instruction)
	if req, ok := a.commandRequest(instruction); ok {
		return req
	}
	for _, path := range a.mentionFiles(instruction) {
		if !stringIn(path, attachments) {
			attachments = append(attachments, path)
		}
	}
	return chatRequest{instruction: instruction, attachments: attachments}
}

// commandRequest is the agent call a "/name args" draft asks for, when the
// agent advertised that command. A command travels to the agent with its own
// prefix and no page context, because the agent runs it instead of answering;
// a skill travels as its "$name" hint, which the agent expands in the system
// prompt while it answers the rest of the draft.
func (a *app) commandRequest(instruction string) (chatRequest, bool) {
	instruction = strings.TrimSpace(instruction)
	name, args := splitDraftCommand(instruction)
	if !strings.HasPrefix(name, "/") {
		return chatRequest{}, false
	}
	want := strings.TrimPrefix(name, "/")
	for _, cmd := range a.commands {
		if cmd.Name != want {
			continue
		}
		if cmd.Kind == commandKindSkill {
			hint := "$" + cmd.Name
			if args != "" {
				hint += " " + args
			}
			return chatRequest{instruction: instruction, hint: hint}, true
		}
		line := a.commandPrefixFor(want)
		if args != "" {
			line += " " + args
		}
		return chatRequest{instruction: instruction, prompt: line}, true
	}
	return chatRequest{}, false
}

// splitDraftCommand splits "/name args" into its command name and arguments.
func splitDraftCommand(draft string) (name, args string) {
	draft = strings.TrimSpace(draft)
	if i := strings.IndexAny(draft, " \t\n"); i >= 0 {
		return draft[:i], strings.TrimSpace(draft[i+1:])
	}
	return draft, ""
}

// commandPrefixFor is the agent's command prefix, which the composer never
// shows: it asks for "/help" and the agent gets its own "/help". ACP agents
// parse the slash commands clients render, so "/" is the default.
func (a *app) commandPrefixFor(name string) string {
	prefix := a.commandPrefix
	if prefix == "" {
		prefix = "/"
	}
	return prefix + name
}

// mentionFiles resolves the "@" mentions in a draft to the pages they name, so
// a reference typed into the sentence is carried as an ACP resource link too.
func (a *app) mentionFiles(instruction string) []string {
	if a.ws == nil || !strings.Contains(instruction, "@") {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, page := range workspace.Pages(a.pages) {
		if !hasMention(instruction, page.Rel) {
			continue
		}
		abs, err := a.ws.Abs(page.Rel)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	return out
}

// hasMention reports whether text names the page at rel after an "@", on a
// word boundary, so "see @notes/a.md," still matches the page.
func hasMention(text, rel string) bool {
	needle := "@" + rel
	for from := 0; from < len(text); {
		i := strings.Index(text[from:], needle)
		if i < 0 {
			return false
		}
		i += from
		before := i == 0 || text[i-1] == ' ' || text[i-1] == '\t' || text[i-1] == '\n'
		end := i + len(needle)
		after := end == len(text) || mentionEnd(text[end])
		if before && after {
			return true
		}
		from = i + 1
	}
	return false
}

// mentionEnd reports whether c ends a mention token.
func mentionEnd(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', ',', '.', ';', ':', '!', '?', ')', ']', '}', '"', '\'':
		return true
	}
	return false
}

// stringIn reports whether values holds want.
func stringIn(want string, values []string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// sendChatRequest appends the writer's turn and sends it to the agent.
func (a *app) sendChatRequest(chat *chatState, req chatRequest) {
	if chat == nil {
		return
	}
	chat.err = ""
	chat.conv.Messages = append(chat.conv.Messages, appdata.Message{
		Role:        appdata.RoleUser,
		Text:        req.instruction,
		Attachments: req.attachments,
		At:          time.Now().UTC(),
	})
	chat.scroll.Y = math.MaxFloat32
	a.saveChat(chat)
	a.queueChatTurn(chat, req)
}

// retryChat sends the failed turn again, without repeating the writer's
// message in the transcript.
func (a *app) retryChat() {
	chat := a.chat
	if chat == nil || (strings.TrimSpace(chat.retry.instruction) == "" && len(chat.retry.attachments) == 0) {
		return
	}
	chat.err = ""
	a.queueChatTurn(chat, chat.retry)
}

// queueChatTurn starts a turn, or cancels the one in flight and waits for it
// to end before starting the next.
func (a *app) queueChatTurn(chat *chatState, req chatRequest) {
	if chat == nil {
		return
	}
	if chat.inFlight {
		chat.pending = append(chat.pending, req)
		a.cancelChat(chat)
		return
	}
	a.runChatTurn(chat, req)
}

// runChatTurn appends the answer the agent will stream into and sends the
// prompt. The session is created on first use, one per conversation.
func (a *app) runChatTurn(chat *chatState, req chatRequest) {
	if chat == nil || a.chat != chat {
		return
	}
	chat.retry = req
	if a.client == nil {
		a.failChat("Bub is not connected.")
		detail := a.connectErr
		if detail == "" {
			detail = "The Bub agent is not running."
		}
		a.notifyError("Bub is not connected", detail, a.reconnect)
		return
	}
	chat.conv.Messages = append(chat.conv.Messages, appdata.Message{
		Role: appdata.RoleAssistant,
		At:   time.Now().UTC(),
	})
	chat.streaming = true
	chat.inFlight = true
	chat.err = ""
	a.aiGen++
	gen := a.aiGen
	chat.gen = gen
	chat.scroll.Y = math.MaxFloat32
	chat.abort = make(chan struct{})
	abort := chat.abort

	client := a.client
	convID := chat.conv.ID
	session := a.chatSessions[convID]
	chat.sessionID = session
	cwd := a.chatWorkspaceDir()
	refs := append([]string(nil), chat.conv.References...)
	instruction := req.instruction
	written := req.prompt
	if written == "" {
		if req.hint != "" {
			// A picked skill travels as its "$name" hint inside the usual
			// context question, so the agent expands it and answers.
			instruction = req.hint
		}
		if instruction == "" {
			instruction = "These are the attached files for this turn."
		}
		written = prompt.Chat(instruction, a.chatPromptContext(refs))
	}
	// The files are resolved here, off the conversation; the prompt blocks,
	// which read images off disk, are built inside the turn's goroutine. The
	// agent's image capability is captured with the session.
	attachments := a.chatAttachmentsForTurn(refs, req.attachments)
	images := a.agentImages
	go func() {
		sessionID := session
		if sessionID == "" {
			result, err := client.NewSession(context.Background(), cwd)
			if err != nil {
				a.dispatch(func() { a.finishChat(gen, "", err) })
				return
			}
			sessionID = result.SessionID
			a.dispatch(func() {
				a.chatSessions[convID] = sessionID
				a.chatConfigs[convID] = result.ConfigOptions
				if a.chat != nil && a.chat.conv.ID == convID {
					a.chat.sessionID = sessionID
					a.chat.configOptions = result.ConfigOptions
				}
			})
		}
		select {
		case <-abort:
			return // the conversation left the rail before its turn began
		default:
		}
		stop, err := client.Prompt(context.Background(), sessionID, chatPromptBlocks(written, attachments, images))
		a.dispatch(func() { a.finishChat(gen, stop, err) })
	}()
}

// chatWorkspaceDir is the workspace the ACP session runs in, as an absolute
// path.
func (a *app) chatWorkspaceDir() string {
	dir := a.settings.Workspace
	if a.ws != nil && strings.TrimSpace(a.ws.Root) != "" {
		dir = a.ws.Root
	}
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// finishChat ends a turn: it keeps a non-empty answer, drops an empty one, and
// starts what was sent meanwhile. An error becomes an inline note with a
// Retry; an agent that went away is the app's to report.
func (a *app) finishChat(gen int, stop string, err error) {
	chat := a.chat
	if chat == nil || chat.gen != gen || !chat.inFlight {
		return
	}
	chat.inFlight = false
	chat.streaming = false
	chat.abort = nil
	if err != nil {
		a.failChat(chatFailure(err))
		return
	}
	dropEmptyAssistant(chat)
	a.saveChat(chat)
	if len(chat.pending) > 0 {
		next := chat.pending[0]
		chat.pending = chat.pending[1:]
		a.runChatTurn(chat, next)
	}
}

// failChat shows a turn's failure in the panel, dropping the empty answer it
// would have streamed into.
func (a *app) failChat(reason string) {
	chat := a.chat
	if chat == nil {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "Bub could not answer."
	}
	chat.streaming = false
	chat.inFlight = false
	chat.pending = nil
	chat.gen = 0
	chat.abort = nil
	chat.err = reason
	dropEmptyAssistant(chat)
	a.saveChat(chat)
}

// stopChat stops the answer arriving now, keeping what has come.
func (a *app) stopChat() {
	chat := a.chat
	if chat == nil || !chat.streaming {
		return
	}
	chat.streaming = false
	chat.pending = nil
	chat.err = ""
	a.cancelChat(chat)
}

// abortChatTurn stops a turn that has not reached the agent yet from starting.
func abortChatTurn(chat *chatState) {
	if chat != nil && chat.abort != nil {
		close(chat.abort)
		chat.abort = nil
	}
}

// cancelChat asks the agent to stop the conversation's running turn.
func (a *app) cancelChat(chat *chatState) {
	client, session := a.client, ""
	if chat != nil {
		session = chat.sessionID
	}
	if client == nil || session == "" {
		return
	}
	go func() { _ = client.Cancel(session) }()
}

// appendChat folds a streamed chunk into the answer arriving now.
func (a *app) appendChat(delta string) {
	chat := a.chat
	if chat == nil || !chat.streaming || delta == "" {
		return
	}
	last := len(chat.conv.Messages) - 1
	if last < 0 || chat.conv.Messages[last].Role != appdata.RoleAssistant {
		return
	}
	chat.conv.Messages[last].Text += delta
	followChatEnd(chat)
}

// followChatEnd keeps the transcript at its end while tokens come, unless the
// writer scrolled up to read.
func followChatEnd(chat *chatState) {
	if chat != nil && chat.scroll.Y >= chat.scroll.MaxY {
		chat.scroll.Y = math.MaxFloat32
	}
}

// dropEmptyAssistant removes the answer placeholder when a turn produced
// nothing.
func dropEmptyAssistant(chat *chatState) {
	last := len(chat.conv.Messages) - 1
	if last < 0 {
		return
	}
	msg := chat.conv.Messages[last]
	if msg.Role == appdata.RoleAssistant && strings.TrimSpace(msg.Text) == "" {
		chat.conv.Messages = chat.conv.Messages[:last]
	}
}

// chatFailure is what an inline note says about a failed turn.
func chatFailure(err error) string {
	if errors.Is(err, acp.ErrClosed) {
		return "The agent stopped."
	}
	if message := strings.TrimSpace(friendlyError(err)); message != "" {
		return message
	}
	return "Bub could not answer."
}

// saveChat writes the conversation off the main thread, one snapshot at a
// time, and refreshes the saved list when it lands.
func (a *app) saveChat(chat *chatState) {
	if chat == nil || a.chats == nil || chat.conv.ID == "" || len(chat.conv.Messages) == 0 {
		return
	}
	if chat.saving {
		chat.savePending = true
		return
	}
	chat.saving = true
	conv := chat.conv
	conv.Messages = append([]appdata.Message(nil), chat.conv.Messages...)
	conv.References = append([]string(nil), chat.conv.References...)
	chats := a.chats
	go func() {
		err := chats.Save(&conv)
		a.dispatch(func() {
			chat.saving = false
			if err != nil {
				log.Printf("bub-desktop: save chat: %v", err)
			} else {
				a.reloadChats()
			}
			if chat.savePending {
				chat.savePending = false
				a.saveChat(chat)
			}
		})
	}()
}

// chatPromptContext gathers the context a turn carries: the pages the
// conversation references, and, for the referenced page that happens to be
// open, the title, outline and caret the settings allow. A page that is merely
// open is not context.
func (a *app) chatPromptContext(refs []string) prompt.ChatContext {
	ctx := prompt.ChatContext{Refs: append([]string(nil), refs...)}
	if a.current == "" || !stringIn(a.current, ctx.Refs) {
		return ctx
	}
	ctx.Title = a.documentTitle()
	if a.settings.AIContextOutline {
		for _, entry := range a.outline() {
			level := entry.Level
			if level < 1 {
				level = 1
			}
			ctx.Outline = append(ctx.Outline, strings.Repeat("#", level)+" "+entry.Text)
		}
	}
	caret := a.caretBlock()
	if caret != nil {
		ctx.Caret = truncate(strings.TrimSpace(caret.Text), 480)
	}
	if a.settings.AIContextNeighbors && caret != nil {
		ctx.Prev, ctx.Next = contextAround(a.blocks(), caret.Index)
	}
	return ctx
}

// chatAttachmentsForTurn is every file a turn carries: the composer's files,
// then the conversation's referenced pages as absolute paths, each once.
func (a *app) chatAttachmentsForTurn(refs, files []string) []string {
	out := make([]string, 0, len(files)+len(refs))
	for _, path := range files {
		path = strings.TrimSpace(path)
		if path == "" || stringIn(path, out) {
			continue
		}
		out = append(out, path)
	}
	for _, rel := range refs {
		rel = strings.TrimSpace(rel)
		if rel == "" || a.ws == nil {
			continue
		}
		abs, err := a.ws.Abs(rel)
		if err != nil || stringIn(abs, out) {
			continue
		}
		out = append(out, abs)
	}
	return out
}

// addChatReference attaches a page to the conversation's context, if it is not
// there yet.
func (a *app) addChatReference(chat *chatState, rel string) {
	rel = strings.TrimSpace(rel)
	if chat == nil || rel == "" || stringIn(rel, chat.conv.References) {
		return
	}
	chat.conv.References = append(chat.conv.References, rel)
}

// removeChatReference takes a page off the conversation's context.
func removeChatReference(chat *chatState, rel string) {
	if chat == nil {
		return
	}
	refs := chat.conv.References[:0]
	for _, ref := range chat.conv.References {
		if ref == rel {
			continue
		}
		refs = append(refs, ref)
	}
	chat.conv.References = refs
}

// referenceOpenPage attaches the open page to the conversation, for the
// composer's add-context menu.
func (a *app) referenceOpenPage(chat *chatState) {
	if strings.TrimSpace(a.current) == "" {
		return
	}
	a.addChatReference(chat, a.current)
}

// chatReferenceName names a referenced page in a chip: the open page's title,
// else the page's title in the workspace tree, else the file's name.
func (a *app) chatReferenceName(rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return ""
	}
	if rel == a.current {
		if title := strings.TrimSpace(a.documentTitle()); title != "" {
			return title
		}
	}
	for _, page := range workspace.Pages(a.pages) {
		if page.Rel == rel && strings.TrimSpace(page.Name) != "" {
			return page.Name
		}
	}
	return strings.TrimSuffix(filepath.Base(rel), ".md")
}

// chatReferenceSummary names the conversation's context in one phrase: the
// first referenced page, and a count of the rest.
func (a *app) chatReferenceSummary(chat *chatState) string {
	if chat == nil || len(chat.conv.References) == 0 {
		return ""
	}
	first := a.chatReferenceName(chat.conv.References[0])
	if len(chat.conv.References) == 1 {
		return first
	}
	return fmt.Sprintf("%s +%d", first, len(chat.conv.References)-1)
}

// chatReferencesOpen reports whether the open page is already a reference.
func chatReferencesOpen(chat *chatState, rel string) bool {
	return chat != nil && rel != "" && stringIn(rel, chat.conv.References)
}

// --- The rail -------------------------------------------------------------

// chatPanel is the AI conversation in the rail: what was said, the composer,
// and the conversation's few controls.
func (a *app) chatPanel(c *ui.Context) {
	chat := a.chat
	if chat == nil {
		chat = &chatState{conv: appdata.NewConversation()}
		chat.scroll.Y = math.MaxFloat32
		a.chat = chat
	}
	a.ensureChatSession(chat)
	ui.Column(c).Grow(1).FillWidth().Children(func() {
		a.chatHeader(c) // may start another conversation
		chat = a.chat
		if len(chat.conv.Messages) == 0 && !chat.streaming && !chat.inFlight {
			a.chatEmpty(c)
		} else {
			a.chatMessages(c)
		}
		a.chatComposer(c)
		if chat.history {
			a.chatHistory(c)
		}
	})
}

// chatHeader is the conversation's title and the two things that change it.
func (a *app) chatHeader(c *ui.Context) {
	chat := a.chat
	t := c.Theme()
	ui.Row(c).Height(34).Padding(0, 12).Gap(2).AlignItems(ui.Center).Children(func() {
		ui.Text(c, chatTitle(chat)).FontSize(tokens.FontBody).FontWeight(tokens.WeightStrong).TextColor(t.Text).Grow(1).SingleLine()
		if iconButton(c, "chat-history", icoChat, "Recent chats", railIcon, chat.history, true, true) {
			chat.history = !chat.history
		}
		if iconButton(c, "chat-new", icoPlus, "New chat", railIcon, false, true, true) {
			a.newChatInRail()
		}
	})
}

// chatHistory is the small popover of saved conversations behind the history
// button.
func (a *app) chatHistory(c *ui.Context) {
	chat := a.chat
	t := c.Theme()
	const limit = 8
	rows := a.chatList
	if len(rows) > limit {
		rows = rows[:limit]
	}
	panel := ui.Column(c).Key("chat-recent").Absolute().Right(8).Top(38).Width(252).
		Padding(6).Radius(tokens.RadiusLG).Background(t.Background).Border(1, t.Border).
		Shadow(0, 8, 22, -10, ui.RGBA(0, 0, 0, 0.18)).Gap(1)
	panel.Children(func() {
		if len(rows) == 0 {
			ui.Text(c, "No saved chats yet.").FontSize(tokens.FontCaption).TextColor(t.TextMuted).
				Padding(3, 4, 4, 4).LineHeight(1.45)
			return
		}
		for _, conv := range rows {
			conv := conv
			title := chatListTitle(conv)
			current := chat != nil && chat.conv.ID == conv.ID
			row := ui.Row(c).Key("chat-saved-"+conv.ID).Height(30).Padding(0, 6).Radius(tokens.RadiusSM).
				AlignItems(ui.Center).Gap(8).Focusable()
			if row.Hovered() {
				row.Background(t.SurfaceHover)
			}
			if row.Clicked() {
				a.openChatInRail(conv.ID)
				if a.chat != nil {
					a.chat.history = false
				}
			}
			row.Children(func() {
				color := t.Text
				if current {
					color = t.Accent
				}
				ui.Text(c, title).FontSize(tokens.FontBody).TextColor(color).Grow(1).SingleLine()
				ui.Text(c, shortTime(conv.UpdatedAt)).FontSize(tokens.FontMicro).TextColor(t.TextMuted).SingleLine()
			})
			row.Label(title)
		}
	})
	if panel.PressedOutside() {
		chat.history = false
	}
}

// chatEmpty is the panel before the first message: a mark, one line and the
// suggestions that start the conversation. The line names the context the
// writer attached, and asks what they are working on when there is none.
func (a *app) chatEmpty(c *ui.Context) {
	t := c.Theme()
	title := "What are you working on?"
	if summary := a.chatReferenceSummary(a.chat); summary != "" {
		title = "Ask Bub about " + summary
	}
	ui.Column(c).Grow(1).FillWidth().Padding(18).Justify(ui.Center).AlignItems(ui.Center).Gap(9).Children(func() {
		ui.Box(c).Size(34, 34).Radius(tokens.RadiusLG).Background(t.Accent.Alpha(0.12)).
			AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Icon(c, icoSparkle).Size(17, 17).TextColor(t.Accent)
		})
		ui.Text(c, truncate(title, 40)).FontSize(tokens.FontBody).FontWeight(tokens.WeightMedium).TextColor(t.Text).TextAlign(ui.Center)
		ui.Column(c).FillWidth().Gap(5).Padding(8, 0, 0, 0).Children(func() {
			for i, suggestion := range chatSuggestions {
				if chatSuggestion(c, i, suggestion) {
					a.sendChatPrompt(suggestion)
				}
			}
			if a.chatAddPageContext(c, a.chat, false) {
				a.addChatReference(a.chat, a.current)
			}
		})
		if a.chat.err != "" {
			a.chatError(c)
		}
	})
}

// chatSuggestion is one of the empty state's chips.
func chatSuggestion(c *ui.Context, index int, label string) bool {
	t := c.Theme()
	b := ui.Row(c).Key("chat-suggest-"+itoa(index)).FillWidth().Height(30).Padding(0, 10).
		Radius(tokens.RadiusSM).Gap(6).AlignItems(ui.Center).Background(t.Background).Border(1, t.Border).
		Focusable()
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	clicked := b.Clicked()
	b.Children(func() {
		ui.Icon(c, icoSparkle).Size(11, 11).TextColor(t.Accent)
		ui.Text(c, label).FontSize(tokens.FontControl).TextColor(t.Text).SingleLine()
	})
	b.Label(label)
	return clicked
}

// chatAddPageContext is the empty state's quiet way to give a fresh chat the
// open page as context. It is offered, never assumed: a new chat reads nothing
// until the writer presses it.
func (a *app) chatAddPageContext(c *ui.Context, chat *chatState, wide bool) bool {
	if chat == nil || strings.TrimSpace(a.current) == "" || chatReferencesOpen(chat, a.current) {
		return false
	}
	t := c.Theme()
	label := "Add this page as context"
	b := ui.Row(c).Key("chat-empty-context")
	if wide {
		b = b.Height(tokens.RowHeight).Padding(0, tokens.Space3).Radius(tokens.RadiusPill).Gap(6)
	} else {
		b = b.FillWidth().Height(30).Padding(0, 10).Radius(tokens.RadiusSM).Gap(6)
	}
	b = b.Border(1, t.Border).Background(t.Background).AlignItems(ui.Center).Focusable()
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	clicked := b.Clicked()
	b.Children(func() {
		ui.Icon(c, icoPlus).Size(11, 11).TextColor(t.TextMuted)
		ui.Text(c, label).FontSize(tokens.FontControl).TextColor(t.Text).SingleLine()
	})
	b.Label(label)
	return clicked
}

// chatMessages is the transcript, following the end while an answer streams.
func (a *app) chatMessages(c *ui.Context) {
	chat := a.chat
	// Every message shares one Options, so the look is assigned on it rather
	// than copied per view: Options holds a sync.Map, and a streaming answer
	// and the same answer once finished must not differ.
	a.chat.md.Theme = a.markdownLook(c, false)
	ui.Scroll(c).Grow(1).Shrink(1).FillWidth().MinHeight(0).TrackScroll(&chat.scroll).Children(func() {
		ui.Column(c).FillWidth().Padding(10, 12).Gap(14).Children(func() {
			for i := range chat.conv.Messages {
				a.chatMessage(c, i, chat.conv.Messages[i])
			}
			if chat.err != "" {
				a.chatError(c)
			}
		})
	})
}

// chatMessage is one turn: the writer's words in a quiet block, the agent's
// answer as Markdown, and small actions under a finished answer.
//
// An answer is read-only, so it renders through markdown.View/Stream, whose
// DefaultCodeBlock gives code the full treatment: the language, a copy button,
// a bordered panel and token highlighting. The page body is different: there
// richtext.Editor paints its own quiet code panel (theme.go's editorLook) but
// has no such chrome, because that code is being written, not read.
func (a *app) chatMessage(c *ui.Context, index int, msg appdata.Message) {
	t := c.Theme()
	if msg.Role == appdata.RoleUser {
		ui.Box(c).FillWidth().Padding(10).Radius(tokens.RadiusMD).Background(t.Background).Border(1, t.Border).
			Children(func() {
				ui.Text(c, msg.Text).FontSize(tokens.FontBody).LineHeight(1.55).TextColor(t.Text).Selectable()
				if len(msg.Attachments) > 0 {
					a.chatSentAttachments(c, msg.Attachments, 200)
				}
			})
		return
	}
	last := index == len(a.chat.conv.Messages)-1
	streaming := a.chat.streaming && last
	col := ui.Column(c).FillWidth().Gap(4)
	hovered := col.Hovered()
	col.Children(func() {
		if strings.TrimSpace(msg.Text) == "" {
			if streaming {
				ui.Text(c, "Bub is thinking…").FontSize(tokens.FontControl).TextColor(t.TextMuted)
			}
			return
		}
		if streaming {
			markdown.Stream(c, msg.Text, &a.chat.md)
		} else {
			markdown.View(c, msg.Text, &a.chat.md)
		}
		if !streaming {
			a.chatAnswerActions(c, index, msg.Text, hovered, true)
		}
	})
}

// chatAnswerActions are the small things the writer may do with a finished
// answer. A chat is a chat: Copy the answer, and Regenerate the turn that
// just gave it. The rail, where the page sits beside the conversation, also
// lets the answer enter the page; the full chat page does not, because there
// the page is hidden and "Insert" would act on something the writer cannot
// see.
func (a *app) chatAnswerActions(c *ui.Context, index int, text string, hovered bool, allowInsert bool) {
	visible := !c.Preferences().Hover() || hovered
	ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
		if allowInsert {
			if iconButton(c, "chat-insert-"+itoa(index), icoInsert, "Insert", answerIcon, false, true, visible) {
				a.insertChatText(text)
			}
		}
		key := "chat-copy-" + itoa(index)
		copied := a.chat != nil && a.chat.copied == key
		ic, label := icoCopy, "Copy"
		if copied {
			ic, label = icoCheck, "Copied"
		}
		if iconButton(c, key, ic, label, answerIcon, false, true, visible) {
			c.WriteClipboard(text)
			a.markCopied(a.chat, key)
		}
		if a.chat.lastAnswer(index) {
			if iconButton(c, "chat-regen-"+itoa(index), icoRefresh, "Regenerate", answerIcon, false, true, visible) {
				a.regenerateChat(index)
			}
		}
	})
}

// lastAnswer reports whether index is the newest finished answer, the only
// one Regenerate may redo.
func (c *chatState) lastAnswer(index int) bool {
	return c != nil && index == len(c.conv.Messages)-1 && !c.streaming && !c.inFlight &&
		c.conv.Messages[index].Role == appdata.RoleAssistant
}

// markCopied shows the copy action's check for a moment, long enough to read.
func (a *app) markCopied(chat *chatState, key string) {
	if chat == nil {
		return
	}
	chat.copied = key
	time.AfterFunc(1400*time.Millisecond, func() {
		a.dispatch(func() {
			if a.chat == chat && chat.copied == key {
				chat.copied = ""
			}
		})
	})
}

// regenerateChat replaces the last finished answer with a fresh attempt at
// the turn that asked for it: the writer's message stays, its answer goes,
// and the same request runs again.
func (a *app) regenerateChat(index int) {
	chat := a.chat
	if chat == nil || chat.inFlight || chat.streaming {
		return
	}
	if index != len(chat.conv.Messages)-1 || chat.conv.Messages[index].Role != appdata.RoleAssistant {
		return
	}
	user := -1
	for i := index - 1; i >= 0; i-- {
		if chat.conv.Messages[i].Role == appdata.RoleUser {
			user = i
			break
		}
	}
	if user < 0 {
		return
	}
	last := chat.conv.Messages[user]
	req := a.newChatRequest(last.Text, append([]string(nil), last.Attachments...))
	chat.conv.Messages = chat.conv.Messages[:index]
	chat.err = ""
	chat.copied = ""
	a.saveChat(chat)
	a.queueChatTurn(chat, req)
}

// chatError is a turn's failure, inline with a Retry rather than a native
// message.
func (a *app) chatError(c *ui.Context) {
	chat := a.chat
	t := c.Theme()
	ui.Column(c).FillWidth().Padding(9).Radius(tokens.RadiusMD).Gap(5).Background(t.Warning.Alpha(0.12)).
		Children(func() {
			ui.Text(c, chat.err).FontSize(tokens.FontCaption).LineHeight(1.45).TextColor(t.Text)
			if chat.retry.instruction != "" || len(chat.retry.attachments) > 0 {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					if chatTextButton(c, "chat-retry", "Retry") {
						a.retryChat()
					}
				})
			}
		})
}

// chatTextButton is a small accent text action, as the error's Retry.
func chatTextButton(c *ui.Context, key any, label string) bool {
	t := c.Theme()
	b := ui.Box(c).Key(key).Height(20).Padding(0, 3).Radius(tokens.RadiusXS).AlignItems(ui.Center).
		Focusable()
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	clicked := b.Clicked()
	b.Children(func() {
		ui.Text(c, label).FontSize(tokens.FontCaption).FontWeight(tokens.WeightMedium).TextColor(t.Accent)
	})
	b.Label(label)
	return clicked
}

// chatComposer is the rail's message field: the same card as the chat page's,
// narrower, with just the attach plus and the round Send.
func (a *app) chatComposer(c *ui.Context) {
	a.chatComposerCard(c, a.chat, false)
}

// --- The chat page --------------------------------------------------------

// chatPageWidth is the reading measure of the full chat page.
const chatPageWidth = 680

// closeChat returns from the chat page to the page the conversation is about.
func (a *app) closeChat() {
	a.mode = viewPage
	a.railShown = false
	a.focusIn = true
}

// chatPage is the full conversation the sidebar's chats open into: a readable
// transcript column over a composer that can hold more than one line. It is
// the same conversation object as the rail, so opening a chat in either place
// keeps its turns and its draft.
func (a *app) chatPage(c *ui.Context) {
	chat := a.ensureChat()
	a.ensureChatSession(chat)
	ui.Column(c).FillHeight().FillWidth().MinHeight(0).
		Background(c.Theme().Background).Children(func() {
		if len(chat.conv.Messages) == 0 && !chat.streaming && !chat.inFlight {
			a.chatPageEmpty(c)
		} else {
			a.chatPageMessages(c)
		}
		a.chatPageComposer(c)
	})
}

// chatPageEmpty is the chat before its first message: what Bub can read, and a
// few ways to start, rather than an empty transcript.
func (a *app) chatPageEmpty(c *ui.Context) {
	chat := a.chat
	t := c.Theme()
	summary := a.chatReferenceSummary(chat)
	ui.Column(c).Grow(1).FillWidth().Padding(tokens.Space6, tokens.Space4).
		Justify(ui.Center).AlignItems(ui.Center).Gap(tokens.Space3).Children(func() {
		ui.Box(c).Size(52, 52).Radius(tokens.RadiusXL).Background(t.Accent.Alpha(0.12)).
			AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Icon(c, icoSparkle).Size(24, 24).TextColor(t.Accent)
		})
		title := "What are you working on?"
		description := "Start a conversation. Reference a page or attach files from the composer to give Bub context."
		if summary != "" {
			title = "Ask Bub about " + summary
			description = "Bub reads the pages you reference — a page's title, outline and the block at the caret, when the settings allow — and answers in Markdown."
		}
		ui.Text(c, title).FontSize(tokens.FontPage + 1).FontWeight(tokens.WeightStrong).
			TextColor(t.Text).TextAlign(ui.Center)
		ui.Text(c, description).FontSize(tokens.FontCaption).TextColor(t.TextMuted).
			LineHeight(1.6).MaxWidth(400).TextAlign(ui.Center)
		ui.Row(c).Gap(tokens.Space2).Padding(tokens.Space2, 0, 0, 0).Children(func() {
			for i, suggestion := range chatSuggestions {
				if chatPageSuggestion(c, i, suggestion) {
					a.sendChatPrompt(suggestion)
				}
			}
		})
		if a.chatAddPageContext(c, chat, true) {
			a.addChatReference(chat, a.current)
		}
		if chat.err != "" {
			a.chatError(c)
		}
	})
}

// chatPageSuggestion is one pill of the empty state's ways into the chat.
func chatPageSuggestion(c *ui.Context, index int, label string) bool {
	t := c.Theme()
	b := ui.Box(c).Key("chat-page-suggest-"+itoa(index)).Height(tokens.RowHeight).
		Padding(0, tokens.Space3).Radius(tokens.RadiusPill).Background(t.Background).
		Border(1, t.Border).AlignItems(ui.Center).Justify(ui.Center).Focusable()
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	clicked := b.Clicked()
	b.Children(func() {
		ui.Text(c, label).FontSize(tokens.FontControl).TextColor(t.Text).SingleLine()
	})
	b.Label(label)
	return clicked
}

// chatPageMessages is the transcript, following the end while an answer
// streams. It is the rail's transcript at a page's measure and type size.
func (a *app) chatPageMessages(c *ui.Context) {
	chat := a.chat
	a.chat.md.Theme = a.markdownLook(c, true)
	// The scroll container spans the main area, so its scroll bar sits at the
	// window's edge; the transcript inside it keeps the reading measure.
	ui.Scroll(c).Grow(1).Shrink(1).FillWidth().MinHeight(0).TrackScroll(&chat.scroll).Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Column(c).Width(chatPageWidth).MaxWidthPercent(92).
				Padding(tokens.Space5, 0).Gap(tokens.Space5).Children(func() {
				for i := range chat.conv.Messages {
					a.chatPageMessage(c, i, chat.conv.Messages[i])
				}
				if chat.err != "" {
					a.chatError(c)
				}
			})
		})
	})
}

// chatPageMessage is one turn on the page: the writer's words as a quiet bubble
// on the end of the measure, the agent's answer as Markdown, and the small
// actions under a finished answer.
func (a *app) chatPageMessage(c *ui.Context, index int, msg appdata.Message) {
	t := c.Theme()
	if msg.Role == appdata.RoleUser {
		ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
			ui.Box(c).MaxWidth(chatPageWidth*0.8).Padding(10, tokens.Space3).
				Radius(tokens.RadiusLG).Background(t.Surface).Border(1, t.Border).
				Children(func() {
					ui.Text(c, msg.Text).FontSize(tokens.FontBody + 0.5).LineHeight(1.6).
						TextColor(t.Text).Selectable()
					if len(msg.Attachments) > 0 {
						a.chatSentAttachments(c, msg.Attachments, 320)
					}
				})
		})
		return
	}
	last := index == len(a.chat.conv.Messages)-1
	streaming := a.chat.streaming && last
	col := ui.Column(c).FillWidth().Gap(tokens.Space1)
	hovered := col.Hovered()
	col.Children(func() {
		if strings.TrimSpace(msg.Text) == "" {
			if streaming {
				ui.Text(c, "Bub is thinking…").FontSize(tokens.FontCaption).TextColor(t.TextMuted)
			}
			return
		}
		if streaming {
			markdown.Stream(c, msg.Text, &a.chat.md)
		} else {
			markdown.View(c, msg.Text, &a.chat.md)
		}
		if !streaming {
			a.chatAnswerActions(c, index, msg.Text, hovered, false)
		}
	})
}

// chatPageComposer is the page's composer: the shared message card at the
// reading measure, the context chips above its field, and the agent's state
// when it cannot answer. The keyboard line under the card teaches the
// affordances it carries.
func (a *app) chatPageComposer(c *ui.Context) {
	chat := a.chat
	t := c.Theme()
	width, _ := c.Size()
	ui.Column(c).FillWidth().Padding(tokens.Space2, tokens.Space3, tokens.Space5, tokens.Space3).
		Children(func() {
			a.chatComposerCard(c, chat, true)
			if width >= 900 && !chat.streaming && strings.TrimSpace(chat.input) == "" {
				ui.Text(c, "Enter sends · Shift+Enter keeps a line · @ mentions a page · / runs a command").
					FontSize(tokens.FontMicro).TextColor(t.TextMuted).
					Padding(tokens.Space2, 0, 0, 0).TextAlign(ui.Center).FillWidth()
			}
		})
}

// chatComposerCard draws the message field both chat surfaces share: the
// context chips above the field — referenced pages and attached files, which
// are the same kind of thing — then the growing field and one toolbar whose
// add-context button, agent options and round Send all sit on the same line.
// The alignment is the point: the context owns a row above the text, the
// controls own another, so nothing floats beside a line of text.
func (a *app) chatComposerCard(c *ui.Context, chat *chatState, wide bool) {
	if chat == nil {
		return
	}
	t := c.Theme()
	card := ui.Column(c).FillWidth()
	if wide {
		card.MaxWidth(chatPageWidth).AlignSelf(ui.Center).Radius(tokens.RadiusXL).
			Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.03)).
			Shadow(0, 14, 36, -24, ui.RGBA(0, 0, 0, 0.20))
	} else {
		card.Radius(tokens.RadiusLG).Shadow(0, 6, 18, -14, ui.RGBA(0, 0, 0, 0.12))
	}
	card.Border(1, t.Border).Background(t.Background)
	var input ui.Element
	var addContext ui.Element
	// The field starts roomy — three lines on the chat page, two in the rail —
	// and grows with the draft; past the cap it scrolls.
	minLines, maxLines := 2, 10
	if wide {
		minLines, maxLines = 3, 12
	}
	card.Children(func() {
		ui.Column(c).FillWidth().Padding(10, 12).Gap(8).Children(func() {
			if len(chat.conv.References) > 0 || len(chat.attachments) > 0 {
				a.chatContextChips(c, chat)
			}
			input = ui.TextAreaBase(c.Key("chat-input"), &chat.input).
				Placeholder("Message Bub…").Label("Message Bub").
				FontSize(tokens.FontBody).Lines(minLines, maxLines).FillWidth().
				Bind(&a.composer).
				OnTextKey(a.composerKeys(chat))
			if chat.pendingEdit != nil {
				edit := chat.pendingEdit
				chat.pendingEdit = nil
				input.ReplaceText(edit.start, edit.caret, edit.insert)
				a.chatFocus = true
			}
			ui.Row(c).FillWidth().Gap(tokens.Space2).AlignItems(ui.Center).Children(func() {
				addContext = a.chatAddContextButton(c, chat)
				if wide {
					a.chatConfigControls(c, chat)
				}
				ui.Spacer(c)
				if chat.streaming || len(chat.pending) > 0 {
					label := "Bub is answering…"
					if n := len(chat.pending); n > 0 {
						label = fmt.Sprintf("%d queued", n)
					}
					ui.Text(c, label).FontSize(tokens.FontMicro).TextColor(t.TextMuted).SingleLine()
				}
				a.chatSendButton(c, chat)
			})
			a.chatAddContextMenu(c, chat, addContext)
		})
	})
	if a.chatFocus {
		input.Focus()
		a.chatFocus = false
	}
	if input.Focused() {
		card.BorderColor(t.Focus)
	}
	caret, _ := input.TextSelection()
	a.updateComposerMenu(chat, a.composer.FocusWithin(c), caret)
	a.chatComposerPalette(c, chat, card, wide)
}

// composerKeys is the composer field's key handling: while the palette is open
// the arrow keys move its selection, Enter and Tab pick its row instead of
// sending, and Escape closes it. Otherwise Enter sends and Shift+Enter keeps
// a line.
func (a *app) composerKeys(chat *chatState) func(ui.TextKey) bool {
	return func(k ui.TextKey) bool {
		if chat.menu != nil {
			switch k.Key {
			case ui.KeyUp, ui.KeyDown:
				delta := 1
				if k.Key == ui.KeyUp {
					delta = -1
				}
				a.moveComposerSelection(chat, delta)
				return true
			case ui.KeyTab:
				a.stageComposerPick(chat, a.composerSelection(chat))
				return true
			case ui.KeyEnter:
				if k.Mods == 0 {
					a.stageComposerPick(chat, a.composerSelection(chat))
					return true
				}
			case ui.KeyEscape:
				dismissComposerMenu(chat)
				return true
			}
		}
		if k.Key != ui.KeyEnter {
			return false
		}
		if k.Mods != 0 && k.Mods != ui.Cmd {
			return false // Shift+Enter breaks the line
		}
		text := strings.TrimSpace(k.Text())
		if text == "" && len(chat.attachments) == 0 {
			return true
		}
		k.SetText("")
		a.sendChatDraft(chat, text)
		return true
	}
}

// chatSendButton is the composer's one round action: Send while the draft has
// something, Stop while an answer streams.
func (a *app) chatSendButton(c *ui.Context, chat *chatState) {
	t := c.Theme()
	if chat.streaming {
		b := ui.Box(c).Key("chat-stop").Size(30, 30).Radius(tokens.RadiusPill).
			Border(1, t.Border).Background(t.Surface).AlignItems(ui.Center).
			Justify(ui.Center).Focusable()
		if b.Hovered() {
			b.Background(t.SurfaceHover)
		}
		clicked := b.Clicked()
		b.Children(func() {
			ui.Icon(c, icoStop).Size(12, 12).TextColor(t.TextMuted)
		})
		b.Label("Stop")
		if clicked {
			a.stopChat()
		}
		return
	}
	enabled := strings.TrimSpace(chat.input) != "" || len(chat.attachments) > 0
	b := ui.Box(c).Key("chat-send").Size(30, 30).Radius(tokens.RadiusPill).
		AlignItems(ui.Center).Justify(ui.Center).Focusable()
	switch {
	case !enabled:
		b.Disabled(true).Background(t.Surface)
	case b.Pressed():
		b.Background(t.AccentPressed)
	case b.Hovered():
		b.Background(t.AccentHover)
	default:
		b.Background(t.Accent)
	}
	clicked := b.Clicked()
	color := t.AccentText
	if !enabled {
		color = t.TextMuted
	}
	b.Children(func() {
		ui.Icon(c, icoSend).Size(14, 14).TextColor(color)
	})
	b.Label("Send")
	if clicked && enabled {
		a.sendChat()
	}
}

// --- The composer's mention and command palette ---------------------------

// composerPaletteWidth is the menu's width on the page and in the rail.
const (
	composerPalettePage = 380
	composerPaletteRail = 264
)

// updateComposerMenu opens (or keeps, or closes) the palette from the draft's
// text and the caret: "@" names the page list, "/" the commands and skills
// the agent advertised. It closes when the trigger no longer applies, and
// keeps the selection while the query only grows. Escape's dismissal lasts
// for the trigger that dismissed it.
func (a *app) updateComposerMenu(chat *chatState, focused bool, caret int) {
	if chat == nil {
		return
	}
	if !focused {
		chat.menu = nil
		return
	}
	menu := composerTrigger(chat.input, caret)
	if chat.menuDismiss >= 0 {
		if menu != nil && menu.start == chat.menuDismiss {
			chat.menu = nil
			return
		}
		chat.menuDismiss = -1
	}
	if menu == nil {
		chat.menu = nil
		return
	}
	if chat.menu != nil && chat.menu.kind == menu.kind && chat.menu.start == menu.start && chat.menu.query == menu.query {
		return
	}
	chat.menu = menu
}

// composerTrigger finds the "@" or "/" token the caret sits in, or nil when
// the draft has none open.
func composerTrigger(text string, caret int) *composerMenu {
	runes := []rune(text)
	if caret < 0 || caret > len(runes) {
		return nil
	}
	i := caret - 1
	for i >= 0 {
		r := runes[i]
		if r == ' ' || r == '\t' || r == '\n' {
			return nil
		}
		if r == '@' || r == '/' {
			break
		}
		i--
	}
	if i < 0 {
		return nil
	}
	start := i
	kind := byte(runes[start])
	if kind == '/' && start != 0 {
		return nil // a command opens the draft
	}
	if kind == '@' && start > 0 && runes[start-1] != ' ' && runes[start-1] != '\t' && runes[start-1] != '\n' {
		return nil
	}
	query := string(runes[start+1 : caret])
	if strings.ContainsAny(query, " \t\n") {
		return nil
	}
	return &composerMenu{kind: kind, start: start, caret: caret, query: query}
}

// dismissComposerMenu closes the palette and remembers its trigger, so an
// Escape does not reopen it on the same token.
func dismissComposerMenu(chat *chatState) {
	if chat == nil || chat.menu == nil {
		return
	}
	chat.menuDismiss = chat.menu.start
	chat.menu = nil
}

// composerItems are the palette's rows for the open trigger.
func (a *app) composerItems(chat *chatState) []composerItem {
	if chat == nil || chat.menu == nil {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(chat.menu.query))
	if chat.menu.kind == '/' {
		return a.composerCommandItems(query)
	}
	return a.composerMentionItems(query)
}

// composerMentionItems are the workspace pages that match the "@" query.
func (a *app) composerMentionItems(query string) []composerItem {
	var items []composerItem
	for _, page := range workspace.Pages(a.pages) {
		if !composerMatch(query, page.Name, page.Rel) {
			continue
		}
		items = append(items, composerItem{
			title:  page.Name,
			detail: page.Rel,
			insert: "@" + page.Rel + " ",
			icon:   icoFile,
		})
	}
	return items
}

// composerCommandItems are the commands and skills the agent advertised that
// match the "/" query.
func (a *app) composerCommandItems(query string) []composerItem {
	var items []composerItem
	for _, cmd := range a.commands {
		if !composerMatch(query, cmd.Name, cmd.Description) {
			continue
		}
		icon := icoTerminal
		detail := commandDetail(cmd.Description, cmd.Hint)
		if cmd.Kind == commandKindSkill {
			icon = icoSparkle
			if detail != "" {
				detail = "Skill · " + detail
			} else {
				detail = "Skill"
			}
		}
		items = append(items, composerItem{
			title:  "/" + cmd.Name,
			detail: detail,
			insert: "/" + cmd.Name + " ",
			cmd:    cmd,
			icon:   icon,
		})
	}
	return items
}

// composerMatch reports whether an item matches the typed query.
func composerMatch(query string, fields ...string) bool {
	if query == "" {
		return true
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

// commandDetail is a command's line in the palette: the agent's first
// sentence, cut at a word boundary, so a tool's docstring does not spill into
// the row. The command's input hint stands in when it has no description.
func commandDetail(description, hint string) string {
	description = strings.TrimSpace(strings.Join(strings.Fields(description), " "))
	if description == "" {
		if strings.TrimSpace(hint) != "" {
			return "Takes " + strings.TrimSpace(hint)
		}
		return ""
	}
	if i := strings.Index(description, ". "); i >= 0 {
		description = description[:i+1]
	}
	runes := []rune(description)
	const limit = 76
	if len(runes) <= limit {
		return description
	}
	cut := string(runes[:limit])
	if i := strings.LastIndexByte(cut, ' '); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:.") + "…"
}

// moveComposerSelection moves the highlighted palette row.
func (a *app) moveComposerSelection(chat *chatState, delta int) {
	if chat == nil || chat.menu == nil {
		return
	}
	n := len(a.composerItems(chat))
	if n == 0 {
		return
	}
	chat.menu.sel = (chat.menu.sel + delta + n) % n
}

// composerSelection is the palette's highlighted row; a palette with nothing
// to pick returns an empty row, which staging turns into a dismiss.
func (a *app) composerSelection(chat *chatState) composerItem {
	if chat == nil || chat.menu == nil {
		return composerItem{}
	}
	items := a.composerItems(chat)
	if len(items) == 0 {
		return composerItem{}
	}
	return items[min(max(chat.menu.sel, 0), len(items)-1)]
}

// stageComposerPick records a palette pick to apply on the next build, when
// the composer's field element is valid again: a key handler is not a build,
// so the edit is staged and the card's build applies it.
func (a *app) stageComposerPick(chat *chatState, item composerItem) {
	menu := chat.menu
	if menu == nil || item.insert == "" {
		dismissComposerMenu(chat)
		return
	}
	chat.pendingEdit = &composerEdit{start: menu.start, caret: menu.caret, insert: item.insert}
	chat.menu = nil
	chat.menuDismiss = -1
}

// chatComposerPalette draws the mention or command menu above the composer.
func (a *app) chatComposerPalette(c *ui.Context, chat *chatState, anchor ui.Element, wide bool) {
	if chat == nil || chat.menu == nil {
		return
	}
	items := a.composerItems(chat)
	if len(items) > 0 {
		chat.menu.sel = min(max(chat.menu.sel, 0), len(items)-1)
	}
	t := c.Theme()
	width := float32(composerPalettePage)
	empty := "No pages match. Use + to reference a page or attach a file."
	if chat.menu.kind == '/' {
		width = float32(composerPalettePage)
		if !wide {
			width = float32(composerPaletteRail)
		}
		empty = "The agent has advertised no commands here."
	} else if !wide {
		width = float32(composerPaletteRail)
	}
	open := true
	ui.PopoverBase(c, anchor, &open, func(panel ui.Element) {
		panel.Key("composer-menu").Width(width).Padding(6).Gap(1).Radius(tokens.RadiusLG).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 12, 32, -12, ui.RGBA(0, 0, 0, 0.24))
		panel.Children(func() {
			if len(items) == 0 {
				ui.Text(c, empty).FontSize(tokens.FontCaption).TextColor(t.TextMuted).
					LineHeight(1.45).Padding(6, 8)
			}
			for i, item := range items {
				i, item := i, item
				row := ui.Row(c).Key("composer-item-"+itoa(i)).Height(40).Padding(0, 8).
					Radius(tokens.RadiusSM).Gap(9).AlignItems(ui.Center).Focusable()
				if i == chat.menu.sel {
					row.Background(selectedFace(t))
				} else if row.Hovered() {
					row.Background(t.SurfaceHover)
				}
				if row.Clicked() {
					a.stageComposerPick(chat, item)
				}
				row.Children(func() {
					ui.Box(c).Size(24, 24).Radius(tokens.RadiusSM).Background(t.Surface).
						AlignItems(ui.Center).Justify(ui.Center).Children(func() {
						ui.Icon(c, item.icon).Size(13, 13).TextColor(t.TextMuted)
					})
					ui.Column(c).Grow(1).Gap(1).Children(func() {
						ui.Text(c, item.title).FontSize(tokens.FontControl).TextColor(t.Text).SingleLine()
						if item.detail != "" {
							ui.Text(c, item.detail).FontSize(tokens.FontMicro).TextColor(t.TextMuted).SingleLine()
						}
					})
				})
				row.Label(item.title)
			}
			ui.Text(c, "↑↓ to move · Enter to insert · Esc to close").
				FontSize(tokens.FontMicro).TextColor(t.TextMuted.Alpha(0.8)).
				Padding(tokens.Space1, 2, 1, 8)
		})
	})
	if !open {
		dismissComposerMenu(chat)
	}
}

// chatConfigControls are the session options the agent advertises — its model,
// its thinking level, and whatever else it names — beside the page the answer
// reads. No agent, no controls: the session is what knows them.
func (a *app) chatConfigControls(c *ui.Context, chat *chatState) {
	if len(chat.configOptions) == 0 {
		return
	}
	for i := range chat.configOptions {
		option := chat.configOptions[i]
		switch option.Kind {
		case acp.ConfigKindSelect:
			if len(option.Choices) == 0 {
				continue
			}
			names := make([]string, len(option.Choices))
			current := ""
			for j, choice := range option.Choices {
				names[j] = option.Name + " · " + choice.Name
				if choice.Value == option.Current {
					current = names[j]
				}
			}
			selected := current
			sel := ui.Select(c.Key("chat-config-"+option.ID), &selected, names).
				Label(option.Name).FontSize(tokens.FontMicro)
			if sel.Changed() {
				for j, name := range names {
					if name == selected {
						a.setChatConfig(chat, option.ID, option.Choices[j].Value)
						break
					}
				}
			}
		case acp.ConfigKindBoolean:
			on := option.Boolean
			sw := ui.Switch(c.Key("chat-config-"+option.ID), &on).Label(option.Name)
			if sw.Changed() {
				a.setChatConfigBool(chat, option.ID, on)
			}
		}
	}
}

// setChatConfig changes a select option for the conversation's session and
// keeps the options the agent sends back.
func (a *app) setChatConfig(chat *chatState, optionID, value string) {
	client, session, convID := a.client, chat.sessionID, chat.conv.ID
	if client == nil || session == "" {
		return
	}
	go func() {
		options, err := client.SetConfigOption(context.Background(), session, optionID, value)
		a.dispatch(func() {
			if err != nil {
				a.notifyError("Could not change this chat setting", err.Error(), nil)
				return
			}
			a.chatConfigs[convID] = options
			if a.chat == chat {
				chat.configOptions = options
			}
		})
	}()
}

// setChatConfigBool changes a boolean option the same way.
func (a *app) setChatConfigBool(chat *chatState, optionID string, value bool) {
	client, session, convID := a.client, chat.sessionID, chat.conv.ID
	if client == nil || session == "" {
		return
	}
	go func() {
		options, err := client.SetConfigOptionBool(context.Background(), session, optionID, value)
		a.dispatch(func() {
			if err != nil {
				a.notifyError("Could not change this chat setting", err.Error(), nil)
				return
			}
			a.chatConfigs[convID] = options
			if a.chat == chat {
				chat.configOptions = options
			}
		})
	}()
}

// attachChatFiles asks for files off the main thread and adds them to the
// composer, to be sent as resource links with the next turn.
func (a *app) attachChatFiles() {
	chat := a.chat
	parent := a.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      parent,
			Title:       "Attach files",
			ButtonLabel: "Attach",
			Multiple:    true,
		})
		a.dispatch(func() {
			if err != nil {
				a.notifyError("Could not attach files", err.Error(), nil)
				return
			}
			if a.chat != chat {
				return
			}
			for _, path := range paths {
				path = strings.TrimSpace(path)
				if path == "" || chatHasAttachment(chat, path) {
					continue
				}
				chat.attachments = append(chat.attachments, path)
			}
		})
	}()
}

// chatHasAttachment reports whether the draft already carries path.
func chatHasAttachment(chat *chatState, path string) bool {
	for _, attached := range chat.attachments {
		if attached == path {
			return true
		}
	}
	return false
}

// chatContextChips is the composer's row above the field: the pages the
// conversation references and the files the next turn carries, each a chip
// with a way off. A reference and a file read alike because both are context
// for the turn.
func (a *app) chatContextChips(c *ui.Context, chat *chatState) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(tokens.Space1).Wrap().Children(func() {
		for i, rel := range chat.conv.References {
			i, rel := i, rel
			name := a.chatReferenceName(rel)
			chip := ui.Row(c).Key("chat-ref-chip-"+itoa(i)).Height(24).Padding(0, 8).
				Radius(tokens.RadiusPill).Border(1, t.Border).Background(t.Surface).
				Gap(tokens.Space1).AlignItems(ui.Center)
			chip.Children(func() {
				ui.Icon(c, icoFile).Size(10, 10).TextColor(t.TextMuted)
				ui.Text(c, name).FontSize(tokens.FontMicro).TextColor(t.Text).SingleLine()
				a.chatChipRemove(c, "chat-ref-remove-"+itoa(i), "Remove reference "+name, func() {
					removeChatReference(chat, rel)
				})
			})
		}
		for i, path := range chat.attachments {
			i, path := i, path
			name := attachmentName(path)
			chip := ui.Row(c).Key("chat-att-chip-"+itoa(i)).Height(24).Padding(0, 8).
				Radius(tokens.RadiusPill).Border(1, t.Border).Background(t.Surface).
				Gap(tokens.Space1).AlignItems(ui.Center)
			chip.Children(func() {
				if src := a.picture(path); src != nil {
					ui.Image(c, src).Size(16, 16).Radius(tokens.RadiusXS).Fit(ui.Cover)
				} else {
					ui.Icon(c, icoFile).Size(10, 10).TextColor(t.TextMuted)
				}
				ui.Text(c, name).FontSize(tokens.FontMicro).TextColor(t.Text).SingleLine()
				a.chatChipRemove(c, "chat-att-remove-"+itoa(i), "Remove attachment "+name, func() {
					chat.attachments = append(chat.attachments[:i:i], chat.attachments[i+1:]...)
				})
			})
		}
	})
}

// chatChipRemove is the small × at the end of a context chip.
func (a *app) chatChipRemove(c *ui.Context, key any, label string, remove func()) {
	t := c.Theme()
	b := ui.Box(c).Key(key).Size(16, 16).Radius(tokens.RadiusPill).
		AlignItems(ui.Center).Justify(ui.Center).Focusable()
	if b.Hovered() {
		b.Background(t.SurfacePressed)
	}
	clicked := b.Clicked()
	b.Children(func() {
		ui.Icon(c, icoClose).Size(9, 9).TextColor(t.TextMuted)
	})
	b.Label(label)
	if clicked {
		remove()
	}
}

// chatAddContextButton is the composer's plus: one quiet round button that
// opens the menu which gives the conversation context.
func (a *app) chatAddContextButton(c *ui.Context, chat *chatState) ui.Element {
	t := c.Theme()
	b := ui.Box(c).Key("chat-add-context").Size(30, 30).Radius(tokens.RadiusPill).
		Background(t.Surface).AlignItems(ui.Center).Justify(ui.Center).Focusable()
	switch {
	case chat.contextOpen:
		b.Background(t.SurfacePressed)
	case b.Pressed():
		b.Background(t.SurfacePressed)
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	if b.Clicked() {
		chat.contextOpen = !chat.contextOpen
	}
	b.Children(func() {
		ui.Icon(c, icoPlus).Size(15, 15).TextColor(t.TextMuted)
	})
	b.Label("Add context")
	return b
}

// chatAddContextMenu is the composer's add-context popover: a page or files,
// both of which land in the same chip row above the field.
func (a *app) chatAddContextMenu(c *ui.Context, chat *chatState, anchor ui.Element) {
	if !chat.contextOpen {
		return
	}
	t := c.Theme()
	page := strings.TrimSpace(a.current)
	referenced := chatReferencesOpen(chat, page)
	panel := ui.PopoverBase(c, anchor, &chat.contextOpen, func(panel ui.Element) {
		panel.AttachTo(anchor, ui.AnchorTopStart, ui.AnchorBottomStart).Margin(0, 0, 6, 0).
			MinWidth(232).Padding(4).Gap(1).Radius(tokens.RadiusMD).
			Background(t.Background).Border(1, t.Border).
			Shadow(0, 10, 24, -10, ui.RGBA(0, 0, 0, 0.20))
		detail := "No page is open"
		switch {
		case page == "":
		case referenced:
			detail = "Already referenced"
		default:
			detail = a.chatReferenceName(page)
		}
		a.chatContextRow(c, "Reference this page", detail, page != "" && !referenced, func() {
			a.referenceOpenPage(chat)
			chat.contextOpen = false
		})
		a.chatContextRow(c, "Attach files…", "Images, notes, anything Bub can read", true, func() {
			chat.contextOpen = false
			a.attachChatFiles()
		})
	})
	if panel.Dismissed() {
		chat.contextOpen = false
	}
}

// chatContextRow is one row of the add-context menu: a title, a quiet detail,
// and the action the row stands for. A disabled row still shows, so the
// menu says why it cannot be picked.
func (a *app) chatContextRow(c *ui.Context, title, detail string, enabled bool, run func()) {
	t := c.Theme()
	row := ui.Row(c).Height(34).Padding(0, 8).Radius(tokens.RadiusSM).Gap(8).
		AlignItems(ui.Center).Focusable()
	if !enabled {
		row.Opacity(0.55)
		row.IgnoreInput(true)
	}
	if enabled && c.Preferences().Hover() && row.Hovered() {
		row.Background(t.SurfaceHover)
	}
	clicked := row.Clicked()
	row.Children(func() {
		ui.Icon(c, icoFile).Size(13, 13).TextColor(t.TextMuted)
		ui.Column(c).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(tokens.FontControl).TextColor(t.Text).SingleLine()
			if detail != "" {
				ui.Text(c, detail).FontSize(tokens.FontMicro).TextColor(t.TextMuted).SingleLine()
			}
		})
	})
	row.Label(title)
	if clicked && enabled && run != nil {
		run()
	}
}

// chatSentAttachments shows what a sent turn carried: pictures rendered
// inline, other files as quiet chips. Clicking a picture opens it.
func (a *app) chatSentAttachments(c *ui.Context, paths []string, maxWidth float32) {
	t := c.Theme()
	ui.Row(c).FillWidth().Padding(tokens.Space1, 0, 0, 0).Gap(tokens.Space1).Wrap().
		Children(func() {
			for i, path := range paths {
				i, path := i, path
				name := attachmentName(path)
				if src := a.picture(path); src != nil {
					row := ui.Row(c).Key("chat-picture-" + itoa(i)).Radius(tokens.RadiusMD).
						Focusable()
					if row.Hovered() {
						row.Border(1, t.Focus)
					}
					clicked := row.Clicked()
					row.Children(func() {
						ui.Image(c, src).MaxWidth(maxWidth).Radius(tokens.RadiusMD).Fit(ui.Contain)
					})
					row.Label("Open " + name)
					if clicked {
						a.openAttachment(path)
					}
					continue
				}
				ui.Row(c).Height(20).Padding(0, 7).Radius(tokens.RadiusPill).
					Border(1, t.Border).Background(t.Background).Gap(tokens.Space1).
					AlignItems(ui.Center).Children(func() {
					ui.Icon(c, icoFile).Size(10, 10).TextColor(t.TextMuted)
					ui.Text(c, name).FontSize(tokens.FontMicro).
						TextColor(t.TextMuted).SingleLine()
				})
			}
		})
}

// attachmentName is how a file reads in a chip, its folder dropped.
func attachmentName(path string) string {
	name := filepath.Base(path)
	if name == "" || name == "." {
		name = path
	}
	return truncate(name, 30)
}

// chatPromptBlocks is a turn's prompt: its text, then each file — a picture as
// an inline image block when images is true, any other file as a resource
// link, which every ACP agent must accept. An image that cannot be read or
// decoded falls back to a link.
func chatPromptBlocks(prompt string, attachments []string, images bool) []acp.ContentBlock {
	blocks := []acp.ContentBlock{acp.TextBlock(prompt)}
	for _, path := range attachments {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if mime := imageMime(path); mime != "" && images {
			if data, err := os.ReadFile(path); err == nil {
				// Only bytes that decode as a picture travel inline; anything
				// else keeps the file's resource link.
				if _, derr := ui.DecodeImage(data); derr == nil {
					blocks = append(blocks, acp.ImageBlock(base64.StdEncoding.EncodeToString(data), mime))
					continue
				}
			}
		}
		blocks = append(blocks, acp.ResourceLinkBlock(filepath.Base(path), acp.FileURI(path)))
	}
	return blocks
}

// imageMime is the media type of a picture the window can attach inline, ""
// for every other kind of file.
func imageMime(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".bmp":
		return "image/bmp"
	}
	return ""
}

// picture returns an image file decoded once and kept, nil for files that are
// not pictures or fail to decode (the caller's file chip still shows).
func (a *app) picture(path string) ui.ImageSource {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if src, ok := a.pictures[path]; ok {
		return src
	}
	var src ui.ImageSource
	if data, err := os.ReadFile(path); err == nil {
		if decoded, derr := ui.DecodeImage(data); derr == nil {
			src = decoded
		}
	}
	a.pictures[path] = src
	return src
}

// openAttachment opens an attached file with the platform's default
// application, off the main thread.
func (a *app) openAttachment(path string) {
	go func() { _ = mygo.Shell.OpenPath(path) }()
}

// chatTitle names the conversation in the header: its saved title, or the
// first thing the writer asked.
func chatTitle(chat *chatState) string {
	if chat == nil {
		return "New chat"
	}
	if title := strings.TrimSpace(chat.conv.Title); title != "" {
		return title
	}
	for _, msg := range chat.conv.Messages {
		if msg.Role != appdata.RoleUser {
			continue
		}
		text := strings.TrimSpace(msg.Text)
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = strings.TrimSpace(text[:i])
		}
		if text != "" {
			return truncate(text, 44)
		}
	}
	return "New chat"
}

// chatListTitle names a saved conversation in the recent popover.
func chatListTitle(conv *appdata.Conversation) string {
	if conv == nil {
		return "Untitled chat"
	}
	if title := strings.TrimSpace(conv.Title); title != "" {
		return title
	}
	return "Untitled chat"
}

// insertChatText puts an answer into the page at the caret, as one undo step.
func (a *app) insertChatText(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	a.doc.Insert(richtext.ParseMarkdown(text))
	a.doc.IdentifyParagraphs(a.newParagraphID)
	a.setDirty(true)
	a.scheduleSave()
	a.updateTitle()
	a.focusIn = true
}
