package acp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sdk "github.com/caelis-labs/acp-go-sdk"
	"github.com/caelis-labs/acp-go-sdk/transport/stdio"
)

// Environment variables the app reads to find the agent.
const (
	// EnvCommand is the agent command line, as a shell would split it into
	// an executable and its arguments. Default: "bub acp".
	EnvCommand = "BUB_ACP_CMD"
	// EnvWorkspace is the directory the agent runs in and the session is
	// rooted at. Default: the app's working directory.
	EnvWorkspace = "BUB_ACP_WORKSPACE"
	// EnvDotenv names a .env file whose values the agent inherits. The agent
	// runs in the workspace, so it cannot find a checkout's .env itself, and
	// without one it falls back to a credential-less default model.
	EnvDotenv = "BUB_ACP_DOTENV"
)

// DefaultCommand is what the app runs when BUB_ACP_CMD is unset.
const DefaultCommand = "bub acp"

// closeGrace is how long Close lets the agent exit on its own after its
// standard input closes, before the SDK kills its process tree.
const closeGrace = 2 * time.Second

// Config configures Start.
type Config struct {
	// Command is the agent's command line: the executable first, then its
	// arguments. Empty means DefaultCommand.
	Command []string
	// Workspace is the agent's working directory and the session's root;
	// "" inherits the app's.
	Workspace string
	// Stderr receives the agent's standard error, which the SDK drains and
	// the app also keeps for the error banner. Nil tees to os.Stderr.
	Stderr io.Writer
	// Env replaces the agent's environment when not nil.
	Env []string
}

// ConfigFromEnv reads BUB_ACP_CMD and BUB_ACP_WORKSPACE. An explicit command,
// as the app's settings give it, wins over the environment. Without an
// explicit command it resolves one for the workspace, so a checkout runs with
// no environment at all (see ResolveCommand).
func ConfigFromEnv(explicitCommand ...string) Config {
	workspace := os.Getenv(EnvWorkspace)
	if workspace == "" {
		if wd, err := os.Getwd(); err == nil {
			workspace = wd
		}
	}
	cfg := Config{Workspace: workspace}
	explicit := ""
	if len(explicitCommand) > 0 {
		explicit = strings.TrimSpace(explicitCommand[0])
	}
	if explicit == "" {
		explicit = strings.TrimSpace(os.Getenv(EnvCommand))
	}
	if explicit != "" {
		cfg.Command = SplitCommandLine(explicit)
		cfg.Env = DotenvEnv(FindDotenv(workspace))
		return cfg
	}
	cfg.Command = ResolveCommand(workspace)
	cfg.Env = DotenvEnv(FindDotenv(workspace))
	return cfg
}

// FindDotenv returns the .env to pass to the agent: BUB_ACP_DOTENV when set,
// else the first .env found in start or the directories above it. The agent
// runs in the workspace, so a checkout's .env is otherwise out of reach and
// its model credentials would be missing.
func FindDotenv(start string) string {
	if explicit := strings.TrimSpace(os.Getenv(EnvDotenv)); explicit != "" {
		return explicit
	}
	for _, dir := range []string{start, cwd()} {
		if dir == "" {
			continue
		}
		for {
			candidate := filepath.Join(dir, ".env")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return ""
}

// cwd reports the app's working directory, or "" when it is unavailable.
func cwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

// DotenvEnv returns the agent's environment with the values of a .env file
// layered over the app's own, or nil when path is empty or unreadable. The
// agent runs in the workspace, so a checkout's .env is not on its search path
// and its model credentials would otherwise be missing.
func DotenvEnv(path string) []string {
	if path == "" {
		return nil
	}
	values, err := ParseDotenv(path)
	if err != nil || len(values) == 0 {
		return nil
	}
	return append(os.Environ(), values...)
}

// ParseDotenv reads KEY=VALUE lines in "KEY=VALUE" form, accepting `export `
// prefixes, surrounding single or double quotes, and # comments. It returns
// them as the "KEY=VALUE" strings an environment expects.
func ParseDotenv(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || strings.ContainsAny(key, " \t") {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			} else if i := strings.Index(value, " #"); i >= 0 {
				value = strings.TrimSpace(value[:i])
			}
		}
		out = append(out, key+"="+value)
	}
	return out, nil
}

