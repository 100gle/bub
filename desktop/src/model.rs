//! The vocabulary the bridge and the view share.
//!
//! Everything here is plain data plus pure helpers: no GPUI element code and no
//! process handling. Keeping it separate means the protocol shape can be read
//! and tested without a window.

use gpui_kit::assets::IconName;
use gpui_kit::Hsla;

use serde_json::Value;

use crate::theme;

/// One step inside an assistant turn.
///
/// A turn is a list of these because that is what the model actually produces:
/// a thought, a tool call, its result, more prose. They are steps of one answer
/// rather than separate messages, which is why they live inside one block.
#[derive(Clone)]
pub(crate) enum Step {
    Reasoning(String),
    Text(String),
    Tool {
        name: String,
        detail: String,
        state: Status,
        approval_id: Option<String>,
        result: Option<String>,
    },
    /// A tool result that arrived with no card waiting for it.
    Result(String),
    Usage(String),
    Error(String),
}

impl Step {
    /// The text slot a streamed delta can be appended to, if this step has one.
    pub(crate) fn text_mut(&mut self) -> Option<&mut String> {
        match self {
            Step::Reasoning(text) | Step::Text(text) => Some(text),
            _ => None,
        }
    }

    /// Whether this step is a tool call still waiting on something.
    pub(crate) fn is_open_tool(&self) -> bool {
        matches!(
            self,
            Step::Tool {
                state,
                result: None,
                ..
            } if matches!(
                state,
                Status::Running | Status::Awaiting | Status::Allowed | Status::Denied
            )
        )
    }
}

/// One block in the transcript.
#[derive(Clone)]
pub(crate) enum Turn {
    User {
        text: String,
        attachments: Vec<String>,
    },
    /// Everything the model produced in one turn, in the order it produced it.
    ///
    /// It is one block — and it is created the moment the message is sent, while
    /// it is still empty — because it is one message. The reader asked a
    /// question; this is the answer, however many steps it took to write, and
    /// the wait belongs to it rather than floating somewhere else on screen.
    Assistant(Vec<Step>),
}

/// A transcript row plus a stable id, so its blocks can be collapsed.
#[derive(Clone)]
pub(crate) struct Entry {
    pub(crate) id: u64,
    pub(crate) turn: Turn,
}

/// Identifies a collapsible block: the turn, and which step inside it.
///
/// Steps are only ever appended, so this key is stable for the life of a turn —
/// which is what lets a disclosure stay open while the answer grows under it.
pub(crate) type BlockKey = (u64, usize);

/// Which streamed block is currently open for delta appends.
#[derive(Clone, Copy, PartialEq)]
pub(crate) enum Open {
    None,
    Text,
    Reasoning,
}

/// Lifecycle of one agent action.
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

/// One entry in the capability list.
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

/// One selectable model, as the runtime reports it.
#[derive(Clone)]
pub(crate) struct ModelChoice {
    pub(crate) id: String,
    pub(crate) name: String,
}

/// What a queued attachment is, so it can be labelled and sent with the right
/// media type. Kept coarse: the runtime only distinguishes four kinds.
#[derive(Clone, Copy, PartialEq)]
pub(crate) enum AttachmentKind {
    Image,
    Audio,
    Video,
    Document,
}

impl AttachmentKind {
    /// Infer the kind from a file name, the way the bridge infers its media
    /// type. Unknown extensions are documents.
    pub(crate) fn of(path: &str) -> Self {
        let ext = std::path::Path::new(path)
            .extension()
            .and_then(|ext| ext.to_str())
            .unwrap_or_default()
            .to_ascii_lowercase();
        match ext.as_str() {
            "png" | "jpg" | "jpeg" | "gif" | "webp" | "bmp" | "tiff" | "heic" | "svg" => {
                AttachmentKind::Image
            }
            "mp3" | "wav" | "m4a" | "ogg" | "flac" | "aiff" | "webm" => AttachmentKind::Audio,
            "mp4" | "mov" | "mkv" | "avi" => AttachmentKind::Video,
            _ => AttachmentKind::Document,
        }
    }

    pub(crate) fn icon(self) -> IconName {
        match self {
            AttachmentKind::Image => IconName::Image,
            AttachmentKind::Audio => IconName::Music,
            AttachmentKind::Video => IconName::FilePlay,
            AttachmentKind::Document => IconName::FileText,
        }
    }

