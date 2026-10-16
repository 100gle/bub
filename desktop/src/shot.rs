//! Paint one frame to a PNG, for reviewing the shell on a machine that cannot
//! screen-capture a window.
//!
//! GPUI renders every frame into a Metal texture before it presents it, and
//! `Window::render_to_image` reads that texture back — headless in the sense
//! that it never needs the window to be composited to a display. It is gated
//! behind gpui's `test-support` feature, so this whole module is behind the
//! crate's `shot` feature and the app is unchanged without it.
//!
//! Enable it with environment variables, since the app takes no arguments:
//!
//! | variable | meaning |
//! | --- | --- |
//! | `BUB_SHOT` | output path. Without it this module does nothing at all. |
//! | `BUB_SHOT_AFTER` | ms to wait before touching anything (default 900). |
//! | `BUB_SHOT_PROMPT` | text to start a run with, to fill the transcript. |
//! | `BUB_SHOT_SETTLE` | ms to wait after that run starts (default 1900). |
//! | `BUB_SHOT_INSPECTOR` | capture with the agent panel open. |
//! | `BUB_SHOT_TAB` | `tools` to capture the panel's other half. |
//!
//! A typical review pass:
//!
//! ```text
//! BUB_DESKTOP_CMD="python3 tools/fake_bridge.py" FAKE_MODE=demo \
//! BUB_SHOT=/tmp/shell.png BUB_SHOT_PROMPT="Add a --dry-run flag to deploy" \
//!   cargo run --features shot
//! ```

use std::env;
use std::path::PathBuf;
use std::time::Duration;

use gpui_kit::{AnyWindowHandle, App, AppContext as _, Entity, Window};

use crate::app::DesktopApp;
use crate::model::InspectorTab;

/// Hands the app a one-shot capture task, or does nothing when `BUB_SHOT` is
/// unset. Called once, with the window already open, from `main`.
pub(crate) fn arm(handle: AnyWindowHandle, app: &Entity<DesktopApp>, cx: &mut App) {
    let Some(path) = env::var_os("BUB_SHOT").map(PathBuf::from) else {
        return;
    };
    let app = app.clone();

    cx.spawn(async move |cx| {
        // Let the bridge connect and the first frame settle: a capture taken
        // before `ready` arrives would show the offline state instead.
        wait(cx, millis("BUB_SHOT_AFTER", 900)).await;

        if let Some(prompt) = env::var("BUB_SHOT_PROMPT").ok().filter(|p| !p.is_empty()) {
            cx.update(|cx| {
                app.update(cx, |app, cx| app.prompt_directly(&prompt, cx));
            });
            wait(cx, millis("BUB_SHOT_SETTLE", 1900)).await;
        }

        if env::var_os("BUB_SHOT_INSPECTOR").is_some() {
            let tab = match env::var("BUB_SHOT_TAB").as_deref() {
                Ok("tools") => InspectorTab::Tools,
                _ => InspectorTab::Activity,
            };
            cx.update(|cx| {
                app.update(cx, |app, cx| {
                    app.toggle_inspector(cx);
                    app.inspector_tab = tab;
                    cx.notify();
                });
            });
            // The panel resizes into place; give it a beat so the captured
            // frame has the final column widths rather than the opening ones.
            wait(cx, Duration::from_millis(250)).await;
        }

        let shot = cx
            .update_window(handle, |_, window: &mut Window, cx| {
                // Paint explicitly rather than trusting the run loop: a window
                // this machine never composites to a display gets no display
                // link, so no frame is ever scheduled and `render_to_image`
                // would keep handing back the scene from the first one.
                window.refresh();
                window.draw(cx).clear(cx);
                window.render_to_image().map_err(|error| error.to_string())
            })
            .unwrap_or_else(|error| Err(error.to_string()));

        match shot {
            Ok(image) => match image.save(&path) {
                Ok(()) => println!(
                    "bub-desktop: captured {}x{} to {}",
                    image.width(),
                    image.height(),
                    path.display()
                ),
                Err(error) => eprintln!("bub-desktop: cannot write {}: {error}", path.display()),
            },
            Err(error) => eprintln!("bub-desktop: cannot capture the frame: {error}"),
        }

        // Always quit, so a failed capture does not leave the app waiting on a
        // window nobody is looking at.
        cx.update(|cx| cx.quit());
    })
    .detach();
}

/// A real-time pause on the executor that drives the render loop.
async fn wait(cx: &mut gpui_kit::AsyncApp, duration: Duration) {
    cx.background_executor().timer(duration).await;
}

fn millis(name: &str, fallback: u64) -> Duration {
    Duration::from_millis(
        env::var(name)
            .ok()
            .and_then(|value| value.parse().ok())
            .unwrap_or(fallback),
    )
}
