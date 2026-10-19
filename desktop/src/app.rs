//! The application state and the protocol it speaks.
//!
//! [`DesktopApp`] owns the transcript and the capability lists, and it is the
//! only thing that writes to the bridge. The view code in [`crate::ui`] reads
//! these fields and calls back into the methods here; no rendering decision
//! leaks into this module beyond the top-level composition in [`Render`].

use std::collections::HashMap;
use std::collections::HashSet;
use std::io::Write;
use std::process::Child;
use std::process::ChildStdin;
use std::time::{Duration, Instant};

use gpui_kit::component::{
    h_flex,
    input::{InputEvent, InputState, TextareaState},
    message_scroller::MessageScrollerState,
    v_flex,
};
use gpui_kit::{
    App, AppContext as _, Context, Entity, InteractiveElement as _, IntoElement,
    ParentElement as _, Render, Styled as _, Subscription, Window,
};

use serde_json::{json, Map, Value};

use crate::bridge;
use crate::model::{
    array_field, format_usage, parse_commands, parse_skill, parse_tool, pretty, pretty_arg,
    result_looks_failed, string_field, ApprovalMode, Attachment, BlockKey, Command, Entry,
    ModelChoice, Open, SettingsSection, SkillInfo, Status, Step, ToolInfo, Turn, View,
};
use crate::theme;

/// What the running turn is doing, for the status line under the transcript.
///
/// The three phases are the three things a reader waits through, and each one
/// is worth naming: the model has not answered yet, a tool is on the wire, or
/// text is arriving. It is derived rather than stored, so it cannot disagree
/// with the transcript it sits under.
/// How long the "still working" nudge stays up after a swallowed Enter.
const BLOCKED_HINT: Duration = Duration::from_millis(2600);

#[derive(Clone, PartialEq)]
pub(crate) enum RunPhase {
    /// A turn is open but nothing has come back yet.
    Thinking,
    /// A tool call is in flight, named.
    Running(String),
    /// A tool call is blocked on the reader's verdict.
    Awaiting(String),
    /// Text or reasoning is streaming.
    Writing,
}

impl RunPhase {
    /// The words the status line shows.
    pub(crate) fn label(&self) -> String {
        match self {
            RunPhase::Thinking => "Thinking…".to_string(),
            RunPhase::Running(tool) => format!("Running {tool}…"),
            RunPhase::Awaiting(_) => "Waiting for your approval".to_string(),
            RunPhase::Writing => "Writing…".to_string(),
        }
    }

    /// Whether the model is the one working. Approval is the one phase that is
    /// waiting on the reader, so it is the one that does not shimmer.
    pub(crate) fn is_working(&self) -> bool {
        !matches!(self, RunPhase::Awaiting(_))
    }
}

/// A duration as the status line prints it: one decimal, so the number visibly
/// moves without the width jumping around.
pub(crate) fn format_elapsed(elapsed: Duration) -> String {
    format!("{:.1}s", elapsed.as_secs_f32())
}

pub(crate) struct DesktopApp {
    pub(crate) entries: Vec<Entry>,
    /// Blocks whose disclosure state has been flipped away from the default.
    pub(crate) toggled: HashSet<BlockKey>,
    pub(crate) scroller: Entity<MessageScrollerState>,
    pub(crate) input: Entity<TextareaState>,
    _input_sub: Subscription,
    pub(crate) model: String,
    pub(crate) workspace: String,
    pub(crate) protocol: u64,
    pub(crate) connected: bool,
    /// Whether `ready` has ever arrived, so a bridge that died can be told
    /// apart from one that has not finished starting.
    pub(crate) ready: bool,
    pub(crate) generating: bool,
    /// When the current turn started, so the status line can count up. `None`
    /// between turns.
    pub(crate) started_at: Option<Instant>,
    /// How long the current turn has been running, refreshed by the ticker.
    pub(crate) elapsed: Duration,
    /// When the reader tried to send during a run. The bridge takes one turn at
    /// a time, so the keystroke has nowhere to go — this is what lets the status
    /// line say so instead of swallowing it. Cleared by the ticker after a beat.
    pub(crate) blocked: Option<Instant>,
    open: Open,
    next_id: u64,
    child: Option<Child>,
    stdin: Option<ChildStdin>,
    // Agent surfaces.
    pub(crate) approval_mode: ApprovalMode,
    pub(crate) tools: Vec<ToolInfo>,
    pub(crate) skills: Vec<SkillInfo>,
    pub(crate) commands: Vec<Command>,
    // Composer surfaces.
    pub(crate) model_choices: Vec<ModelChoice>,
    models_requested: bool,
    pub(crate) attachments: Vec<Attachment>,
    /// The prefix the runtime uses for commands (Bub's default is a comma).
    pub(crate) command_prefix: String,
    // Settings.
    /// Which surface the window is showing. The conversation and the settings
    /// pages are peers; moving between them is navigation, so there is nothing
    /// to open and nothing to close.
    pub(crate) view: View,
    /// The settings page being shown, and the one ⌘, returns to.
    pub(crate) settings_section: SettingsSection,
    pub(crate) settings_values: Map<String, Value>,
    pub(crate) settings_overridden: HashSet<String>,
    pub(crate) settings_config_file: String,
    pub(crate) settings_home: String,
    settings_requested: bool,
    /// Which settings have an unsaved switch edit, keyed by dotted path.
    pub(crate) settings_toggles: HashMap<String, bool>,
    /// Single-line fields, created on first paint and keyed by dotted path.
    settings_inputs: HashMap<String, Entity<InputState>>,
    settings_subs: Vec<Subscription>,
}

