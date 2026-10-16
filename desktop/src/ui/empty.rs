//! What the window shows before the first message.
//!
//! The hero is the one place the app can afford to be generous: a mark, a
//! question, the composer, and three ways in. It replaces the old onboarding
//! panel, which framed the same content in a dashed outline and closed with a
//! line of trivia ("22 tools · 60 skills · Enter to send") that told the reader
//! nothing they were about to do.

use gpui_kit::assets::IconName;
use gpui_kit::component::{h_flex, v_flex, Icon, Sizable as _, StyledExt as _};
use gpui_kit::{
    div, linear_color_stop, linear_gradient, px, AnyElement, Context, InteractiveElement as _,
    IntoElement, ParentElement as _, StatefulInteractiveElement as _, Styled as _,
};

use crate::app::DesktopApp;
use crate::theme;

/// Ways in, offered only while the transcript is empty.
const SUGGESTIONS: [&str; 3] = [
    "Summarize this workspace",
    "What changed recently?",
    "Find TODOs in the code",
];

/// The centred opening state, with the composer placed inside it.
pub(crate) fn hero(cx: &mut Context<DesktopApp>, field: AnyElement) -> AnyElement {
    v_flex()
        .size_full()
        .items_center()
        .justify_center()
        .gap(px(24.))
        .px(px(28.))
        .pb(px(64.))
        .child(mark())
        .child(heading())
        .child(div().w_full().max_w(theme::CONTENT_MAX).child(field))
        .child(suggestions(cx))
        .into_any_element()
}

/// The app mark: a filled squircle in the accent gradient.
fn mark() -> AnyElement {
    div()
        .w(px(56.))
        .h(px(56.))
        .rounded(theme::RADIUS_MARK)
        .bg(linear_gradient(
            160.,
            linear_color_stop(theme::accent_hover(), 0.),
            linear_color_stop(theme::accent_active(), 1.),
        ))
        .flex()
        .items_center()
        .justify_center()
        .shadow_lg()
        .child(
            Icon::new(IconName::Sparkles)
                .text_color(theme::on_accent())
                .with_size(px(26.)),
        )
        .into_any_element()
}

fn heading() -> AnyElement {
    v_flex()
        .items_center()
        .gap(px(8.))
        .child(
            div()
                .text_size(theme::TEXT_DISPLAY)
                .font_semibold()
                .text_color(theme::ink())
                .child("What can I help with?"),
        )
        .child(
            div()
                .max_w(px(420.))
                .text_center()
                .text_size(theme::TEXT_UI)
                .line_height(px(20.))
                .text_color(theme::ink_secondary())
                // Naming the workspace here read as "Bub works in desktop" and
                // "Bub works in bub": a directory name is not a place. Where it
                // is working is a fact for the panel.
                .child("Ask a question, or describe a task."),
        )
        .into_any_element()
}

fn suggestions(cx: &mut Context<DesktopApp>) -> AnyElement {
    let mut row = h_flex().flex_wrap().justify_center().gap(px(8.));

    for text in SUGGESTIONS {
        let entity = cx.entity();
        row = row.child(
            div()
                .id(text)
                .cursor_pointer()
                .px(px(14.))
                .py(px(8.))
                .rounded(px(9999.))
                .border_1()
                .border_color(theme::hairline())
                .text_size(theme::TEXT_UI)
                .text_color(theme::ink_secondary())
                .hover(|style| style.bg(theme::fill()).text_color(theme::ink()))
                .child(text)
                .on_click(move |_, window, cx| {
                    let text = text.to_string();
                    entity.update(cx, |app, cx| app.submit(text, window, cx));
                }),
        );
    }

    row.into_any_element()
}
