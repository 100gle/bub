package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui/richtext"

	"github.com/bubbuild/bub/desktop/internal/acp"
	"github.com/bubbuild/bub/desktop/internal/block"
)

// actionKind is one of the block actions of the Bub menu.
type actionKind string

const (
	actionRewrite   actionKind = "rewrite"
	actionShorten   actionKind = "shorten"
	actionExpand    actionKind = "expand"
	actionContinue  actionKind = "continue"
	actionTranslate actionKind = "translate"
	actionAsk       actionKind = "ask"
)

// task is the sentence of the prompt that says what to do with the block.
func (k actionKind) task() string {
	switch k {
	case actionRewrite:
		return "Rewrite the paragraph below in the same voice and tense, with the same length and meaning."
	case actionShorten:
		return "Shorten the paragraph below by about 30%, keeping its meaning, voice and tense."
	case actionExpand:
		return "Expand the paragraph below with a little more detail, keeping its voice and tense."
	case actionTranslate:
		return "Translate the paragraph below into English, keeping its formatting and meaning."
	default:
		return ""
	}
}

// actionState is a block action waiting to run, or the one a suggestion came
// from: its target and the little context the prompt carries.
type actionState struct {
	kind        actionKind
	instruction string
	target      block.Block
	title       string
	prev, next  string
	outline     []string
}

// prompt is what session/prompt gets: the instruction, the target block and
// just enough page context to write it back in its place. It asks for the
// replacement text and nothing else.
func buildActionPrompt(act actionState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are editing one paragraph of a Markdown document titled %q.\n", act.title)
	if act.kind == actionContinue {
		b.WriteString("Continue the document after the paragraph below, in the same voice and tense, staying on its subject.\n")
		b.WriteString("Reply with the next paragraph only: no quotation marks, no code fences, no headings, no explanation.\n\n")
	} else {
		if task := act.kind.task(); task != "" {
			b.WriteString(task)
			b.WriteString("\n")
		}
		b.WriteString("Reply with the replacement paragraph only: no quotation marks, no code fences, no explanation.\n\n")
		if act.instruction != "" {
			fmt.Fprintf(&b, "Instruction: %s\n\n", act.instruction)
		}
	}
	if len(act.outline) > 0 {
		b.WriteString("Document outline:\n")
		for _, line := range act.outline {
			fmt.Fprintf(&b, "- %s\n", line)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Target paragraph:\n%s\n", act.target.Text)
	if act.prev != "" {
		fmt.Fprintf(&b, "\nPreceding paragraph: %s\n", act.prev)
	}
	// A continuation goes after the block, so the block that follows it now is
	// not context for it.
	if act.next != "" && act.kind != actionContinue {
		fmt.Fprintf(&b, "\nFollowing paragraph: %s\n", act.next)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// cleanSuggestion turns a model answer into the text a paragraph would take: no
// wrapping fences, no quotation marks around the whole answer, trimmed.
func cleanSuggestion(raw string) string {
	s := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		} else {
			return ""
		}
		s = strings.TrimSpace(s)
	}
	if strings.HasSuffix(s, "```") {
		s = strings.TrimSpace(strings.TrimSuffix(s, "```"))
	}
	switch {
	case len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"':
		s = strings.TrimSpace(s[1 : len(s)-1])
	case len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'':
		s = strings.TrimSpace(s[1 : len(s)-1])
	case strings.HasPrefix(s, "“") && strings.HasSuffix(s, "”"):
		s = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "“"), "”"))
	}
	return strings.TrimSpace(s)
}

// suggestionStatus is where a suggestion is.
type suggestionStatus int

const (
	suggestThinking suggestionStatus = iota
	suggestStreaming
	suggestReady
)

func (s suggestionStatus) busy() bool {
	return s == suggestThinking || s == suggestStreaming
}

// suggestionState is the agent's answer to an action: text that the page does
// not hold yet, and the target it would replace.
type suggestionState struct {
	action actionState
	gen    int
	raw    string
	text   string
	status suggestionStatus
}

// askState is the small "Ask Bub…" popover: a block and its one-line
// instruction.
type askState struct {
	target      block.Block
	instruction string
}

// --- Page blocks ----------------------------------------------------------

// blocks lists the open page's paragraphs.
func (a *app) blocks() []block.Block {
	return block.List(a.doc.Text())
}

