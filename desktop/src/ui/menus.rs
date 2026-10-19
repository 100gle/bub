//! Floating menus.
//!
//! Three controls in the window open a menu — the composer's permission picker
//! and model picker, and the title bar's gear — and the composer opens a fourth
//! (the command palette) when a message starts with the command prefix. They all
//! share one surface: a rounded card on [`theme::surface`] that the component
//! library anchors to its trigger and dismisses on an outside click, restyled
//! here so it belongs to this window's palette rather than the library's.
//!
//! The rows are ordinary stateful divs rather than `Button`s: a menu row is read
//! far more than pressed, and it needs a leading glyph, a two-line body and a
//! trailing check on its own terms.

use gpui_kit::assets::IconName;
use gpui_kit::component::{
    h_flex, popover::Popover, v_flex, Icon, Selectable, Sizable as _, StyledExt as _,
};
use gpui_kit::prelude::FluentBuilder as _;
use gpui_kit::{
    div, px, Anchor, AnyElement, App, ClickEvent, InteractiveElement as _, IntoElement,
    ParentElement as _, StatefulInteractiveElement as _, Styled as _, Window,
};

use crate::theme;

/// A popover styled as this window's menu surface.
///
/// The library draws a theme popover by default; the style set here refines it
/// afterwards, so the card keeps the library's anchoring, focus handling and
/// outside-click dismissal while taking this app's colours, radius and padding.
pub(crate) fn menu(
    id: impl Into<gpui_kit::ElementId>,
    anchor: Anchor,
    trigger: impl Selectable + IntoElement + 'static,
    content: impl Fn(&mut Window, &mut App) -> AnyElement + 'static,
) -> Popover {
    Popover::new(id)
        .anchor(anchor)
        .offset(px(8.))
        .trigger(trigger)
        .bg(theme::surface())
        .border_1()
        .border_color(theme::hairline())
        .rounded(theme::RADIUS_CARD)
        .shadow_lg()
        .p(px(5.))
        .content(move |_, window, cx| (content)(window, cx))
}

/// The column a menu's rows sit in, bounded to a menu width.
pub(crate) fn column() -> gpui_kit::Div {
    v_flex()
        .min_w(theme::MENU_MIN_W)
        .max_w(theme::MENU_MAX_W)
        .gap(px(1.))
}

/// A quiet caption at the top of a group of rows.
pub(crate) fn heading(text: impl Into<String>) -> AnyElement {
    div()
        .px(px(8.))
        .pt(px(6.))
        .pb(px(3.))
        .text_size(theme::TEXT_MICRO)
        .font_medium()
        .text_color(theme::ink_tertiary())
        .child(text.into())
        .into_any_element()
}

/// One actionable menu row.
pub(crate) fn action(
    id: impl Into<gpui_kit::ElementId>,
    icon: Option<IconName>,
    label: &str,
    detail: Option<String>,
    selected: bool,
    on_click: impl Fn(&ClickEvent, &mut Window, &mut App) + 'static,
) -> AnyElement {
    let mut row = h_flex()
        .id(id)
        .w_full()
        .items_center()
        .gap(px(10.))
        .px(px(8.))
        .py(px(7.))
        .rounded(px(7.))
        .cursor_pointer()
        .hover(|style| style.bg(theme::hover()))
        .on_click(on_click);

    if let Some(icon) = icon {
        row = row.child(
            Icon::new(icon)
                .text_color(theme::ink_secondary())
                .with_size(px(15.)),
        );
    }

    row = row.child(
        v_flex()
            .flex_1()
            .min_w_0()
            .gap(px(1.))
            .child(
                div()
                    .w_full()
                    .truncate()
                    .text_size(theme::TEXT_UI)
                    .text_color(theme::ink())
                    .child(label.to_string()),
            )
            .when_some(detail.filter(|text| !text.is_empty()), |this, detail| {
                this.child(
                    div()
                        .w_full()
                        .truncate()
                        .text_size(theme::TEXT_MICRO)
                        .text_color(theme::ink_tertiary())
                        .child(detail),
                )
            }),
    );

    row = row.child(div().flex_shrink_0().w(px(14.)).child(if selected {
        Icon::new(IconName::Check)
            .text_color(theme::accent())
            .with_size(px(14.))
            .into_any_element()
    } else {
        div().into_any_element()
    }));

    row.into_any_element()
}
