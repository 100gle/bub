//! The conversation column.
//!
//! One column holds the transcript and the composer, and there is no bar under
//! the composer: the field is a floating capsule on the same surface as the
//! text it sends. That is also why transcript rows are bounded to
//! [`theme::CONTENT_MAX`] and centred rather than bled across a wide window.
//!
//! A turn is one row. The reader's message is one; the model's answer is one,
//! however many steps it took — a thought, a tool call, its result, some prose.
//! That is why the answer's row is created empty the moment the message is sent:
//! the wait belongs to the answer, so it is drawn at the answer's own tail and
//! not somewhere else on screen. The steps inside the row are set closer
//! together than rows are, because they are one message rather than several.
//!
//! On an empty session the composer is built inside the hero instead of at the
//! foot of the column, so opening the app shows one centred thought rather than
//! a chat box waiting at the bottom of an empty page.

use std::collections::HashSet;
use std::rc::Rc;
use std::time::Duration;

use gpui_kit::assets::IconName;
use gpui_kit::component::{
    h_flex, message_scroller::MessageScroller, shimmer::ShimmerText, spinner::Spinner,
    text::TextView, v_flex, ActiveTheme as _, Icon, Sizable as _, StyledExt as _,
};
use gpui_kit::prelude::FluentBuilder as _;
use gpui_kit::{
    div, px, AnyElement, App, Context, Entity, InteractiveElement as _, IntoElement,
    ParentElement as _, StatefulInteractiveElement as _, StyleRefinement, Styled as _,
};

use crate::app::{format_elapsed, DesktopApp, RunPhase};
use crate::model::{disclosure, first_line, preview, BlockKey, Entry, Status, Step, Turn};
use crate::theme;
use crate::ui::{atoms, composer, empty};

impl DesktopApp {
    pub(crate) fn transcript(&self, cx: &mut Context<Self>) -> AnyElement {
        let entries = Rc::new(self.entries.clone());
        let toggled = Rc::new(self.toggled.clone());
        let entity = cx.entity();
        let started = !entries.is_empty();
        // The answer still being written, if any. Only its row gets the stream
        // motion and the tail line.
        let open = if self.generating {
            self.entries.last().map(|entry| entry.id)
        } else {
            None
        };
        let phase = self.phase();
        let elapsed = self.elapsed;

        let mut column = v_flex().size_full().min_h_0();

        if !started {
            // Built per branch rather than shared: an element is consumed by
            // its parent, so one value cannot serve both placements.
            let field = composer::hero_field(self, cx);
            return column.child(empty::hero(cx, field)).into_any_element();
        }

        let rows_entries = entries.clone();
        let rows_toggled = toggled.clone();
        let rows_entity = entity.clone();
        column = column
            .child(
                MessageScroller::new("bub-transcript", self.scroller.clone(), {
                    move |index, _, cx| match rows_entries.get(index) {
                        Some(entry) => row(
                            entry,
                            &rows_toggled,
                            &rows_entity,
                            open == Some(entry.id),
                            &phase,
                            elapsed,
                            cx,
                        ),
                        None => div().into_any_element(),
                    }
                })
                // The library's defaults are a chat-app rhythm: 12px gutters
                // and 32px between rows. This window wants a wider measure
                // with a tighter cadence, so both are restated here.
                .with_row_style(StyleRefinement::default().px(px(28.)).pb(px(18.)))
                .with_bottom_fade(theme::canvas())
                .size_full(),
            )
            .child(composer::footer(self, cx));

        column.into_any_element()
    }
}

/// A transcript row, centred in the column and bounded to a readable measure.
fn framed(inner: impl IntoElement) -> AnyElement {
    h_flex()
        .w_full()
        .justify_center()
        .child(div().w_full().max_w(theme::CONTENT_MAX).child(inner))
        .into_any_element()
}

#[allow(clippy::too_many_arguments)]
fn row(
    entry: &Entry,
    toggled: &HashSet<BlockKey>,
    entity: &Entity<DesktopApp>,
    open: bool,
    phase: &RunPhase,
    elapsed: Duration,
    cx: &App,
) -> AnyElement {
    match &entry.turn {
        Turn::User { text, attachments } => framed(user_row(text, attachments)),
        Turn::Assistant(steps) => framed(answer(
            entry.id, steps, toggled, entity, open, phase, elapsed, cx,
        )),
    }
}