impl DesktopApp {
    pub(crate) fn new(window: &mut Window, cx: &mut Context<Self>) -> Self {
        let scroller = cx.new(|cx| MessageScrollerState::new(0, cx));
        cx.observe(&scroller, |_, _, cx| cx.notify()).detach();

        let input = cx.new(|cx| {
            TextareaState::new(window, cx)
                .auto_grow(1, 8)
                .submit_on_enter(true)
                .placeholder("Ask Bub anything…")
        });
        let _input_sub = cx.subscribe_in(
            &input,
            window,
            |this: &mut Self, input, event: &InputEvent, window, cx| match event {
                InputEvent::PressEnter { shift, .. } if !shift => this.send(input, window, cx),
                InputEvent::Change => cx.notify(),
                _ => {}
            },
        );

        let mut app = Self {
            entries: Vec::new(),
            toggled: HashSet::new(),
            scroller,
            input,
            _input_sub,
            model: String::new(),
            workspace: String::new(),
            protocol: 0,
            connected: false,
            ready: false,
            generating: false,
            started_at: None,
            elapsed: Duration::ZERO,
            blocked: None,
            open: Open::None,
            next_id: 1,
            child: None,
            stdin: None,
            approval_mode: ApprovalMode::Auto,
            tools: Vec::new(),
            skills: Vec::new(),
            commands: Vec::new(),
            model_choices: Vec::new(),
            models_requested: false,
            attachments: Vec::new(),
            // Mirrors Bub's own default until `ready` reports the live value.
            command_prefix: ",".to_string(),
            settings_requested: false,
            view: View::Conversation,
            settings_section: SettingsSection::General,
            settings_values: Map::new(),
            settings_overridden: HashSet::new(),
            settings_config_file: String::new(),
            settings_home: String::new(),
            settings_toggles: HashMap::new(),
            settings_inputs: HashMap::new(),
            settings_subs: Vec::new(),
        };

        match bridge::spawn() {
            Ok(process) => {
                app.stdin = Some(process.stdin);
                app.child = Some(process.child);
                let lines = process.lines;
                cx.spawn(async move |this, cx| {
                    while let Ok(line) = lines.recv().await {
                        if this
                            .update(cx, |app, cx| app.handle_line(&line, cx))
                            .is_err()
                        {
                            break;
                        }
                    }
                    // The stream ended: the bridge is gone. Say so rather than
                    // leaving a stale "connected" on screen for the rest of the
                    // session.
                    let _ = this.update(cx, |app, cx| {
                        app.connected = false;
                        cx.notify();
                    });
                })
                .detach();
            }
            Err(err) => {
                app.push_turn(
                    Turn::Assistant(vec![Step::Error(format!(
                        "could not start the bub bridge: {err}\n\n\
                         Install the plugin (`uv pip install -e desktop/bridge`) or set BUB_DESKTOP_CMD."
                    ))]),
                    cx,
                );
            }
        }

        app
    }

    fn next_id(&mut self) -> u64 {
        let id = self.next_id;
        self.next_id += 1;
        id
    }

    pub(crate) fn toggle(&mut self, key: BlockKey, cx: &mut Context<Self>) {
        if !self.toggled.remove(&key) {
            self.toggled.insert(key);
        }
        cx.notify();
    }

    fn write(&mut self, value: Value) {
        if let Some(stdin) = self.stdin.as_mut() {
            let mut line = value.to_string();
            line.push('\n');
            if stdin.write_all(line.as_bytes()).is_ok() {
                let _ = stdin.flush();
            }
        }
    }

    /// Append a row and keep the view pinned to the tail when it already was.
    fn push_turn(&mut self, turn: Turn, cx: &mut Context<Self>) {
        let id = self.next_id();
        self.entries.push(Entry { id, turn });
        self.grow(1, cx);
    }

