//! The settings view.
//!
//! A full-window page, opened from the title-bar gear or ⌘,. A rail on the left
//! names four pages; the reading column on the right holds groups of rows, one
//! control each. The fields are the settings Bub itself reads — the model and
//! its fallbacks, the command prefix, the channels — so this page is a front end
//! to the runtime's configuration rather than a second, private set of options.
//!
//! Writes go back through the bridge to Bub's config file. A key the environment
//! already overrides is marked, because writing it would be silently ignored.

use gpui_kit::assets::IconName;
use gpui_kit::component::{
    h_flex, input::Input, scroll::ScrollableElement as _, v_flex, ActiveTheme as _, Icon,
    Sizable as _, StyledExt as _,
};
use gpui_kit::prelude::FluentBuilder as _;
use gpui_kit::{
    div, px, AnyElement, App, Context, Div, InteractiveElement as _, IntoElement,
    ParentElement as _, StatefulInteractiveElement as _, Styled as _, Window,
};

use serde_json::Value;

use crate::app::DesktopApp;
use crate::model::{ApprovalMode, Command, SettingsSection};
use crate::theme;
use crate::ui::atoms;

/// How a settings row is edited.
#[derive(Clone, Copy, PartialEq)]
enum FieldKind {
    Text,
    Secret,
    Number,
    List,
}

/// The note under a page whose controls write to Bub's config file.
const SAVE_NOTE: &str =
    "Saved changes apply to the next launch; the model and command prefix take effect now.";

/// A row that edits one configuration key.
struct Field {
    key: &'static str,
    label: &'static str,
    help: &'static str,
    kind: FieldKind,
}

const fn field(
    key: &'static str,
    label: &'static str,
    help: &'static str,
    kind: FieldKind,
) -> Field {
    Field {
        key,
        label,
        help,
        kind,
    }
}