fn user_row(text: &str, attachments: &[String]) -> AnyElement {
    h_flex()
        .w_full()
        .justify_end()
        .child(
            v_flex()
                .max_w(px(560.))
                .items_end()
                .gap(px(6.))
                .when(!attachments.is_empty(), |this| {
                    this.child(h_flex().flex_wrap().justify_end().gap(px(6.)).children(
                        attachments.iter().map(|name| {
                            h_flex()
                                .gap(px(6.))
                                .items_center()
                                .px(px(9.))
                                .py(px(5.))
                                .rounded(px(10.))
                                .bg(theme::fill())
                                .child(
                                    Icon::new(IconName::Paperclip)
                                        .text_color(theme::ink_tertiary())
                                        .with_size(px(12.)),
                                )
                                .child(
                                    div()
                                        .max_w(px(220.))
                                        .truncate()
                                        .text_size(theme::TEXT_CAPTION)
                                        .text_color(theme::ink_secondary())
                                        .child(name.clone()),
                                )
                                .into_any_element()
                        }),
                    ))
                })
                .when(!text.is_empty(), |this| {
                    this.child(
                        div()
                            .px(px(14.))
                            .py(px(9.))
                            .rounded(theme::RADIUS_BUBBLE)
                            .bg(theme::fill())
                            .text_size(theme::TEXT_BODY)
                            .line_height(px(22.))
                            .text_color(theme::ink())
                            .child(text.to_string()),
                    )
                }),
        )
        .into_any_element()
}

/// The model's answer: its steps, and the line that says what it is doing now.
#[allow(clippy::too_many_arguments)]
fn answer(
    id: u64,
    steps: &[Step],
    toggled: &HashSet<BlockKey>,
    entity: &Entity<DesktopApp>,
    open: bool,
    phase: &RunPhase,
    elapsed: Duration,
    cx: &App,
) -> AnyElement {
    let last = steps.len().saturating_sub(1);
    let mut block = v_flex().w_full().gap(px(12.));

    for (index, step) in steps.iter().enumerate() {
        block = block.child(step_row(
            (id, index),
            step,
            toggled,
            entity,
            // Only the step still being written into eases its new text in; a
            // settled block that faded on every repaint would twitch.
            open && index == last,
            cx,
        ));
    }

    // The tail of the answer, while it is still being written. This is where
    // the wait lives: on the message, at the point it will continue from.
    if open {
        block = block.child(pending(phase, elapsed));
    }

    block.into_any_element()
}

/// What the open turn is doing, drawn as the answer's last line.
///
/// A shimmer while the model is the one working, a still warning dot while it is
/// blocked on the reader: motion is a claim about who is busy, and it would be a
/// lie during an approval. A block with no steps yet is the wait itself — the
/// state a reader sees most often, and the one that used to be nothing at all.
fn pending(phase: &RunPhase, elapsed: Duration) -> AnyElement {
    let working = phase.is_working();

    let label: AnyElement = if working {
        ShimmerText::new(phase.label())
            .id("turn-status")
            .text_size(theme::TEXT_CAPTION)
            .text_color(theme::ink_secondary())
            .into_any_element()
    } else {
        h_flex()
            .items_center()
            .gap(px(6.))
            .child(atoms::dot(theme::warning()))
            .child(
                div()
                    .text_size(theme::TEXT_CAPTION)
                    .font_medium()
                    .text_color(theme::warning())
                    .child(phase.label()),
            )
            .into_any_element()
    };

    h_flex()
        .w_full()
        .items_center()
        .gap(px(8.))
        .child(label)
        .child(
            div()
                .flex_shrink_0()
                .text_size(theme::TEXT_CAPTION)
                .text_color(theme::ink_tertiary())
                .child(format_elapsed(elapsed)),
        )
        .into_any_element()
}

