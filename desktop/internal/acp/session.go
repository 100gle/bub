package acp

import (
	"context"
	"fmt"
)

// Initialize negotiates the protocol and reports what the agent can do. The
// client advertises no capabilities: Bub's tools act on the workspace
// themselves, so nothing is delegated to the window.
func (c *Client) Initialize(ctx context.Context, client Implementation) (*InitializeResult, error) {
	params := initializeParams{
		ProtocolVersion:    ProtocolVersion,
		ClientCapabilities: DefaultClientCapabilities(),
		ClientInfo:         &client,
	}
	var res InitializeResult
	if err := c.Call(ctx, "initialize", params, &res); err != nil {
		return nil, err
	}
	if res.ProtocolVersion != ProtocolVersion {
		return nil, fmt.Errorf("acp: agent speaks protocol version %d, this client speaks %d", res.ProtocolVersion, ProtocolVersion)
	}
	return &res, nil
}

// NewSession creates a session rooted at cwd, which must be absolute, with no
// MCP servers.
func (c *Client) NewSession(ctx context.Context, cwd string) (*SessionResult, error) {
	var res SessionResult
	if err := c.Call(ctx, "session/new", newSessionParams{Cwd: cwd, McpServers: []any{}}, &res); err != nil {
		return nil, err
	}
	if res.SessionID == "" {
		return nil, fmt.Errorf("acp: session/new: agent returned no session id")
	}
	return &res, nil
}

// Prompt sends one user message and returns the reason the agent stopped the
// turn. The agent streams its answer as session/update notifications while
// this call waits, so pass a ctx without a deadline for a turn the user stops
// with Cancel.
func (c *Client) Prompt(ctx context.Context, sessionID string, prompt []ContentBlock) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("acp: prompt: no session")
	}
	if len(prompt) == 0 {
		return "", fmt.Errorf("acp: prompt: no content")
	}
	var res PromptResult
	params := promptParams{SessionID: sessionID, Prompt: prompt}
	if err := c.Call(ctx, "session/prompt", params, &res); err != nil {
		return "", err
	}
	switch res.StopReason {
	case StopEndTurn, StopMaxTokens, StopMaxTurnRequests, StopRefusal, StopCancelled:
		return res.StopReason, nil
	case "":
		return StopUnknown, fmt.Errorf("acp: session/prompt: agent returned no stop reason")
	default:
		return StopUnknown, nil
	}
}

// Cancel asks the agent to stop the turn running in a session. It is a
// notification: the prompt call answers with the "cancelled" stop reason.
func (c *Client) Cancel(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("acp: cancel: no session")
	}
	return c.notify("session/cancel", cancelParams{SessionID: sessionID})
}
