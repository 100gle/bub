package acp

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/caelis-labs/acp-go-sdk"
)

// fakeAgent is an ACP agent made with the SDK's agent side. It records what
// the adapter sends and runs a per-test script for prompts, so tests drive the
// client and the agent in one process over in-memory pipes.
type fakeAgent struct {
	mu       sync.Mutex
	conn     *sdk.AgentSideConnection
	protocol int
	info     *sdk.Implementation
	caps     sdk.ClientCapabilities
	cwd      string
	mcpCount int
	prompts  []sdk.PromptRequest
	cancels  []string
	script   func(ctx context.Context, f *fakeAgent, params sdk.PromptRequest) (sdk.PromptResponse, error)
}

func newFakeAgent() *fakeAgent { return &fakeAgent{} }

var _ sdk.Agent = (*fakeAgent)(nil)

func (f *fakeAgent) Initialize(_ context.Context, params sdk.InitializeRequest) (sdk.InitializeResponse, error) {
	f.mu.Lock()
	f.protocol = int(params.ProtocolVersion)
	f.info = params.ClientInfo
	f.caps = params.ClientCapabilities
	f.mu.Unlock()
	return sdk.InitializeResponse{
		ProtocolVersion: sdk.ProtocolVersion(ProtocolVersion),
		AgentInfo:       &sdk.Implementation{Name: "fake-bub", Version: "9.9.9"},
	}, nil
}

func (f *fakeAgent) NewSession(_ context.Context, params sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	f.mu.Lock()
	f.cwd = params.Cwd
	f.mcpCount = len(params.McpServers)
	f.mu.Unlock()
	return sdk.NewSessionResponse{SessionId: "sess-1"}, nil
}

func (f *fakeAgent) Cancel(_ context.Context, params sdk.CancelNotification) error {
	f.mu.Lock()
	f.cancels = append(f.cancels, string(params.SessionId))
	f.mu.Unlock()
	return nil
}

func (f *fakeAgent) Prompt(ctx context.Context, params sdk.PromptRequest) (sdk.PromptResponse, error) {
	f.mu.Lock()
	f.prompts = append(f.prompts, params)
	script := f.script
	f.mu.Unlock()
	if script != nil {
		return script(ctx, f, params)
	}
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
}

// snapshot copies what the agent heard, for assertions.
func (f *fakeAgent) snapshot() (info *sdk.Implementation, protocol int, caps sdk.ClientCapabilities) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.info, f.protocol, f.caps
}

func (f *fakeAgent) session() (cwd string, mcpCount int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cwd, f.mcpCount
}

func (f *fakeAgent) promptTexts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	texts := make([]string, 0, len(f.prompts))
	for _, p := range f.prompts {
		text := ""
		for _, block := range p.Prompt {
			if block.Text != nil {
				text += block.Text.Text
			}
		}
		texts = append(texts, text)
	}
	return texts
}

func (f *fakeAgent) cancelledSessions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.cancels...)
}

// loopback is the adapter connected to a fake agent in-process: two in-memory
// pipe pairs carrying the same newline-delimited JSON a subprocess would.
type loopback struct {
	client    *Client
	agent     *fakeAgent
	conn      *sdk.ClientSideConnection
	agentConn *sdk.AgentSideConnection
	closeOnce sync.Once
}

func newLoopback(t *testing.T) *loopback {
	t.Helper()
	agent := newFakeAgent()
	client := newClient()

	toAgentR, toAgentW := io.Pipe()
	toClientR, toClientW := io.Pipe()
	conn := sdk.NewClientSideConnection(client, toAgentW, toClientR)
	agentConn := sdk.NewAgentSideConnection(agent, toClientW, toAgentR)

	agent.mu.Lock()
	agent.conn = agentConn
	agent.mu.Unlock()
	client.attach(conn)

	lb := &loopback{client: client, agent: agent, conn: conn, agentConn: agentConn}
	t.Cleanup(lb.close)
	return lb
}