fn step_row(
    key: BlockKey,
    step: &Step,
    toggled: &HashSet<BlockKey>,
    entity: &Entity<DesktopApp>,
    streaming: bool,
    cx: &App,
) -> AnyElement {
    match step {
        Step::Text(text) => div()
            .w_full()
            .py(px(2.))
            .text_size(theme::TEXT_BODY)
            .line_height(px(24.))
            .text_color(theme::ink())
            .child(
                TextView::markdown(format!("text-{}-{}", key.0, key.1), text.clone())
                    .selectable(true)
                    .stream_fade(streaming),
            )
            .into_any_element(),
        Step::Reasoning(text) => {
            let open = disclosure(toggled, key, true);
            v_flex()
                .w_full()
                .child(quiet_header(
                    entity,
                    key,
                    IconName::Brain,
                    "Thought process",
                    // The preview is the body's first line, so showing it on an
                    // expanded block would print that line twice.
                    if open {
                        String::new()
                    } else {
                        first_line(text)
                    },
                    open,
                ))
                .when(open, |this| {
                    this.child(
                        div()
                            .mt(px(6.))
                            .pl(px(12.))
                            .border_l_1()
                            .border_color(theme::hairline())
                            .text_size(theme::TEXT_UI)
                            .line_height(px(21.))
                            .text_color(theme::ink_secondary())
                            .child(text.clone()),
                    )
                })
                .into_any_element()
        }
        Step::Tool {
            name,
            detail,
            state,
            approval_id,
            result,
        } => {
            let open = disclosure(toggled, key, *state == Status::Awaiting);
            tool_card(
                cx,
                entity,
                key,
                name,
                detail,
                *state,
                approval_id.as_deref(),
                result.as_deref(),
                open,
            )
        }
        Step::Result(detail) => {
            let open = disclosure(toggled, key, false);
            result_card(cx, entity, key, detail, open)
        }
        Step::Usage(text) => atoms::caption(text.clone()).into_any_element(),
        Step::Error(text) => error_card(text),
    }
}

/// A tool invocation.
///
/// Collapsed, the card is one line: what ran, how it ended, and the first line
/// of its arguments. Expanding reveals the payload. The header keeps its own
/// click target so the approval buttons inside can be pressed without also
/// toggling the card.
#[allow(clippy::too_many_arguments)]
fn tool_card(
    cx: &App,
    entity: &Entity<DesktopApp>,
    key: BlockKey,
    name: &str,
    detail: &str,
    state: Status,
    approval_id: Option<&str>,
    result: Option<&str>,
    open: bool,
) -> AnyElement {
    let awaiting = state == Status::Awaiting;
    let (turn, step) = key;

    let mut card = v_flex()
        .w_full()
        .rounded(theme::RADIUS_CARD)
        .border_1()
        .border_color(if awaiting {
            theme::warning().opacity(0.35)
        } else {
            theme::hairline()
        })
        .bg(theme::recess())
        .px(px(12.))
        .py(px(9.))
        .child(
            h_flex()
                .w_full()
                .items_center()
                .gap(px(8.))
                .cursor_pointer()
                .child(if state == Status::Running {
                    // A running tool is the one moment the row is doing
                    // something; a still wrench next to the word "running"
                    // reads as finished-but-labelled.
                    Spinner::new().small().into_any_element()
                } else {
                    Icon::new(IconName::Wrench)
                        .text_color(theme::ink_tertiary())
                        .with_size(px(12.))
                        .into_any_element()
                })
                .child(
                    div()
                        .flex_shrink_0()
                        .font_family(cx.theme().mono_font_family.clone())
                        .text_size(theme::TEXT_CAPTION)
                        .font_medium()
                        .text_color(theme::ink())
                        .child(name.to_string()),
                )
                .child(status_tag(state))
                .child(
                    div()
                        .flex_1()
                        .min_w_0()
                        .truncate()
                        .text_size(theme::TEXT_CAPTION)
                        .text_color(theme::ink_tertiary())
                        .child(preview(detail)),
                )
                .child(atoms::disclosure_chevron(open))
                .id(format!("tool-header-{turn}-{step}"))
                .on_click({
                    let entity = entity.clone();
                    move |_, _, cx| entity.update(cx, |app, cx| app.toggle(key, cx))
                }),
        );

    if awaiting {
        if let Some(approval_id) = approval_id {
            let mut verdicts = h_flex()
                .w_full()
                .items_center()
                .gap(px(8.))
                .pt(px(10.))
                .child(
                    div()
                        .flex_1()
                        .text_size(theme::TEXT_CAPTION)
                        .text_color(theme::ink_secondary())
                        .child("Run this tool?"),
                );
            for (label, allow, plate) in [
                ("Deny", false, atoms::Plate::Quiet),
                ("Allow", true, atoms::Plate::Accent),
            ] {
                let entity = entity.clone();
                let approval_id = approval_id.to_string();
                verdicts = verdicts.child(atoms::pill_button(
                    cx,
                    format!("{}-{turn}-{step}-{}", label.to_lowercase(), allow),
                    label,
                    plate,
                    move |_, _, cx| {
                        let approval_id = approval_id.clone();
                        entity.update(cx, |app, cx| app.respond_approval(&approval_id, allow, cx));
                    },
                ));
            }
            card = card.child(verdicts);
        }
    }

    if open {
        card = card.child(div().w_full().pt(px(10.)).child(payload(
            format!("tool-args-{turn}-{step}"),
            "Arguments",
            cx,
            detail,
        )));
        if let Some(result) = result {
            card = card.child(div().pt(px(10.)).child(payload(
                format!("tool-result-{turn}-{step}"),
                "Result",
                cx,
                result,
            )));
        }
    }

    card.into_any_element()
}

