package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeAgent drives a Client from the agent's side of an in-memory connection:
// the test reads the messages the client sends and answers them, as a real
// agent does. It runs on the test's goroutine, so it may call t.Fatal; calls
// that wait for a response run in goroutines the test starts.
type fakeAgent struct {
	t    *testing.T
	conn net.Conn
	rd   *bufio.Reader
}

func newFakeAgent(t *testing.T) (*Client, *fakeAgent) {
	t.Helper()
	// TCP on the loopback buffers writes, as a process's pipes do; an
	// in-memory net.Pipe would block a write until the other side reads it,
	// which deadlocks a client call that writes while the test reads later.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("fake agent: listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("fake agent: dial: %v", err)
	}
	agentConn, err := ln.Accept()
	if err != nil {
		t.Fatalf("fake agent: accept: %v", err)
	}
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = agentConn.Close()
	})
	return NewClient(clientConn), &fakeAgent{t: t, conn: agentConn, rd: bufio.NewReader(agentConn)}
}

// next reads one JSON-RPC message.
func (f *fakeAgent) next() map[string]any {
	f.t.Helper()
	_ = f.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := f.rd.ReadBytes('\n')
	if err != nil {
		f.t.Fatalf("fake agent: reading a message: %v", err)
	}
	var msg map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(line), &msg); err != nil {
		f.t.Fatalf("fake agent: message %q is not JSON: %v", line, err)
	}
	return msg
}

// expect reads a message and checks its method, returning its id and params.
func (f *fakeAgent) expect(method string) (id any, params map[string]any) {
	f.t.Helper()
	msg := f.next()
	if got := msg["method"]; got != method {
		f.t.Fatalf("fake agent: got method %v, want %s (%v)", got, method, msg)
	}
	id = msg["id"]
	if raw, ok := msg["params"].(map[string]any); ok {
		params = raw
	}
	return id, params
}

// writef writes a raw line, valid JSON or not.
func (f *fakeAgent) writef(format string, args ...any) {
	f.t.Helper()
	line := format
	if len(args) > 0 {
		line = fmt.Sprintf(format, args...)
	}
	_ = f.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := f.conn.Write([]byte(line + "\n")); err != nil {
		f.t.Fatalf("fake agent: writing: %v", err)
	}
}

func (f *fakeAgent) send(v any) {
	f.t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		f.t.Fatalf("fake agent: marshal: %v", err)
	}
	f.writef("%s", data)
}

func (f *fakeAgent) respond(id any, result any) {
	f.t.Helper()
	f.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (f *fakeAgent) respondError(id any, code int, message string) {
	f.t.Helper()
	f.send(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}})
}

func (f *fakeAgent) notify(method string, params any) {
	f.t.Helper()
	f.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (f *fakeAgent) sessionUpdate(update map[string]any) {
	f.t.Helper()
	f.notify("session/update", map[string]any{"sessionId": "sess-1", "update": update})
}

// textChunk builds the update of a message or thought chunk.
func textChunk(t *testing.T, kind, text string) map[string]any {
	t.Helper()
	return map[string]any{
		"sessionUpdate": kind,
		"content":       map[string]any{"type": "text", "text": text},
	}
}

type callResult[T any] struct {
	v   T
	err error
}

// goCall runs a call that waits for the agent's answer in a goroutine, so the
// test can answer as the agent: awaiting it first would deadlock.
func goCall[T any](fn func() (T, error)) <-chan callResult[T] {
	ch := make(chan callResult[T], 1)
	go func() {
		v, err := fn()
		ch <- callResult[T]{v: v, err: err}
	}()
	return ch
}

func await[T any](t *testing.T, ch <-chan callResult[T]) (T, error) {
	t.Helper()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-time.After(5 * time.Second):
		var zero T
		t.Fatal("call did not return")
		return zero, nil
	}
}

