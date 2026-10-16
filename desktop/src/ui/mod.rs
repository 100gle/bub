//! The window's views.
//!
//! Each module here is one surface of the shell and adds its `impl DesktopApp`
//! methods, so [`crate::app::DesktopApp::render`] reads as a table of contents
//! rather than a wall of element builders. Shared primitives live in
//! [`atoms`]; every colour comes from [`crate::theme`].

pub(crate) mod atoms;
mod composer;
mod empty;
mod inspector;
mod titlebar;
mod transcript;
