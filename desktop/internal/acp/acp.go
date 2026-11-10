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

// ContentBlock is one block of a prompt, as the SDK types it. The window only
// sends text; the SDK carries the other kinds without loss.
type ContentBlock = sdk.ContentBlock

// TextBlock is a prompt block of plain text.
func TextBlock(text string) ContentBlock { return sdk.TextBlock(text) }

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
}

// SessionResult is the agent's answer to session/new.
type SessionResult struct {
	SessionID string
}