// ResolveCommand picks the agent command for a workspace when BUB_ACP_CMD is
// unset: an installed `bub acp` wins; else, in a directory whose parents hold
// pyproject.toml, `uv run bub acp` starts the checkout's own Bub; else the
// default `bub acp`, whose start error the app surfaces when an action needs
// the agent.
func ResolveCommand(workspace string) []string {
	if _, err := exec.LookPath("bub"); err == nil {
		return []string{"bub", "acp"}
	}
	if _, err := exec.LookPath("uv"); err == nil && hasPyproject(workspace) {
		return []string{"uv", "run", "bub", "acp"}
	}
	return SplitCommandLine(DefaultCommand)
}

// hasPyproject reports whether dir or one of its parents holds a
// pyproject.toml, as a Bub checkout does.
func hasPyproject(dir string) bool {
	if dir == "" {
		return false
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil && !info.IsDir() {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// SplitCommandLine splits a command line the way a POSIX shell splits words,
// for the simple case of BUB_ACP_CMD: spaces separate arguments, and single
// quotes, double quotes and backslashes group them.
func SplitCommandLine(s string) []string {
	var (
		args    []string
		word    strings.Builder
		started bool
		quote   rune
		escaped bool
	)
	flush := func() {
		if started {
			args = append(args, word.String())
			word.Reset()
			started = false
		}
	}
	for _, r := range s {
		switch {
		case escaped:
			word.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped = true
			started = true
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			word.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			started = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			word.WriteRune(r)
			started = true
		}
	}
	flush()
	return args
}

// Proc is a spawned agent process and the client that speaks to it: one
// process, one session, one workspace. The SDK owns the process tree and the
// protocol pipes; Proc exposes what the window needs of them.
type Proc struct {
	// Client is the ACP connection to the process, as the app's model.
	Client *Client

	process   *stdio.ClientProcess
	stderr    *tailBuffer
	exited    chan struct{}
	mu        sync.Mutex
	waitErr   error
	closeOnce sync.Once
	closeErr  error
}

// Start spawns the agent command and connects to it over its standard input
// and output. The SDK launches the executable directly (no shell), drains its
// standard error, and owns the process tree. Start returns once the process
// started; the handshake is the caller's.
func Start(cfg Config) (*Proc, error) {
	argv := cfg.Command
	if len(argv) == 0 {
		argv = SplitCommandLine(DefaultCommand)
	}
	if len(argv) == 0 || argv[0] == "" {
		return nil, errors.New("acp: no agent command: set " + EnvCommand)
	}
	stderr := newTailBuffer(16 << 10)
	dest := cfg.Stderr
	if dest == nil {
		dest = os.Stderr
	}
	client := newClient()
	process, err := stdio.StartClient(context.Background(), client, stdio.Command{
		Executable: argv[0],
		Args:       argv[1:],
		Dir:        cfg.Workspace,
		Env:        cfg.Env,
		Stderr:     io.MultiWriter(dest, stderr),
	}, sdk.ConnectionOptions{})
	if err != nil {
		return nil, fmt.Errorf("acp: start %q: %w", strings.Join(argv, " "), err)
	}
	client.attach(process.Connection)
	p := &Proc{
		Client:  client,
		process: process,
		stderr:  stderr,
		exited:  make(chan struct{}),
	}
	go func() {
		err := p.process.Process.Wait(context.Background())
		p.mu.Lock()
		p.waitErr = err
		p.mu.Unlock()
		close(p.exited)
	}()
	return p, nil
}

// Exited is closed once the agent process has exited.
func (p *Proc) Exited() <-chan struct{} { return p.exited }

// Wait blocks until the agent process exits and returns why it did.
func (p *Proc) Wait() error {
	<-p.exited
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.waitErr
}

// Stderr returns the last of what the agent wrote to its standard error, for
// the error banner when it fails.
func (p *Proc) Stderr() string { return p.stderr.String() }

// Close ends the connection and stops the agent: its standard input closes, it
// gets closeGrace to exit on its own, and the SDK then kills its process tree.
// Close waits for that, and returns the SDK's account of it.
func (p *Proc) Close() error {
	p.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), closeGrace)
		defer cancel()
		p.closeErr = p.process.Shutdown(ctx)
	})
	return p.closeErr
}

// tailBuffer keeps the last max bytes written to it, from any goroutine.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func newTailBuffer(max int) *tailBuffer {
	return &tailBuffer{max: max}
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		trimmed := make([]byte, b.max)
		copy(trimmed, b.buf[len(b.buf)-b.max:])
		b.buf = trimmed
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
