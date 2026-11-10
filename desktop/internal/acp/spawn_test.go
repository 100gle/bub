package acp

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// buildTestdataAgent builds internal/acp/testdata/agent, a real ACP agent
// process made with the SDK's agent side, so Start can be tested against a
// process rather than an in-memory connection.
func buildTestdataAgent(t *testing.T) string {
	t.Helper()
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(goBin); err != nil {
		goBin, err = exec.LookPath("go")
		if err != nil {
			t.Skipf("no go command to build the testdata agent: %v", err)
		}
	}
	binary := filepath.Join(t.TempDir(), "testdata-agent")
	build := exec.CommandContext(context.Background(), goBin, "build", "-o", binary, "./testdata/agent")
	build.Dir = "."
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("building the testdata agent: %v: %s", err, output)
	}
	return binary
}

func TestStartTalksToAProcess(t *testing.T) {
	binary := buildTestdataAgent(t)
	proc, err := Start(Config{
		Command: []string{binary},
		Stderr:  io.Discard, // keep the test output clean; the tail is kept anyway
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = proc.Close() })

	var mu sync.Mutex
	var updates []Update
	proc.Client.SetUpdateHandler(func(u Update) {
		mu.Lock()
		updates = append(updates, u)
		mu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, err := proc.Client.Initialize(ctx, Implementation{Name: "bub-desktop", Version: "0.1.0"})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if info.AgentInfo == nil || info.AgentInfo.Name != "testdata-agent" {
		t.Errorf("agentInfo = %+v, want testdata-agent", info.AgentInfo)
	}
	session, err := proc.Client.NewSession(ctx, "/tmp")
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	if session.SessionID != "test-session" {
		t.Errorf("session id = %q, want test-session", session.SessionID)
	}
	stop, err := proc.Client.Prompt(ctx, session.SessionID, []ContentBlock{TextBlock("hi")})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if stop != StopEndTurn {
		t.Errorf("stop reason = %q, want end_turn", stop)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		count := len(updates)
		mu.Unlock()
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("got %d updates, want one message chunk", count)
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	first := updates[0]
	mu.Unlock()
	if first.Type != UpdateAgentMessageChunk || !strings.Contains(first.Text, "testdata agent") {
		t.Errorf("update = %+v, want the agent's message chunk", first)
	}
	if !strings.Contains(proc.Stderr(), "testdata agent ready") {
		t.Errorf("stderr = %q, want the agent's line captured", proc.Stderr())
	}

	// Closing ends the process, as quitting the app does.
	if err := proc.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
	select {
	case <-proc.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("the agent process did not exit after Close")
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