    /// The assistant turn the stream is filling, if the last row is one.
    ///
    /// Every turn opens with an empty assistant block, so this is where a
    /// streamed step belongs; a step that arrives with no block open — a stray
    /// result, say — gets one of its own rather than being dropped.
    fn last_assistant(&self) -> Option<usize> {
        match self.entries.last() {
            Some(Entry {
                turn: Turn::Assistant(_),
                ..
            }) => Some(self.entries.len() - 1),
            _ => None,
        }
    }

    fn assistant(&mut self) -> &mut Vec<Step> {
        let index = match self.last_assistant() {
            Some(index) => index,
            None => {
                // A step arrived before any turn was open. Rather than invent a
                // place for it, open a turn to hold it.
                let id = self.next_id();
                self.entries.push(Entry {
                    id,
                    turn: Turn::Assistant(Vec::new()),
                });
                self.entries.len() - 1
            }
        };
        match &mut self.entries[index].turn {
            Turn::Assistant(steps) => steps,
            Turn::User { .. } => unreachable!("last_assistant checked the variant"),
        }
    }

    /// Tell the virtual list that `count` rows were appended.
    fn grow(&mut self, count: usize, cx: &mut Context<Self>) {
        self.scroller.update(cx, |state, cx| {
            let following = state.is_following_tail();
            let _ = state.append(count, cx);
            if following {
                state.scroll_to_end(cx);
            }
        });
    }

    /// A row grew without the count changing: a step was added to a turn, or
    /// text was appended to a step.
    fn remeasure(&mut self, index: usize, cx: &mut Context<Self>) {
        self.scroller.update(cx, |state, cx| {
            let _ = state.remeasure_items(index..index + 1, cx);
        });
    }

    /// Open or close the running turn.
    ///
    /// The single place `generating` moves, so the clock can never disagree
    /// with it: the timer is armed on the way in and frozen on the way out,
    /// whichever side — the reader sending, or the bridge reporting — moved it.
    fn set_generating(&mut self, active: bool, cx: &mut Context<Self>) {
        if active == self.generating {
            return;
        }
        self.generating = active;
        if active {
            self.started_at = Some(Instant::now());
            self.elapsed = Duration::ZERO;
            self.blocked = None;
            self.start_ticker(cx);
        } else {
            self.open = Open::None;
            self.elapsed = self
                .started_at
                .map(|at| at.elapsed())
                .unwrap_or(self.elapsed);
            self.started_at = None;
            self.blocked = None;
        }
    }

    /// Tick the status line's clock while a turn runs.
    ///
    /// A task rather than a render-time clock: the elapsed time is state the
    /// view reads, so the view stays a pure function of the app and the number
    /// cannot drift from what was last painted.
    fn start_ticker(&self, cx: &mut Context<Self>) {
        cx.spawn(async move |this, cx| loop {
            smol::Timer::after(Duration::from_millis(100)).await;
            let running = this
                .update(cx, |app, cx| {
                    if !app.generating {
                        return false;
                    }
                    app.elapsed = app.started_at.map(|at| at.elapsed()).unwrap_or_default();
                    // A nudge the reader has had time to read is just noise on
                    // top of the phase it replaced.
                    if app.blocked.is_some_and(|at| at.elapsed() > BLOCKED_HINT) {
                        app.blocked = None;
                    }
                    cx.notify();
                    true
                })
                .unwrap_or(false);
            if !running {
                break;
            }
        })
        .detach();
    }

    /// What the open turn is doing, shown at the tail of its own block.
    ///
    /// Derived from the transcript rather than tracked alongside it: a step
    /// kept in a second list could only ever disagree with the row the reader
    /// is looking at.
    pub(crate) fn phase(&self) -> RunPhase {
        let open_tool = self
            .entries
            .iter()
            .rev()
            .find_map(|entry| match &entry.turn {
                Turn::Assistant(steps) => steps.iter().rev().find_map(|step| match step {
                    Step::Tool { name, state, .. } => Some((name.clone(), *state)),
                    _ => None,
                }),
                Turn::User { .. } => None,
            });
        if let Some((name, state)) = open_tool {
            if !state.is_terminal() {
                return if state == Status::Awaiting {
                    RunPhase::Awaiting(name)
                } else {
                    RunPhase::Running(name)
                };
            }
        }
        match self.open {
            Open::Text | Open::Reasoning => RunPhase::Writing,
            Open::None => RunPhase::Thinking,
        }
    }

    pub(crate) fn send(
        &mut self,
        input: &Entity<TextareaState>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let text = input.read(cx).value().trim().to_string();
        // Nothing to say is not the same as nowhere to say it. The first is a
        // keystroke worth ignoring; the second is one the reader needs an
        // answer for, because the bridge takes one turn at a time and the
        // message they just wrote is still sitting in the field.
        if text.is_empty() && self.attachments.is_empty() {
            return;
        }
        if self.generating {
            self.blocked = Some(Instant::now());
            cx.notify();
            return;
        }
        input.update(cx, |state, cx| state.set_value("", window, cx));
        let attachments = std::mem::take(&mut self.attachments);
        self.begin(text, attachments, cx);
    }

