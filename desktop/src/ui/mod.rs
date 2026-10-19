//! The window's views.
//!
//! Each module here is one surface of the shell and adds its `impl DesktopApp`
//! methods, so [`crate::app::DesktopApp::render`] reads as a table of contents
//! rather than a wall of element builders. Shared primitives live in
//! [`atoms`]; every colour comes from [`crate::theme`].
//!
//! There is no inspector module any more. The agent panel that used to live
//! here was a second view of the transcript — every tool call it traced was
//! already a row in the conversation, with the same status — and a user had no
//! reason to open it. What it held that a reader could not see elsewhere (the
//! capabilities the runtime discovered) now sits in [`settings`], where
//! configuration belongs.

pub(crate) mod atoms;
mod composer;
mod empty;
mod menus;
pub(crate) mod settings;
mod sidebar;
mod titlebar;
mod transcript;