// blockByID finds the paragraph with an ID, wherever the page moved it.
func (a *app) blockByID(id string) *block.Block {
	b, ok := block.ByID(a.doc.Text(), id)
	if !ok {
		return nil
	}
	return &b
}

// blockAt finds the paragraph holding a rune offset, as the caret does. A
// caret past the end belongs to the last block; a page with none has none.
func (a *app) blockAt(offset int) *block.Block {
	if b, ok := block.At(a.doc.Text(), offset); ok {
		return &b
	}
	blocks := a.blocks()
	if len(blocks) == 0 {
		return nil
	}
	last := blocks[len(blocks)-1]
	return &last
}

// caretBlock is the paragraph the caret is in, the target of the menu actions.
func (a *app) caretBlock() *block.Block {
	_, caret := a.doc.Selection()
	return a.blockAt(caret)
}

// actionFor builds an action on a block: the caret's, for the menu.
func (a *app) actionFor(kind actionKind, instruction string, target block.Block) actionState {
	blocks := a.blocks()
	prev, next := contextAround(blocks, target.Index)
	if !a.settings.AIContextNeighbors {
		prev, next = "", ""
	}
	var outline []string
	if a.settings.AIContextOutline {
		for _, entry := range a.outline() {
			outline = append(outline, entry.Text)
		}
	}
	return actionState{
		kind:        kind,
		instruction: instruction,
		target:      target,
		title:       a.documentTitle(),
		prev:        prev,
		next:        next,
		outline:     outline,
	}
}

// menuAction runs one of the Bub menu's block actions on the caret's block.
func (a *app) menuAction(kind actionKind) {
	target := a.caretBlock()
	if target == nil {
		return
	}
	a.startAction(a.actionFor(kind, "", *target))
}

// menuAsk opens the one-line instruction popover for the caret's block.
func (a *app) menuAsk() {
	target := a.caretBlock()
	if target == nil {
		return
	}
	a.dismissPopovers()
	a.ask = &askState{target: *target}
}

// runAsk sends the Ask Bub… instruction.
func (a *app) runAsk() {
	if a.ask == nil {
		return
	}
	instruction := strings.TrimSpace(a.ask.instruction)
	if instruction == "" {
		return
	}
	target := a.ask.target
	a.ask = nil
	a.startAction(a.actionFor(actionAsk, instruction, target))
}

// closeAsk closes the Ask popover.
func (a *app) closeAsk() { a.ask = nil }

// contextAround is the neighbouring text a prompt carries, trimmed to what a
// model needs to keep the voice.
func contextAround(blocks []block.Block, index int) (prev, next string) {
	const limit = 320
	if index > 0 && index-1 < len(blocks) {
		prev = truncate(blocks[index-1].Text, limit)
	}
	if index+1 < len(blocks) {
		next = truncate(blocks[index+1].Text, limit)
	}
	return prev, next
}

func truncate(s string, limit int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return strings.TrimSpace(string(runes[:limit])) + "…"
}

// newParagraphID names a new paragraph, so a block action can find it again.
func (a *app) newParagraphID() string {
	return "p" + strconv.FormatInt(a.idSeq.Add(1), 10)
}

// --- Action lifecycle -----------------------------------------------------

// startAction runs an action, or queues it behind the one still streaming: one
// ACP turn at a time, so the next prompt cancels the last and waits for it.
func (a *app) startAction(act actionState) {
	if a.client == nil || a.sessionID == "" {
		detail := a.connectErr
		if detail == "" {
			detail = "The Bub agent is not running."
		}
		a.notifyError("Bub is not connected", detail, a.retryAfterReconnect(&act))
		return
	}
	if s := a.suggestion; s != nil && s.status.busy() {
		a.pendingAction = &act
		a.cancelCurrent()
		return
	}
	a.runAction(act)
}

// runAction sends the prompt and streams its answer into the suggestion.
func (a *app) runAction(act actionState) {
	client, session := a.client, a.sessionID
	if client == nil || session == "" {
		a.notifyError("Bub is not connected", "The Bub agent is not running.", a.retryAfterReconnect(&act))
		return
	}
	a.aiGen++
	gen := a.aiGen
	a.ask = nil
	a.suggestion = &suggestionState{action: act, gen: gen, status: suggestThinking}
	prompt := buildActionPrompt(act)
	go func() {
		stop, err := client.Prompt(context.Background(), session, []acp.ContentBlock{acp.TextBlock(prompt)})
		a.dispatch(func() { a.finishSuggestion(gen, stop, err) })
	}()
}

