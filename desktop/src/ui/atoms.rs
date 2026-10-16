//! The small elements more than one view needs.
//!
//! These wrap gpui-component's controls in this app's palette. A plate — the
//! filled shape under a button — is painted through the button's own style
//! rather than through the library's custom variant; [`plated`] explains why.

use gpui_kit::assets::IconName;
use gpui_kit::component::{
    button::{Button, ButtonCustomVariant, ButtonRounded, ButtonVariants as _},
    scroll::ScrollableElement as _,
    ActiveTheme as _, Disableable as _, Icon, Sizable as _, StyledExt as _,
};
use gpui_kit::prelude::FluentBuilder as _;
use gpui_kit::{
    div, px, AnyElement, App, ClickEvent, Div, ElementId, Hsla, InteractiveElement as _,
    IntoElement as _, ParentElement as _, Styled as _, Window,
};

use crate::theme;

/// Which plate a control sits on.
#[derive(Clone, Copy, PartialEq)]
pub(crate) enum Plate {
    /// The single accent colour: the one action that matters.
    Accent,
    /// A quiet fill: secondary actions and icon controls.
    Quiet,
    /// A label chip, which is read rather than pressed.
    Tag,
    /// No plate at all until the pointer arrives.
    Bare,
}

/// The colours one plate is painted from.
#[derive(Clone, Copy)]
struct Skin {
    /// The resting fill.
    plate: Hsla,
    /// The fill under the pointer.
    hover: Hsla,
    /// The fill while the button is held.
    press: Hsla,
    /// The text and icons drawn on the plate.
    ink: Hsla,
}

impl Skin {
    fn of(cx: &App, which: Plate, enabled: bool) -> Self {
        match (which, enabled) {
            (Plate::Accent, true) => Self {
                plate: theme::accent(),
                hover: theme::accent_hover(),
                press: theme::accent_active(),
                ink: theme::on_accent(),
            },
            (Plate::Quiet, true) => Self {
                plate: theme::fill(),
                hover: theme::fill_strong(),
                press: theme::fill_strong(),
                ink: theme::ink(),
            },
            (Plate::Tag, true) => Self {
                plate: theme::tag(),
                hover: theme::fill_strong(),
                press: theme::fill_strong(),
                ink: theme::ink_secondary(),
            },
            (Plate::Bare, true) => Self {
                plate: cx.theme().transparent,
                hover: theme::fill(),
                press: theme::fill_strong(),
                ink: theme::ink_secondary(),
            },
            // A disabled control keeps its shape and gives up its colour, so an
            // accent plate turns grey rather than washing out to pale blue.
            (Plate::Accent | Plate::Quiet | Plate::Tag, false) => Self {
                plate: theme::fill(),
                hover: theme::fill(),
                press: theme::fill(),
                ink: theme::ink_tertiary(),
            },
            (Plate::Bare, false) => Self {
                plate: cx.theme().transparent,
                hover: cx.theme().transparent,
                press: cx.theme().transparent,
                ink: theme::ink_tertiary(),
            },
        }
    }
}

/// Give a button a solid plate.
///
/// Not through [`ButtonCustomVariant`]'s `color`, which the library paints at
/// 20% alpha and only fills in on hover: an accent control would arrive as a
/// pale wash and then jump to full colour under the pointer. The resting fill
/// is therefore set on the button's own style, which the library refines into
/// every state last — including `disabled`, where the library would otherwise
/// override a caller's plate with its own faded one. The variant keeps the
/// pointer states, since those live outside the base style.
fn plated(button: Button, cx: &App, which: Plate, enabled: bool) -> Button {
    let skin = Skin::of(cx, which, enabled);
    button
        .custom(
            ButtonCustomVariant::new(cx)
                .color(skin.plate)
                .foreground(skin.ink)
                .hover(skin.hover)
                .active(skin.press),
        )
        .bg(skin.plate)
        .text_color(skin.ink)
}

/// A square icon control. Sized so the icon lands at 14px inside 28px.
pub(crate) fn icon_button(
    cx: &App,
    id: impl Into<ElementId>,
    icon: IconName,
    tooltip: &'static str,
    on_click: impl Fn(&ClickEvent, &mut Window, &mut App) + 'static,
) -> Button {
    plated(Button::new(id), cx, Plate::Bare, true)
        .small()
        .w(theme::CONTROL)
        .h(theme::CONTROL)
        .rounded(ButtonRounded::Size(theme::RADIUS_CONTROL))
        .icon(icon)
        .tooltip(tooltip)
        .on_click(on_click)
}

