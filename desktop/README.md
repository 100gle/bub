# Bub Desktop

A native desktop chat client for [Bub](../../README.md), written in Go with the
[MyGo](https://mygo.egoist.dev) UI toolkit, and connected to a Bub agent through
the [Agent Client Protocol](https://agentclientprotocol.com) (ACP) v1 over
stdio.

The window draws itself with MyGo's native UI: no webview, no browser runtime,
no CGO. The agent is a separate `bub acp` process. The two halves share no code,
only JSON-RPC 2.0 messages, one per line, over the process's stdin and stdout.
Anything that speaks ACP could replace either end.

The wire protocol — JSON-RPC 2.0, typed ACP v1 messages, stdio framing, request
correlation and subprocess lifecycle — is handled by
[`github.com/caelis-labs/acp-go-sdk`](https://github.com/caelis-labs/acp-go-sdk)
v1.6.0 (Apache-2.0). `internal/acp` is a thin adapter over it: it spawns the
agent, maps typed session updates to the model the view renders, and declines
capabilities the window does not implement.

See [DESIGN.md](DESIGN.md) for the contract between the two halves. The planned
redesign into a block-based, chat-free writing surface is in
[EDITOR_PLAN.md](EDITOR_PLAN.md).

```
┌──────────────────────────────┐    ACP v1, JSON-RPC 2.0, one per line   ┌───────────────────────────┐
│  Bub Desktop (Go, MyGo)      │  ── initialize, session/new ─────────► │  bub acp (Python)         │
│  header, transcript,         │  ── session/prompt ───────────────────► │  BubFramework turns       │
│  composer, error banner      │  ◄─ session/update notifications ────── │  model, tools, tape       │
│                              │  ◄─ session/prompt response ─────────── │                           │
│  internal/acp client         │  ── session/cancel ───────────────────► │  asyncio task cancellation│
└──────────────────────────────┘                                        └───────────────────────────┘
        client                                                                agent
```

![Bub Desktop answering a prompt with a tool call and a Markdown reply](screenshot.png)

## What is in the box

| Path | What it is |
| --- | --- |
| `main.go` | The app's state, the view, and the agent wiring. One Go value holds the transcript; `view` draws it. |
| `internal/acp/acp.go` | The app-facing ACP vocabulary: `Update`, tool call/status models, stop reasons, prompt content blocks. |
| `internal/acp/client.go` | The adapter: the SDK's `acp.Client` (permission requests declined, typed session updates mapped to `Update`) plus the app's `Initialize`/`NewSession`/`Prompt`/`Cancel` calls and a close watcher. |
| `internal/acp/spawn.go` | `Config`/`ConfigFromEnv`, `Start` (spawns the agent through the SDK's `stdio.StartClient`, streaming stderr into a tail for the error banner) and `Proc.Close` (SDK `Shutdown` with a 2s grace). |
| `internal/acp/testdata/agent/main.go` | A fake ACP agent built with the SDK's agent side; the spawn test builds and runs it as a real process. |
| `internal/acp/loopback_test.go` | SDK-level tests: a fake agent over in-memory pipes — handshake, streaming, tool calls, cancel, error responses, `-32601` for unimplemented capabilities, close. |
| `main_test.go` | `ui.NewTester` view tests: typing and Enter/Send, streaming and finished Markdown, tool rows, Stop, the error banner, the empty state. |
| `e2e.go` | `-tags e2e` only: registers MyGo's automation plugin so the running window can be driven from outside. |
| `mygo.json` | App metadata for `mygo build` (name, bundle identifier, version, output directory). |
| `resources/icon.png` | The Bub mark, copied from `website/public/examples/bub-todo-agent/static/bub-logo.png`. It is the window's mark and, by MyGo's convention, the app icon. |

## Running it

The agent command comes from `BUB_ACP_CMD` (default `bub acp`), split like a
shell splits words. Its working directory — and the ACP session's `cwd` — is
`BUB_ACP_WORKSPACE` when set, else the app's own working directory.

From a checkout with the Python environment installed (`uv sync`):

```sh
# terminal 1: from the repo root, so the agent sees the whole project
export BUB_ACP_WORKSPACE="$PWD"
export BUB_ACP_CMD="uv run bub acp"
cd desktop && go run .
```

`BUB_ACP_WORKSPACE` is what pins the agent to the repo root even though
`go run` starts from `desktop/`; without it the agent's workspace is `desktop/`.

With a `bub` already on `PATH`:

```sh
cd desktop
BUB_ACP_CMD="bub acp" go run .
```

Packaged builds use MyGo's own tools, which read `mygo.json`:

```sh
cd desktop
go run github.com/egoist/mygo/cmd/mygo@v0.8.3 build   # bundle in build/
go run github.com/egoist/mygo/cmd/mygo@v0.8.3 dev     # rebuild and relaunch on change
```

Go is not necessarily on `PATH` in a non-interactive shell; on a Homebrew setup
`export PATH=/usr/local/opt/go/bin:$PATH` first.

### Environment

| Variable | Meaning | Default |
| --- | --- | --- |
| `BUB_ACP_CMD` | The agent command line. Quotes group arguments. | `bub acp` |
| `BUB_ACP_WORKSPACE` | The agent's working directory and the session's workspace. | the app's working directory |
| `BUB_MODEL` | Shown in the composer footer until the agent reports its own name; the agent reads it too. | unset |
| `MYGO_AUTOMATION` | Socket path for the automation plugin, in `-tags e2e` builds only. | unset |

## How it works

**Connection.** On start the app spawns the agent, then runs `initialize`
(`protocolVersion: 1`; `clientCapabilities` with no filesystem and no terminal,
since Bub's tools act on the workspace themselves) and `session/new`
(`cwd` absolute, `mcpServers: []`). The header chip says `connecting`, then
`ready`. If either step fails — `bub` missing, `uv` failing, a handshake timeout
of 20 s — the window stays usable, shows the error with the tail of the agent's
stderr, and offers **Retry**, which respawns it.

**Turns.** The composer sends the text as one `text` content block with
`session/prompt`. While the turn runs the header says `running`, Send becomes
Stop (sending `session/cancel`), and an activity pill at the end of the
transcript shows the tool that is running. The response's `stopReason` ends the
turn (`end_turn`, `cancelled`, `max_tokens`, …) and is reported when it is not a
plain end.

**Streaming.** `session/update` notifications are folded into the transcript:

| ACP update | In the window |
| --- | --- |
| `agent_message_chunk` | Appended to the assistant message: `markdown.Stream` while it comes, `markdown.View` once the turn or the segment ends. |
| `agent_thought_chunk` | A muted reasoning line above the reply while it streams. |
| `tool_call` | A compact row: status glyph, title, status and a preview of its arguments. |
| `tool_call_update` | The row is updated in place: status, title, output preview. A row for a call the window never saw is added, not dropped. |
| `plan`, `available_commands_update`, …, unknown updates | Ignored. |

A tool call still running is not drawn as a row: the activity pill stands for
it, so a running tool is not shown twice. Once it ends its row appears. The SDK
skips a malformed line from the agent rather than failing the connection. A
request the client does not implement (it implements permission requests as
"declined", and no `fs/*` or `terminal/*` capability) is answered by the SDK
with JSON-RPC error `-32601`, so the agent never waits forever.

**Threading.** Everything that mutates the transcript runs on the main thread
through `win.Update`: the SDK's notification goroutine only queues updates. The
agent never blocks a frame, and the view never blocks on the agent. Tests build
the same view with `ui.NewTester`, where there is no window and state changes
apply directly.

**Errors.** A failed handshake, a dead agent process, a failed turn or a cancel
that could not be sent set a red banner between the header and the transcript.
The banner carries **Retry**; the transcript and composer stay usable, and a
message typed while disconnected is kept and the banner explains why nothing
was sent.

## Keys for automation

For `-tags e2e` builds driven through `MYGO_AUTOMATION`, these elements have
stable keys:

| Key | Element |
| --- | --- |
| `composer` | The message input, keyed with `ui.TextAreaBase(c.Key("composer"), …)`. |
| `send` | The Send button. |
| `stop` | The Stop button, drawn while a turn runs. |
| `error` | The error banner, when it shows. |
| `status` | The connection chip's text: `connecting`, `ready`, `running` or `offline`. |

```sh
cd desktop
go build -tags e2e -o build/bub-desktop .
MYGO_AUTOMATION=/tmp/bub-desktop.sock ./build/bub-desktop &
mygo-automation --socket /tmp/bub-desktop.sock windows
mygo-automation --socket /tmp/bub-desktop.sock capture --window 1 > shot.png
```

## Tests

```sh
cd desktop
go build ./...
go test ./...
go vet ./...
```

`go build -tags e2e ./...` covers the automation build. The client tests need no
process and no network: a fake agent answers on a loopback connection, and each
test drives both sides. The view tests open no window and start no agent.

## Limitations

- **One session, one workspace.** The app opens a session when it starts and
  keeps it until the agent dies or Retry respawns it. There is no session list,
  no resume and no per-window workspace.
- **No permissions UI.** `session/request_permission` is answered with the
  "cancelled" outcome, so a tool that needs consent does not run. Bub's tool
  denials via hooks still work in the agent; the window cannot ask, and no
  `fs/*` or `terminal/*` request is served (the SDK answers `-32601`).
- **No fs or terminal delegation.** The client advertises neither; the agent is
  expected to act on the workspace itself, as `bub acp` does.
- **Light theme only.** The zinc palette is fixed; the window does not follow
  the desktop's dark appearance yet.
- **Text prompts only.** No image, audio, embedded resource or resource-link
  prompts, no file attachments, no slash-command autocomplete.
- **No message history.** The transcript lives in memory for the process's
  lifetime; the agent's tape keeps the real conversation.
- **Sending is not queued.** A message typed before the handshake finishes is
  kept in the transcript with an explanation but not sent; send it again once
  the chip says `ready`.
- **Tool previews are raw.** Arguments and output are shown as one truncated
  JSON line, not pretty-printed or diff-rendered.
- **`BUB_ACP_CMD` is split like a shell but not by one.** Pipes, `&&` and
  environment assignments are not interpreted; use quotes for arguments with
  spaces.
