//! The window's top bar.
//!
//! It is the system title bar, not a band inside the window: the window opens
//! with `appears_transparent`, so the traffic lights float over this row and
//! the whole strip drags. That leaves one chrome line instead of two, which is
//! why the bar carries no background or rule of its own.
//!
//! What it shows is deliberately thin — the wordmark, the model, how the
//! session is doing, and the panel toggle. Everything the old header carried
//! (the workspace path, the protocol version, a tool count) was metadata
//! pretending to be navigation, and now lives in the agent panel where it can
//! be looked up rather than read past.

use gpui_kit::assets::IconName;
use gpui_kit::component::{h_flex, spinner::Spinner, StyledExt as _, TitleBar};
use gpui_kit::{div, px, Context, IntoElement, ParentElement as _, Styled as _};

use crate::app::DesktopApp;
use crate::theme;
use crate::ui::atoms;

impl DesktopApp {
    pub(crate) fn titlebar(&self, cx: &mut Context<Self>) -> impl IntoElement {
        let toggle = if self.inspector_open {
            IconName::PanelRightClose
        } else {
            IconName::PanelRight
        };

        // The session light doubles as the working indicator: while a turn is
        // running there is nothing useful to say about connectivity, and a
        // second dedicated slot would only add noise to a 34px row.
        let light = if self.generating {
            Spinner::new().into_any_element()
        } else {
            atoms::dot(if self.connected {
                theme::success()
            } else {
                theme::warning()
            })
            .into_any_element()
        };

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
                        div()
                            .flex_shrink_0()
                            .text_size(theme::TEXT_UI)
                            .font_semibold()
                            .text_color(theme::ink())
                            .child("Bub"),
                    )
                    .child(
                        h_flex()
                            .flex_shrink_0()
                            .items_center()
                            .gap(px(6.))
                            .child(
                                h_flex()
                                    .min_w_0()
                                    .max_w(px(240.))
                                    .items_center()
                                    .gap(px(8.))
                                    .child(light)
                                    .child(
                                        div()
                                            .min_w_0()
                                            .truncate()
                                            .text_size(theme::TEXT_CAPTION)
                                            .text_color(theme::ink_tertiary())
                                            .child(self.model_label()),
                                    ),
                            )
                            .child(atoms::icon_button(
                                cx,
                                "toggle-inspector",
                                toggle,
                                "Agent panel (⌘I)",
                                cx.listener(|this, _, _, cx| this.toggle_inspector(cx)),
                            )),
                    ),
            )
    }

    /// The model line: the id once known, a quiet placeholder until then.
    fn model_label(&self) -> String {
        if self.model.is_empty() {
            "connecting…".to_string()
        } else {
            self.model.clone()
        }
    }
}
