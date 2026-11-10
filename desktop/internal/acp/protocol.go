// Package acp implements a small, transport-agnostic client for the Agent
// Client Protocol (ACP) v1: JSON-RPC 2.0 messages, one per line, over any
// io.ReadWriteCloser.
//
// The client owns the protocol half of a Bub Desktop session: it frames
// requests, dispatches responses and session/update notifications, and
// answers requests the agent makes of the client. It never starts a process
// itself; Start does that, and NewClient takes any connection, so tests can
// drive the client through in-memory pipes.
package acp

import (
	"encoding/json"
	"fmt"
)

// ProtocolVersion is the ACP version this client speaks.
const ProtocolVersion = 1

// JSON-RPC 2.0 error codes the client uses or reports.
const (
	// ErrCodeMethodNotFound is what the client answers requests it does not
	// implement, so that an agent never waits for a reply that never comes.
	ErrCodeMethodNotFound = -32601
	// ErrCodeParseError is what a peer reports for a line that is not JSON.
	ErrCodeParseError = -32700
	// ErrCodeInvalidParams reports a request whose parameters cannot be used.
	ErrCodeInvalidParams = -32602
	// ErrCodeInternalError reports a failure inside a peer.
	ErrCodeInternalError = -32603
)

// Stop reasons of a prompt turn.
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

// Session update kinds, the value of the "sessionUpdate" field.
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
)

// Tool call statuses.
const (
	ToolStatusPending    = "pending"
	ToolStatusInProgress = "in_progress"
	ToolStatusCompleted  = "completed"
	ToolStatusFailed     = "failed"
)

// RPCError is a JSON-RPC error object.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("json-rpc error %d: %s", e.Code, e.Message)
}