func TestHandshakePromptStreamingAndToolCalls(t *testing.T) {
	client, agent := newFakeAgent(t)
	var updates []Update
	client.SetUpdateHandler(func(u Update) { updates = append(updates, u) })

	initCh := goCall(func() (*InitializeResult, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return client.Initialize(ctx, Implementation{Name: "bub-desktop", Version: "0.1.0"})
	})
	initID, params := agent.expect("initialize")
	if got := params["protocolVersion"]; got != float64(ProtocolVersion) {
		t.Errorf("initialize protocolVersion = %v, want %d", got, ProtocolVersion)
	}
	caps, _ := params["clientCapabilities"].(map[string]any)
	if caps["terminal"] != false {
		t.Errorf("clientCapabilities.terminal = %v, want false", caps["terminal"])
	}
	fs, _ := caps["fs"].(map[string]any)
	if fs["readTextFile"] != false || fs["writeTextFile"] != false {
		t.Errorf("clientCapabilities.fs = %v, want both false", fs)
	}
	info, _ := params["clientInfo"].(map[string]any)
	if info["name"] != "bub-desktop" {
		t.Errorf("clientInfo.name = %v, want bub-desktop", info["name"])
	}
	agent.respond(initID, map[string]any{
		"protocolVersion": ProtocolVersion,
		"agentCapabilities": map[string]any{
			"loadSession":        false,
			"promptCapabilities": map[string]any{},
		},
		"agentInfo": map[string]any{"name": "bub", "version": "9.9.9"},
	})
	init, err := await(t, initCh)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if init.AgentInfo == nil || init.AgentInfo.Name != "bub" || init.AgentInfo.Version != "9.9.9" {
		t.Errorf("agentInfo = %+v, want bub 9.9.9", init.AgentInfo)
	}

	sessionCh := goCall(func() (*SessionResult, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return client.NewSession(ctx, "/tmp/workspace")
	})
	sessionID, params := agent.expect("session/new")
	if params["cwd"] != "/tmp/workspace" {
		t.Errorf("session/new cwd = %v, want /tmp/workspace", params["cwd"])
	}
	if servers, ok := params["mcpServers"].([]any); !ok || len(servers) != 0 {
		t.Errorf("session/new mcpServers = %v, want []", params["mcpServers"])
	}
	agent.respond(sessionID, map[string]any{"sessionId": "sess-1"})
	session, err := await(t, sessionCh)
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	if session.SessionID != "sess-1" {
		t.Fatalf("session = %+v, want sess-1", session)
	}

	stopCh := goCall(func() (string, error) {
		return client.Prompt(context.Background(), session.SessionID, []ContentBlock{TextBlock("hello")})
	})
	promptID, params := agent.expect("session/prompt")
	if params["sessionId"] != "sess-1" {
		t.Errorf("session/prompt sessionId = %v", params["sessionId"])
	}
	blocks, _ := params["prompt"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("prompt blocks = %v, want one", params["prompt"])
	}
	block, _ := blocks[0].(map[string]any)
	if block["type"] != "text" || block["text"] != "hello" {
		t.Errorf("prompt block = %v, want text hello", block)
	}
	agent.sessionUpdate(textChunk(t, "agent_message_chunk", "Hello"))
	agent.sessionUpdate(textChunk(t, "agent_thought_chunk", "thinking"))
	agent.sessionUpdate(map[string]any{
		"sessionUpdate": "tool_call",
		"toolCallId":    "call-1",
		"title":         "read README.md",
		"kind":          "read",
		"status":        "pending",
		"rawInput":      map[string]any{"path": "README.md"},
	})
	agent.sessionUpdate(map[string]any{
		"sessionUpdate": "tool_call_update",
		"toolCallId":    "call-1",
		"status":        "completed",
		"rawOutput":     "the readme",
	})
	agent.respond(promptID, map[string]any{"stopReason": "end_turn"})
	stop, err := await(t, stopCh)
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if stop != StopEndTurn {
		t.Errorf("stop reason = %q, want %q", stop, StopEndTurn)
	}

	if len(updates) != 4 {
		t.Fatalf("got %d updates, want 4: %+v", len(updates), updates)
	}
	if updates[0].Type != UpdateAgentMessageChunk || updates[0].Text != "Hello" {
		t.Errorf("update 0 = %+v, want an agent_message_chunk Hello", updates[0])
	}
	if updates[1].Type != UpdateAgentThoughtChunk || updates[1].Text != "thinking" {
		t.Errorf("update 1 = %+v, want an agent_thought_chunk thinking", updates[1])
	}
	if updates[2].ToolCall == nil || updates[2].ToolCall.ToolCallID != "call-1" || updates[2].ToolCall.Status != "pending" {
		t.Errorf("update 2 = %+v, want a pending tool_call", updates[2])
	}
	if updates[3].ToolCallUpdate == nil || updates[3].ToolCallUpdate.ToolCallID != "call-1" ||
		updates[3].ToolCallUpdate.Status == nil || *updates[3].ToolCallUpdate.Status != "completed" {
		t.Errorf("update 3 = %+v, want a completed tool_call_update", updates[3])
	}
}

