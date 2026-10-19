//! The navigation rail.
//!
//! One column, always present, holding the two kinds of place this window has:
//! the conversation, and the settings pages. They are peers — the rail says
//! where you are and takes you somewhere else, and there is nothing to open and
//! nothing to close, because moving between them is navigation rather than a
//! modal that has to be dismissed.
//!
//! It replaced a gear in the title bar that opened a full-window settings page
//! with an X on it. That arrangement made settings a thing you *enter* and
//! *leave*; this one makes it a place you are, which is what it always was.

use gpui_kit::assets::IconName;
use gpui_kit::component::{h_flex, v_flex, Icon, Sizable as _, StyledExt as _};
use gpui_kit::prelude::FluentBuilder as _;
use gpui_kit::{
    div, px, AnyElement, ClickEvent, Context, ElementId, InteractiveElement as _, IntoElement,
    ParentElement as _, StatefulInteractiveElement as _, Styled as _, Window,
};

use crate::app::DesktopApp;
use crate::model::SettingsSection;
use crate::theme;

impl DesktopApp {
    /// The rail, top to bottom.
    pub(crate) fn sidebar(&self, cx: &mut Context<Self>) -> AnyElement {
        let mut rail = v_flex()
            .w(theme::SIDEBAR)
            .h_full()
            .flex_shrink_0()
            .gap(px(1.))
            .px(px(10.))
            .py(px(12.))
            .bg(theme::sidebar())
            .border_r_1()
            .border_color(theme::hairline());

        let conversation = self.view.is_conversation();
        rail = rail.child(row(
            "nav-conversation",
            IconName::MessageSquare,
            "Conversation",
            conversation,
            cx.listener(|this, _, _, cx| this.close_settings(cx)),
        ));

        // A group heading rather than a divider: the settings pages are not
        // siblings of the conversation, they are the other half of the window.
        rail = rail.child(
            div()
                .px(px(9.))
                .pt(px(16.))
                .pb(px(5.))
                .text_size(theme::TEXT_MICRO)
                .font_medium()
                .text_color(theme::ink_tertiary())
                .child("Settings"),
        );

        for section in SettingsSection::ALL {
            let selected = !conversation && self.settings_section == section;
            rail = rail.child(row(
                format!("nav-{section:?}"),
                section.icon(),
                section.label(),
                selected,
                cx.listener(move |this, _, _, cx| this.select_settings_section(section, cx)),
            ));
        }

        rail.into_any_element()
    }
}

/// One row of the rail: a glyph and a name, filled in when it is where you are.
fn row(
    id: impl Into<ElementId>,
    icon: IconName,
    label: &str,
    selected: bool,
    on_click: impl Fn(&ClickEvent, &mut Window, &mut gpui_kit::App) + 'static,
) -> AnyElement {
    h_flex()
        .id(id)
        .w_full()
        .items_center()
        .gap(px(9.))
        .px(px(9.))
        .py(px(6.))
        .rounded(px(8.))
        .when(selected, |this| this.bg(theme::accent_wash()))
        .when(!selected, |this| {
            this.cursor_pointer().hover(|style| style.bg(theme::fill()))
        })
        .child(
            Icon::new(icon)
                .text_color(if selected {
                    theme::ink()
                } else {
                    theme::ink_secondary()
                })
                .with_size(px(15.)),
        )
        .child(
            div()
                .text_size(theme::TEXT_UI)
                .when(selected, |this| this.font_medium())
                .text_color(if selected {
                    theme::ink()
                } else {
                    theme::ink_secondary()
                })
                .child(label.to_string()),
        )
        .on_click(on_click)
        .into_any_element()
}
