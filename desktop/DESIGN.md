# Bub Desktop — design notes (experiment)

> The planned next step — turning this chat client into an AI-native writing
> surface (rich text, block-level actions, a standby bar, voice input) — is in
> [EDITOR_PLAN.md](EDITOR_PLAN.md), with mocks in `design/`.

Bub Desktop is a small experiment: a native desktop chat client for Bub, written in
Go with the [MyGo](https://mygo.egoist.dev) UI toolkit and connected to a Bub agent
through the [Agent Client Protocol](https://agentclientprotocol.com) (ACP v1) over
stdio.

The point is to keep the two halves completely decoupled:

```
┌──────────────────────────────┐        ACP v1, JSON-RPC 2.0 over stdin/stdout       ┌───────────────────────────┐
│  desktop/ (Go, MyGo native)  │  ── session/new, session/prompt ────────────────►  │  bub acp (Python)         │
│  Bub Desktop window          │  ◄── session/update notifications ───────────────  │  BubFramework turns       │
│  chat, tool calls, markdown  │  ◄── session/request_permission (future) ───────   │  model, tools, tape       │
└──────────────────────────────┘                                                     └───────────────────────────┘
        ACP client                                                                       ACP agent
```

Anything that speaks ACP could sit on either side later (Zed as the client, another
agent as the backend); the experiment only ships the two ends above.

## Components

| Piece | Path | Owner |
| --- | --- | --- |
| ACP agent (`bub acp`) | `src/bub/builtin/acp.py`, `tests/test_acp.py` | `acp-server` |
| MyGo desktop client | `desktop/` (Go module) | `desktop-ui` |
| Design + integration/E2E | this file, branch `feat/bub-desktop-mygo` | lead |

## ACP agent: `bub acp`

A new Typer subcommand that speaks ACP over the process's stdin/stdout.

- One JSON-RPC 2.0 object per line, UTF-8. Nothing else may be written to stdout;
  logs go to stderr (loguru already does).
- `protocolVersion` is `1`.
- Methods implemented:
  - `initialize` → `{protocolVersion: 1, agentCapabilities: {...}, agentInfo: {name, version}}`
  - `session/new` → `{sessionId}`; `cwd` must be absolute. The session id maps to a
    Bub session id `acp:<sessionId>` so tapes are isolated per ACP session.
  - `session/prompt` → runs one Bub turn, streams updates, then responds
    `{stopReason: "end_turn"}` (or `"cancelled"`).
  - `session/cancel` → cancels the active turn of that session.
  - `session/load` and the other session-management methods are not supported and
    answer with JSON-RPC `-32601` (Method not found); `agentCapabilities.loadSession`
    is `false`.
- Capabilities are deliberately conservative: no image/audio/embeddedContext prompt
  support, no MCP, no terminals, no fs delegation. Bub tools act on the workspace
  itself; the client is not asked to read or write files.
- One process = one workspace. The first `session/new` `cwd` sets
  `BubFramework.workspace`; `--workspace` on the CLI still wins when given.

### Streaming mapping

The turn runs through the normal `BubFramework.process_inbound(..., stream_output=True)`
path with a small `ChannelRouter` bound to the ACP connection:

| Bub stream event | ACP `session/update` |
| --- | --- |
| `text` delta | `agent_message_chunk` (`content: {type: "text", text}`) |
| `reasoning` delta | `agent_thought_chunk` |
| `tool_call` | `tool_call` (pending, title = tool name, rawInput = arguments) |
| `tool_result` | `tool_call_update` (completed / failed, rawOutput = result) |
| `error` | logged; the prompt still ends with `end_turn` |

Tool call ids are stable per call: the agent assigns one when `ToolCallStarted`
is seen and reuses it on `ToolCallFinished`. `ToolKind` is mapped from the tool
name (`read`/`edit`/`execute`/`fetch`/`think`/`other`).

Cancellation is `asyncio.Task.cancel()` on the turn task; the framework's
`process_inbound` already saves state in a `finally` block, so a cancelled turn
keeps its tape.

### Not in scope (documented, not built)

- `session/request_permission` (Bub has tool denials via hooks; wiring a UI
  prompt is a later step).
- `session/load`, session list/delete/resume, MCP servers, terminals, fs delegation.
- Slash-command autocomplete (`available_commands_update`).

## Desktop client: `desktop/`

- Go module `github.com/bubbuild/bub/desktop` (own module; the repo stays a Python
  project), MyGo `v0.8.3`, native UI only (`ui` package, no webview, no Bun).
- `internal/acp`: a thin adapter over the ACP Go SDK
  (`github.com/caelis-labs/acp-go-sdk` v1.6.0), which owns the JSON-RPC/typed v1
  framing and the agent subprocess. The adapter keeps the spawn config, the
  app-facing `Update` model, and the typed-update mapping; tests drive it over
  in-memory pipes and against a spawned SDK test agent.
- The app spawns the agent (`BUB_ACP_CMD`, default `bub acp`), runs
  `initialize` + `session/new {cwd: <process cwd>}`, and shows a chat:
  - markdown replies, streaming (`markdown.Stream`) and finished (`markdown.View`),
  - tool calls as compact rows with a status (`pending` / `in_progress` / `completed` / `failed`),
  - a Stop button that sends `session/cancel`,
  - an error banner when the agent dies or a request fails.
- All UI state is updated on the main thread via `win.Update`.
- Tests: view test with `ui.NewTester`; client tests with an SDK fake agent over
  in-memory pipes and a spawned SDK test agent.

## How to run

```sh
# terminal 1 (or let the app spawn it): from the repo root
uv run bub acp            # protocol on stdout, logs on stderr

# terminal 2
cd desktop
BUB_ACP_CMD="uv run bub acp" go run .
```

Use `--workspace` on the agent or the app's cwd to choose the project the agent sees.

## Verification

- `uv run pytest tests/test_acp.py -q` plus the existing suite.
- `go build ./...`, `go test ./...`, `go vet ./...` in `desktop/`.
- End-to-end: launch the desktop app against `uv run bub acp` with the repo's
  real model settings, send a prompt, screenshot the window through MyGo's
  automation plugin.

What was actually run on this branch (`feat/bub-desktop-mygo`):

- `uv run pytest -q` → 744 passed, 21 skipped; `ruff check` and `mypy src` clean.
- `cd desktop && go build ./... && go test ./... && go vet ./...` → clean
  (also `go build -tags e2e ./...`).
- Live ACP smoke against the real model: `initialize`, `session/new`,
  `session/prompt` streamed `agent_message_chunk`s and returned `end_turn`; a
  turn using `fs.read` produced `tool_call` + `tool_call_update`; `session/cancel`
  returned `cancelled`.
- End-to-end through the built window (`-tags e2e`, `MYGO_AUTOMATION`):
  automation typed a prompt through the `composer` key, pressed `send`, saw the
  tool row and the Markdown reply, and captured `screenshot.png`.
