package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"sync"
	"time"

	sdk "github.com/caelis-labs/acp-go-sdk"
)

// cancelTimeout bounds sending session/cancel, so a wedged agent cannot leak
// the goroutine that stops a turn.
const cancelTimeout = 5 * time.Second

// Client is the window's view of the ACP connection: a thin adapter that the
// SDK drives as its acp.Client, with the app-facing calls the window makes on
// top. It implements only the two baseline client methods; the filesystem,
// terminal and permission-request capabilities stay unimplemented, which the
// SDK answers with JSON-RPC "method not found" (-32601) so an agent asking for
// one never waits.
type Client struct {
	mu        sync.Mutex
	conn      *sdk.ClientSideConnection
	onUpdate  func(Update)
	onClose   func(error)
	failed    bool
	closedErr error
	closeOnce sync.Once
}

func newClient() *Client { return &Client{} }

// The adapter is the SDK's baseline client: permission requests and session
// updates, and nothing else.
var _ sdk.Client = (*Client)(nil)

// attach wires the SDK connection in and watches it: when it ends, for a clean
// close or a dead agent, the close handler hears why once.
func (c *Client) attach(conn *sdk.ClientSideConnection) {
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	go func() {
		<-conn.Done()
		c.fail(conn.Err())
	}()
}

// SetUpdateHandler sets what receives session updates, as the app's model.
// The handler runs on the SDK's notification goroutine: the window queues
// win.Update from it rather than touching state here.
func (c *Client) SetUpdateHandler(fn func(Update)) {
	c.mu.Lock()
	c.onUpdate = fn
	c.mu.Unlock()
}

// SetCloseHandler sets what hears that the connection ended, once, with the
// reason (ErrClosed when the agent simply went away). A handler set after the
// connection already ended hears at once, so a process that dies between the
// handshake and installing the handler cannot leave the window "ready".
func (c *Client) SetCloseHandler(fn func(error)) {
	c.mu.Lock()
	c.onClose = fn
	failed, err := c.failed, c.closedErr
	c.mu.Unlock()
	if failed && fn != nil {
		fn(err)
	}
}

// SetLogger sends the SDK's own diagnostics to l, at warning level.
func (c *Client) SetLogger(l *log.Logger) {
	if l == nil {
		l = log.New(io.Discard, "", 0)
	}
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn != nil {
		conn.SetLogger(slog.New(slog.NewTextHandler(l.Writer(), &slog.HandlerOptions{Level: slog.LevelWarn})))
	}
}

// RequestPermission answers a permission request by declining it: this build
// has no permissions UI, so a tool that needs consent does not run.
func (c *Client) RequestPermission(context.Context, sdk.RequestPermissionRequest) (sdk.RequestPermissionResponse, error) {
	return sdk.RequestPermissionResponse{Outcome: sdk.NewRequestPermissionOutcomeCancelled()}, nil
}

// SessionUpdate maps one typed SDK notification onto the app's Update model.
func (c *Client) SessionUpdate(_ context.Context, notification sdk.SessionNotification) error {
	update := mapSessionUpdate(notification)
	c.mu.Lock()
	handler := c.onUpdate
	c.mu.Unlock()
	if handler != nil {
		handler(update)
	}
	return nil
}

// Initialize negotiates the protocol and reports what the agent can do. The
// client advertises no capabilities: Bub's tools act on the workspace
// themselves, so nothing is delegated to the window.
func (c *Client) Initialize(ctx context.Context, clientInfo Implementation) (*InitializeResult, error) {
	conn, err := c.connection()
	if err != nil {
		return nil, err
	}
	info := &sdk.Implementation{Name: clientInfo.Name, Version: clientInfo.Version}
	if clientInfo.Title != "" {
		info.Title = sdk.Ptr(clientInfo.Title)
	}
	res, err := conn.Initialize(ctx, sdk.InitializeRequest{
		ProtocolVersion: sdk.ProtocolVersion(ProtocolVersion),
		ClientInfo:      info,
	})
	if err != nil {
		return nil, c.callError("initialize", err)
	}
	if int(res.ProtocolVersion) != ProtocolVersion {
		return nil, fmt.Errorf("acp: agent speaks protocol version %d, this client speaks %d", int(res.ProtocolVersion), ProtocolVersion)
	}
	out := &InitializeResult{ProtocolVersion: int(res.ProtocolVersion)}
	if res.AgentInfo != nil {
		out.AgentInfo = &Implementation{
			Name:    res.AgentInfo.Name,
			Version: res.AgentInfo.Version,
		}
		if res.AgentInfo.Title != nil {
			out.AgentInfo.Title = *res.AgentInfo.Title
		}
	}
	return out, nil
}

// NewSession creates a session rooted at cwd, which must be absolute, with no
// MCP servers.
func (c *Client) NewSession(ctx context.Context, cwd string) (*SessionResult, error) {
	conn, err := c.connection()
	if err != nil {
		return nil, err
	}
	res, err := conn.NewSession(ctx, sdk.NewSessionRequest{
		Cwd:        cwd,
		McpServers: []sdk.McpServer{},
	})
	if err != nil {
		return nil, c.callError("session/new", err)
	}
	if res.SessionId == "" {
		return nil, fmt.Errorf("acp: session/new: agent returned no session id")
	}
	return &SessionResult{SessionID: string(res.SessionId)}, nil
}

