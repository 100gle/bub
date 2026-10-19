//! The window's top bar.
//!
//! It is the system title bar, not a band inside the window: the window opens
//! with `appears_transparent`, so the traffic lights float over this row and
//! the whole strip drags. That leaves one chrome line instead of two, which is
//! why the bar carries no background or rule of its own.
//!
//! What it shows is deliberately thin: the wordmark, whether a turn is running,
//! and the one thing that can go wrong. Everything else the old bar carried —
//! the model, the connection dot beside it, the workspace path, the protocol
//! version, the settings gear, the panel toggle — was a fact wearing the clothes
//! of navigation. The model already has a home on the composer's own picker and
//! in Settings; a green dot next to the word "connected" is a sentence saying
//! nothing; the panel it toggled was a second copy of the transcript; and the
//! gear opened a page that the rail now simply goes to. Only the failure
//! survives here: when the bridge stops answering, the bar says so.

use gpui_kit::component::{h_flex, spinner::Spinner, StyledExt as _, TitleBar};
use gpui_kit::prelude::FluentBuilder as _;
use gpui_kit::{div, px, AnyElement, IntoElement, ParentElement as _, Styled as _};

use crate::app::DesktopApp;
use crate::theme;
use crate::ui::atoms;

impl DesktopApp {
    pub(crate) fn titlebar(&self) -> impl IntoElement {
        TitleBar::new()
            .bg(theme::canvas())
            .border_color(theme::canvas())
            .child(
                h_flex()
                    .w_full()
                    .items_center()
                    .justify_between()
                    .gap(px(12.))
                    .child(
                        h_flex()
                            .flex_shrink_0()
                            .items_center()
                            .gap(px(8.))
                            .child(
                                div()
                                    .text_size(theme::TEXT_UI)
                                    .font_semibold()
                                    .text_color(theme::ink())
                                    .child("Bub"),
                            )
                            // The one thing this row still owes a reader is that
                            // a turn is running, and only while it is: the
                            // spinner is the whole status report.
                            .when(self.generating, |this| {
                                this.child(Spinner::new().into_any_element())
                            }),
                    )
                    .child(status(self.connected, self.ready)),
            )
    }
}

/// How the chrome reports the bridge.
///
/// Three states, three weights: connected says nothing at all; a bridge that has
/// not answered yet is a quiet caption, because that is what a launch looks
/// like; and only a bridge that answered and then stopped gets a badge, because
/// only that is a failure a reader has to act on.
fn status(connected: bool, ready: bool) -> AnyElement {
    if connected {
        return div().into_any_element();
    }
    if !ready {
        return div()
            .pr(px(2.))
            .text_size(theme::TEXT_MICRO)
            .text_color(theme::ink_tertiary())
            .child("Connecting…")
            .into_any_element();
    }
    offline_badge()
}

/// The one failure the chrome reports: the bridge stopped answering.
fn offline_badge() -> AnyElement {
    h_flex()
        .items_center()
        .gap(px(5.))
        .pl(px(7.))
        .pr(px(9.))
        .py(px(3.))
        .rounded(px(9999.))
        .bg(theme::warning_wash())
        .child(atoms::dot(theme::warning()))
        .child(
            div()
                .text_size(theme::TEXT_MICRO)
                .font_medium()
                .text_color(theme::warning())
                .child("Offline"),
        )
        .into_any_element()
}
