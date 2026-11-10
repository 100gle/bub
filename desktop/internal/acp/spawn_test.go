package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// TestHelperAgent is not a test: it is the fake agent Start should spawn. The
// test binary runs itself with -test.run=TestHelperAgent and ACP_HELPER_AGENT
// set, so the process speaks ACP on its standard input and output, as a real
// agent does, and writes one line to its standard error.
func TestHelperAgent(t *testing.T) {
	if os.Getenv("ACP_HELPER_AGENT") != "1" {
		t.Skip("helper process only: run by TestStartTalksToAProcess")
	}
	os.Stderr.WriteString("fake agent ready\n")
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	type message struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Method  string          `json:"method,omitempty"`
		Params  json.RawMessage `json:"params,omitempty"`
	}
	respond := func(id json.RawMessage, result any) {
		_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	}
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			os.Exit(0) // standard input closed: the client quit
		}
		var msg message
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		switch msg.Method {
		case "initialize":
			respond(msg.ID, map[string]any{
				"protocolVersion": ProtocolVersion,
				"agentInfo":       map[string]any{"name": "fake-bub", "version": "0.0.1"},
			})
		case "session/new":
			respond(msg.ID, map[string]any{"sessionId": "fake-session"})
		case "session/prompt":
			_ = encoder.Encode(map[string]any{
				"jsonrpc": "2.0",
				"method":  "session/update",
				"params": map[string]any{
					"sessionId": "fake-session",
					"update": map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"content":       map[string]any{"type": "text", "text": "hello from a process"},
					},
				},
			})
			respond(msg.ID, map[string]any{"stopReason": "end_turn"})
		case "session/cancel":
			// A cancel notification asks for nothing back.
		}
	}
}

func TestStartTalksToAProcess(t *testing.T) {
	if os.Getenv("ACP_HELPER_AGENT") == "1" {
		t.Skip("inside the helper process")
	}
	t.Setenv("ACP_HELPER_AGENT", "1")
	proc, err := Start(Config{
		Command: []string{os.Args[0], "-test.run=TestHelperAgent"},
		Stderr:  io.Discard, // keep the test output clean; the tail is kept anyway
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = proc.Close() })

	var updates []Update
	proc.Client.SetUpdateHandler(func(u Update) { updates = append(updates, u) })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, err := proc.Client.Initialize(ctx, Implementation{Name: "bub-desktop", Version: "0.1.0"})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if info.AgentInfo == nil || info.AgentInfo.Name != "fake-bub" {
		t.Errorf("agentInfo = %+v, want fake-bub", info.AgentInfo)
	}
	session, err := proc.Client.NewSession(ctx, "/tmp")
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	if session.SessionID != "fake-session" {
		t.Errorf("session id = %q, want fake-session", session.SessionID)
	}
	stop, err := proc.Client.Prompt(ctx, session.SessionID, []ContentBlock{TextBlock("hi")})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if stop != StopEndTurn {
		t.Errorf("stop reason = %q, want end_turn", stop)
	}
	if len(updates) != 1 || updates[0].Text != "hello from a process" {
		t.Errorf("updates = %+v, want one message chunk", updates)
	}
	if !strings.Contains(proc.Stderr(), "fake agent ready") {
		t.Errorf("stderr = %q, want the agent's line captured", proc.Stderr())
	}

	// Closing the client ends the process, as quitting the app does.
	if err := proc.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
	select {
	case <-proc.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("the agent process did not exit after Close")
	}
}