// Prompt sends one user message and returns the reason the agent stopped the
// turn. The agent streams its answer through the update handler while this
// call waits, so pass a ctx without a deadline for a turn the user stops with
// Cancel.
func (c *Client) Prompt(ctx context.Context, sessionID string, prompt []ContentBlock) (string, error) {
	conn, err := c.connection()
	if err != nil {
		return "", err
	}
	if sessionID == "" {
		return "", errors.New("acp: prompt: no session")
	}
	if len(prompt) == 0 {
		return "", errors.New("acp: prompt: no content")
	}
	res, err := conn.Prompt(ctx, sdk.PromptRequest{
		SessionId: sdk.SessionId(sessionID),
		Prompt:    prompt,
	})
	if err != nil {
		return "", c.callError("session/prompt", err)
	}
	switch res.StopReason {
	case sdk.StopReasonEndTurn, sdk.StopReasonMaxTokens, sdk.StopReasonMaxTurnRequests,
		sdk.StopReasonRefusal, sdk.StopReasonCancelled:
		return string(res.StopReason), nil
	case "":
		return StopUnknown, errors.New("acp: session/prompt: agent returned no stop reason")
	default:
		return StopUnknown, nil
	}
}

// Cancel asks the agent to stop the turn running in a session. It is a
// notification: the prompt call answers with the "cancelled" stop reason.
func (c *Client) Cancel(sessionID string) error {
	conn, err := c.connection()
	if err != nil {
		return err
	}
	if sessionID == "" {
		return errors.New("acp: cancel: no session")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cancelTimeout)
	defer cancel()
	return c.callError("session/cancel", conn.Cancel(ctx, sdk.CancelNotification{SessionId: sdk.SessionId(sessionID)}))
}

// connection returns the SDK connection, or ErrClosed before the process
// started and after it ended.
func (c *Client) connection() (*sdk.ClientSideConnection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil, ErrClosed
	}
	return c.conn, nil
}

// callError labels a failed call and folds the SDK's connection errors into
// ErrClosed, which the app treats as "the agent went away" rather than a
// failed turn.
func (c *Client) callError(method string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sdk.ErrConnectionClosed) || errors.Is(err, sdk.ErrPeerClosed) {
		return fmt.Errorf("acp: %s: %w", method, ErrClosed)
	}
	return fmt.Errorf("acp: %s: %w", method, err)
}

// fail tells the close handler once why the connection ended.
func (c *Client) fail(cause error) {
	c.closeOnce.Do(func() {
		err := ErrClosed
		if cause != nil && !errors.Is(cause, sdk.ErrConnectionClosed) && !errors.Is(cause, sdk.ErrPeerClosed) {
			err = fmt.Errorf("acp: connection ended: %w", cause)
		}
		c.mu.Lock()
		c.failed = true
		c.closedErr = err
		handler := c.onClose
		c.mu.Unlock()
		if handler != nil {
			handler(err)
		}
	})
}

// mapSessionUpdate flattens one typed SDK notification into the app's Update.
// Kinds the window does not draw keep their name and nothing else, so it can
// ignore them.
func mapSessionUpdate(n sdk.SessionNotification) Update {
	u := Update{SessionID: string(n.SessionId)}
	update := n.Update
	switch {
	case update.AgentMessageChunk != nil:
		u.Type = UpdateAgentMessageChunk
		u.Text = contentText(update.AgentMessageChunk.Content)
		u.MessageID = messageID(update.AgentMessageChunk.MessageId)
	case update.AgentThoughtChunk != nil:
		u.Type = UpdateAgentThoughtChunk
		u.Text = contentText(update.AgentThoughtChunk.Content)
		u.MessageID = messageID(update.AgentThoughtChunk.MessageId)
	case update.UserMessageChunk != nil:
		u.Type = UpdateUserMessageChunk
		u.Text = contentText(update.UserMessageChunk.Content)
		u.MessageID = messageID(update.UserMessageChunk.MessageId)
	case update.ToolCall != nil:
		u.Type = UpdateToolCall
		u.ToolCall = mapToolCall(update.ToolCall)
	case update.ToolCallUpdate != nil:
		u.Type = UpdateToolCallUpdate
		u.ToolCallUpdate = mapToolCallUpdate(update.ToolCallUpdate)
	case update.Plan != nil:
		u.Type = UpdatePlan
	case update.AvailableCommandsUpdate != nil:
		u.Type = UpdateAvailableCommands
	case update.CurrentModeUpdate != nil:
		u.Type = UpdateCurrentMode
	case update.SessionInfoUpdate != nil:
		u.Type = UpdateSessionInfoUpdate
	default:
		u.Type = UpdateUnknown
	}
	return u
}

func mapToolCall(call *sdk.SessionUpdateToolCall) *ToolCall {
	return &ToolCall{
		ToolCallID: string(call.ToolCallId),
		Title:      call.Title,
		Name:       stringValue(call.Name),
		Kind:       string(call.Kind),
		Status:     string(call.Status),
		RawInput:   rawValue(call.RawInput),
		RawOutput:  rawValue(call.RawOutput),
	}
}

func mapToolCallUpdate(update *sdk.SessionToolCallUpdate) *ToolCallUpdate {
	return &ToolCallUpdate{
		ToolCallID: string(update.ToolCallId),
		Title:      update.Title,
		Name:       update.Name,
		Kind:       stringPointer(update.Kind),
		Status:     stringPointer(update.Status),
		RawInput:   rawValue(update.RawInput),
		RawOutput:  rawValue(update.RawOutput),
	}
}

// contentText is the text of a content block, "" for blocks of other kinds.
func contentText(block sdk.ContentBlock) string {
	if block.Text == nil {
		return ""
	}
	return block.Text.Text
}

func messageID(id *sdk.MessageId) string {
	if id == nil {
		return ""
	}
	return string(*id)
}

func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func stringPointer[T ~string](s *T) *string {
	if s == nil {
		return nil
	}
	value := string(*s)
	return &value
}

// rawValue is the JSON the SDK carried for a raw field, so the row's preview
// shows what the agent sent.
func rawValue(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}