func TestCancelSendsNotification(t *testing.T) {
	client, agent := newFakeAgent(t)
	client.SetUpdateHandler(func(Update) {})

	done := make(chan error, 1)
	go func() {
		_, err := client.Prompt(context.Background(), "sess-1", []ContentBlock{TextBlock("go")})
		done <- err
	}()
	promptID, _ := agent.expect("session/prompt")
	if err := client.Cancel("sess-1"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	msg := agent.next()
	if msg["method"] != "session/cancel" {
		t.Fatalf("got method %v, want session/cancel", msg["method"])
	}
	if _, hasID := msg["id"]; hasID {
		t.Errorf("session/cancel carries an id %v: it is a notification", msg["id"])
	}
	params, _ := msg["params"].(map[string]any)
	if params["sessionId"] != "sess-1" {
		t.Errorf("session/cancel sessionId = %v", params["sessionId"])
	}
	agent.respond(promptID, map[string]any{"stopReason": "cancelled"})
	if err := <-done; err != nil {
		t.Fatalf("prompt after cancel: %v", err)
	}
}

func TestErrorResponse(t *testing.T) {
	client, agent := newFakeAgent(t)

	done := make(chan error, 1)
	go func() {
		_, err := client.NewSession(context.Background(), "/tmp/nope")
		done <- err
	}()
	id, _ := agent.expect("session/new")
	agent.respondError(id, -32002, "workspace not found")
	err := <-done
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("error = %v, want an *RPCError", err)
	}
	if rpcErr.Code != -32002 || !strings.Contains(rpcErr.Message, "workspace") {
		t.Errorf("error = %+v, want -32002 workspace not found", rpcErr)
	}
}

func TestMalformedLineIsSkipped(t *testing.T) {
	client, agent := newFakeAgent(t)
	var logged bytes.Buffer
	client.SetLogger(log.New(&logged, "", 0))
	var updates []Update
	client.SetUpdateHandler(func(u Update) { updates = append(updates, u) })

	done := make(chan error, 1)
	go func() {
		_, err := client.Prompt(context.Background(), "sess-1", []ContentBlock{TextBlock("hi")})
		done <- err
	}()
	id, _ := agent.expect("session/prompt")
	agent.writef("this is not JSON")
	agent.writef(`{"jsonrpc":"2.0","id":"not-a-number"}`)
	agent.sessionUpdate(textChunk(t, "agent_message_chunk", "still here"))
	agent.respond(id, map[string]any{"stopReason": "end_turn"})
	if err := <-done; err != nil {
		t.Fatalf("prompt after malformed lines: %v", err)
	}
	if len(updates) != 1 || updates[0].Text != "still here" {
		t.Errorf("updates = %+v, want the update after the malformed lines", updates)
	}
	if !strings.Contains(logged.String(), "malformed line") {
		t.Errorf("log %q does not mention the malformed line", logged.String())
	}
}

func TestAgentRequestIsAnsweredWithMethodNotFound(t *testing.T) {
	client, agent := newFakeAgent(t)

	done := make(chan error, 1)
	go func() {
		_, err := client.Prompt(context.Background(), "sess-1", []ContentBlock{TextBlock("hi")})
		done <- err
	}()
	promptID, _ := agent.expect("session/prompt")
	// The agent asks the client to read a file; the client supports no fs
	// methods, so it must answer instead of leaving the agent waiting.
	agent.send(map[string]any{
		"jsonrpc": "2.0",
		"id":      99,
		"method":  "fs/read_text_file",
		"params":  map[string]any{"sessionId": "sess-1", "path": "/tmp/x"},
	})
	answer := agent.next()
	if answer["id"] != float64(99) {
		t.Errorf("answer id = %v, want 99", answer["id"])
	}
	errObj, _ := answer["error"].(map[string]any)
	if errObj == nil {
		t.Fatalf("answer = %v, want an error", answer)
	}
	if errObj["code"] != float64(ErrCodeMethodNotFound) {
		t.Errorf("answer code = %v, want %d", errObj["code"], ErrCodeMethodNotFound)
	}
	if _, ok := answer["result"]; ok {
		t.Errorf("answer = %v, want no result", answer)
	}
	agent.respond(promptID, map[string]any{"stopReason": "end_turn"})
	if err := <-done; err != nil {
		t.Fatalf("prompt: %v", err)
	}
}

