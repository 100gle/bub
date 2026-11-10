package acp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Environment variables the app reads to find the agent.
const (
	// EnvCommand is the agent command line, as a shell would split it into
	// an executable and its arguments. Default: "bub acp".
	EnvCommand = "BUB_ACP_CMD"
	// EnvWorkspace is the directory the agent runs in and the session is
	// rooted at. Default: the app's working directory.
	EnvWorkspace = "BUB_ACP_WORKSPACE"
)

// DefaultCommand is what the app runs when BUB_ACP_CMD is unset.
const DefaultCommand = "bub acp"

// Config configures Start.
type Config struct {
	// Command is the agent's command line: the executable first, then its
	// arguments. Empty means DefaultCommand.
	Command []string
	// Workspace is the agent's working directory and the session's root;
	// "" inherits the app's.
	Workspace string
	// Stderr receives the agent's standard error, which is also kept for
	// the error banner. Nil tees to os.Stderr.
	Stderr io.Writer
	// Env replaces the agent's environment when not nil.
	Env []string
}

// ConfigFromEnv reads BUB_ACP_CMD and BUB_ACP_WORKSPACE.
func ConfigFromEnv() Config {
	command := os.Getenv(EnvCommand)
	if strings.TrimSpace(command) == "" {
		command = DefaultCommand
	}
	return Config{
		Command:   SplitCommandLine(command),
		Workspace: os.Getenv(EnvWorkspace),
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
// process, one session, one workspace.
type Proc struct {
	// Client is the ACP connection to the process.
	Client *Client

	cmd       *exec.Cmd
	stderr    *tailBuffer
	exited    chan struct{}
	mu        sync.Mutex
	waitErr   error
	closeOnce sync.Once
}

// Start spawns the agent command with pipes for ACP on its standard input and
// output, and the process's standard error forwarded to cfg.Stderr (and kept
// for the error banner). It returns once the process started; the handshake is
// the caller's.
func Start(cfg Config) (*Proc, error) {
	argv := cfg.Command
	if len(argv) == 0 {
		argv = SplitCommandLine(DefaultCommand)
	}
	if len(argv) == 0 || argv[0] == "" {
		return nil, errors.New("acp: no agent command: set " + EnvCommand)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = cfg.Workspace
	if cfg.Env != nil {
		cmd.Env = cfg.Env
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: pipe to %s: %w", argv[0], err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("acp: pipe from %s: %w", argv[0], err)
	}
	stderr := newTailBuffer(16 << 10)
	dest := cfg.Stderr
	if dest == nil {
		dest = os.Stderr
	}
	cmd.Stderr = io.MultiWriter(dest, stderr)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("acp: start %q: %w", strings.Join(argv, " "), err)
	}
	p := &Proc{
		Client: NewClient(&pipeConn{Reader: stdout, WriteCloser: stdin}),
		cmd:    cmd,
		stderr: stderr,
		exited: make(chan struct{}),
	}
	go func() {
		err := cmd.Wait()
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

// Close ends the connection: the agent's standard input closes, so it exits
// as it does when a terminal asks it to, and a process that has not exited a
// couple of seconds later is killed. It returns without waiting.
func (p *Proc) Close() error {
	var err error
	p.closeOnce.Do(func() {
		err = p.Client.Close()
		go func() {
			select {
			case <-p.exited:
			case <-time.After(closeGrace):
				if p.cmd.Process != nil {
					_ = p.cmd.Process.Kill()
				}
			}
		}()
	})
	return err
}

// closeGrace is how long Close lets an agent exit on its own before killing it.
const closeGrace = 2 * time.Second

// pipeConn is the ACP connection over a process's pipes: reading its standard
// output, writing its standard input.
type pipeConn struct {
	io.Reader
	io.WriteCloser
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
