//! The vocabulary the bridge and the view share.
//!
//! Everything here is plain data plus pure helpers: no GPUI element code and no
//! process handling. Keeping it separate means the protocol shape can be read
//! and tested without a window.

use gpui_kit::assets::IconName;
use gpui_kit::Hsla;

use serde_json::Value;

use crate::theme;

/// One block in the transcript.
#[derive(Clone)]
pub(crate) enum Item {
    User(String),
    Assistant(String),
    Reasoning(String),
    ToolCall {
        name: String,
        detail: String,
        state: Status,
        approval_id: Option<String>,
        result: Option<String>,
    },
    ToolResult {
        detail: String,
    },
    Usage(String),
    Error(String),
}

impl Item {
    /// The text slot a streamed delta can be appended to, if this block has one.
    pub(crate) fn text_mut(&mut self) -> Option<&mut String> {
        match self {
            Item::Assistant(text) | Item::Reasoning(text) => Some(text),
            _ => None,
        }
    }
}

/// A transcript row plus a stable id, so its block can be collapsed.
#[derive(Clone)]
pub(crate) struct Entry {
    pub(crate) id: u64,
    pub(crate) item: Item,
}

/// Which streamed block is currently open for delta appends.
#[derive(Clone, Copy, PartialEq)]
pub(crate) enum Open {
    None,
    Assistant,
    Reasoning,
}

/// Lifecycle of one agent action, shared by tool cards and the activity trace.
#[derive(Clone, Copy, PartialEq)]
pub(crate) enum Status {
    Running,
    Awaiting,
    Allowed,
    Done,
    Denied,
    Failed,
}

impl Status {
    pub(crate) fn is_terminal(self) -> bool {
        matches!(self, Status::Done | Status::Denied | Status::Failed)
    }

    /// The sentence-case label shown next to a tool card.
    pub(crate) fn label(self) -> &'static str {
        match self {
            Status::Running => "running",
            Status::Awaiting => "needs approval",
            Status::Allowed => "allowed",
            Status::Done => "done",
            Status::Denied => "denied",
            Status::Failed => "failed",
        }
    }

    pub(crate) fn color(self) -> Hsla {
        match self {
            Status::Awaiting => theme::warning(),
            Status::Denied | Status::Failed => theme::danger(),
            Status::Done | Status::Allowed => theme::success(),
            Status::Running => theme::ink_tertiary(),
        }
    }
}

/// One row in the activity trace.
#[derive(Clone)]
pub(crate) struct ActivityEntry {
    pub(crate) icon: IconName,
    pub(crate) label: String,
    pub(crate) detail: String,
    pub(crate) status: Status,
    pub(crate) tool: Option<String>,
}

/// One entry in the capability panel.
#[derive(Clone)]
pub(crate) struct ToolInfo {
    pub(crate) name: String,
    pub(crate) description: String,
    pub(crate) exposure: String,
    pub(crate) deferred: bool,
}

#[derive(Clone)]
pub(crate) struct SkillInfo {
    pub(crate) name: String,
    pub(crate) description: String,
    pub(crate) source: String,
}

/// Whether model tool calls run straight away or wait for a verdict.
#[derive(Clone, Copy, PartialEq)]
pub(crate) enum ApprovalMode {
    Auto,
    Ask,
}

impl ApprovalMode {
    pub(crate) fn as_str(self) -> &'static str {
        match self {
            ApprovalMode::Auto => "auto",
            ApprovalMode::Ask => "ask",
        }
    }

    pub(crate) fn from_str(value: &str) -> Self {
        if value == "ask" {
            ApprovalMode::Ask
        } else {
            ApprovalMode::Auto
        }
    }
}

/// Which half of the agent panel is showing.
#[derive(Clone, Copy, PartialEq)]
pub(crate) enum InspectorTab {
    Activity,
    Tools,
}

/// Whether a block is expanded, given its default and the user's overrides.
pub(crate) fn disclosure(
    toggled: &std::collections::HashSet<u64>,
    id: u64,
    default_open: bool,
) -> bool {
    if toggled.contains(&id) {
        !default_open
    } else {
        default_open
    }
}

