//! The desktop shell's visual language.
//!
//! Every colour the app paints comes from here, not from gpui-component's
//! semantic tokens. The component theme is a shadcn-flavoured neutral scale
//! whose "primary" is near-black and whose borders are opaque greys; this
//! window wants something quieter — a white canvas, one accent, and separators
//! drawn as a fraction of black so they sit correctly on any surface.
//!
//! Keeping the palette in one module is also what makes the layout code
//! readable: `theme::ink_secondary()` says what a colour is *for*, where
//! `cx.theme().muted_foreground` only says which slot of the component theme it
//! happens to occupy.

use gpui_kit::{px, rgb, rgba, Hsla, Pixels};

/// An opaque colour from a `0xRRGGBB` literal.
fn solid(hex: u32) -> Hsla {
    Hsla::from(rgb(hex))
}

/// A translucent colour from a `0xRRGGBBAA` literal.
fn veiled(hex: u32) -> Hsla {
    Hsla::from(rgba(hex))
}

// ---- Surfaces -------------------------------------------------------------

/// The window background, and the only surface the transcript is drawn on.
pub fn canvas() -> Hsla {
    solid(0xFFFFFF)
}

/// A raised surface sitting on the canvas: the composer, tool cards.
pub fn surface() -> Hsla {
    solid(0xFFFFFF)
}

/// A recessed surface: the agent panel, code payloads.
pub fn recess() -> Hsla {
    solid(0xF7F7F8)
}

/// A quiet fill for user bubbles, chips, and the grooves of controls.
pub fn fill() -> Hsla {
    solid(0xF0F0F3)
}

/// The recessed rail behind the settings navigation, one step below the canvas
/// but a shade cooler than [`recess`] so a full-bleed page reads as chrome
/// rather than as a message surface.
pub fn sidebar() -> Hsla {
    solid(0xF5F5F6)
}

/// The wash under a menu row on hover: one step lighter than [`fill`] because
/// menu rows sit on [`surface`], not on [`recess`].
pub fn hover() -> Hsla {
    solid(0xF2F2F4)
}

/// `fill` one step darker, for hover and pressed states.
pub fn fill_strong() -> Hsla {
    solid(0xE3E3E8)
}

// ---- Text -----------------------------------------------------------------

/// Primary text.
pub fn ink() -> Hsla {
    solid(0x1D1D1F)
}

/// Supporting text: descriptions, metadata, inactive tabs.
pub fn ink_secondary() -> Hsla {
    solid(0x6E6E73)
}

/// The quietest text: captions and placeholders.
pub fn ink_tertiary() -> Hsla {
    solid(0x9A9AA2)
}

/// Text drawn on top of [`accent`].
pub fn on_accent() -> Hsla {
    solid(0xFFFFFF)
}

// ---- Lines ----------------------------------------------------------------

/// A 1px separator. Expressed as a wash of black so it reads the same on the
/// canvas, on [`recess`], and on [`fill`].
pub fn hairline() -> Hsla {
    veiled(0x00000014)
}

/// A hairline with a little more body, for the edge of a control that sits on a
/// near-white surface — a white field or a white chip. [`hairline`] alone
/// disappears against white; this holds the shape without becoming a rule.
pub fn edge() -> Hsla {
    veiled(0x0000001A)
}

// ---- Primary ---------------------------------------------------------------

/// The filled primary control: the send plate and the chosen segment.
///
/// Near-black rather than the blue of [`accent`], which is the shape a native
/// Mac control takes — a filled default button reads as ink, and colour is
/// spent on state (focus, selection, the model light) instead of on every
/// action. It is also what keeps a window with a model picker, a permission
/// menu and a settings page from turning into four competing blues.
pub fn primary() -> Hsla {
    solid(0x1D1D1F)
}

pub fn primary_hover() -> Hsla {
    solid(0x2C2C2E)
}

pub fn primary_active() -> Hsla {
    solid(0x0A0A0B)
}

/// Text and glyphs drawn on top of [`primary`].
pub fn on_primary() -> Hsla {
    solid(0xFFFFFF)
}

// ---- Accent and state -----------------------------------------------------

/// The single interactive colour in the window: focus rings, links, and the
/// selected row in a menu. Filled actions use [`primary`] instead.
pub fn accent() -> Hsla {
    solid(0x0071E3)
}

pub fn accent_hover() -> Hsla {
    solid(0x0077ED)
}

pub fn accent_active() -> Hsla {
    solid(0x0068CF)
}

/// A very light wash of [`accent`], for a selected menu row.
pub fn accent_wash() -> Hsla {
    veiled(0x0071E30F)
}

// ---- State ----------------------------------------------------------------

pub fn success() -> Hsla {
    solid(0x30D158)
}

pub fn warning() -> Hsla {
    solid(0xFF9F0A)
}

pub fn danger() -> Hsla {
    solid(0xFF3B30)
}

/// The danger colour at wash strength, for error cards.
pub fn danger_wash() -> Hsla {
    veiled(0xFF3B3012)
}

/// The warning colour at wash strength, for the offline badge in the chrome.
pub fn warning_wash() -> Hsla {
    veiled(0xFF9F0A14)
}

// ---- Metrics --------------------------------------------------------------

/// Corner radii, in the order they nest: a row inside a card inside a page.
pub const RADIUS_CONTROL: Pixels = px(9.);
pub const RADIUS_CARD: Pixels = px(12.);
pub const RADIUS_BUBBLE: Pixels = px(18.);
pub const RADIUS_COMPOSER: Pixels = px(22.);
pub const RADIUS_MARK: Pixels = px(17.);

/// The widest the transcript and the composer ever grow. Both are centred in
/// whatever column they are given, so a wide window keeps a readable measure
/// instead of stretching lines to the panel.
pub const CONTENT_MAX: Pixels = px(720.);

/// The navigation rail: how wide it is, and how wide the reading column on a
/// settings page is bounded to once the rail is subtracted.
pub const SIDEBAR: Pixels = px(208.);
pub const SETTINGS_MAX: Pixels = px(640.);

/// Floating menus: how narrow a picker may get before its labels truncate.
pub const MENU_MIN_W: Pixels = px(232.);
pub const MENU_MAX_W: Pixels = px(360.);

/// The composer's circular action button.
pub const ACTION: Pixels = px(28.);

/// A pill control inside the composer's toolbar (attach, permissions, model).
pub const TOOLBAR_H: Pixels = px(28.);

/// The two switch dimensions, chosen so the knob is a whole circle inside the
/// track: a 36x20 track with 2px of padding holds a 16px knob exactly.
pub const SWITCH_W: Pixels = px(36.);
pub const SWITCH_H: Pixels = px(20.);
pub const SWITCH_KNOB: Pixels = px(16.);

// ---- Type -----------------------------------------------------------------

pub const TEXT_DISPLAY: Pixels = px(22.);
pub const TEXT_TITLE: Pixels = px(17.);
pub const TEXT_BODY: Pixels = px(15.);
pub const TEXT_UI: Pixels = px(13.);
pub const TEXT_CAPTION: Pixels = px(12.);
pub const TEXT_MICRO: Pixels = px(11.);
