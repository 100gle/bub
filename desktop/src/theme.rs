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

/// The fill of a label chip.
///
/// A step darker than [`fill`], because chips sit on [`recess`] rather than on
/// the canvas: at `fill` they would be a 7-level difference and read as text
/// with a rumour of a box around it.
pub fn tag() -> Hsla {
    solid(0xE9E9EB)
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

// ---- Accent and state -----------------------------------------------------

/// The single interactive colour in the window.
pub fn accent() -> Hsla {
    solid(0x0071E3)
}

pub fn accent_hover() -> Hsla {
    solid(0x0077ED)
}

pub fn accent_active() -> Hsla {
    solid(0x0068CF)
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

// ---- Metrics --------------------------------------------------------------

/// Corner radii, in the order they nest: a chip inside a card inside a panel.
pub const RADIUS_CHIP: Pixels = px(6.);
pub const RADIUS_CONTROL: Pixels = px(9.);
pub const RADIUS_CARD: Pixels = px(12.);
pub const RADIUS_BUBBLE: Pixels = px(18.);
pub const RADIUS_COMPOSER: Pixels = px(22.);
pub const RADIUS_MARK: Pixels = px(17.);

/// The widest the transcript and the composer ever grow. Both are centred in
/// whatever column they are given, so a wide window keeps a readable measure
/// instead of stretching lines to the panel.
pub const CONTENT_MAX: Pixels = px(720.);

/// The agent panel's default, minimum, and maximum width.
pub const PANEL_WIDTH: Pixels = px(320.);
pub const PANEL_MIN: Pixels = px(272.);
pub const PANEL_MAX: Pixels = px(460.);

/// The square hit area of a bare icon button.
pub const CONTROL: Pixels = px(28.);

/// The composer's circular action button.
pub const ACTION: Pixels = px(28.);

/// The two switch dimensions, chosen so the knob is a whole circle inside the
/// track: a 36x20 track with 2px of padding holds a 16px knob exactly.
pub const SWITCH_W: Pixels = px(36.);
pub const SWITCH_H: Pixels = px(20.);
pub const SWITCH_KNOB: Pixels = px(16.);

// ---- Type -----------------------------------------------------------------

pub const TEXT_DISPLAY: Pixels = px(22.);
pub const TEXT_BODY: Pixels = px(15.);
pub const TEXT_UI: Pixels = px(13.);
pub const TEXT_CAPTION: Pixels = px(12.);
pub const TEXT_MICRO: Pixels = px(11.);