impl DesktopApp {
    /// The settings surface: a page toolbar, then the reading column.
    ///
    /// The navigation is the window's rail — see [`crate::ui::sidebar`] — so
    /// this is only the page itself. There is no close button, because there is
    /// nothing to close: the rail is how you leave, and leaving is going
    /// somewhere else rather than dismissing what is in front of you.
    pub(crate) fn settings_page(
        &mut self,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> AnyElement {
        v_flex()
            .size_full()
            .min_h_0()
            .child(self.settings_toolbar(cx))
            .child(
                v_flex()
                    .id("settings-body")
                    .flex_1()
                    .min_w_0()
                    .min_h_0()
                    .overflow_y_scrollbar()
                    .child(
                        div()
                            .w_full()
                            .max_w(theme::SETTINGS_MAX)
                            .mx_auto()
                            .px(px(32.))
                            .py(px(24.))
                            .child(self.settings_section_body(window, cx)),
                    ),
            )
            .into_any_element()
    }

    /// The page's own header: what this page is, and what it would write.
    fn settings_toolbar(&self, cx: &mut Context<Self>) -> AnyElement {
        let dirty = self.settings_dirty(cx);

        h_flex()
            .w_full()
            .flex_shrink_0()
            .items_center()
            .justify_between()
            .gap(px(12.))
            .px(px(32.))
            .py(px(14.))
            .border_b_1()
            .border_color(theme::hairline())
            .child(
                div()
                    .text_size(theme::TEXT_TITLE)
                    .font_semibold()
                    .text_color(theme::ink())
                    .child(self.settings_section.label()),
            )
            // Save and Discard appear together, and only once there is
            // something to save. A read-only page — the capabilities list, the
            // about page — has no Save button at all, rather than one that does
            // nothing.
            .when(dirty, |this| {
                this.child(
                    h_flex()
                        .flex_shrink_0()
                        .items_center()
                        .gap(px(8.))
                        .child(
                            div()
                                .id("settings-reset")
                                .px(px(10.))
                                .py(px(5.))
                                .rounded(px(9999.))
                                .cursor_pointer()
                                .text_size(theme::TEXT_UI)
                                .text_color(theme::ink_secondary())
                                .hover(|style| style.bg(theme::fill()))
                                .child("Discard")
                                .on_click(cx.listener(|this, _, window, cx| {
                                    this.reset_settings(window, cx);
                                })),
                        )
                        .child(atoms::pill_button(
                            cx,
                            "settings-save",
                            "Save",
                            atoms::Plate::Accent,
                            cx.listener(|this, _, _, cx| this.save_settings(cx)),
                        )),
                )
            })
            .into_any_element()
    }

    fn settings_section_body(&mut self, window: &mut Window, cx: &mut Context<Self>) -> AnyElement {
        match self.settings_section {
            SettingsSection::General => self.settings_general(window, cx),
            SettingsSection::Model => self.settings_model(window, cx),
            SettingsSection::Capabilities => self.settings_capabilities(cx),
            SettingsSection::Channels => self.settings_channels(window, cx),
            SettingsSection::About => self.settings_about(cx),
        }
    }

    /// What the runtime reported it can do here.
    ///
    /// This is the half of the deleted agent panel that a reader could not see
    /// anywhere else. It is read-only — the runtime discovers it, Bub does not
    /// take it as input — so it lives with the other facts rather than in the
    /// conversation, where it would be a wall of names between two messages.
    fn settings_capabilities(&mut self, cx: &mut Context<Self>) -> AnyElement {
        let tools: Vec<AnyElement> = self
            .tools
            .iter()
            .map(|tool| {
                capability_row(
                    exposure_icon(&tool.exposure),
                    &tool.name,
                    &tool.description,
                    tool.deferred.then_some("deferred"),
                )
                .into_any_element()
            })
            .collect();
        let skills: Vec<AnyElement> = self
            .skills
            .iter()
            .map(|skill| {
                // Where a skill came from is the thing that tells two skills of
                // the same name apart, so it rides on the row rather than being
                // dropped on the floor.
                capability_row(
                    IconName::Sparkles,
                    &skill.name,
                    &skill.description,
                    (!skill.source.is_empty()).then_some(skill.source.as_str()),
                )
                .into_any_element()
            })
            .collect();
        let commands = if self.commands.is_empty() {
            Vec::new()
        } else {
            vec![command_row(&self.commands, cx)]
        };

        self.page(
            "What Bub found it can do in this workspace. The runtime reports these when the window connects; nothing here is edited by hand.",
            vec![
                group("Tools", tools, "No tools reported."),
                group("Skills", skills, "No skills discovered."),
                group("Commands", commands, "No commands reported."),
            ],
            None,
        )
    }

    fn settings_general(&mut self, window: &mut Window, cx: &mut Context<Self>) -> AnyElement {
        let ask = self.approval_mode == ApprovalMode::Ask;

        let permission = self.switch_row(
            "permission",
            "Ask before tools run",
            "When on, every tool call waits for Allow or Deny before it runs.",
            ask,
            false,
            cx.listener(move |this, _, _, cx| {
                this.set_approval_mode(
                    if ask {
                        ApprovalMode::Auto
                    } else {
                        ApprovalMode::Ask
                    },
                    cx,
                );
            }),
        );

        let prefix = self.field_row(
            &field(
                "command_prefix",
                "Command prefix",
                "The character that starts a command. Bub's default is a comma.",
                FieldKind::Text,
            ),
            window,
            cx,
        );

        let channels = self.field_row(
            &field(
                "enabled_channels",
                "Enabled channels",
                "Comma-separated names, or “all” to run every channel but the CLI.",
                FieldKind::Text,
            ),
            window,
            cx,
        );

        let stream = self.switch_row(
            "stream_output",
            "Stream output to channels",
            "Send model output as it is produced instead of waiting for the turn to end.",
            self.settings_switch("stream_output"),
            true,
            cx.listener(|this, _, _, cx| {
                let next = !this.settings_switch("stream_output");
                this.set_settings_switch("stream_output", next, cx);
            }),
        );

        self.page(
            "How Bub behaves in this workspace and how commands are written.",
            vec![
                group("Permissions", vec![permission], ""),
                group("Commands", vec![prefix], ""),
                group("Channels", vec![channels, stream], ""),
            ],
            Some(SAVE_NOTE),
        )
    }

    fn settings_model(&mut self, window: &mut Window, cx: &mut Context<Self>) -> AnyElement {
        let model = self.field_row(
            &field(
                "model",
                "Default model",
                "provider:model, e.g. openai:gpt-4o.",
                FieldKind::Text,
            ),
            window,
            cx,
        );
        let fallbacks = self.field_row(
            &field(
                "fallback_models",
                "Fallback models",
                "Tried in order when the default is unavailable. Comma-separated.",
                FieldKind::List,
            ),
            window,
            cx,
        );
        let tokens = self.field_row(
            &field(
                "max_tokens",
                "Max tokens",
                "Ceiling on the model's response length.",
                FieldKind::Number,
            ),
            window,
            cx,
        );
        let steps = self.field_row(
            &field(
                "max_steps",
                "Max steps",
                "How many tool steps one turn may take. Empty means no limit.",
                FieldKind::Number,
            ),
            window,
            cx,
        );
        let timeout = self.field_row(
            &field(
                "model_timeout_seconds",
                "Model timeout",
                "Seconds before a request is abandoned. Empty means no limit.",
                FieldKind::Number,
            ),
            window,
            cx,
        );
        let key = self.field_row(
            &field(
                "api_key",
                "API key",
                "Sent to the provider. Stored in Bub's config file.",
                FieldKind::Secret,
            ),
            window,
            cx,
        );
        let base = self.field_row(
            &field(
                "api_base",
                "API base",
                "Override the provider endpoint.",
                FieldKind::Text,
            ),
            window,
            cx,
        );

        self.page(
            "The model this workspace runs on and the limits it runs under.",
            vec![
                group("Default", vec![model, fallbacks], ""),
                group("Limits", vec![tokens, steps, timeout], ""),
                group("Credentials", vec![key, base], ""),
            ],
            Some(SAVE_NOTE),
        )
    }

    fn settings_channels(&mut self, window: &mut Window, cx: &mut Context<Self>) -> AnyElement {
        let token = self.field_row(
            &field(
                "telegram.token",
                "Bot token",
                "The token BotFather issued for this bot.",
                FieldKind::Secret,
            ),
            window,
            cx,
        );
        let users = self.field_row(
            &field(
                "telegram.allow_users",
                "Allowed users",
                "Comma-separated user IDs or usernames. Empty allows everyone.",
                FieldKind::Text,
            ),
            window,
            cx,
        );
        let chats = self.field_row(
            &field(
                "telegram.allow_chats",
                "Allowed chats",
                "Comma-separated chat IDs. Empty allows every chat.",
                FieldKind::Text,
            ),
            window,
            cx,
        );
        let proxy = self.field_row(
            &field(
                "telegram.proxy",
                "Proxy",
                "Optional proxy URL for reaching the Telegram API.",
                FieldKind::Text,
            ),
            window,
            cx,
        );

        self.page(
            "Bub speaks to more than this window. These are the Telegram channel's settings.",
            vec![group("Telegram", vec![token, users, chats, proxy], "")],
            Some(SAVE_NOTE),
        )
    }

    fn settings_about(&self, _cx: &mut Context<Self>) -> AnyElement {
        let model = if self.model.is_empty() {
            "—".to_string()
        } else {
            self.model.clone()
        };
        // The counts that used to close this page moved to the page they count:
        // a number is a signpost, and a signpost belongs on the road, not at
        // the end of a different one.
        let rows = vec![
            group(
                "Runtime",
                vec![
                    info_row(
                        "Connection",
                        if self.connected {
                            "connected"
                        } else if self.ready {
                            "offline"
                        } else {
                            "connecting"
                        },
                    ),
                    info_row("Protocol", &format!("v{}", self.protocol)),
                    info_row("Model", &model),
                    info_row("Workspace", &fallback(&self.workspace)),
                ],
                "",
            ),
            group(
                "Configuration",
                vec![
                    info_row("Config file", &fallback(&self.settings_config_file)),
                    info_row("Home", &fallback(&self.settings_home)),
                ],
                "",
            ),
        ];

        self.page(
            "Where Bub is running and what it has discovered here.",
            rows,
            None,
        )
    }

    /// The shared page scaffold: what the page is for, and the groups.
    ///
    /// The title is not here — the toolbar above carries it, because the page
    /// and its actions belong on one line rather than two.
    ///
    /// The footer is per page: a page that writes to Bub's config file says so,
    /// and a page that only reads says nothing rather than repeating a promise
    /// it cannot keep.
    fn page(&self, description: &str, groups: Vec<AnyElement>, footer: Option<&str>) -> AnyElement {
        v_flex()
            .w_full()
            .gap(px(22.))
            .child(
                div()
                    .text_size(theme::TEXT_UI)
                    .line_height(px(21.))
                    .text_color(theme::ink_secondary())
                    .child(description.to_string()),
            )
            .children(groups)
            .when_some(footer, |this, footer| this.child(atoms::caption(footer)))
            .into_any_element()
    }

    /// A row that edits one text-shaped configuration key.
    fn field_row(
        &mut self,
        spec: &Field,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> AnyElement {
        let overridden = self.settings_is_overridden(spec.key);

        // A white field on the grey page, the way a native text field reads:
        // the fill says "you can type here" and the edge holds the shape. The
        // old grey plate sat at the same value as the rows around it and read
        // as a label rather than an editor.
        let mut control = h_flex()
            .id(format!("field-{}", spec.key))
            .w(px(272.))
            .flex_shrink_0()
            .h(px(32.))
            .items_center()
            .px(px(10.))
            .rounded(theme::RADIUS_CONTROL)
            .border_1()
            .border_color(theme::edge())
            .bg(theme::surface());

        // Before the runtime's snapshot arrives there is nothing to seed a
        // field from, so the row stays a placeholder rather than becoming an
        // editor that would open empty and never catch up.
        if self.settings_values.is_empty() {
            control = control.child(
                div()
                    .text_size(theme::TEXT_UI)
                    .text_color(theme::ink_tertiary())
                    .child("Loading…"),
            );
        } else {
            let input = self.settings_input(spec.key, window, cx);
            let focus = input.clone();
            control = control
                .cursor_text()
                .on_click(move |_, window, cx| {
                    focus.update(cx, |state, cx| state.focus(window, cx));
                })
                .child(
                    Input::new(&input)
                        .appearance(false)
                        .bordered(false)
                        .w_full()
                        .text_size(theme::TEXT_UI),
                );
        }

        let mut meta = v_flex()
            .flex_1()
            .min_w_0()
            .gap(px(2.))
            .child(
                div()
                    .text_size(theme::TEXT_UI)
                    .text_color(theme::ink())
                    .child(spec.label),
            )
            .when(!spec.help.is_empty(), |this| {
                this.child(
                    div()
                        .text_size(theme::TEXT_MICRO)
                        .line_height(px(16.))
                        .text_color(theme::ink_tertiary())
                        .child(spec.help),
                )
            });
        if overridden {
            meta = meta.child(
                div()
                    .text_size(theme::TEXT_MICRO)
                    .text_color(theme::warning())
                    .child("Overridden by the environment; edits here may be ignored."),
            );
        }
        if spec.kind == FieldKind::Secret {
            meta = meta.child(atoms::caption("Stored in plain text in Bub's config file."));
        }

        h_flex()
            .w_full()
            .items_start()
            .justify_between()
            .gap(px(28.))
            .child(meta)
            .child(control)
            .into_any_element()
    }

    /// A row whose control is the switch, clickable anywhere along its width.
    fn switch_row(
        &self,
        id: &'static str,
        label: &'static str,
        help: &'static str,
        checked: bool,
        _persisted: bool,
        on_click: impl Fn(&gpui_kit::ClickEvent, &mut Window, &mut gpui_kit::App) + 'static,
    ) -> AnyElement {
        h_flex()
            .id(format!("switch-{id}"))
            .w_full()
            .items_center()
            .justify_between()
            .gap(px(28.))
            .cursor_pointer()
            .on_click(on_click)
            .child(
                v_flex()
                    .flex_1()
                    .min_w_0()
                    .gap(px(2.))
                    .child(
                        div()
                            .text_size(theme::TEXT_UI)
                            .text_color(theme::ink())
                            .child(label),
                    )
                    .when(!help.is_empty(), |this| {
                        this.child(
                            div()
                                .text_size(theme::TEXT_MICRO)
                                .line_height(px(16.))
                                .text_color(theme::ink_tertiary())
                                .child(help),
                        )
                    }),
            )
            .child(atoms::switch_track(checked))
            .into_any_element()
    }
}

/// Map a field's edited text to the JSON the bridge should persist.
pub(crate) fn encode_value(key: &str, text: &str) -> Value {
    match key {
        "fallback_models" => Value::Array(
            text.split(',')
                .map(str::trim)
                .filter(|part| !part.is_empty())
                .map(|part| Value::String(part.to_string()))
                .collect(),
        ),
        "max_steps" => {
            // Empty means "no limit", which the runtime spells as the sentinel.
            let trimmed = text.trim();
            if trimmed.is_empty() {
                Value::from(i64::MAX)
            } else {
                Value::from(trimmed.parse::<i64>().unwrap_or(i64::MAX))
            }
        }
        "max_tokens" => Value::from(text.trim().parse::<i64>().unwrap_or_default()),
        "model_timeout_seconds" => {
            let trimmed = text.trim();
            if trimmed.is_empty() {
                Value::Null
            } else {
                Value::from(trimmed.parse::<i64>().unwrap_or_default())
            }
        }
        _ => Value::String(text.to_string()),
    }
}

/// The card a group of rows sits in, with hairlines between them.
///
/// A group with nothing in it still gets its card, holding one quiet line: on
/// the capabilities page an empty group is a real answer ("no tools reported"),
/// not a reason to drop the heading and leave the reader wondering.
fn group(title: &str, rows: Vec<AnyElement>, empty: &str) -> AnyElement {
    let mut card = v_flex()
        .w_full()
        .rounded(theme::RADIUS_CARD)
        .border_1()
        .border_color(theme::hairline())
        .bg(theme::surface())
        .overflow_hidden();

    if rows.is_empty() {
        card = card.child(
            div()
                .w_full()
                .px(px(16.))
                .py(px(13.))
                .child(atoms::caption(empty)),
        );
    } else {
        let count = rows.len();
        for (index, row) in rows.into_iter().enumerate() {
            card = card.child(
                div()
                    .w_full()
                    .px(px(16.))
                    .py(px(13.))
                    .when(index + 1 < count, |this| {
                        this.border_b_1().border_color(theme::hairline())
                    })
                    .child(row),
            );
        }
    }

    v_flex()
        .w_full()
        .gap(px(8.))
        .child(atoms::group_title(title))
        .child(card)
        .into_any_element()
}

/// One discovered capability: its glyph, its name, and what it does.
///
/// Returns the row rather than an element so a caller that needs it to be
/// interactive can still attach an id and a handler.
fn capability_row(icon: IconName, name: &str, description: &str, badge: Option<&str>) -> Div {
    h_flex()
        .w_full()
        .items_start()
        .gap(px(10.))
        .child(
            div().flex_shrink_0().pt(px(1.)).child(
                Icon::new(icon)
                    .text_color(theme::ink_tertiary())
                    .with_size(px(14.)),
            ),
        )
        .child(
            v_flex()
                .flex_1()
                .min_w_0()
                .gap(px(2.))
                .child(
                    h_flex()
                        .w_full()
                        .items_center()
                        .gap(px(8.))
                        .child(
                            div()
                                .flex_1()
                                .min_w_0()
                                .truncate()
                                .text_size(theme::TEXT_UI)
                                .text_color(theme::ink())
                                .child(name.to_string()),
                        )
                        .when_some(badge, |this, badge| this.child(note(badge))),
                )
                .when(!description.is_empty(), |this| {
                    this.child(
                        div()
                            .w_full()
                            .truncate()
                            .text_size(theme::TEXT_MICRO)
                            .text_color(theme::ink_tertiary())
                            .child(description.to_string()),
                    )
                }),
        )
}

/// A quiet marker on a capability row, e.g. where a skill came from.
fn note(text: &str) -> AnyElement {
    div()
        .flex_shrink_0()
        .max_w(px(160.))
        .truncate()
        .px(px(6.))
        .py(px(1.))
        .rounded(px(5.))
        .bg(theme::fill())
        .text_size(theme::TEXT_MICRO)
        .text_color(theme::ink_secondary())
        .child(text.to_string())
        .into_any_element()
}

/// The glyph for how a tool is exposed to the model.
fn exposure_icon(exposure: &str) -> IconName {
    match exposure {
        "command" => IconName::Terminal,
        "code" => IconName::FileCode,
        _ => IconName::Wrench,
    }
}

/// The commands, as the triggers they are rather than as rows of prose.
fn command_row(commands: &[Command], cx: &App) -> AnyElement {
    let mut flow = h_flex().w_full().flex_wrap().gap(px(6.));
    for command in commands {
        flow = flow.child(
            div()
                .px(px(8.))
                .py(px(3.))
                .rounded(px(6.))
                .bg(theme::fill())
                .font_family(cx.theme().mono_font_family.clone())
                .text_size(theme::TEXT_CAPTION)
                .text_color(theme::ink())
                .child(command.trigger.clone()),
        );
    }
    v_flex()
        .w_full()
        .gap(px(8.))
        .child(flow)
        .child(atoms::caption(
            "Type one in the composer, at the start of the message.",
        ))
        .into_any_element()
}

/// A read-only fact row.
fn info_row(label: &str, value: &str) -> AnyElement {
    h_flex()
        .w_full()
        .items_center()
        .justify_between()
        .gap(px(24.))
        .child(
            div()
                .text_size(theme::TEXT_UI)
                .text_color(theme::ink())
                .child(label.to_string()),
        )
        .child(
            div()
                .min_w_0()
                .truncate()
                .text_size(theme::TEXT_UI)
                .text_color(theme::ink_secondary())
                .child(value.to_string()),
        )
        .into_any_element()
}

fn fallback(value: &str) -> String {
    if value.is_empty() {
        "—".to_string()
    } else {
        value.to_string()
    }
}