    /// Start a turn from `text`: show it, hand it to the bridge, and mark the
    /// run active. Separate from [`Self::send`] so that the composer's textarea
    /// is the only thing `send` has to clear.
    fn begin(&mut self, text: String, attachments: Vec<Attachment>, cx: &mut Context<Self>) {
        let names: Vec<String> = attachments.iter().map(|file| file.name.clone()).collect();
        self.push_turn(
            Turn::User {
                text: text.clone(),
                attachments: names,
            },
            cx,
        );
        // The answer's block goes up now, while it is still empty, so the wait
        // is something the reader watches in the conversation rather than a
        // blank under the message they just sent.
        self.push_turn(Turn::Assistant(Vec::new()), cx);
        self.open = Open::None;
        // The clock starts when the message leaves, not when the model first
        // answers: the wait the reader is sitting through begins here.
        self.set_generating(true, cx);
        let id = self.next_id();
        let payload = json!({
            "id": id,
            "type": "send",
            "text": text,
            "attachments": attachments
                .iter()
                .map(|file| json!({
                    "path": file.path,
                    "name": file.name,
                    "media_type": file.kind.media_type(),
                }))
                .collect::<Vec<_>>(),
        });
        self.write(payload);
        cx.notify();
    }

    /// Start a turn without a composer. Only the `shot` feature uses this, to
    /// photograph a transcript without a keyboard.
    #[cfg(feature = "shot")]
    pub(crate) fn prompt_directly(&mut self, text: &str, cx: &mut Context<Self>) {
        self.begin(text.to_string(), Vec::new(), cx);
    }

    pub(crate) fn submit(&mut self, text: String, window: &mut Window, cx: &mut Context<Self>) {
        self.input
            .update(cx, |state, cx| state.set_value(text, window, cx));
        let input = self.input.clone();
        self.input.update(cx, |state, cx| state.focus(window, cx));
        self.send(&input, window, cx);
    }

    pub(crate) fn stop(&mut self, cx: &mut Context<Self>) {
        let id = self.next_id();
        self.write(json!({ "id": id, "type": "stop" }));
        self.set_generating(false, cx);
        cx.notify();
    }

    pub(crate) fn set_approval_mode(&mut self, mode: ApprovalMode, cx: &mut Context<Self>) {
        self.approval_mode = mode;
        let id = self.next_id();
        self.write(json!({ "id": id, "type": "approval_mode", "mode": mode.as_str() }));
        cx.notify();
    }

    pub(crate) fn respond_approval(
        &mut self,
        approval_id: &str,
        allow: bool,
        cx: &mut Context<Self>,
    ) {
        let id = self.next_id();
        self.write(json!({
            "id": id,
            "type": "approval_response",
            "approval_id": approval_id,
            "allow": allow,
        }));
        cx.notify();
    }

    /// Whether the composer holds something worth sending.
    pub(crate) fn can_send(&self, cx: &App) -> bool {
        !self.input.read(cx).value().trim().is_empty() || !self.attachments.is_empty()
    }

    /// Whether the reader just tried to send into a running turn.
    pub(crate) fn blocked_hint(&self) -> bool {
        self.blocked.is_some()
    }

    // ---- Attachments ------------------------------------------------------

    /// Open the system file picker and queue whatever comes back.
    pub(crate) fn pick_attachments(&mut self, cx: &mut Context<Self>) {
        let receiver = cx.prompt_for_paths(gpui_kit::PathPromptOptions {
            files: true,
            directories: false,
            multiple: true,
            prompt: Some("Attach files".into()),
        });
        cx.spawn(async move |this, cx| {
            if let Ok(Ok(Some(paths))) = receiver.await {
                let _ = this.update(cx, |app, cx| {
                    for path in paths {
                        let file = Attachment::from_path(&path);
                        if !app
                            .attachments
                            .iter()
                            .any(|queued| queued.path == file.path)
                        {
                            app.attachments.push(file);
                        }
                    }
                    cx.notify();
                });
            }
        })
        .detach();
    }

    pub(crate) fn remove_attachment(&mut self, path: &str, cx: &mut Context<Self>) {
        self.attachments.retain(|file| file.path != path);
        cx.notify();
    }

    // ---- Models -----------------------------------------------------------

    /// Ask the runtime which models it can switch between. Cached per session.
    pub(crate) fn request_models(&mut self, cx: &mut Context<Self>) {
        if self.models_requested {
            return;
        }
        self.models_requested = true;
        let id = self.next_id();
        self.write(json!({ "id": id, "type": "models" }));
        cx.notify();
    }

