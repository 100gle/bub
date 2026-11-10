package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui/richtext"

	"github.com/bubbuild/bub/desktop/internal/acp"
)

// actionKind is one of the block actions the action bar offers.
type actionKind string

const (
	actionRewrite   actionKind = "rewrite"
	actionShorten   actionKind = "shorten"
	actionExpand    actionKind = "expand"
	actionContinue  actionKind = "continue"
	actionTranslate actionKind = "translate"
	actionAsk       actionKind = "ask"
)

// actionLabels are the buttons of the action bar, in order.
var actionLabels = []struct {
	kind  actionKind
	label string
}{
	{actionRewrite, "Rewrite"},
	{actionShorten, "Shorten"},
	{actionExpand, "Expand"},
	{actionContinue, "Continue"},
	{actionTranslate, "Translate"},
	{actionAsk, "Ask…"},
}

func (k actionKind) label() string {
	for _, a := range actionLabels {
		if a.kind == k {
			return a.label
		}
	}
	return string(k)
}

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

// block is one paragraph of the document, as the app addresses it.
type block struct {
	id    string
	index int
	start int // rune offset of the first character
	end   int // rune offset just after the last character, before the newline
	text  string
	style richtext.ParagraphStyle
}

// actionState is a block action waiting to run, or the one a suggestion came
// from: its target and the little context the prompt carries.
type actionState struct {
	kind        actionKind
	instruction string
	target      block
	title       string
	prev, next  string
}

