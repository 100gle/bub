# bub desktop

A GPUI desktop front-end for Bub. It holds **no Bub logic**: it spawns the
`bub desktop` bridge as a child process and speaks newline-delimited JSON with
it over stdio, then renders the stream.

```
desktop/
├── src/
│   ├── main.rs        # entry point: window, key bindings, the module map
│   ├── app.rs         # all state and the whole protocol; the only writer to the bridge
│   ├── bridge.rs      # the child process and its stdout stream
│   ├── model.rs       # the data vocabulary both sides share
│   ├── theme.rs       # palette, metrics, type scale
│   ├── shot.rs        # dev-only: paint a frame to a PNG (`shot` feature)
│   └── ui/            # one module per surface
│       ├── atoms.rs     # shared primitives (plates, chips, code blocks)
│       ├── titlebar.rs  # the system title bar
│       ├── transcript.rs# the conversation column
│       ├── composer.rs  # the field and its one action
│       ├── empty.rs     # the session-start state
│       └── inspector.rs # the agent panel
├── tools/fake_bridge.py  # a scripted stand-in for the bridge
├── bridge/            # the Python bridge plugin (`bub-desktop`)
└── Cargo.toml
```

## The shell

One window, one chrome line. The window opens with its system title bar hidden,
so the traffic lights sit inside the app's own top row and the shell draws only
what a reader needs:

- **Transcript** — a centred column at a fixed measure. The conversation is the
  document: assistant text is set on the canvas, only the user's turns carry a
  bubble, and each tool call collapses to one line that expands into its
  arguments and result.
- **Composer** — a floating capsule at the foot of the column, with no hint line
  and no rule above it. The accent plate is the only filled control in the
  window, and it is the same element in the session-start state.
- **Agent panel** (⌘I, closed by default) — a resizable inspector with two
  halves. *Activity* is the live trace of the run; *Tools* holds the approval
  policy and the tools, skills and commands the runtime reported, as chips whose
  tooltips carry the descriptions.
- **Tool approvals** — when *Ask before tools run* is on, every model tool call
  renders an inline Allow / Deny card and the run blocks until you answer. Off
  (the default) runs tools without asking.

Everything the runtime reports but a reader cannot act on — the protocol
version, the session id, a tool count — is either in the panel or gone.

## Run it

The bridge plugin is installed into the repo environment by `uv sync` (it is a
dev dependency with an editable source). From the repo root:

```sh
# 1. build (first time ~3 min, afterwards ~3s)
cd desktop && cargo build

# 2. run
cd .. && ./desktop/target/debug/bub-desktop
```

The app tries `bub desktop`, then falls back to `uv run bub desktop`. Override
the command with `BUB_DESKTOP_CMD`, e.g.:

```sh
BUB_DESKTOP_CMD="uv run bub desktop" ./desktop/target/debug/bub-desktop
```

### Reviewing the shell without a model

`tools/fake_bridge.py` speaks the same protocol, so the front-end can be driven
with no model, network, or workspace:

```sh
BUB_DESKTOP_CMD="python3 tools/fake_bridge.py" FAKE_MODE=demo \
  ./target/debug/bub-desktop
```

| variable | effect |
| --- | --- |
| `FAKE_MODE` | `empty` for the session-start state, `demo` (default) to stream a full run per message |
| `FAKE_APPROVAL` | `1` to have the first run stop on an approval |
| `FAKE_WORKSPACE` | the path the bridge reports (defaults to the working directory) |

### Capturing a frame

The `shot` feature paints one frame into an offscreen texture and writes it to a
PNG, which is the only way to review the layout on a machine that cannot
screen-capture a window. GPUI gates `Window::render_to_image` behind its
`test-support` feature, which is why this is a feature rather than a flag.

```sh
BUB_DESKTOP_CMD="python3 tools/fake_bridge.py" FAKE_MODE=demo \
BUB_SHOT=/tmp/shell.png BUB_SHOT_PROMPT="Add a --dry-run flag to deploy" \
  cargo run --features shot
```

| variable | effect |
| --- | --- |
| `BUB_SHOT` | output path; without it the feature does nothing |
| `BUB_SHOT_PROMPT` | text to start a run with, to fill the transcript |
| `BUB_SHOT_AFTER` | ms to wait before touching anything (default 900) |
| `BUB_SHOT_SETTLE` | ms to wait after starting that run (default 1900) |
| `BUB_SHOT_INSPECTOR` | capture with the agent panel open |
| `BUB_SHOT_TAB` | `tools` to capture the panel's other half |

## Build cost

GPUI is pulled from crates.io via `gpui-kit` — **no manual clone** of zed and no
Xcode Metal Toolchain download. GPU shaders compile at runtime
(`gpui-pre-platform` with `runtime_shaders`).

| step | cost |
| --- | --- |
| first full build | ~3 min |
| rebuild after editing `src/` | ~3 s |
| `--features shot` | one extra crate (`image`) |

To reduce peak load on a busy machine, cap parallel jobs:

```sh
CARGO_BUILD_JOBS=4 cargo build
```

## Protocol (v2)

Every message has an `id`. Requests that produce a reply echo that `id` on the
reply; unsolicited events omit it.

UI → bridge:

| type | payload |
| --- | --- |
| `send` | `text`, optional `session_id` |
| `stop` | optional `session_id` — quits the turn via the channel router |
| `models` | optional `session_id` |
| `approval_response` | `approval_id`, `allow` (bool) |
| `approval_mode` | `mode` = `auto` \| `ask` |
| `ping` | — |

Bridge → UI:

| type | payload |
| --- | --- |
| `ready` | `protocol`, `workspace`, `model`, `command_prefix`, `session_id`, `commands`, `tools`, `skills`, `approval_mode` |
| `stream` | `kind` = `text`/`reasoning`/`tool_call`/`tool_result`/`usage`/`error`/`final`, raw `StreamEvent` in `data` |
| `outbound` | `session_id`, `channel`, `kind`, `content` |
| `generating` | `active` (bool) |
| `approval` | `approval_id`, `tool`, `arguments` — the run is blocked until answered |
| `approval_resolved` | `approval_id`, `allow`, `reason` |
| `approval_mode` | `mode` — the gate's mode after an `approval_mode` request |
| `result` | `ok`, optional `error` — acknowledges a request by `id` |
| `pong` | echoes a `ping` |
| `models` | `current`, `models[]` |

`ready.tools[]` carries `{ name, description, exposure, deferred }`;
`ready.skills[]` carries `{ name, description, source }`.

### Tool approval

Approval is opt-in. The gate starts in `auto` (inert) and only blocks once the
front-end requests `ask` mode — via the *Ask before tools run* switch, or the
`BUB_DESKTOP_APPROVE=ask` environment variable at launch. In `ask` mode a
`before_tool_call` hook emits an `approval` event and awaits the matching
`approval_response` (600 s timeout, then deny). Because each turn runs in its own
task, the stdin reader keeps servicing requests while a turn waits, so answering
never deadlocks. When the bridge stops, any pending approval is denied.
