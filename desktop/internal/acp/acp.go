// Package acp adapts the ACP Go SDK to Bub Desktop.
//
// The SDK (github.com/caelis-labs/acp-go-sdk, Apache-2.0) owns the wire
// protocol: JSON-RPC 2.0 as newline-delimited JSON, the typed ACP v1 messages,
// request/response correlation, notification dispatch and the lifecycle of the
// spawned agent process. This package keeps only what the window needs from
// that:
//
//   - Config and Start, which spawn and shut down the agent process.
//   - Client, the SDK client implementation the window hands to the SDK: it
//     answers permission requests by declining them, and folds the SDK's typed
//     session notifications into the Update model the view already renders.
//   - The small vocabulary of that model, so the UI does not import the SDK.
package acp

import (
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"

	sdk "github.com/caelis-labs/acp-go-sdk"
)

// ProtocolVersion is the ACP version this client speaks.
const ProtocolVersion = 1

// Stop reasons of a prompt turn, as the SDK's typed ones spell them.
const (
	StopEndTurn         = "end_turn"
	StopMaxTokens       = "max_tokens"
	StopMaxTurnRequests = "max_turn_requests"
	StopRefusal         = "refusal"
	StopCancelled       = "cancelled"
	// StopUnknown is what the client reports for a stop reason it does not
	// know, so that new protocol values do not break it.
	StopUnknown = "unknown"
)

// Session update kinds, the value of the update's type in the app's model.
const (
	UpdateUserMessageChunk  = "user_message_chunk"
	UpdateAgentMessageChunk = "agent_message_chunk"
	UpdateAgentThoughtChunk = "agent_thought_chunk"
	UpdateToolCall          = "tool_call"
	UpdateToolCallUpdate    = "tool_call_update"
	UpdatePlan              = "plan"
	UpdateAvailableCommands = "available_commands_update"
	UpdateCurrentMode       = "current_mode_update"
	UpdateSessionInfoUpdate = "session_info_update"
	// UpdateUnknown is an update the adapter does not map: the UI ignores it.
	UpdateUnknown = "unknown"
)

// Tool call statuses.
const (
	ToolStatusPending    = "pending"
	ToolStatusInProgress = "in_progress"
	ToolStatusCompleted  = "completed"
	ToolStatusFailed     = "failed"
)

// ErrClosed is reported by calls made or in flight when the connection ends,
// as when the agent process exits.
var ErrClosed = errors.New("acp: agent connection closed")

// ContentBlock is one block of a prompt, as the SDK types it. The window sends
// text, resource links for files, and inline images; the SDK carries the rest
// without loss.
type ContentBlock = sdk.ContentBlock

// TextBlock is a prompt block of plain text.
func TextBlock(text string) ContentBlock { return sdk.TextBlock(text) }

// ResourceLinkBlock is a prompt block that attaches a file by name and URI
// (for example file:///home/me/notes.md) without sending its bytes. All agents
// must accept resource links.
func ResourceLinkBlock(name, uri string) ContentBlock { return sdk.ResourceLinkBlock(name, uri) }

// ImageBlock is a prompt block that attaches a picture by its base64 data, for
// agents that advertise the image prompt capability.
func ImageBlock(data, mimeType string) ContentBlock { return sdk.ImageBlock(data, mimeType) }

// Update is one session/update notification, flattened so that the UI has a
// single value to handle: chunks carry Text, tool calls carry ToolCall or
// ToolCallUpdate, and everything else carries its Type alone.
type Update struct {
	// SessionID is the session the update belongs to.
	SessionID string
	// Type is the update kind: UpdateAgentMessageChunk, ...
	Type string
	// Text is the text of a message or thought chunk; "" for other kinds.
	Text string
	// MessageID groups the chunks of one message, when the agent gives it.
	MessageID string
	// ToolCall is set for Type == UpdateToolCall.
	ToolCall *ToolCall
	// ToolCallUpdate is set for Type == UpdateToolCallUpdate.
	ToolCallUpdate *ToolCallUpdate
	// Commands is set for Type == UpdateAvailableCommands: the composer's
	// slash-menu entries, in the agent's order.
	Commands []AvailableCommand
	// CommandPrefix is the character the agent expects before a command name,
	// from the update's metadata; "" when the agent gives none.
	CommandPrefix string
}

// Chunk reports whether the update carries text.
func (u Update) Chunk() bool {
	return u.Type == UpdateAgentMessageChunk || u.Type == UpdateAgentThoughtChunk || u.Type == UpdateUserMessageChunk
}

// IsKnown reports whether the update is one the app maps.
func (u Update) IsKnown() bool {
	switch u.Type {
	case UpdateUserMessageChunk, UpdateAgentMessageChunk, UpdateAgentThoughtChunk,
		UpdateToolCall, UpdateToolCallUpdate, UpdatePlan,
		UpdateAvailableCommands, UpdateCurrentMode, UpdateSessionInfoUpdate:
		return true
	}
	return false
}

// ToolCall is a tool call the agent started.
type ToolCall struct {
	ToolCallID string
	Title      string
	Name       string
	Kind       string
	Status     string
	// RawInput and RawOutput are the JSON the SDK carried, for the row's
	// preview.
	RawInput  json.RawMessage
	RawOutput json.RawMessage
}

