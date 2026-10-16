//! The message composer.
//!
//! One capsule: the field and its single action share a surface, and the send
//! control is the only filled accent plate in the window. There is no hint
//! line under it and no rule above it — the shortcuts are conventional, and a
//! persistent legend under an empty box is chrome that never stops costing
//! attention.
//!
//! The capsule is built once and placed twice: pinned under a running
//! transcript, or inside the empty state's hero.

use gpui_kit::assets::IconName;
use gpui_kit::component::{h_flex, input::Textarea};
use gpui_kit::{div, px, AnyElement, Context, IntoElement, ParentElement as _, Styled as _};

use crate::app::DesktopApp;
use crate::theme;
use crate::ui::atoms;

/// The capsule in its pinned position at the foot of the transcript column.
pub(crate) fn footer(app: &DesktopApp, cx: &mut Context<DesktopApp>) -> AnyElement {
    h_flex()
        .w_full()
        .justify_center()
        .px(px(28.))
        .pt(px(6.))
        .pb(px(22.))
        .child(capsule(app, cx))
        .into_any_element()
}

/// The capsule inside the empty state, where the hero owns the spacing.
pub(crate) fn hero_field(app: &DesktopApp, cx: &mut Context<DesktopApp>) -> AnyElement {
    h_flex()
        .w_full()
        .justify_center()
        .child(capsule(app, cx))
        .into_any_element()
}

fn capsule(app: &DesktopApp, cx: &mut Context<DesktopApp>) -> AnyElement {
    let generating = app.generating;

    let action = if generating {
        atoms::circle_button(
            cx,
            "stop",
            IconName::Square,
            atoms::Plate::Quiet,
            "Stop",
            true,
            cx.listener(|this, _, _, cx| this.stop(cx)),
        )
    } else {
        let can_send = app.can_send(cx);
        atoms::circle_button(
            cx,
            "send",
            IconName::ArrowUp,
            atoms::Plate::Accent,
            "Send",
            can_send,
            cx.listener(|this, _, window, cx| {
                let input = this.input.clone();
                this.send(&input, window, cx);
            }),
        )
    };

    h_flex()
        .w_full()
        .max_w(theme::CONTENT_MAX)
        .items_end()
        .gap(px(8.))
        .px(px(14.))
        .py(px(11.))
        .rounded(theme::RADIUS_COMPOSER)
        .border_1()
        .border_color(theme::hairline())
        .bg(theme::surface())
        .shadow_lg()
        .child(
            // The 4px of vertical padding centres a single line of text
            // against the action button once the row is bottom-aligned.
            div().flex_1().min_w_0().py(px(4.)).child(
                Textarea::new(&app.input)
                    .appearance(false)
                    .bordered(false)
                    .text_size(theme::TEXT_BODY),
            ),
        )
        .child(action)
        .into_any_element()
}