/// First non-empty line of a payload, trimmed for use as a one-line preview.
pub(crate) fn first_line(text: &str) -> String {
    let line = text
        .lines()
        .find(|line| !line.trim().is_empty())
        .unwrap_or("");
    shorten(line.to_string())
}

/// A one-line preview of a tool argument or result.
///
/// These arrive pretty-printed, so their first line is usually a lone `{` —
/// which is what a collapsed card would then show. Skip the punctuation, and
/// unwrap the JSON quoting, so the row reads `command: rg -n 'deploy'` instead
/// of nothing at all.
pub(crate) fn preview(text: &str) -> String {
    let line = text
        .lines()
        .map(str::trim)
        .find(|line| {
            !line.is_empty()
                && line
                    .chars()
                    .any(|ch| !ch.is_whitespace() && !"{}[],".contains(ch))
        })
        .unwrap_or("");

    let line = line.trim_end_matches(',');
    let line = match line
        .strip_prefix('"')
        .and_then(|rest| rest.split_once("\": "))
    {
        Some((key, value)) => format!("{key}: {}", value.trim_matches('"')),
        None => line.to_string(),
    };
    shorten(line)
}

/// Cut a preview to the width a single row can show.
fn shorten(mut text: String) -> String {
    const MAX: usize = 120;
    if text.chars().count() > MAX {
        text = text.chars().take(MAX).collect();
        text.push('…');
    }
    text
}

/// Collapse a token count into the shortest form that stays honest.
pub(crate) fn format_usage(data: &Value) -> String {
    let mut parts: Vec<String> = Vec::new();
    if let Some(seconds) = data.get("elapsed_seconds").and_then(Value::as_f64) {
        parts.push(format!("{seconds:.1}s"));
    }
    if let Some(total) = data
        .get("usage")
        .and_then(|usage| usage.get("total_tokens"))
        .and_then(Value::as_u64)
    {
        parts.push(format!("{total} tokens"));
    }
    parts.join(" · ")
}

pub(crate) fn string_field(value: &Value, key: &str) -> String {
    value
        .get(key)
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_string()
}

pub(crate) fn array_field<'a>(value: &'a Value, key: &str) -> &'a [Value] {
    value
        .get(key)
        .and_then(Value::as_array)
        .map(Vec::as_slice)
        .unwrap_or(&[])
}

pub(crate) fn parse_tool(value: &Value) -> ToolInfo {
    ToolInfo {
        name: string_field(value, "name"),
        description: string_field(value, "description"),
        exposure: string_field(value, "exposure"),
        deferred: value
            .get("deferred")
            .and_then(Value::as_bool)
            .unwrap_or(false),
    }
}

pub(crate) fn parse_skill(value: &Value) -> SkillInfo {
    SkillInfo {
        name: string_field(value, "name"),
        description: string_field(value, "description"),
        source: string_field(value, "source"),
    }
}

/// A tool result that reports a Bub error should read as failed, not done.
///
/// The executor serializes a failed call as `BubError.as_dict()` — a
/// `{ "kind": <error kind>, "message": ... }` object. Successful calls are
/// rendered to text, so they never take this shape.
pub(crate) fn result_looks_failed(value: &Value) -> bool {
    const ERROR_KINDS: [&str; 7] = [
        "invalid_input",
        "config",
        "provider",
        "tool",
        "temporary",
        "not_found",
        "unknown",
    ];
    let Value::Object(map) = value else {
        return false;
    };
    if map.get("error").is_some_and(|error| !error.is_null()) {
        return true;
    }
    map.contains_key("message")
        && map
            .get("kind")
            .and_then(Value::as_str)
            .is_some_and(|kind| ERROR_KINDS.contains(&kind))
}

pub(crate) fn pretty(value: &Value) -> String {
    match value {
        Value::String(text) => text.clone(),
        other => serde_json::to_string_pretty(other).unwrap_or_else(|_| other.to_string()),
    }
}

/// Pretty-print a tool argument, which may arrive as a JSON string.
pub(crate) fn pretty_arg(value: &Value) -> String {
    if let Value::String(text) = value {
        if let Ok(parsed) = serde_json::from_str::<Value>(text) {
            return pretty(&parsed);
        }
        return text.clone();
    }
    pretty(value)
}