// ToolCallUpdate changes a tool call; every field but the id is optional.
type ToolCallUpdate struct {
	ToolCallID string
	Title      *string
	Name       *string
	Kind       *string
	Status     *string
	RawInput   json.RawMessage
	RawOutput  json.RawMessage
}

// AvailableCommand is one entry of the agent's slash menu: a command the agent
// executes, or a skill the agent can load. Kind is "" for a plain command and
// "skill" for a skill.
type AvailableCommand struct {
	// Name is the command or skill name, without the command prefix.
	Name string
	// Description is the one-line text the menu shows.
	Description string
	// Hint is the agent's hint for the command's input, "" when it takes none.
	Hint string
	// Kind is "" for a plain command and "skill" for a skill.
	Kind string
}

// Implementation describes a client or an agent, for the protocol handshake
// and for the window's footer.
type Implementation struct {
	Name    string
	Title   string
	Version string
}

// InitializeResult is the agent's answer to initialize.
type InitializeResult struct {
	ProtocolVersion int
	AgentInfo       *Implementation
	// PromptImage reports whether the agent accepts inline image content
	// blocks, so the window sends pictures, not just file links.
	PromptImage bool
}

// Kinds of a session configuration option, the value of ConfigOption.Kind.
const (
	// ConfigKindSelect is a single-choice option, such as the model or the
	// thinking level.
	ConfigKindSelect = "select"
	// ConfigKindBoolean is an on/off option.
	ConfigKindBoolean = "boolean"
)

// ConfigChoice is one value a select config option offers.
type ConfigChoice struct {
	// Value is the id to send back in SetConfigOption.
	Value string
	// Name is the label the window shows.
	Name string
	// Description is optional secondary text; "" when the agent gives none.
	Description string
}

// ConfigOption is one session configuration option the agent advertises: a
// select (the model and thinking-level pickers) or a boolean toggle. Kind says
// which: ConfigKindSelect fills Choices and Current, ConfigKindBoolean fills
// Boolean.
type ConfigOption struct {
	// ID is the option id to send in SetConfigOption.
	ID string
	// Name is the label the window shows for the option.
	Name string
	// Category is the agent's semantic hint ("model", "thought_level", ...)
	// for placement and icons, "" when it gives none.
	Category string
	// Kind is ConfigKindSelect or ConfigKindBoolean.
	Kind string
	// Current is the selected value id of a select option; "" for boolean.
	Current string
	// Boolean is the current value of a boolean option.
	Boolean bool
	// Choices are the values of a select option, flattened in the agent's
	// order: the ungrouped choices first, then each group's choices in order.
	// Group headers are dropped; a choice keeps its own name and description.
	Choices []ConfigChoice
}

// SessionResult is the agent's answer to session/new.
type SessionResult struct {
	SessionID string
	// ConfigOptions are the per-session selectors the agent advertises for the
	// composer (model, thinking level, ...); empty when it advertises none.
	ConfigOptions []ConfigOption
}

// mapConfigOptions flattens the SDK's config options into the window's model.
// Options of a kind this build does not know are skipped, so a newer agent
// cannot break the composer.
func mapConfigOptions(options []sdk.SessionConfigOption) []ConfigOption {
	if len(options) == 0 {
		return nil
	}
	out := make([]ConfigOption, 0, len(options))
	for _, option := range options {
		switch {
		case option.Select != nil:
			out = append(out, mapSelectConfigOption(option.Select))
		case option.Boolean != nil:
			out = append(out, mapBooleanConfigOption(option.Boolean))
		}
	}
	return out
}

func mapSelectConfigOption(option *sdk.SessionConfigOptionSelect) ConfigOption {
	out := ConfigOption{
		ID:       string(option.Id),
		Name:     option.Name,
		Category: configCategory(option.Category),
		Kind:     ConfigKindSelect,
		Current:  string(option.CurrentValue),
	}
	choices := option.Options
	if choices.Ungrouped != nil {
		out.Choices = append(out.Choices, mapConfigChoices(*choices.Ungrouped)...)
	}
	if choices.Grouped != nil {
		for _, group := range *choices.Grouped {
			out.Choices = append(out.Choices, mapConfigChoices(group.Options)...)
		}
	}
	return out
}

func mapBooleanConfigOption(option *sdk.SessionConfigOptionBoolean) ConfigOption {
	return ConfigOption{
		ID:       string(option.Id),
		Name:     option.Name,
		Category: configCategory(option.Category),
		Kind:     ConfigKindBoolean,
		Boolean:  option.CurrentValue,
	}
}

func mapConfigChoices(options []sdk.SessionConfigSelectOption) []ConfigChoice {
	choices := make([]ConfigChoice, 0, len(options))
	for _, option := range options {
		choice := ConfigChoice{Value: string(option.Value), Name: option.Name}
		if option.Description != nil {
			choice.Description = *option.Description
		}
		choices = append(choices, choice)
	}
	return choices
}

func configCategory(category *sdk.SessionConfigOptionCategory) string {
	if category == nil {
		return ""
	}
	return string(*category)
}

// FileURI is the file:// URI of a local path, the form a resource link
// carries. The path is made absolute and escaped as a URI must be: a space, a
// "#" that would start a fragment, a "?" that would start a query, and
// non-ASCII all become their percent-escapes.
func FileURI(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}
