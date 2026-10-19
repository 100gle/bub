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
│       ├── atoms.rs     # shared primitives (plates, captions, code blocks)
│       ├── sidebar.rs   # the navigation rail
│       ├── titlebar.rs  # the system title bar
│       ├── transcript.rs# the conversation column
│       ├── composer.rs  # the field and its one action
│       ├── menus.rs     # the floating pickers and the command palette
│       ├── settings.rs  # the settings pages
│       └── empty.rs     # the session-start state
├── tools/fake_bridge.py  # a scripted stand-in for the bridge
├── bridge/            # the Python bridge plugin (`bub-desktop`)
└── Cargo.toml
```

## The shell

One window, one chrome line and one rail. The window opens with its system title
bar hidden, so the traffic lights sit inside the app's own top row; under that,
a rail on the left says where you are, and everything else is the main area.

- **Rail** — the conversation, then the settings pages, one under the other. They
  are peers: clicking one is navigation, not opening a modal, which is why there
  is nothing to close anywhere in the window. ⌘, goes to settings and Esc comes
  back; both land on the page you left.
- **Chrome** — the wordmark and a spinner that appears only while a turn is
  running. The model, the connection light, the workspace path and a settings
  gear all used to live here; the model already has a picker on the composer and
  a page in Settings, a green dot beside the word "connected" says nothing, and
  the gear opened a page the rail now simply goes to. Only a bridge that answered
  and then stopped gets a badge.
- **Transcript** — a centred column at a fixed measure, and **one turn is one
  row**. The reader's message is one; the model's answer is one, however many
  steps it took — a thought, a tool call, its result, some prose — set closer
  together inside the block than rows are, because they are one message. Markdown
  is set for reading: headings, inline code and fenced blocks with syntax
  highlighting, and the step still being written into eases its new text in
  rather than replacing itself.
- **The wait lives on the answer.** The answer's row is created empty the moment
  the message is sent, and its last line says what is happening: *Thinking…*,
  *Running grep…*, *Writing…*, or *Waiting for your approval*, with the seconds
  counting up. It shimmers while the model is the one working and goes still, in
  warning orange, when the run is blocked on you. The phase is derived from the
  transcript rather than tracked beside it, so it cannot contradict the row it
  sits under — and because it is drawn at the tail of the answer, the gap between
  sending and the first token is part of the conversation rather than a blank
  somewhere else on screen.
- **A keystroke with nowhere to go** — the bridge takes one turn at a time, so
  Enter during a run cannot send. The field keeps the text and a dark pill says
  *Still working — press Stop to interrupt* for a beat. It floats over the
  transcript's fade rather than sitting in the column, so nothing moves when it
  arrives or leaves. Swallowing the keystroke was the one thing that made the
  composer read as broken rather than busy; typing to draft the next message
  still works.
- **Composer** — a floating capsule at the foot of the column, with no hint line
  and no rule above it. The capsule carries everything the turn needs: an attach
  control, the permission the next turn runs under, the model it runs on, and the
  one filled action, which becomes Stop while a turn is in flight. Files dropped
  in the picker queue as removable chips above the field and ride along with the
  message. The accent plate is the only filled control in the window, and it is
  the same element in the session-start state.
- **Commands** — typing the command prefix (Bub's default is a comma, and the
  prefix is configurable) at the start of the field floats a palette of the
  commands the runtime advertises above the capsule. Picking one drops its trigger
  into the field, ready for arguments.
- **Settings** (⌘,) — a page per subject in the main area, under its own toolbar:
  how it behaves, the model and its limits, what it can do here, the channels,
  and the runtime facts. Edits are written back to Bub's config file through the
  bridge; a key the environment already supplies is marked, because writing it
  changes nothing. Save and Discard appear only on the pages that write — and
  only once there is something to save.
- **Tool approvals** — when *Ask before tools run* is on, every model tool call
  renders an inline Allow / Deny card inside the answer and the run blocks until
  you answer; the answer's tail line says who it is waiting on. Off (the default)
  runs tools without asking. The composer's permission picker flips the same
  switch.

Everything the runtime reports but a reader cannot act on — the protocol
version, the session id, a tool count — is in Settings → About or gone.

There is no agent panel. An earlier build had one behind ⌘I, and it was a second
copy of the transcript: every tool call it traced was already a row in the
conversation with the same status, and the reader had no reason to open it. The
one thing it held that could not be seen elsewhere — the tools, skills and
commands the runtime discovered — is now a page in Settings, where reference
material belongs.

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
| `BUB_SHOT_SETTLE` | ms to wait after starting that run (default 1900); lower it to catch a turn in flight, with the status line up |
| `BUB_SHOT_SETTINGS` | capture the settings page instead of the shell |
| `BUB_SHOT_SECTION` | `model`, `capabilities`, `channels`, or `about` for the settings page |
| `BUB_SHOT_COMPOSE` | text to seed the composer with, e.g. `,` for the command palette |
| `BUB_SHOT_BLOCKED` | send mid-run, to capture the *still working* nudge |

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
| `send` | `text`, optional `session_id`, optional `attachments[]` = `{ path, name, media_type }` |
| `stop` | optional `session_id` — quits the turn via the channel router |
| `models` | optional `session_id` |
| `settings` | — reads the effective configuration and the overridden keys |
| `settings_update` | `values` — dotted keys mapped to their new values |
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
| `settings` | `values` (dotted keys), `overridden[]`, `config_file`, `home` |

`ready.tools[]` carries `{ name, description, exposure, deferred }`;
`ready.skills[]` carries `{ name, description, source }`.

### Settings

The settings page edits the same keys Bub reads, so the bridge both serves an
effective snapshot (config file, defaults, and environment merged) and writes
edits back through `bub.configure`, which validates them before persisting. A
key present in the environment is listed in `overridden`: the config write
succeeds but cannot take effect, so the page marks it instead of pretending.
A `settings_update` that changes `model` also routes Bub's own `,model` command,
so a new default becomes the running session's model at once; a `command_prefix`
change is applied in the window immediately.

### Tool approval

Approval is opt-in. The gate starts in `auto` (inert) and only blocks once the
front-end requests `ask` mode — via the *Ask before tools run* switch, or the
`BUB_DESKTOP_APPROVE=ask` environment variable at launch. In `ask` mode a
`before_tool_call` hook emits an `approval` event and awaits the matching
`approval_response` (600 s timeout, then deny). Because each turn runs in its own
task, the stdin reader keeps servicing requests while a turn waits, so answering
never deadlocks. When the bridge stops, any pending approval is denied.
