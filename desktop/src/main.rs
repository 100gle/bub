//! bub desktop — a GPUI front-end for the Bub runtime.
//!
//! The UI holds no Bub logic: it spawns the `bub desktop` bridge as a child
//! process, speaks newline-delimited JSON with it over stdio, and renders the
//! events it receives.
//!
//! | module | owns |
//! | --- | --- |
//! | [`app`] | state, the protocol, and the top-level composition |
//! | [`bridge`] | the child process and its stdout stream |
//! | [`model`] | the data vocabulary the two sides share |
//! | [`theme`] | the palette, metrics, and type scale |
//! | [`ui`] | one module per surface of the shell |
//!
//! The `shot` feature adds [`shot`], a development-only path that paints one
//! frame to a PNG; see that module for why it has to be a feature.
//!
//! One window, one title bar: the window opens with its system title bar
//! hidden (`appears_transparent`) so the traffic lights sit inside the app's
//! own top row, and the shell draws only what a reader needs — a transcript and
//! a composer. Everything else is behind ⌘,.

mod app;
mod bridge;
mod model;
#[cfg(feature = "shot")]
mod shot;
mod theme;
mod ui;

use gpui_kit::component::TitleBar;
use gpui_kit::{actions, px, size, AppContext as _, KeyBinding, WindowOptions};

use app::DesktopApp;

actions!(bub, [OpenSettings, CloseSettings]);

/// Key context for window-level commands that must work while typing.
pub(crate) const BUB_CONTEXT: &str = "Bub";

fn main() {
    // Register the icon catalog before opening a window. Neither `gpui_kit::init`
    // nor the component library does this: an unregistered source leaves every
    // `Icon` and `Button` glyph blank rather than failing, which is easy to miss.
    // The full catalog is used because the default bundle carries neither the
    // tool-status icons (`Wrench`, `Terminal`, `Brain`) nor `Sparkles`.
    gpui_kit::application()
        .with_assets(gpui_kit::assets::AllAssets)
        .run(|cx| {
            gpui_kit::init(cx);
            cx.bind_keys([
                // ⌘, is the platform convention for Settings on macOS.
                KeyBinding::new("cmd-,", OpenSettings, None),
                KeyBinding::new("escape", CloseSettings, Some(BUB_CONTEXT)),
            ]);
            let options = WindowOptions {
                // Below this the rail and the reading measure start fighting;
                // the window is barred from shrinking into a state with no good
                // layout rather than left to reflow into a bad one.
                window_min_size: Some(size(px(900.), px(560.))),
                ..TitleBar::window_options()
            };
            // The application owns the window and its view from here on; the
            // `shot` feature is the only caller that needs handles to them.
            let (window, app) = gpui_kit::open_window(options, cx, |window, cx| {
                cx.new(|cx| DesktopApp::new(window, cx))
            })
            .expect("open window");

            #[cfg(feature = "shot")]
            shot::arm(window, &app, cx);

            #[cfg(not(feature = "shot"))]
            let _ = (window, app);
        });
}
