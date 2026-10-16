//! The conversation column.
//!
//! One column holds the transcript and the composer, and there is no bar under
//! the composer: the field is a floating capsule on the same surface as the
//! text it sends. That is also why transcript rows are bounded to
//! [`theme::CONTENT_MAX`] and centred rather than bled across a wide window.
//!
//! On an empty session the composer is built inside the hero instead of at the
//! foot of the column, so opening the app shows one centred thought rather than
//! a chat box waiting at the bottom of an empty page.

use std::collections::HashSet;
use std::rc::Rc;

use gpui_kit::assets::IconName;
use gpui_kit::component::{
    h_flex, message_scroller::MessageScroller, text::TextView, v_flex, ActiveTheme as _, Icon,
    Sizable as _, StyledExt as _,
};
use gpui_kit::prelude::FluentBuilder as _;
use gpui_kit::{
    div, px, AnyElement, App, Context, Entity, InteractiveElement as _, IntoElement,
    ParentElement as _, StatefulInteractiveElement as _, StyleRefinement, Styled as _,
};

use crate::app::DesktopApp;
use crate::model::{disclosure, first_line, preview, Entry, Item, Status};
use crate::theme;
use crate::ui::{atoms, composer, empty};

impl DesktopApp {
    pub(crate) fn transcript(&self, cx: &mut Context<Self>) -> AnyElement {
        let entries = Rc::new(self.entries.clone());
        let toggled = Rc::new(self.toggled.clone());
        let entity = cx.entity();
        let started = !entries.is_empty();

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
                        Some(entry) => row(entry, &rows_toggled, &rows_entity, cx),
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

fn row(entry: &Entry, toggled: &HashSet<u64>, entity: &Entity<DesktopApp>, cx: &App) -> AnyElement {
    let id = entry.id;
    match &entry.item {
        Item::User(text) => framed(
            h_flex().w_full().justify_end().child(
                div()
                    .max_w(px(560.))
                    .px(px(14.))
                    .py(px(9.))
                    .rounded(theme::RADIUS_BUBBLE)
                    .bg(theme::fill())
                    .text_size(theme::TEXT_BODY)
                    .line_height(px(22.))
                    .text_color(theme::ink())
                    .child(text.clone()),
            ),
        ),
        // Assistant text is the document on the canvas, so it carries no
        // bubble: a plate around every paragraph would double the ink.
        Item::Assistant(text) => framed(
            div()
                .w_full()
                .py(px(2.))
                .text_size(theme::TEXT_BODY)
                .line_height(px(24.))
                .text_color(theme::ink())
                .child(
                    TextView::markdown(format!("assistant-{id}"), text.clone()).selectable(true),
                ),
        ),
        Item::Reasoning(text) => {
            let open = disclosure(toggled, id, true);
            framed(
                v_flex()
                    .w_full()
                    .child(quiet_header(
                        entity,
                        id,
                        IconName::Brain,
                        "Thought process",
                        // The preview is the body's first line, so showing it on
                        // an expanded block would print that line twice.
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
                    }),
            )
        }
        Item::ToolCall {
            name,
            detail,
            state,
            approval_id,
            result,
        } => {
            let open = disclosure(toggled, id, *state == Status::Awaiting);
            framed(tool_card(
                cx,
                entity,
                id,
                name,
                detail,
                *state,
                approval_id.as_deref(),
                result.as_deref(),
                open,
            ))
        }
        Item::ToolResult { detail } => {
            let open = disclosure(toggled, id, false);
            framed(result_card(cx, entity, id, detail, open))
        }
        Item::Usage(text) => framed(atoms::caption(text.clone())),
        Item::Error(text) => framed(error_card(text)),
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
    id: u64,
    name: &str,
    detail: &str,
    state: Status,
    approval_id: Option<&str>,
    result: Option<&str>,
    open: bool,
) -> AnyElement {
    let awaiting = state == Status::Awaiting;

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
                .child(
                    Icon::new(IconName::Wrench)
                        .text_color(theme::ink_tertiary())
                        .with_size(px(12.)),
                )
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
                .id(format!("tool-header-{id}"))
                .on_click({
                    let entity = entity.clone();
                    move |_, _, cx| entity.update(cx, |app, cx| app.toggle(id, cx))
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
                    format!("{}-{id}", label.to_lowercase()),
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
            format!("tool-args-{id}"),
            "Arguments",
            cx,
            detail,
        )));
        if let Some(result) = result {
            card = card.child(div().pt(px(10.)).child(payload(
                format!("tool-result-{id}"),
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
    id: u64,
    detail: &str,
    open: bool,
) -> AnyElement {
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
                .id(format!("result-header-{id}"))
                .on_click({
                    let entity = entity.clone();
                    move |_, _, cx| entity.update(cx, |app, cx| app.toggle(id, cx))
                }),
        )
        .when(open, |this| {
            this.child(payload(format!("result-{id}"), "", cx, detail))
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
    id: u64,
    icon: IconName,
    title: &str,
    meta: String,
    open: bool,
) -> impl IntoElement {
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
        .id(format!("quiet-header-{id}"))
        .on_click({
            let entity = entity.clone();
            move |_, _, cx| entity.update(cx, |app, cx| app.toggle(id, cx))
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