func (l *loopback) close() {
	l.closeOnce.Do(func() {
		_ = l.conn.Close()
		_ = l.agentConn.Close()
	})
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestInitializeAndNewSession(t *testing.T) {
	lb := newLoopback(t)
	ctx := context.Background()

	info, err := lb.client.Initialize(ctx, Implementation{Name: "bub-desktop", Title: "Bub Desktop", Version: "0.1.0"})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if info.ProtocolVersion != ProtocolVersion {
		t.Errorf("protocol version = %d, want %d", info.ProtocolVersion, ProtocolVersion)
	}
	if info.AgentInfo == nil || info.AgentInfo.Name != "fake-bub" || info.AgentInfo.Version != "9.9.9" {
		t.Errorf("agentInfo = %+v, want fake-bub 9.9.9", info.AgentInfo)
	}

	sent, protocol, caps := lb.agent.snapshot()
	if protocol != ProtocolVersion {
		t.Errorf("agent saw protocolVersion %d, want %d", protocol, ProtocolVersion)
	}
	if sent == nil || sent.Name != "bub-desktop" || sent.Version != "0.1.0" {
		t.Errorf("clientInfo = %+v, want bub-desktop 0.1.0", sent)
	}
	if sent != nil && (sent.Title == nil || *sent.Title != "Bub Desktop") {
		t.Errorf("clientInfo title = %+v, want Bub Desktop", sent.Title)
	}
	// No fs or terminal capabilities are advertised: nothing is delegated.
	if caps.Terminal || caps.Fs.ReadTextFile || caps.Fs.WriteTextFile {
		t.Errorf("clientCapabilities = %+v, want no fs and no terminal", caps)
	}

	session, err := lb.client.NewSession(ctx, "/tmp/workspace")
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	if session.SessionID != "sess-1" {
		t.Errorf("session id = %q, want sess-1", session.SessionID)
	}
	if cwd, mcpCount := lb.agent.session(); cwd != "/tmp/workspace" || mcpCount != 0 {
		t.Errorf("session/new saw cwd %q and %d mcp servers, want /tmp/workspace and none", cwd, mcpCount)
	}
}

func TestSessionUpdateMapping(t *testing.T) {
	lb := newLoopback(t)
	var mu sync.Mutex
	var updates []Update
	lb.client.SetUpdateHandler(func(u Update) {
		mu.Lock()
		updates = append(updates, u)
		mu.Unlock()
	})
	lb.agent.script = func(ctx context.Context, f *fakeAgent, params sdk.PromptRequest) (sdk.PromptResponse, error) {
		send := func(update sdk.SessionUpdate) error {
			return f.conn.SessionUpdate(ctx, sdk.SessionNotification{SessionId: params.SessionId, Update: update})
		}
		steps := []sdk.SessionUpdate{
			sdk.UpdateAgentMessageText("Hel"),
			sdk.UpdateAgentMessageText("lo"),
			sdk.UpdateAgentThoughtText("thinking"),
			sdk.StartToolCall("call-1", "read README.md",
				sdk.WithStartKind(sdk.ToolKindRead),
				sdk.WithStartName("read"),
				sdk.WithStartStatus(sdk.ToolCallStatusInProgress),
				sdk.WithStartRawInput(map[string]any{"path": "README.md"}),
			),
			sdk.UpdateToolCall("call-1",
				sdk.WithUpdateStatus(sdk.ToolCallStatusCompleted),
				sdk.WithUpdateRawOutput("the readme"),
			),
		}
		for _, step := range steps {
			if err := send(step); err != nil {
				return sdk.PromptResponse{}, err
			}
		}
		return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
	}

	stop, err := lb.client.Prompt(context.Background(), "sess-1", []ContentBlock{TextBlock("hello")})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if stop != StopEndTurn {
		t.Errorf("stop reason = %q, want %q", stop, StopEndTurn)
	}
	if texts := lb.agent.promptTexts(); len(texts) != 1 || texts[0] != "hello" {
		t.Errorf("agent saw prompts %q, want [hello]", texts)
	}

	waitFor(t, "five mapped updates", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(updates) == 5
	})
	mu.Lock()
	got := append([]Update(nil), updates...)
	mu.Unlock()

	wantTypes := []string{
		UpdateAgentMessageChunk,
		UpdateAgentMessageChunk,
		UpdateAgentThoughtChunk,
		UpdateToolCall,
		UpdateToolCallUpdate,
	}
	for i, want := range wantTypes {
		if got[i].Type != want {
			t.Errorf("update %d type = %q, want %q", i, got[i].Type, want)
		}
		if got[i].SessionID != "sess-1" {
			t.Errorf("update %d session = %q, want sess-1", i, got[i].SessionID)
		}
	}
	if got[0].Text != "Hel" || got[1].Text != "lo" {
		t.Errorf("message chunks = %q, %q, want Hel and lo", got[0].Text, got[1].Text)
	}
	if got[2].Text != "thinking" {
		t.Errorf("thought chunk = %q, want thinking", got[2].Text)
	}

	call := got[3].ToolCall
	if call == nil {
		t.Fatalf("update 3 carries no tool call: %+v", got[3])
	}
	if call.ToolCallID != "call-1" || call.Title != "read README.md" || call.Name != "read" {
		t.Errorf("tool call = %+v, want call-1 read README.md (name read)", call)
	}
	if call.Kind != string(sdk.ToolKindRead) || call.Status != ToolStatusInProgress {
		t.Errorf("tool call kind/status = %q/%q, want read/in_progress", call.Kind, call.Status)
	}
	if string(call.RawInput) != `{"path":"README.md"}` {
		t.Errorf("tool call rawInput = %s, want the arguments JSON", call.RawInput)
	}

	update := got[4].ToolCallUpdate
	if update == nil {
		t.Fatalf("update 4 carries no tool call update: %+v", got[4])
	}
	if update.ToolCallID != "call-1" || update.Status == nil || *update.Status != ToolStatusCompleted {
		t.Errorf("tool call update = %+v, want call-1 completed", update)
	}
	if string(update.RawOutput) != `"the readme"` {
		t.Errorf("tool call update rawOutput = %s, want the output JSON", update.RawOutput)
	}
	if update.Title != nil || update.Name != nil {
		t.Errorf("tool call update title/name = %v/%v, want nil (unchanged)", update.Title, update.Name)
	}
}

