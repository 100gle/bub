//! The application state and the protocol it speaks.
//!
//! [`DesktopApp`] owns the transcript, the activity trace, and the capability
//! lists, and it is the only thing that writes to the bridge. The view code in
//! [`crate::ui`] reads these fields and calls back into the methods here; no
//! rendering decision leaks into this module beyond the top-level composition
//! in [`Render`].

use std::collections::HashSet;
use std::io::Write;
use std::process::Child;
use std::process::ChildStdin;

use gpui_kit::assets::IconName;
use gpui_kit::component::{
    input::{InputEvent, TextareaState},
    message_scroller::MessageScrollerState,
    resizable::{h_resizable, resizable_panel},
    v_flex,
};
use gpui_kit::{
    App, AppContext as _, Context, Entity, InteractiveElement as _, IntoElement,
    ParentElement as _, Render, Styled as _, Subscription, Window,
};

use serde_json::{json, Value};

use crate::bridge;
use crate::model::{
    array_field, format_usage, parse_skill, parse_tool, pretty, pretty_arg, preview,
    result_looks_failed, string_field, ActivityEntry, ApprovalMode, Entry, InspectorTab, Item,
    Open, SkillInfo, Status, ToolInfo,
};
use crate::theme;

pub(crate) struct DesktopApp {
    pub(crate) entries: Vec<Entry>,
    /// Ids whose disclosure state has been flipped away from the block default.
    pub(crate) toggled: HashSet<u64>,
    pub(crate) activity: Vec<ActivityEntry>,
    pub(crate) scroller: Entity<MessageScrollerState>,
    pub(crate) input: Entity<TextareaState>,
    _input_sub: Subscription,
    pub(crate) model: String,
    pub(crate) workspace: String,
    pub(crate) protocol: u64,
    pub(crate) connected: bool,
    pub(crate) generating: bool,
    open: Open,
    next_id: u64,
    child: Option<Child>,
    stdin: Option<ChildStdin>,
    // Agent surfaces.
    pub(crate) inspector_open: bool,
    pub(crate) inspector_tab: InspectorTab,
    pub(crate) approval_mode: ApprovalMode,
    pub(crate) tools: Vec<ToolInfo>,
    pub(crate) skills: Vec<SkillInfo>,
    pub(crate) commands: Vec<String>,
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
            activity: Vec::new(),
            scroller,
            input,
            _input_sub,
            model: String::new(),
            workspace: String::new(),
            protocol: 0,
            connected: false,
            generating: false,
            open: Open::None,
            next_id: 1,
            child: None,
            stdin: None,
            // The panel is a workspace tool, not part of the reading surface:
            // it starts closed so the first thing on screen is the agent.
            inspector_open: false,
            inspector_tab: InspectorTab::Activity,
            approval_mode: ApprovalMode::Auto,
            tools: Vec::new(),
            skills: Vec::new(),
            commands: Vec::new(),
        };

        match bridge::spawn() {
            Ok(process) => {
                app.stdin = Some(process.stdin);
                app.child = Some(process.child);
                let lines = process.lines;
                cx.spawn(async move |this, cx| {
                    while let Ok(line) = lines.recv().await {
                        if this.update(cx, |app, cx| app.handle_line(&line, cx)).is_err() {
                            break;
                        }
                    }
                })
                .detach();
            }
            Err(err) => app.push(
                Item::Error(format!(
                    "could not start the bub bridge: {err}\n\n\
                     Install the plugin (`uv pip install -e desktop/bridge`) or set BUB_DESKTOP_CMD."
                )),
                cx,
            ),
        }

        app
    }

    fn next_id(&mut self) -> u64 {
        let id = self.next_id;
        self.next_id += 1;
        id
    }

    pub(crate) fn toggle(&mut self, id: u64, cx: &mut Context<Self>) {
        if !self.toggled.remove(&id) {
            self.toggled.insert(id);
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
    fn push(&mut self, item: Item, cx: &mut Context<Self>) {
        let id = self.next_id();
        self.entries.push(Entry { id, item });
        self.grow(1, cx);
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

    fn push_activity(
        &mut self,
        icon: IconName,
        label: String,
        detail: String,
        status: Status,
        tool: Option<String>,
    ) {
        self.activity.push(ActivityEntry {
            icon,
            label,
            detail,
            status,
            tool,
        });
    }

    /// Find the first non-terminal activity row for `tool`.
    fn activity_for_tool(&mut self, tool: &str) -> Option<usize> {
        self.activity
            .iter()
            .position(|entry| entry.tool.as_deref() == Some(tool) && !entry.status.is_terminal())
    }

    pub(crate) fn send(
        &mut self,
        input: &Entity<TextareaState>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let text = input.read(cx).value().trim().to_string();
        if text.is_empty() || self.generating {
            return;
        }
        input.update(cx, |state, cx| state.set_value("", window, cx));
        self.begin(text, cx);
    }

    /// Start a turn from `text`: show it, hand it to the bridge, and mark the
    /// run active. Separate from [`Self::send`] so that the composer's textarea
    /// is the only thing `send` has to clear.
    fn begin(&mut self, text: String, cx: &mut Context<Self>) {
        self.push(Item::User(text.clone()), cx);
        self.activity.clear();
        self.open = Open::None;
        self.generating = true;
        let id = self.next_id();
        self.write(json!({ "id": id, "type": "send", "text": text }));
        cx.notify();
    }

    /// Start a turn without a composer. Only the `shot` feature uses this, to
    /// photograph a transcript without a keyboard.
    #[cfg(feature = "shot")]
    pub(crate) fn prompt_directly(&mut self, text: &str, cx: &mut Context<Self>) {
        self.begin(text.to_string(), cx);
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
        self.generating = false;
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
        !self.input.read(cx).value().trim().is_empty()
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
                self.model = string_field(&value, "model");
                self.workspace = string_field(&value, "workspace");
                self.protocol = value.get("protocol").and_then(Value::as_u64).unwrap_or(0);
                self.approval_mode = ApprovalMode::from_str(&string_field(&value, "approval_mode"));
                self.commands = value
                    .get("commands")
                    .and_then(Value::as_array)
                    .map(|items| {
                        items
                            .iter()
                            .filter_map(Value::as_str)
                            .map(str::to_string)
                            .collect()
                    })
                    .unwrap_or_default();
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
            "generating" => {
                self.generating = value
                    .get("active")
                    .and_then(Value::as_bool)
                    .unwrap_or(false);
                if !self.generating {
                    self.open = Open::None;
                }
            }
            "stream" => self.handle_stream(&value, cx),
            "approval" => self.handle_approval(&value, cx),
            "approval_resolved" => self.handle_approval_resolved(&value, cx),
            "approval_mode" => {
                self.approval_mode = ApprovalMode::from_str(&string_field(&value, "mode"));
            }
            "outbound" if value.get("kind").and_then(Value::as_str) == Some("error") => {
                self.open = Open::None;
                let message = string_field(&value, "content");
                self.push(Item::Error(message.clone()), cx);
                self.push_activity(
                    IconName::CircleAlert,
                    "Error".into(),
                    message,
                    Status::Failed,
                    None,
                );
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
                    self.append_delta(Open::Assistant, delta, cx);
                }
            }
            "reasoning" => {
                if let Some(delta) = data.get("delta").and_then(Value::as_str) {
                    // Reasoning is already on screen as a transcript row, and
                    // the panel is for actions, so it adds nothing here.
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
                    self.push_activity(
                        IconName::Wrench,
                        name.clone(),
                        preview(&detail),
                        Status::Running,
                        Some(name.clone()),
                    );
                    self.push(
                        Item::ToolCall {
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
                    self.push(Item::Usage(usage), cx);
                }
            }
            "error" => {
                self.open = Open::None;
                let message = data
                    .get("message")
                    .and_then(Value::as_str)
                    .map(str::to_string)
                    .unwrap_or_else(|| pretty(&data));
                self.push(Item::Error(message.clone()), cx);
                self.push_activity(
                    IconName::CircleAlert,
                    "Error".into(),
                    message,
                    Status::Failed,
                    None,
                );
            }
            "final" => self.open = Open::None,
            _ => {}
        }
    }

    /// Attach the next tool result to the oldest running tool card.
    fn complete_next_tool(&mut self, detail: &str, failed: bool, cx: &mut Context<Self>) {
        let index = self.entries.iter().position(|entry| {
            matches!(&entry.item, Item::ToolCall { state, result, .. }
                if matches!(
                    state,
                    Status::Running | Status::Awaiting | Status::Allowed | Status::Denied
                ) && result.is_none())
        });
        match index {
            Some(index) => {
                let name = if let Item::ToolCall {
                    name,
                    state,
                    result,
                    ..
                } = &mut self.entries[index].item
                {
                    // A denied card keeps its verdict; only the note is attached.
                    if *state != Status::Denied {
                        *state = if failed { Status::Failed } else { Status::Done };
                    }
                    *result = Some(detail.to_string());
                    Some(name.clone())
                } else {
                    None
                };
                if let Some(name) = name {
                    let status = if failed { Status::Failed } else { Status::Done };
                    if let Some(row) = self.activity_for_tool(&name) {
                        if self.activity[row].status != Status::Denied {
                            self.activity[row].status = status;
                        }
                    }
                }
            }
            None => self.push(
                Item::ToolResult {
                    detail: detail.to_string(),
                },
                cx,
            ),
        }
    }

    fn handle_approval(&mut self, value: &Value, cx: &mut Context<Self>) {
        let approval_id = string_field(value, "approval_id");
        let tool = string_field(value, "tool");
        let arguments = value.get("arguments").cloned().unwrap_or(Value::Null);
        let detail = pretty_arg(&arguments);

        let index = self.entries.iter().position(|entry| {
            matches!(&entry.item, Item::ToolCall { name, state, approval_id, .. }
                if name == &tool && *state == Status::Running && approval_id.is_none())
        });

        match index {
            Some(index) => {
                if let Item::ToolCall {
                    state,
                    approval_id: slot,
                    detail: args,
                    ..
                } = &mut self.entries[index].item
                {
                    *state = Status::Awaiting;
                    *slot = Some(approval_id);
                    if args.is_empty() {
                        *args = detail;
                    }
                }
                if let Some(row) = self.activity_for_tool(&tool) {
                    self.activity[row].status = Status::Awaiting;
                }
            }
            None => self.push(
                Item::ToolCall {
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
        let index = self.entries.iter().position(|entry| {
            matches!(&entry.item, Item::ToolCall { approval_id: slot, .. }
                if slot.as_deref() == Some(approval_id.as_str()))
        });
        if let Some(index) = index {
            let name = if let Item::ToolCall { name, state, .. } = &mut self.entries[index].item {
                *state = if allow {
                    Status::Allowed
                } else {
                    Status::Denied
                };
                Some(name.clone())
            } else {
                None
            };
            if let Some(name) = name {
                if let Some(row) = self.activity_for_tool(&name) {
                    self.activity[row].status = if allow {
                        Status::Allowed
                    } else {
                        Status::Denied
                    };
                }
            }
        }
        cx.notify();
    }

    /// Append a token to the open block, or start a new row for it.
    fn append_delta(&mut self, kind: Open, delta: &str, cx: &mut Context<Self>) {
        if self.open == kind {
            if let Some(text) = self
                .entries
                .last_mut()
                .and_then(|entry| entry.item.text_mut())
            {
                text.push_str(delta);
            }
            let last = self.entries.len().saturating_sub(1);
            self.scroller.update(cx, |state, cx| {
                let _ = state.remeasure_items(last..last + 1, cx);
            });
            return;
        }

        let item = match kind {
            Open::Reasoning => Item::Reasoning(delta.to_string()),
            _ => Item::Assistant(delta.to_string()),
        };
        self.push(item, cx);
        self.open = kind;
    }
}

impl Render for DesktopApp {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        v_flex()
            .size_full()
            .bg(theme::canvas())
            .text_color(theme::ink())
            .key_context(crate::BUB_CONTEXT)
            .on_action(
                cx.listener(|this, _: &crate::ToggleInspector, _window, cx| {
                    this.toggle_inspector(cx);
                }),
            )
            .child(self.titlebar(cx))
            .child(
                h_resizable("bub-shell").child(self.transcript(cx)).child(
                    resizable_panel()
                        .size(theme::PANEL_WIDTH)
                        .size_range(theme::PANEL_MIN..theme::PANEL_MAX)
                        .visible(self.inspector_open)
                        .child(self.inspector(cx)),
                ),
            )
    }
}

impl DesktopApp {
    pub(crate) fn toggle_inspector(&mut self, cx: &mut Context<Self>) {
        self.inspector_open = !self.inspector_open;
        cx.notify();
    }
}