/// A tool result that no card was waiting for.
fn result_card(
    cx: &App,
    entity: &Entity<DesktopApp>,
    key: BlockKey,
    detail: &str,
    open: bool,
) -> AnyElement {
    let (turn, step) = key;
    v_flex()
        .w_full()
        .rounded(theme::RADIUS_CARD)
        .border_1()
        .border_color(theme::hairline())
        .bg(theme::recess())
        .px(px(12.))
        .py(px(9.))
        .child(
            h_flex()
                .w_full()
                .items_center()
                .gap(px(8.))
                .cursor_pointer()
                .child(
                    Icon::new(IconName::CircleCheck)
                        .text_color(theme::success())
                        .with_size(px(12.)),
                )
                .child(
                    div()
                        .flex_shrink_0()
                        .text_size(theme::TEXT_CAPTION)
                        .font_medium()
                        .text_color(theme::ink())
                        .child("Result"),
                )
                .child(
                    div()
                        .flex_1()
                        .min_w_0()
                        .truncate()
                        .text_size(theme::TEXT_CAPTION)
                        .text_color(theme::ink_tertiary())
                        .child(preview(detail)),
                )
                .child(atoms::disclosure_chevron(open))
                .id(format!("result-header-{turn}-{step}"))
                .on_click({
                    let entity = entity.clone();
                    move |_, _, cx| entity.update(cx, |app, cx| app.toggle(key, cx))
                }),
        )
        .when(open, |this| {
            this.child(payload(format!("result-{turn}-{step}"), "", cx, detail))
        })
        .into_any_element()
}

fn error_card(text: &str) -> AnyElement {
    v_flex()
        .w_full()
        .gap(px(6.))
        .rounded(theme::RADIUS_CARD)
        .bg(theme::danger_wash())
        .px(px(12.))
        .py(px(10.))
        .child(
            h_flex()
                .items_center()
                .gap(px(8.))
                .child(
                    Icon::new(IconName::CircleAlert)
                        .text_color(theme::danger())
                        .with_size(px(13.)),
                )
                .child(
                    div()
                        .text_size(theme::TEXT_UI)
                        .font_medium()
                        .text_color(theme::danger())
                        .child("Something went wrong"),
                ),
        )
        .child(
            div()
                .text_size(theme::TEXT_UI)
                .line_height(px(21.))
                .text_color(theme::ink())
                .child(text.to_string()),
        )
        .into_any_element()
}

/// A collapsible header for a block with no card around it.
fn quiet_header(
    entity: &Entity<DesktopApp>,
    key: BlockKey,
    icon: IconName,
    title: &str,
    meta: String,
    open: bool,
) -> impl IntoElement {
    let (turn, step) = key;
    h_flex()
        .w_full()
        .items_center()
        .gap(px(8.))
        .cursor_pointer()
        .child(
            Icon::new(icon)
                .text_color(theme::ink_tertiary())
                .with_size(px(12.)),
        )
        .child(
            div()
                .flex_shrink_0()
                .text_size(theme::TEXT_CAPTION)
                .font_medium()
                .text_color(theme::ink_secondary())
                .child(title.to_string()),
        )
        .child(
            div()
                .flex_1()
                .min_w_0()
                .truncate()
                .text_size(theme::TEXT_CAPTION)
                .text_color(theme::ink_tertiary())
                .child(meta),
        )
        .child(atoms::disclosure_chevron(open))
        .id(format!("quiet-header-{turn}-{step}"))
        .on_click({
            let entity = entity.clone();
            move |_, _, cx| entity.update(cx, |app, cx| app.toggle(key, cx))
        })
}

/// A labelled monospace payload inside a card.
fn payload(id: String, label: &str, cx: &App, text: &str) -> AnyElement {
    v_flex()
        .w_full()
        .gap(px(4.))
        .when(!label.is_empty(), |this| {
            this.child(atoms::group_title(label))
        })
        .child(atoms::code_block(id, cx, text))
        .into_any_element()
}

/// A status word, coloured by how the action ended.
fn status_tag(state: Status) -> AnyElement {
    let color = state.color();
    h_flex()
        .flex_shrink_0()
        .items_center()
        .gap(px(4.))
        .child(atoms::dot(color))
        .child(
            div()
                .text_size(theme::TEXT_MICRO)
                .text_color(color)
                .child(state.label()),
        )
        .into_any_element()
}