/// A circular icon control, the shape of the composer's one action.
pub(crate) fn circle_button(
    cx: &App,
    id: impl Into<ElementId>,
    icon: IconName,
    which: Plate,
    tooltip: &'static str,
    enabled: bool,
    on_click: impl Fn(&ClickEvent, &mut Window, &mut App) + 'static,
) -> Button {
    // The stop glyph is a solid square, so it reads heavier than a stroked icon
    // at the same size; step it down one size to keep the circle calm.
    let button = if icon == IconName::Square {
        Button::new(id).xsmall()
    } else {
        Button::new(id).small()
    };
    plated(button, cx, which, enabled)
        .w(theme::ACTION)
        .h(theme::ACTION)
        .rounded(ButtonRounded::Size(theme::ACTION * 0.5))
        .icon(icon)
        .tooltip(tooltip)
        .disabled(!enabled)
        .on_click(on_click)
}

/// A labelled pill button, used for approval verdicts and the empty state's
/// suggestions.
pub(crate) fn pill_button(
    cx: &App,
    id: impl Into<ElementId>,
    label: &str,
    which: Plate,
    on_click: impl Fn(&ClickEvent, &mut Window, &mut App) + 'static,
) -> Button {
    plated(Button::new(id), cx, which, true)
        .small()
        .h(px(28.))
        .px(px(14.))
        .rounded(ButtonRounded::Size(px(9999.)))
        .text_size(theme::TEXT_UI)
        .label(label.to_string())
        .on_click(on_click)
}

/// A monospace payload, scrolled rather than clipped so nothing is lost.
pub(crate) fn code_block(id: impl Into<ElementId>, cx: &App, text: &str) -> AnyElement {
    div()
        .id(id)
        .w_full()
        .max_h(px(260.))
        .overflow_y_scrollbar()
        .font_family(cx.theme().mono_font_family.clone())
        .text_size(theme::TEXT_CAPTION)
        .line_height(px(18.))
        .text_color(theme::ink_secondary())
        .child(if text.is_empty() {
            "—".to_string()
        } else {
            text.to_string()
        })
        .into_any_element()
}

/// A named capability plate.
///
/// Capabilities are read far more often than they are inspected, so the panel
/// lists them as compact chips rather than as a paragraph each — and the
/// paragraph, where the runtime supplies one, becomes the tooltip. A chip is a
/// real button rather than a styled div so it keeps the library's hover,
/// press, focus and tooltip behaviour for free.
pub(crate) fn chip(
    cx: &App,
    id: impl Into<ElementId>,
    label: &str,
    icon: Option<IconName>,
    detail: Option<String>,
) -> Button {
    let mut chip = plated(Button::new(id), cx, Plate::Tag, true)
        .xsmall()
        .h(px(22.))
        .px(px(7.))
        .rounded(ButtonRounded::Size(theme::RADIUS_CHIP))
        .font_family(cx.theme().mono_font_family.clone())
        .text_size(theme::TEXT_MICRO)
        .label(label.to_string());

    if let Some(icon) = icon {
        chip = chip.icon(icon);
    }
    if let Some(detail) = detail.filter(|detail| !detail.is_empty()) {
        chip = chip.tooltip(detail);
    }
    chip
}

/// A dim caption, for metadata that should be legible but never loud.
pub(crate) fn caption(text: impl Into<String>) -> Div {
    div()
        .text_size(theme::TEXT_MICRO)
        .text_color(theme::ink_tertiary())
        .child(text.into())
}

/// The small heading above a group of rows in the agent panel.
pub(crate) fn group_title(text: impl Into<String>) -> Div {
    div()
        .text_size(theme::TEXT_MICRO)
        .font_medium()
        .text_color(theme::ink_tertiary())
        .child(text.into())
}

/// A 6px status light.
pub(crate) fn dot(color: Hsla) -> Div {
    div().w(px(6.)).h(px(6.)).rounded_full().bg(color)
}

/// The track and knob of an on/off control.
///
/// Built by hand rather than taken from the component library: its switch
/// paints its "on" state with the theme's near-black primary, where this one
/// wants the accent, and the geometry here matches the rest of the window.
pub(crate) fn switch_track(checked: bool) -> Div {
    div()
        .flex_shrink_0()
        .w(theme::SWITCH_W)
        .h(theme::SWITCH_H)
        .p(px(2.))
        .rounded_full()
        .bg(if checked {
            theme::accent()
        } else {
            theme::fill_strong()
        })
        .flex()
        .items_center()
        .when(checked, |this| this.justify_end())
        .child(
            div()
                .w(theme::SWITCH_KNOB)
                .h(theme::SWITCH_KNOB)
                .rounded_full()
                .bg(theme::surface())
                .shadow_xs(),
        )
}

/// The trailing chevron of a collapsible row.
pub(crate) fn disclosure_chevron(open: bool) -> Icon {
    Icon::new(if open {
        IconName::ChevronDown
    } else {
        IconName::ChevronRight
    })
    .text_color(theme::ink_tertiary())
}
