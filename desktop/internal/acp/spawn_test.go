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

// fakeTool puts an executable named name in dir, for LookPath to find.
func fakeTool(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// quietPath points PATH at an empty directory, so resolution finds no tools,
// whatever the machine has installed.
func quietPath(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

func TestConfigFromEnvExplicitWins(t *testing.T) {
	t.Setenv(EnvCommand, `uv run bub acp`)
	t.Setenv(EnvWorkspace, "/tmp/ws")
	cfg := ConfigFromEnv()
	if len(cfg.Command) != 4 || cfg.Command[0] != "uv" || cfg.Command[3] != "acp" {
		t.Errorf("Command = %q, want uv run bub acp", cfg.Command)
	}
	if cfg.Workspace != "/tmp/ws" {
		t.Errorf("Workspace = %q, want /tmp/ws", cfg.Workspace)
	}
}

func TestResolveCommandPrefersInstalledBub(t *testing.T) {
	bin := t.TempDir()
	fakeTool(t, bin, "bub")
	t.Setenv("PATH", bin)
	workspace := t.TempDir() // no pyproject.toml anywhere below
	got := ResolveCommand(workspace)
	if len(got) != 2 || got[0] != "bub" || got[1] != "acp" {
		t.Errorf("ResolveCommand = %q, want bub acp", got)
	}
}

func TestResolveCommandFallsBackToUvCheckout(t *testing.T) {
	bin := t.TempDir()
	fakeTool(t, bin, "uv")
	t.Setenv("PATH", bin)
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "pyproject.toml"), []byte("[project]\nname = \"bub\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ResolveCommand(workspace)
	if len(got) != 4 || got[0] != "uv" || got[1] != "run" || got[2] != "bub" || got[3] != "acp" {
		t.Errorf("ResolveCommand = %q, want uv run bub acp", got)
	}
	// A parent holding pyproject.toml is a checkout too.
	nested := filepath.Join(workspace, "sub", "dir")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ResolveCommand(nested); len(got) != 4 || got[0] != "uv" {
		t.Errorf("ResolveCommand(nested) = %q, want uv run bub acp", got)
	}
}

func TestResolveCommandUvOutsideACheckout(t *testing.T) {
	bin := t.TempDir()
	fakeTool(t, bin, "uv")
	t.Setenv("PATH", bin)
	workspace := t.TempDir() // no pyproject.toml
	got := ResolveCommand(workspace)
	if len(got) != 2 || got[0] != "bub" || got[1] != "acp" {
		t.Errorf("ResolveCommand = %q, want the default bub acp", got)
	}
}

func TestConfigFromEnvResolvesWithoutExplicitCommand(t *testing.T) {
	bin := t.TempDir()
	fakeTool(t, bin, "bub")
	t.Setenv("PATH", bin)
	workspace := t.TempDir()
	t.Setenv(EnvCommand, "")
	t.Setenv(EnvWorkspace, workspace)
	cfg := ConfigFromEnv()
	if len(cfg.Command) != 2 || cfg.Command[0] != "bub" {
		t.Errorf("Command = %q, want the resolved bub acp", cfg.Command)
	}
	if cfg.Workspace != workspace {
		t.Errorf("Workspace = %q, want %q", cfg.Workspace, workspace)
	}
}

func TestConfigFromEnvDefaultWithoutTools(t *testing.T) {
	quietPath(t)
	t.Setenv(EnvCommand, "")
	t.Setenv(EnvWorkspace, t.TempDir())
	cfg := ConfigFromEnv()
	if len(cfg.Command) != 2 || cfg.Command[0] != "bub" || cfg.Command[1] != "acp" {
		t.Errorf("Command = %q, want the default bub acp", cfg.Command)
	}
}

func TestParseDotenvReadsCommonForms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	body := "# a comment\n" +
		"BUB_MODEL=openai:gpt-4o\n" +
		"export BUB_API_KEY=\"sk-secret\"\n" +
		"SINGLE='a b c'\n" +
		"TRAILING=value # trailing comment\n" +
		"\n" +
		"not a pair\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ParseDotenv(path)
	if err != nil {
		t.Fatalf("ParseDotenv: %v", err)
	}
	want := []string{
		"BUB_MODEL=openai:gpt-4o",
		"BUB_API_KEY=sk-secret",
		"SINGLE=a b c",
		"TRAILING=value",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDotenvEnvIsNilWithoutPath(t *testing.T) {
	if got := DotenvEnv(""); got != nil {
		t.Errorf("DotenvEnv(\"\") = %v, want nil", got)
	}
	if got := DotenvEnv(filepath.Join(t.TempDir(), "missing.env")); got != nil {
		t.Errorf("DotenvEnv(missing) = %v, want nil", got)
	}
}

func TestConfigFromEnvLoadsDotenv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("BUB_MODEL=openai:gpt-4o\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvDotenv, path)
	cfg := ConfigFromEnv("bub acp")
	found := false
	for _, entry := range cfg.Env {
		if entry == "BUB_MODEL=openai:gpt-4o" {
			found = true
		}
	}
	if !found {
		t.Errorf("agent env does not carry the .env values: %v", cfg.Env)
	}
}

func TestFindDotenvWalksUpFromWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("BUB_MODEL=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvDotenv, "")
	if got := FindDotenv(deep); got != filepath.Join(root, ".env") {
		t.Errorf("FindDotenv = %q, want %q", got, filepath.Join(root, ".env"))
	}
	explicit := filepath.Join(root, "custom.env")
	t.Setenv(EnvDotenv, explicit)
	if got := FindDotenv(deep); got != explicit {
		t.Errorf("explicit FindDotenv = %q, want %q", got, explicit)
	}
}

func TestFindDotenvEmptyWhenAbsent(t *testing.T) {
	t.Setenv(EnvDotenv, "")
	t.Chdir(t.TempDir())
	if got := FindDotenv(""); got != "" && !filepath.IsAbs(got) {
		t.Errorf("FindDotenv = %q, want \"\"", got)
	}
}