// finishSuggestion ends a turn: ready, or gone with a native message.
func (a *app) finishSuggestion(gen int, stop string, err error) {
	s := a.suggestion
	if s != nil && s.gen == gen {
		act := s.action
		switch {
		case err != nil:
			a.suggestion = nil
			if errors.Is(err, acp.ErrClosed) {
				a.notifyError("Bub stopped", "The agent stopped before the suggestion finished.", a.retryAfterReconnect(&act))
			} else {
				a.notifyError("Bub could not finish that", friendlyError(err), func() { a.startAction(act) })
			}
		case stop == acp.StopCancelled:
			a.suggestion = nil
		case cleanSuggestion(s.raw) == "":
			a.suggestion = nil
			a.notifyError("Bub found nothing to say", "The agent returned no text.", func() { a.startAction(act) })
		default:
			s.text = cleanSuggestion(s.raw)
			s.status = suggestReady
		}
	}
	if pending := a.pendingAction; pending != nil {
		a.pendingAction = nil
		a.runAction(*pending)
	}
}

// appendSuggestion folds a streamed agent chunk into the current suggestion.
func (a *app) appendSuggestion(delta string) {
	s := a.suggestion
	if s == nil || !s.status.busy() || delta == "" {
		return
	}
	s.raw += delta
	s.text = cleanSuggestion(s.raw)
	s.status = suggestStreaming
}

// suggestionStale reports whether the target changed or left since the action
// was captured, so its result no longer fits.
func (a *app) suggestionStale() bool {
	s := a.suggestion
	if s == nil || s.status.busy() {
		return false
	}
	b := a.blockByID(s.action.target.ID)
	return b == nil || b.Text != s.action.target.Text
}

// applySuggestion writes a ready suggestion into the page as one undo step,
// resolved by the paragraph's ID. It refuses a stale target: the paragraph
// moved, changed, or is gone.
func (a *app) applySuggestion() bool {
	s := a.suggestion
	if s == nil || s.status != suggestReady {
		return false
	}
	target := s.action.target
	current := a.blockByID(target.ID)
	if current == nil || current.Text != target.Text {
		return false
	}
	text := cleanSuggestion(s.text)
	if text == "" {
		return false
	}
	insert := s.action.kind == actionContinue
	a.doc.Edit(func(e *richtext.Edit) {
		switch {
		case insert:
			e.ReplaceString(current.End, current.End, "\n"+text, richtext.Attributes{})
			e.Select(current.End+1, current.End+1+len([]rune(text)))
		default:
			e.ReplaceString(current.Start, current.End, text, richtext.Attributes{})
			e.Select(current.Start, current.Start+len([]rune(text)))
		}
	})
	a.doc.IdentifyParagraphs(a.newParagraphID) // name the paragraph an insertion made
	a.setDirty(true)
	a.scheduleSave()
	a.suggestion = nil
	a.updateTitle()
	return true
}

// regenerate runs the same action again, cancelling one still streaming.
func (a *app) regenerate() {
	if a.suggestion == nil {
		return
	}
	act := a.suggestion.action
	a.startAction(act)
}

// dismissPopovers closes the suggestion and ask popovers, cancelling a turn.
func (a *app) dismissPopovers() {
	if s := a.suggestion; s != nil && s.status.busy() {
		a.cancelCurrent()
	}
	a.suggestion = nil
	a.ask = nil
	a.pendingAction = nil
}

// closeAI closes every AI surface and returns the action whose turn was
// running, for a Retry.
func (a *app) closeAI() *actionState {
	var running *actionState
	if s := a.suggestion; s != nil {
		if s.status.busy() {
			a.cancelCurrent()
			act := s.action
			running = &act
		}
	}
	a.suggestion = nil
	a.ask = nil
	a.pendingAction = nil
	return running
}

// cancelCurrent asks the agent to stop the running turn.
func (a *app) cancelCurrent() {
	client, session := a.client, a.sessionID
	if client == nil || session == "" {
		return
	}
	go func() { _ = client.Cancel(session) }()
}

// friendlyError is what the native dialog says when a turn fails.
func friendlyError(err error) string {
	message := err.Error()
	message = strings.TrimPrefix(message, "acp: session/prompt: ")
	return message
}

// menuReconnect starts the agent again.
func (a *app) menuReconnect() {
	a.reconnect()
}