func TestCallTimeout(t *testing.T) {
	client, agent := newFakeAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := client.Call(ctx, "session/prompt", map[string]any{"sessionId": "s"}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want a deadline", err)
	}
	// The agent's late answer goes nowhere, and the client keeps working.
	id, _ := agent.expect("session/prompt")
	agent.respond(id, map[string]any{"stopReason": "end_turn"})
	agent.respondError(999999, -32603, "unknown id")
	time.Sleep(20 * time.Millisecond) // let the read loop drop the answers
}

func TestUnknownUpdateIsIgnored(t *testing.T) {
	client, agent := newFakeAgent(t)
	var updates []Update
	client.SetUpdateHandler(func(u Update) { updates = append(updates, u) })

	done := make(chan error, 1)
	go func() {
		_, err := client.Prompt(context.Background(), "sess-1", []ContentBlock{TextBlock("hi")})
		done <- err
	}()
	id, _ := agent.expect("session/prompt")
	agent.sessionUpdate(map[string]any{"sessionUpdate": "plan", "entries": []any{}})
	agent.sessionUpdate(map[string]any{
		"sessionUpdate": "agent_message_chunk",
		"content":       map[string]any{"type": "image", "data": "AA==", "mimeType": "image/png"},
	})
	agent.sessionUpdate(textChunk(t, "agent_message_chunk", "text after the image"))
	agent.respond(id, map[string]any{"stopReason": "end_turn"})
	if err := <-done; err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if len(updates) != 3 {
		t.Fatalf("got %d updates, want 3: %+v", len(updates), updates)
	}
	if updates[0].Type != UpdatePlan || updates[0].IsKnown() != true {
		t.Errorf("update 0 = %+v, want a known plan update", updates[0])
	}
	if updates[1].Text != "" {
		t.Errorf("image chunk text = %q, want empty", updates[1].Text)
	}
	if updates[2].Text != "text after the image" {
		t.Errorf("update 2 = %+v, want the text chunk", updates[2])
	}
}

func TestSplitCommandLine(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"bub acp", []string{"bub", "acp"}},
		{"  bub   acp  ", []string{"bub", "acp"}},
		{`uv run bub acp`, []string{"uv", "run", "bub", "acp"}},
		{`"/Applications/My Agent/bin/agent" acp`, []string{"/Applications/My Agent/bin/agent", "acp"}},
		{`my-agent --flag="a b" --other c`, []string{"my-agent", "--flag=a b", "--other", "c"}},
		{`agent ''`, []string{"agent", ""}},
		{`agent "unterminated`, []string{"agent", "unterminated"}},
		{`agent back\ slash`, []string{"agent", "back slash"}},
	}
	for _, c := range cases {
		got := SplitCommandLine(c.in)
		if len(got) != len(c.want) {
			t.Errorf("SplitCommandLine(%q) = %q, want %q", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("SplitCommandLine(%q) = %q, want %q", c.in, got, c.want)
				break
			}
		}
	}
}

func TestStartMissingBinaryFails(t *testing.T) {
	_, err := Start(Config{Command: []string{"bub-agent-that-does-not-exist-xyz"}})
	if err == nil {
		t.Fatal("Start of a missing binary succeeded")
	}
	if !strings.Contains(err.Error(), "start") {
		t.Errorf("error = %v, want mention of starting the agent", err)
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv(EnvCommand, `uv run bub acp`)
	t.Setenv(EnvWorkspace, "/tmp/ws")
	cfg := ConfigFromEnv()
	if len(cfg.Command) != 4 || cfg.Command[0] != "uv" || cfg.Command[3] != "acp" {
		t.Errorf("Command = %q, want uv run bub acp", cfg.Command)
	}
	if cfg.Workspace != "/tmp/ws" {
		t.Errorf("Workspace = %q, want /tmp/ws", cfg.Workspace)
	}

	t.Setenv(EnvCommand, "")
	t.Setenv(EnvWorkspace, "")
	cfg = ConfigFromEnv()
	if len(cfg.Command) != 2 || cfg.Command[0] != "bub" || cfg.Command[1] != "acp" {
		t.Errorf("Command = %q, want the default bub acp", cfg.Command)
	}
}