    /// Switch the session model by running Bub's own `,model` command, then ask
    /// the runtime to confirm the new selection.
    pub(crate) fn choose_model(&mut self, model: String, cx: &mut Context<Self>) {
        if model == self.model {
            return;
        }
        self.model = model.clone();
        self.models_requested = false;
        let command = format!("{}model {}", self.command_prefix, model);
        self.send_command(command, cx);
        self.request_models(cx);
        cx.notify();
    }

    // ---- Commands ---------------------------------------------------------

    /// Hand a command line to the bridge exactly as if the user had typed it.
    pub(crate) fn send_command(&mut self, command: String, cx: &mut Context<Self>) {
        self.begin(command, Vec::new(), cx);
    }

    /// Put `trigger` at the start of the composer, ready for arguments.
    pub(crate) fn insert_command(
        &mut self,
        trigger: &str,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let value = format!("{trigger} ");
        self.input.update(cx, |state, cx| {
            state.set_value(value, window, cx);
            state.focus(window, cx);
        });
        cx.notify();
    }

    // ---- Navigation -------------------------------------------------------

    /// Show a settings page. The first time one is opened the runtime is asked
    /// for its snapshot, which the fields are seeded from.
    pub(crate) fn select_settings_section(
        &mut self,
        section: SettingsSection,
        cx: &mut Context<Self>,
    ) {
        self.settings_section = section;
        self.view = View::Settings;
        self.request_settings(cx);
        cx.notify();
    }

    /// Go to settings, on the page last visited.
    pub(crate) fn open_settings(&mut self, cx: &mut Context<Self>) {
        let section = self.settings_section;
        self.select_settings_section(section, cx);
    }

    /// Go back to the conversation. Not a close: the settings page is where the
    /// window was, not something laid over it.
    pub(crate) fn close_settings(&mut self, cx: &mut Context<Self>) {
        self.view = View::Conversation;
        cx.notify();
    }

    fn request_settings(&mut self, _cx: &mut Context<Self>) {
        if self.settings_requested {
            return;
        }
        self.settings_requested = true;
        let id = self.next_id();
        self.write(json!({ "id": id, "type": "settings" }));
    }

    /// A settings value as editable text. Arrays read as comma-separated lists.
    pub(crate) fn settings_text(&self, key: &str) -> String {
        match self.settings_values.get(key) {
            Some(Value::Array(items)) => items
                .iter()
                .filter_map(Value::as_str)
                .collect::<Vec<_>>()
                .join(", "),
            Some(Value::Bool(value)) => value.to_string(),
            Some(Value::Number(value)) => value.to_string(),
            Some(Value::String(value)) => value.clone(),
            _ => String::new(),
        }
    }

    /// Whether the runtime reports this key as overridden by the environment.
    pub(crate) fn settings_is_overridden(&self, key: &str) -> bool {
        self.settings_overridden.contains(key)
    }

    /// A switch's current position: the pending edit if there is one, else the
    /// snapshot.
    pub(crate) fn settings_switch(&self, key: &str) -> bool {
        if let Some(value) = self.settings_toggles.get(key) {
            return *value;
        }
        self.settings_values
            .get(key)
            .and_then(Value::as_bool)
            .unwrap_or(false)
    }

    pub(crate) fn set_settings_switch(&mut self, key: &str, value: bool, cx: &mut Context<Self>) {
        self.settings_toggles.insert(key.to_string(), value);
        cx.notify();
    }