// prompt is what session/prompt gets: the instruction, the target block and
// just enough document context to write it back in its place. It asks for the
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
	fmt.Fprintf(&b, "Target paragraph:\n%s\n", act.target.text)
	if act.prev != "" {
		fmt.Fprintf(&b, "\nPreceding paragraph: %s\n", act.prev)
	}
	// A continuation goes after the block, so the block that follows it now
	// is not context for it.
	if act.next != "" && act.kind != actionContinue {
		fmt.Fprintf(&b, "\nFollowing paragraph: %s\n", act.next)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// cleanSuggestion turns a model answer into the text a paragraph would take:
// no wrapping fences, no quotation marks around the whole answer, trimmed.
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

// changeSummary describes a suggestion in a few words, as the card's label.
func changeSummary(before, after string) string {
	was, now := len(strings.Fields(before)), len(strings.Fields(after))
	switch {
	case before == after:
		return "unchanged"
	case was == now:
		return fmt.Sprintf("%d words", now)
	default:
		return fmt.Sprintf("%d → %d words", was, now)
	}
}

// suggestionStatus is where a suggestion is.
type suggestionStatus int

const (
	suggestThinking suggestionStatus = iota
	suggestStreaming
	suggestReady
	suggestFailed
)

func (s suggestionStatus) busy() bool {
	return s == suggestThinking || s == suggestStreaming
}

func (s suggestionStatus) label() string {
	switch s {
	case suggestThinking:
		return "thinking"
	case suggestStreaming:
		return "writing"
	case suggestFailed:
		return "failed"
	default:
		return ""
	}
}

// suggestionState is the agent's answer to an action: text that the document
// does not hold yet, and the target it would replace.
type suggestionState struct {
	action   actionState
	gen      int
	raw      string
	text     string
	status   suggestionStatus
	errText  string
	wasStale bool
}

// --- Document state -------------------------------------------------------

// newDocument starts an empty document with no path.
func (a *app) newDocument() {
	a.setDocument(richtext.NewText(""), "")
}

// setDocument shows text as the document, saved at path ("" for a new one).
func (a *app) setDocument(text richtext.Text, path string) {
	a.doc.SetText(text)
	a.doc.IdentifyParagraphs(a.newParagraphID)
	a.doc.SetSelection(0, 0) // open a document at its start
	a.path = path
	a.dirty = false
	a.clearAI()
	a.errText = ""
}

// newParagraphID names a new paragraph, for the block actions to address it
// again later.
func (a *app) newParagraphID() string {
	return "p" + strconv.FormatInt(a.idSeq.Add(1), 10)
}

// documentName is the file name, or Untitled before the first save.
func (a *app) documentName() string {
	if a.path == "" {
		return "Untitled"
	}
	return filepath.Base(a.path)
}

// documentPath is the full path, shown in the footer.
func (a *app) documentPath() string {
	if a.path == "" {
		return "Not saved yet"
	}
	return a.path
}

// words counts the document's words, recomputing only when it changed.
func (a *app) words() int {
	if v := a.doc.Version(); v != a.wordsVersion {
		a.wordCount = len(strings.Fields(a.doc.Text().PlainText()))
		a.wordsVersion = v
	}
	return a.wordCount
}

// blocks lists the document's paragraphs, with the IDs the AI actions use.
func (a *app) blocks() []block {
	text := a.doc.Text()
	out := make([]block, 0, text.ParagraphCount())
	index := 0
	for start, p := range text.Paragraphs() {
		var b strings.Builder
		for _, run := range p.Runs {
			b.WriteString(run.Text)
		}
		plain := b.String()
		out = append(out, block{
			id:    p.Style.ID,
			index: index,
			start: start,
			end:   start + len([]rune(plain)),
			text:  plain,
			style: p.Style,
		})
		index++
	}
	return out
}

// blockByID finds the paragraph with an ID, wherever the document moved it.
func (a *app) blockByID(id string) *block {
	if id == "" {
		return nil
	}
	for _, b := range a.blocks() {
		if b.id == id {
			b := b
			return &b
		}
	}
	return nil
}

// blockAt finds the paragraph holding a rune offset, as the caret does.
func (a *app) blockAt(offset int) *block {
	var blocks []block
	for _, b := range a.blocks() {
		blocks = append(blocks, b)
		if offset >= b.start && offset <= b.end {
			b := b
			return &b
		}
	}
	if len(blocks) > 0 {
		last := blocks[len(blocks)-1]
		return &last
	}
	return nil
}

// caretBlock is the paragraph the caret is in, the target of ⌘K.
func (a *app) caretBlock() *block {
	_, caret := a.doc.Selection()
	return a.blockAt(caret)
}

// contextAround is the neighbouring text a prompt carries, trimmed to what a
// model needs to keep the voice.
func contextAround(blocks []block, index int) (prev, next string) {
	const limit = 320
	if index > 0 && index-1 < len(blocks) {
		prev = truncate(blocks[index-1].text, limit)
	}
	if index+1 < len(blocks) {
		next = truncate(blocks[index+1].text, limit)
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

// --- Action lifecycle -----------------------------------------------------

// openActionForBlock opens the action bar for a paragraph, as a click on its
// gutter mark or ⌘K on its caret does.
func (a *app) openActionForBlock(b block) {
	blocks := a.blocks()
	var current *block
	for _, candidate := range blocks {
		if candidate.id == b.id {
			c := candidate
			current = &c
			break
		}
	}
	if current == nil {
		return
	}
	prev, next := contextAround(blocks, current.index)
	a.action = &actionState{
		target: *current,
		title:  a.documentName(),
		prev:   prev,
		next:   next,
	}
	a.askMode = false
	a.askText = ""
}

// openActionAtCaret opens the bar for the paragraph holding the caret; the
// editor's ⌘K calls it.
func (a *app) openActionAtCaret() {
	if b := a.caretBlock(); b != nil {
		a.openActionForBlock(*b)
	}
}

// chooseAction starts the action the bar was clicked for.
func (a *app) chooseAction(kind actionKind, instruction string) {
	if a.action == nil {
		return
	}
	act := *a.action
	act.kind = kind
	act.instruction = strings.TrimSpace(instruction)
	a.askMode = false
	a.askText = ""
	a.startAction(act)
}

// startAction runs an action, or queues it behind the one still streaming: one
// ACP turn at a time, so the next prompt cancels the last and waits for it.
func (a *app) startAction(act actionState) {
	if a.client == nil || a.sessionID == "" {
		a.errText = "Not connected to a Bub agent. Press Retry to start one, then try again."
		return
	}
	if s := a.suggestion; s != nil && s.status.busy() {
		a.pendingAction = &act
		a.cancelCurrent()
		return
	}
	a.runAction(act)
}

// runAction sends the prompt and streams its answer into a suggestion.
func (a *app) runAction(act actionState) {
	client, session := a.client, a.sessionID
	if client == nil || session == "" {
		a.errText = "Not connected to a Bub agent. Press Retry to start one, then try again."
		return
	}
	a.aiGen++
	gen := a.aiGen
	a.suggestion = &suggestionState{action: act, gen: gen, status: suggestThinking}
	a.errText = ""
	a.activity = "Thinking…"
	prompt := buildActionPrompt(act)
	go func() {
		stop, err := client.Prompt(context.Background(), session, []acp.ContentBlock{acp.TextBlock(prompt)})
		a.dispatch(func() { a.finishSuggestion(gen, stop, err) })
	}()
}

// finishSuggestion ends a turn: ready, failed, or cancelled and gone.
func (a *app) finishSuggestion(gen int, stop string, err error) {
	a.activity = ""
	if s := a.suggestion; s != nil && s.gen == gen {
		switch {
		case errors.Is(err, acp.ErrClosed):
			// The connection went away: the banner says why. Keep any
			// message the close handler already left.
			if s.status != suggestFailed {
				s.status = suggestFailed
				s.errText = "The agent stopped before the suggestion finished."
			}
		case err != nil:
			s.status = suggestFailed
			s.errText = friendlyError(err)
		case stop == acp.StopCancelled:
			a.suggestion = nil
		case cleanSuggestion(s.raw) == "":
			s.status = suggestFailed
			s.errText = "The agent returned no text."
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
	b := a.blockByID(s.action.target.id)
	return b == nil || b.text != s.action.target.text
}

// applySuggestion writes a ready suggestion into the document as one undo
// step, resolved by the paragraph's ID. It refuses a stale target: the
// paragraph moved, changed, or is gone.
func (a *app) applySuggestion() bool {
	s := a.suggestion
	if s == nil || s.status != suggestReady {
		return false
	}
	target := s.action.target
	current := a.blockByID(target.id)
	if current == nil || current.text != target.text {
		s.wasStale = true
		return false
	}
	text := cleanSuggestion(s.text)
	if text == "" {
		s.wasStale = true
		return false
	}
	insert := s.action.kind == actionContinue
	a.doc.Edit(func(e *richtext.Edit) {
		switch {
		case insert:
			e.ReplaceString(current.end, current.end, "\n"+text, richtext.Attributes{})
			e.Select(current.end+1, current.end+1+len([]rune(text)))
		default:
			e.ReplaceString(current.start, current.end, text, richtext.Attributes{})
			e.Select(current.start, current.start+len([]rune(text)))
		}
	})
	// New paragraphs an insertion made get names too.
	a.doc.IdentifyParagraphs(a.newParagraphID)
	a.dirty = true
	a.suggestion = nil
	a.action = nil
	a.activity = ""
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

// dismissSuggestion closes the card, cancelling the turn if it still runs.
func (a *app) dismissSuggestion() {
	if s := a.suggestion; s != nil && s.status.busy() {
		a.cancelCurrent()
	}
	a.pendingAction = nil
	a.suggestion = nil
	a.action = nil
	a.activity = ""
}

// clearAI closes every AI surface, as opening another document does.
func (a *app) clearAI() {
	if s := a.suggestion; s != nil && s.status.busy() {
		a.cancelCurrent()
	}
	a.suggestion = nil
	a.action = nil
	a.pendingAction = nil
	a.askMode = false
	a.askText = ""
	a.activity = ""
}

// cancelCurrent asks the agent to stop the running turn.
func (a *app) cancelCurrent() {
	client, session := a.client, a.sessionID
	if client == nil || session == "" {
		return
	}
	go func() { _ = client.Cancel(session) }()
}

// friendlyError is what the card says when a turn fails.
func friendlyError(err error) string {
	message := err.Error()
	if i := strings.Index(message, "acp: session/prompt: "); i == 0 {
		message = strings.TrimPrefix(message, "acp: session/prompt: ")
	}
	return message
}

// --- Files ----------------------------------------------------------------

// markdownFilter is the file dialog's filter, so Open and Save speak Markdown.
func markdownFilter() []mygo.FileFilter {
	return []mygo.FileFilter{{
		Name:       "Markdown",
		Extensions: []string{"md", "markdown", "mdown", "txt"},
	}}
}

// readMarkdownFile parses a Markdown file into the editor's text.
func readMarkdownFile(path string) (richtext.Text, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return richtext.Text{}, err
	}
	return richtext.ParseMarkdown(string(data)), nil
}

// writeMarkdownFile writes the document as Markdown.
func writeMarkdownFile(path string, text richtext.Text) error {
	return os.WriteFile(path, []byte(text.Markdown()), 0o644)
}

// newAsync starts an empty document, asking first if writing would be lost.
func (a *app) newAsync() {
	a.confirmDiscard(func() { a.newDocument() })
}

// openAsync asks for a Markdown file and shows it, off the view goroutine: the
// dialog and the read block.
func (a *app) openAsync() {
	a.confirmDiscard(func() {
		go func() {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
				Title:   "Open Markdown",
				Filters: markdownFilter(),
			})
			if err != nil {
				a.dispatch(func() { a.errText = "Could not open a file: " + err.Error() })
				return
			}
			if len(paths) == 0 {
				return
			}
			text, err := readMarkdownFile(paths[0])
			if err != nil {
				a.dispatch(func() { a.errText = "Could not read " + paths[0] + ": " + err.Error() })
				return
			}
			a.dispatch(func() { a.setDocument(text, paths[0]) })
		}()
	})
}

// saveAsync writes the document, asking for a path when it has none. The text
// is snapshotted on the main thread, so typing continues while it writes.
func (a *app) saveAsync() {
	path := a.path
	text := a.doc.Text()
	version := a.doc.Version()
	go func() {
		if path == "" {
			name := "Untitled.md"
			chosen, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
				Title:       "Save Markdown",
				DefaultPath: name,
				Filters:     markdownFilter(),
			})
			if err != nil {
				a.dispatch(func() { a.errText = "Could not save: " + err.Error() })
				return
			}
			if chosen == "" {
				return
			}
			path = chosen
		}
		if err := writeMarkdownFile(path, text); err != nil {
			a.dispatch(func() { a.errText = "Could not write " + path + ": " + err.Error() })
			return
		}
		a.dispatch(func() {
			a.path = path
			if a.doc.Version() == version {
				a.dirty = false
			}
			a.errText = ""
		})
	}()
}

// confirmDiscard asks before an action would throw away unsaved writing.
func (a *app) confirmDiscard(then func()) {
	if !a.dirty {
		then()
		return
	}
	go func() {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{
			Title:              "Discard changes?",
			Message:            "The document has unsaved changes.",
			Detail:             "They will be lost.",
			Buttons:            []string{"Cancel", "Discard"},
			DefaultButton:      0,
			CancelButton:       0,
			DestructiveButtons: []int{1},
		})
		if err != nil {
			a.dispatch(func() { a.errText = "Could not ask about unsaved changes: " + err.Error() })
			return
		}
		if result.Button != 1 {
			return
		}
		a.dispatch(then)
	}()
}