// ContentBlock is one block of a prompt: text, image, audio, a resource link
// or an embedded resource. The client only sends text; the other fields are
// kept so that blocks read from an agent parse without loss.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// URI and MimeType describe resource_link and resource blocks.
	URI      string `json:"uri,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// TextBlock is a prompt block of plain text.
func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

// BlockText returns the text of a content block, "" for blocks of other
// kinds.
func BlockText(b ContentBlock) string {
	if b.Type != "text" {
		return ""
	}
	return b.Text
}

// Implementation describes a client or an agent.
type Implementation struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// FileSystemCapabilities advertises the fs/* methods a client implements.
type FileSystemCapabilities struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

// ClientCapabilities advertises what the client can do for the agent.
type ClientCapabilities struct {
	FS       FileSystemCapabilities `json:"fs"`
	Terminal bool                   `json:"terminal"`
}

// DefaultClientCapabilities is what Bub Desktop supports: nothing delegated.
// Bub's tools act on the workspace themselves, so the agent never has to ask
// the client to read a file, write one or run a terminal command.
func DefaultClientCapabilities() ClientCapabilities {
	return ClientCapabilities{
		FS:       FileSystemCapabilities{ReadTextFile: false, WriteTextFile: false},
		Terminal: false,
	}
}

// InitializeResult is the agent's answer to initialize.
type InitializeResult struct {
	ProtocolVersion   int             `json:"protocolVersion"`
	AgentCapabilities json.RawMessage `json:"agentCapabilities,omitempty"`
	AgentInfo         *Implementation `json:"agentInfo,omitempty"`
	AuthMethods       json.RawMessage `json:"authMethods,omitempty"`
}

// SessionResult is the agent's answer to session/new.
type SessionResult struct {
	SessionID string          `json:"sessionId"`
	Modes     json.RawMessage `json:"modes,omitempty"`
}

// PromptResult is the agent's answer to session/prompt.
type PromptResult struct {
	StopReason string `json:"stopReason"`
}

// ToolCall is a tool call the agent started.
type ToolCall struct {
	ToolCallID string            `json:"toolCallId"`
	Title      string            `json:"title,omitempty"`
	Name       string            `json:"name,omitempty"`
	Kind       string            `json:"kind,omitempty"`
	Status     string            `json:"status,omitempty"`
	RawInput   json.RawMessage   `json:"rawInput,omitempty"`
	RawOutput  json.RawMessage   `json:"rawOutput,omitempty"`
	Content    []json.RawMessage `json:"content,omitempty"`
}

// ToolCallUpdate changes a tool call; every field but the id is optional.
type ToolCallUpdate struct {
	ToolCallID string          `json:"toolCallId"`
	Title      *string         `json:"title,omitempty"`
	Name       *string         `json:"name,omitempty"`
	Kind       *string         `json:"kind,omitempty"`
	Status     *string         `json:"status,omitempty"`
	RawInput   json.RawMessage `json:"rawInput,omitempty"`
	RawOutput  json.RawMessage `json:"rawOutput,omitempty"`
}

// Update is one session/update notification, flattened so that the UI has a
// single value to handle: chunks carry Text, tool calls carry ToolCall or
// ToolCallUpdate, and everything else carries its Type alone.
type Update struct {
	// SessionID is the session the update belongs to.
	SessionID string
	// Type is the "sessionUpdate" value: UpdateAgentMessageChunk, ...
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

// IsKnown reports whether the client understands the update.
func (u Update) IsKnown() bool {
	switch u.Type {
	case UpdateUserMessageChunk, UpdateAgentMessageChunk, UpdateAgentThoughtChunk,
		UpdateToolCall, UpdateToolCallUpdate, UpdatePlan,
		UpdateAvailableCommands, UpdateCurrentMode, UpdateSessionInfoUpdate:
		return true
	}
	return false
}

// wireMessage is any incoming JSON-RPC message: a response has no method, a
// request an id and a method, a notification a method alone.
type wireMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// request is an outgoing JSON-RPC request.
type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// response is an outgoing JSON-RPC response: Result or Error, never both.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// notification is an outgoing JSON-RPC notification.
type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type initializeParams struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         *Implementation    `json:"clientInfo,omitempty"`
}

type newSessionParams struct {
	Cwd        string `json:"cwd"`
	McpServers []any  `json:"mcpServers"`
}

type promptParams struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

type cancelParams struct {
	SessionID string `json:"sessionId"`
}

type sessionNotification struct {
	SessionID string          `json:"sessionId"`
	Update    json.RawMessage `json:"update"`
}

type contentChunk struct {
	Content   ContentBlock `json:"content"`
	MessageID string       `json:"messageId,omitempty"`
}

// parseSessionNotification reads the params of a session/update notification
// into an Update. An update of a kind the client does not know, or a chunk
// whose content is not text, yields a value with its Type set and nothing
// else, which callers ignore.
func parseSessionNotification(params json.RawMessage) (Update, error) {
	var n sessionNotification
	if err := json.Unmarshal(params, &n); err != nil {
		return Update{}, fmt.Errorf("acp: parse session/update: %w", err)
	}
	var head struct {
		SessionUpdate string `json:"sessionUpdate"`
	}
	if err := json.Unmarshal(n.Update, &head); err != nil {
		return Update{}, fmt.Errorf("acp: parse session/update body: %w", err)
	}
	u := Update{SessionID: n.SessionID, Type: head.SessionUpdate}
	switch head.SessionUpdate {
	case UpdateUserMessageChunk, UpdateAgentMessageChunk, UpdateAgentThoughtChunk:
		var chunk contentChunk
		if err := json.Unmarshal(n.Update, &chunk); err != nil {
			return u, fmt.Errorf("acp: parse %s: %w", head.SessionUpdate, err)
		}
		u.Text = BlockText(chunk.Content)
		u.MessageID = chunk.MessageID
	case UpdateToolCall:
		var call ToolCall
		if err := json.Unmarshal(n.Update, &call); err != nil {
			return u, fmt.Errorf("acp: parse tool_call: %w", err)
		}
		u.ToolCall = &call
	case UpdateToolCallUpdate:
		var update ToolCallUpdate
		if err := json.Unmarshal(n.Update, &update); err != nil {
			return u, fmt.Errorf("acp: parse tool_call_update: %w", err)
		}
		u.ToolCallUpdate = &update
	}
	return u, nil
}