func TestCancelSendsSessionCancel(t *testing.T) {
	lb := newLoopback(t)
	started := make(chan struct{})
	// The SDK's agent side cancels the prompt's context when the client sends
	// session/cancel, then calls Agent.Cancel; an agent that honors both
	// answers with the cancelled stop reason.
	lb.agent.script = func(ctx context.Context, f *fakeAgent, params sdk.PromptRequest) (sdk.PromptResponse, error) {
		close(started)
		<-ctx.Done()
		return sdk.PromptResponse{StopReason: sdk.StopReasonCancelled}, nil
	}

	type promptResult struct {
		stop string
		err  error
	}
	done := make(chan promptResult, 1)
	go func() {
		stop, err := lb.client.Prompt(context.Background(), "sess-1", []ContentBlock{TextBlock("go")})
		done <- promptResult{stop: stop, err: err}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the agent never started the turn")
	}
	if err := lb.client.Cancel("sess-1"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("prompt after cancel: %v", result.err)
		}
		if result.stop != StopCancelled {
			t.Errorf("stop reason = %q, want %q", result.stop, StopCancelled)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the prompt did not return after cancel")
	}
	waitFor(t, "the agent's Cancel callback", func() bool {
		sessions := lb.agent.cancelledSessions()
		return len(sessions) == 1 && sessions[0] == "sess-1"
	})
}

func TestErrorResponse(t *testing.T) {
	lb := newLoopback(t)
	lb.agent.script = func(context.Context, *fakeAgent, sdk.PromptRequest) (sdk.PromptResponse, error) {
		return sdk.PromptResponse{}, sdk.NewInvalidParams(map[string]any{"reason": "no workspace"})
	}
	_, err := lb.client.Prompt(context.Background(), "sess-1", []ContentBlock{TextBlock("hi")})
	if err == nil {
		t.Fatal("a failing prompt returned no error")
	}
	var requestErr *sdk.RequestError
	if !errors.As(err, &requestErr) {
		t.Fatalf("error = %v, want an *acp.RequestError", err)
	}
	if requestErr.Code != -32602 {
		t.Errorf("error code = %d, want -32602", requestErr.Code)
	}
}

// TestUnsupportedClientRequestIsAnswered checks that the SDK answers a request
// for a capability the adapter does not implement with "method not found", so
// an agent that asks for one does not hang.
func TestUnsupportedClientRequestIsAnswered(t *testing.T) {
	lb := newLoopback(t)
	var code atomic.Int32
	lb.agent.script = func(ctx context.Context, f *fakeAgent, params sdk.PromptRequest) (sdk.PromptResponse, error) {
		_, err := f.conn.ReadTextFile(ctx, sdk.ReadTextFileRequest{
			SessionId: params.SessionId,
			Path:      "/tmp/whatever",
		})
		var requestErr *sdk.RequestError
		if errors.As(err, &requestErr) {
			code.Store(int32(requestErr.Code))
		}
		return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
	}
	stop, err := lb.client.Prompt(context.Background(), "sess-1", []ContentBlock{TextBlock("hi")})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if stop != StopEndTurn {
		t.Errorf("stop reason = %q, want end_turn", stop)
	}
	if got := code.Load(); got != -32601 {
		t.Errorf("fs/read_text_file answered with code %d, want -32601", got)
	}
}

func TestCloseReportsErrClosed(t *testing.T) {
	lb := newLoopback(t)
	closed := make(chan error, 1)
	lb.client.SetCloseHandler(func(err error) { closed <- err })

	lb.close()
	select {
	case err := <-closed:
		if !errors.Is(err, ErrClosed) {
			t.Errorf("close handler got %v, want ErrClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the close handler never ran")
	}

	// A handler installed after the end hears at once.
	late := make(chan error, 1)
	lb.client.SetCloseHandler(func(err error) { late <- err })
	select {
	case err := <-late:
		if !errors.Is(err, ErrClosed) {
			t.Errorf("late close handler got %v, want ErrClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the late close handler never ran")
	}

	// Calls after the connection ended report ErrClosed, not an opaque error.
	if _, err := lb.client.Initialize(context.Background(), Implementation{Name: "x", Version: "0"}); !errors.Is(err, ErrClosed) {
		t.Errorf("initialize after close = %v, want ErrClosed", err)
	}
}