    /// The single-line editor for a settings key, created and seeded on first
    /// use and kept so partial edits survive a re-render.
    pub(crate) fn settings_input(
        &mut self,
        key: &str,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Entity<InputState> {
        if let Some(input) = self.settings_inputs.get(key) {
            return input.clone();
        }
        let seeded = self.settings_text(key);
        let input = cx.new(|cx| {
            InputState::new(window, cx)
                .default_value(seeded)
                .placeholder("—")
        });
        let sub = cx.subscribe_in(
            &input,
            window,
            |_this, _input, event: &InputEvent, _window, cx| {
                if matches!(event, InputEvent::Change) {
                    cx.notify();
                }
            },
        );
        self.settings_subs.push(sub);
        self.settings_inputs.insert(key.to_string(), input.clone());
        input
    }

    /// Whether any settings control differs from the last snapshot.
    pub(crate) fn settings_dirty(&self, cx: &App) -> bool {
        if !self.settings_toggles.is_empty() {
            return true;
        }
        self.settings_inputs.iter().any(|(key, input)| {
            let value = input.read(cx).value().to_string();
            value != self.settings_text(key)
        })
    }

    /// Collect the diff and hand it to the bridge to persist.
    pub(crate) fn save_settings(&mut self, cx: &mut Context<Self>) {
        let mut patch: Vec<(String, Value)> = Vec::new();
        // The edited text of each changed field, kept so the snapshot baseline
        // can move to it: the form should read as saved, not as still pending,
        // and the runtime does not echo a fresh snapshot back.
        let mut baseline: Vec<(String, String)> = Vec::new();
        for (key, value) in &self.settings_toggles {
            patch.push((key.clone(), json!(value)));
        }
        for (key, input) in &self.settings_inputs {
            let text = input.read(cx).value().to_string();
            if text == self.settings_text(key) {
                continue;
            }
            patch.push((key.clone(), crate::ui::settings::encode_value(key, &text)));
            baseline.push((key.clone(), text));
        }
        if patch.is_empty() {
            return;
        }
        // A prefix edit takes effect in this window at once: the runtime reads
        // it from the file on its next command, but the composer's palette and
        // the command matcher should not wait for a restart.
        if let Some((_, Value::String(prefix))) =
            patch.iter().find(|(key, _)| key == "command_prefix")
        {
            if !prefix.is_empty() {
                let raw: Vec<String> = self
                    .commands
                    .iter()
                    .map(|command| format!("{prefix}{}", command.name))
                    .collect();
                self.command_prefix = prefix.clone();
                self.commands = parse_commands(&raw, &self.command_prefix);
            }
        }
        let id = self.next_id();
        let values: Map<String, Value> = patch.into_iter().collect();
        self.write(json!({ "id": id, "type": "settings_update", "values": values }));
        for (key, text) in baseline {
            self.settings_values.insert(key, Value::String(text));
        }
        for (key, value) in &self.settings_toggles {
            self.settings_values.insert(key.clone(), json!(value));
        }
        self.settings_toggles.clear();
        cx.notify();
    }

    /// Drop unsaved edits and reset fields to the last snapshot.
    pub(crate) fn reset_settings(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        self.settings_toggles.clear();
        let keys: Vec<String> = self.settings_inputs.keys().cloned().collect();
        for key in keys {
            let text = self.settings_text(&key);
            if let Some(input) = self.settings_inputs.get(&key) {
                input.update(cx, |state, cx| state.set_value(text, window, cx));
            }
        }
        cx.notify();
    }

    fn handle_line(&mut self, line: &str, cx: &mut Context<Self>) {
        let Ok(value) = serde_json::from_str::<Value>(line) else {
            return;
        };
        match value
            .get("type")
            .and_then(Value::as_str)
            .unwrap_or_default()
        {
            "ready" => {
                self.connected = true;
                self.ready = true;
                self.model = string_field(&value, "model");
                self.workspace = string_field(&value, "workspace");
                self.protocol = value.get("protocol").and_then(Value::as_u64).unwrap_or(0);
                self.command_prefix = {
                    let prefix = string_field(&value, "command_prefix");
                    if prefix.is_empty() {
                        ",".to_string()
                    } else {
                        prefix
                    }
                };
                self.approval_mode = ApprovalMode::from_str(&string_field(&value, "approval_mode"));
                let raw_commands = value
                    .get("commands")
                    .and_then(Value::as_array)
                    .map(|items| {
                        items
                            .iter()
                            .filter_map(Value::as_str)
                            .map(str::to_string)
                            .collect::<Vec<_>>()
                    })
                    .unwrap_or_default();
                self.commands = parse_commands(&raw_commands, &self.command_prefix);
                self.tools = value
                    .get("tools")
                    .and_then(Value::as_array)
                    .map(|items| items.iter().map(parse_tool).collect())
                    .unwrap_or_default();
                self.skills = value
                    .get("skills")
                    .and_then(Value::as_array)
                    .map(|items| items.iter().map(parse_skill).collect())
                    .unwrap_or_default();
            }
            "models" => {
                self.models_requested = true;
                if let Some(current) = value.get("current").and_then(Value::as_str) {
                    if !current.is_empty() {
                        self.model = current.to_string();
                    }
                }
                self.model_choices = value
                    .get("models")
                    .and_then(Value::as_array)
                    .map(|items| {
                        items
                            .iter()
                            .map(|choice| ModelChoice {
                                id: string_field(choice, "id"),
                                name: {
                                    let name = string_field(choice, "name");
                                    if name.is_empty() {
                                        string_field(choice, "id")
                                    } else {
                                        name
                                    }
                                },
                            })
                            .collect()
                    })
                    .unwrap_or_default();
            }
            "settings" => {
                self.settings_requested = true;
                if let Some(values) = value.get("values").and_then(Value::as_object) {
                    self.settings_values = values.clone();
                }
                self.settings_overridden = value
                    .get("overridden")
                    .and_then(Value::as_array)
                    .map(|items| {
                        items
                            .iter()
                            .filter_map(Value::as_str)
                            .map(str::to_string)
                            .collect()
                    })
                    .unwrap_or_default();
                self.settings_config_file = string_field(&value, "config_file");
                self.settings_home = string_field(&value, "home");
                // The snapshot carries the live prefix, so a change made on
                // another surface (or a previous launch) is reflected here too.
                if let Some(prefix) = value
                    .get("values")
                    .and_then(|values| values.get("command_prefix"))
                    .and_then(Value::as_str)
                {
                    if !prefix.is_empty() && prefix != self.command_prefix {
                        let raw: Vec<String> = self
                            .commands
                            .iter()
                            .map(|command| format!("{prefix}{}", command.name))
                            .collect();
                        self.command_prefix = prefix.to_string();
                        self.commands = parse_commands(&raw, &self.command_prefix);
                    }
                }
                // A fresh snapshot replaces local switch edits, and reseeds any
                // open field that is not the one being typed in.
                self.settings_toggles.clear();
            }
            "generating" => {
                let active = value
                    .get("active")
                    .and_then(Value::as_bool)
                    .unwrap_or(false);
                self.set_generating(active, cx);
            }
            "stream" => self.handle_stream(&value, cx),
            "approval" => self.handle_approval(&value, cx),
            "approval_resolved" => self.handle_approval_resolved(&value, cx),
            "approval_mode" => {
                self.approval_mode = ApprovalMode::from_str(&string_field(&value, "mode"));
            }
            "outbound" if value.get("kind").and_then(Value::as_str) == Some("error") => {
                self.open = Open::None;
                self.push_step(Step::Error(string_field(&value, "content")), cx);
            }
            _ => {}
        }
        cx.notify();
    }

    fn handle_stream(&mut self, value: &Value, cx: &mut Context<Self>) {
        let kind = value
            .get("kind")
            .and_then(Value::as_str)
            .unwrap_or_default();
        let data = value.get("data").cloned().unwrap_or(Value::Null);
        match kind {
            "text" => {
                if let Some(delta) = data.get("delta").and_then(Value::as_str) {
                    self.append_delta(Open::Text, delta, cx);
                }
            }
            "reasoning" => {
                if let Some(delta) = data.get("delta").and_then(Value::as_str) {
                    self.append_delta(Open::Reasoning, delta, cx);
                }
            }
            "tool_call" => {
                self.open = Open::None;
                for call in array_field(&data, "tool_calls") {
                    let name = call
                        .get("name")
                        .or_else(|| call.get("function").and_then(|f| f.get("name")))
                        .and_then(Value::as_str)
                        .unwrap_or("tool")
                        .to_string();
                    let detail = call
                        .get("arguments")
                        .or_else(|| call.get("function").and_then(|f| f.get("arguments")))
                        .map(pretty_arg)
                        .unwrap_or_default();
                    self.push_step(
                        Step::Tool {
                            name,
                            detail,
                            state: Status::Running,
                            approval_id: None,
                            result: None,
                        },
                        cx,
                    );
                }
            }
            "tool_result" => {
                self.open = Open::None;
                for result in array_field(&data, "tool_results") {
                    let detail = pretty(result);
                    let failed = result_looks_failed(result);
                    self.complete_next_tool(&detail, failed, cx);
                }
            }
            "usage" => {
                let usage = format_usage(&data);
                if !usage.is_empty() {
                    self.push_step(Step::Usage(usage), cx);
                }
            }
            "error" => {
                self.open = Open::None;
                let message = data
                    .get("message")
                    .and_then(Value::as_str)
                    .map(str::to_string)
                    .unwrap_or_else(|| pretty(&data));
                self.push_step(Step::Error(message), cx);
            }
            "final" => self.open = Open::None,
            _ => {}
        }
    }

    /// Add a step to the answer being written.
    fn push_step(&mut self, step: Step, cx: &mut Context<Self>) {
        let existed = self.last_assistant().is_some();
        let index = {
            self.assistant().push(step);
            self.entries.len() - 1
        };
        if existed {
            self.remeasure(index, cx);
        } else {
            self.grow(1, cx);
        }
    }

    /// Attach the next tool result to the oldest tool call still waiting.
    fn complete_next_tool(&mut self, detail: &str, failed: bool, cx: &mut Context<Self>) {
        let found = self
            .entries
            .iter()
            .enumerate()
            .find_map(|(turn, entry)| match &entry.turn {
                Turn::Assistant(steps) => steps
                    .iter()
                    .position(Step::is_open_tool)
                    .map(|step| (turn, step)),
                Turn::User { .. } => None,
            });
        match found {
            Some((turn, step)) => {
                if let Turn::Assistant(steps) = &mut self.entries[turn].turn {
                    if let Step::Tool { state, result, .. } = &mut steps[step] {
                        // A denied card keeps its verdict; only the note is
                        // attached.
                        if *state != Status::Denied {
                            *state = if failed { Status::Failed } else { Status::Done };
                        }
                        *result = Some(detail.to_string());
                    }
                }
                self.remeasure(turn, cx);
            }
            None => self.push_step(Step::Result(detail.to_string()), cx),
        }
    }

    fn handle_approval(&mut self, value: &Value, cx: &mut Context<Self>) {
        let approval_id = string_field(value, "approval_id");
        let tool = string_field(value, "tool");
        let arguments = value.get("arguments").cloned().unwrap_or(Value::Null);
        let detail = pretty_arg(&arguments);

        let found = self
            .entries
            .iter()
            .enumerate()
            .find_map(|(turn, entry)| match &entry.turn {
                Turn::Assistant(steps) => steps
                    .iter()
                    .position(|step| {
                        matches!(step, Step::Tool { name, state, approval_id, .. }
                            if name == &tool
                                && *state == Status::Running
                                && approval_id.is_none())
                    })
                    .map(|step| (turn, step)),
                Turn::User { .. } => None,
            });

        match found {
            Some((turn, step)) => {
                if let Turn::Assistant(steps) = &mut self.entries[turn].turn {
                    if let Step::Tool {
                        state,
                        approval_id: slot,
                        detail: args,
                        ..
                    } = &mut steps[step]
                    {
                        *state = Status::Awaiting;
                        *slot = Some(approval_id);
                        if args.is_empty() {
                            *args = detail;
                        }
                    }
                }
                self.remeasure(turn, cx);
            }
            None => self.push_step(
                Step::Tool {
                    name: tool,
                    detail,
                    state: Status::Awaiting,
                    approval_id: Some(approval_id),
                    result: None,
                },
                cx,
            ),
        }
        self.grow(0, cx);
    }

    fn handle_approval_resolved(&mut self, value: &Value, cx: &mut Context<Self>) {
        let approval_id = string_field(value, "approval_id");
        let allow = value.get("allow").and_then(Value::as_bool).unwrap_or(false);
        let found = self
            .entries
            .iter()
            .enumerate()
            .find_map(|(turn, entry)| match &entry.turn {
                Turn::Assistant(steps) => steps
                    .iter()
                    .position(|step| {
                        matches!(step, Step::Tool { approval_id: slot, .. }
                            if slot.as_deref() == Some(approval_id.as_str()))
                    })
                    .map(|step| (turn, step)),
                Turn::User { .. } => None,
            });
        if let Some((turn, step)) = found {
            if let Turn::Assistant(steps) = &mut self.entries[turn].turn {
                if let Step::Tool { state, .. } = &mut steps[step] {
                    *state = if allow {
                        Status::Allowed
                    } else {
                        Status::Denied
                    };
                }
            }
            self.remeasure(turn, cx);
        }
        cx.notify();
    }

    /// Append a token to the open step, or open a new one for it.
    fn append_delta(&mut self, kind: Open, delta: &str, cx: &mut Context<Self>) {
        let continued = if self.open == kind {
            self.entries
                .last_mut()
                .and_then(|entry| match &mut entry.turn {
                    Turn::Assistant(steps) => steps.last_mut().and_then(Step::text_mut),
                    Turn::User { .. } => None,
                })
                .map(|text| text.push_str(delta))
                .is_some()
        } else {
            false
        };

        if continued {
            let last = self.entries.len().saturating_sub(1);
            self.remeasure(last, cx);
            return;
        }

        self.push_step(
            match kind {
                Open::Reasoning => Step::Reasoning(delta.to_string()),
                _ => Step::Text(delta.to_string()),
            },
            cx,
        );
        self.open = kind;
    }
}

impl Render for DesktopApp {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let main = if self.view.is_conversation() {
            self.transcript(cx)
        } else {
            self.settings_page(window, cx)
        };

        v_flex()
            .size_full()
            .bg(theme::canvas())
            .text_color(theme::ink())
            .key_context(crate::BUB_CONTEXT)
            .on_action(cx.listener(|this, _: &crate::OpenSettings, _window, cx| {
                this.open_settings(cx);
            }))
            .on_action(cx.listener(|this, _: &crate::CloseSettings, _window, cx| {
                this.close_settings(cx);
            }))
            .child(self.titlebar())
            // One rail, one main area. The rail is navigation and stays put;
            // what it points at is the only thing that changes.
            .child(
                h_flex()
                    .size_full()
                    .min_h_0()
                    .child(self.sidebar(cx))
                    .child(v_flex().flex_1().h_full().min_w_0().min_h_0().child(main)),
            )
    }
}