    /// The media type the runtime expects.
    pub(crate) fn media_type(self) -> &'static str {
        match self {
            AttachmentKind::Image => "image",
            AttachmentKind::Audio => "audio",
            AttachmentKind::Video => "video",
            AttachmentKind::Document => "document",
        }
    }
}

/// A file queued in the composer, awaiting the next message.
#[derive(Clone)]
pub(crate) struct Attachment {
    pub(crate) path: String,
    pub(crate) name: String,
    pub(crate) kind: AttachmentKind,
    pub(crate) size: u64,
}

impl Attachment {
    pub(crate) fn from_path(path: &std::path::Path) -> Self {
        let name = path
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap_or("file")
            .to_string();
        let path_str = path.to_string_lossy().to_string();
        let size = std::fs::metadata(path).map(|meta| meta.len()).unwrap_or(0);
        Attachment {
            kind: AttachmentKind::of(&path_str),
            path: path_str,
            name,
            size,
        }
    }
}

/// Collapse a byte count into the shortest honest form.
pub(crate) fn format_size(bytes: u64) -> String {
    const KB: f64 = 1024.;
    const MB: f64 = KB * 1024.;
    const GB: f64 = MB * 1024.;
    let bytes = bytes as f64;
    if bytes >= GB {
        format!("{:.1} GB", bytes / GB)
    } else if bytes >= MB {
        format!("{:.1} MB", bytes / MB)
    } else if bytes >= KB {
        format!("{:.0} KB", bytes / KB)
    } else {
        format!("{} B", bytes as u64)
    }
}

/// A command the runtime advertises, split from its prefix.
#[derive(Clone)]
pub(crate) struct Command {
    /// The bare name, e.g. `model`.
    pub(crate) name: String,
    /// The full trigger, e.g. `,model`.
    pub(crate) trigger: String,
}

/// Split the runtime's command list (`[",help", ",model"]`) into a prefix and
/// bare names. Tolerates entries that do not carry the declared prefix.
pub(crate) fn parse_commands(commands: &[String], prefix: &str) -> Vec<Command> {
    commands
        .iter()
        .map(|command| {
            let name = command
                .strip_prefix(prefix)
                .unwrap_or(command)
                .trim()
                .to_string();
            Command {
                trigger: command.clone(),
                name,
            }
        })
        .collect()
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

/// Which page of the settings view is showing.
#[derive(Clone, Copy, Debug, PartialEq)]
pub(crate) enum SettingsSection {
    General,
    Model,
    Capabilities,
    Channels,
    About,
}

impl SettingsSection {
    /// Every page, in navigation order.
    pub(crate) const ALL: [SettingsSection; 5] = [
        SettingsSection::General,
        SettingsSection::Model,
        SettingsSection::Capabilities,
        SettingsSection::Channels,
        SettingsSection::About,
    ];

    pub(crate) fn label(self) -> &'static str {
        match self {
            SettingsSection::General => "General",
            SettingsSection::Model => "Model",
            SettingsSection::Capabilities => "Capabilities",
            SettingsSection::Channels => "Channels",
            SettingsSection::About => "About",
        }
    }

    pub(crate) fn icon(self) -> IconName {
        match self {
            SettingsSection::General => IconName::SlidersHorizontal,
            SettingsSection::Model => IconName::Cpu,
            SettingsSection::Capabilities => IconName::Blocks,
            SettingsSection::Channels => IconName::Globe,
            SettingsSection::About => IconName::Info,
        }
    }
}

/// Which surface the window is showing.
///
/// The conversation and the settings pages are peers, not a page and a modal
/// over it: moving between them is navigation in one direction or the other,
/// which is why there is nothing to close. Which settings page is a separate
/// fact — [`SettingsSection`] — so leaving settings and coming back returns to
/// where you were rather than to the first page.
#[derive(Clone, Copy, Debug, PartialEq)]
pub(crate) enum View {
    Conversation,
    Settings,
}

impl View {
    pub(crate) fn is_conversation(self) -> bool {
        matches!(self, View::Conversation)
    }
}

/// Whether a block is expanded, given its default and the user's overrides.
pub(crate) fn disclosure(
    toggled: &std::collections::HashSet<BlockKey>,
    key: BlockKey,
    default_open: bool,
) -> bool {
    if toggled.contains(&key) {
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
