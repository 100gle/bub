//! The message composer.
//!
//! One capsule, four controls and a field. The field is the only thing that
//! grows; under it a single toolbar row carries everything that used to be
//! chrome elsewhere: the attach control, the permission the next turn runs
//! under, the model it runs on, and the action that starts or stops it. That is
//! the shape a chat composer takes on the desktop, and it keeps the reading
//! surface free of switches and pickers.
//!
//! What the running turn is doing is *not* here. It is drawn at the tail of the
//! answer it belongs to, in the transcript — see [`crate::ui::transcript`]. The
//! only thing this module says about itself is the answer to a keystroke that
//! had nowhere to go.
//!
//! The capsule is built once and placed twice: pinned under a running
//! transcript, or inside the empty state's hero.

use gpui_kit::assets::IconName;
use gpui_kit::component::{
    h_flex, input::Textarea, v_flex, ActiveTheme as _, Icon, Sizable as _, StyledExt as _,
};
use gpui_kit::prelude::FluentBuilder as _;
use gpui_kit::{
    div, px, Anchor, AnyElement, App, Context, Entity, InteractiveElement as _, IntoElement,
    ParentElement as _, StatefulInteractiveElement as _, Styled as _,
};

use crate::app::DesktopApp;
use crate::model::{ApprovalMode, Attachment, Command};
use crate::theme;
use crate::ui::{atoms, menus};

/// The capsule in its pinned position at the foot of the transcript column.
pub(crate) fn footer(app: &DesktopApp, cx: &mut Context<DesktopApp>) -> AnyElement {
    h_flex()
        .w_full()
        .justify_center()
        .px(px(28.))
        .pt(px(6.))
        .pb(px(22.))
        .child(
            v_flex()
                .relative()
                .w_full()
                .max_w(theme::CONTENT_MAX)
                .child(capsule(app, cx))
                // The answer to a swallowed Enter. It floats over the
                // transcript's fade rather than sitting in the column, so
                // nothing moves when it arrives and nothing moves when it goes.
                .when(app.blocked_hint(), |this| {
                    this.child(
                        h_flex()
                            .absolute()
                            .bottom_full()
                            .left(px(0.))
                            .right(px(0.))
                            .justify_center()
                            .pb(px(10.))
                            .child(blocked_pill()),
                    )
                }),
        )
        .into_any_element()
}

/// Why the keystroke went nowhere, and the one thing to do about it.
///
/// A dark plate rather than a tinted one: it is an interruption the reader did
/// not ask for, so it should be legible at a glance and gone in a beat — which
/// is what the ink plate and the two-and-a-half seconds are for.
fn blocked_pill() -> AnyElement {
    h_flex()
        .items_center()
        .gap(px(7.))
        .px(px(12.))
        .py(px(6.))
        .rounded(px(9999.))
        .bg(theme::primary())
        .shadow_lg()
        .child(
            Icon::new(IconName::CircleSlash)
                .text_color(theme::on_primary().opacity(0.65))
                .with_size(px(12.)),
        )
        .child(
            div()
                .text_size(theme::TEXT_CAPTION)
                .font_medium()
                .text_color(theme::on_primary())
                .child("Still working — press Stop to interrupt"),
        )
        .into_any_element()
}

/// The capsule inside the empty state, where the hero owns the spacing.
pub(crate) fn hero_field(app: &DesktopApp, cx: &mut Context<DesktopApp>) -> AnyElement {
    h_flex()
        .w_full()
        .justify_center()
        .child(capsule(app, cx))
        .into_any_element()
}

fn capsule(app: &DesktopApp, cx: &mut Context<DesktopApp>) -> AnyElement {
    let entity = cx.entity();
    let mut card = v_flex()
        .relative()
        .w_full()
        .max_w(theme::CONTENT_MAX)
        .gap(px(8.))
        .px(px(13.))
        .pt(px(11.))
        .pb(px(9.))
        .rounded(theme::RADIUS_COMPOSER)
        .border_1()
        .border_color(theme::hairline())
        .bg(theme::surface())
        .shadow_lg();

    // The command palette floats above the capsule while a message is still a
    // bare command, so the field keeps its width and nothing reflows as the
    // suggestions appear.
    if let Some(matches) = command_matches(app, cx) {
        card = card.child(command_palette(
            &matches,
            app.command_prefix.clone(),
            entity.clone(),
            cx,
        ));
    }

    if !app.attachments.is_empty() {
        card = card.child(attachment_row(app, &entity));
    }

    card = card.child(
        div().w_full().min_w_0().py(px(2.)).child(
            Textarea::new(&app.input)
                .appearance(false)
                .bordered(false)
                .text_size(theme::TEXT_BODY),
        ),
    );

    card.child(toolbar(app, cx, &entity)).into_any_element()
}

/// The row of controls under the field.
fn toolbar(
    app: &DesktopApp,
    cx: &mut Context<DesktopApp>,
    entity: &Entity<DesktopApp>,
) -> AnyElement {
    let generating = app.generating;

    let attach = atoms::circle_button(
        cx,
        "attach",
        IconName::Plus,
        atoms::Plate::Bare,
        "Attach files",
        !generating,
        cx.listener(|this, _, _window, cx| this.pick_attachments(cx)),
    );

    let action = if generating {
        atoms::circle_button(
            cx,
            "stop",
            IconName::Square,
            atoms::Plate::Quiet,
            "Stop",
            true,
            cx.listener(|this, _, _, cx| this.stop(cx)),
        )
    } else {
        let can_send = app.can_send(cx);
        atoms::circle_button(
            cx,
            "send",
            IconName::ArrowUp,
            atoms::Plate::Accent,
            "Send",
            can_send,
            cx.listener(|this, _, window, cx| {
                let input = this.input.clone();
                this.send(&input, window, cx);
            }),
        )
    };

    h_flex()
        .w_full()
        .items_center()
        .justify_between()
        .gap(px(8.))
        .child(
            h_flex()
                .min_w_0()
                .items_center()
                .gap(px(2.))
                .child(attach)
                .child(permission_picker(app, cx, entity)),
        )
        .child(
            h_flex()
                .min_w_0()
                .items_center()
                .gap(px(6.))
                .child(model_picker(app, cx, entity))
                .child(action),
        )
        .into_any_element()
}

/// The permission the next turn runs under.
fn permission_picker(
    app: &DesktopApp,
    cx: &mut Context<DesktopApp>,
    entity: &Entity<DesktopApp>,
) -> AnyElement {
    let ask = app.approval_mode == ApprovalMode::Ask;
    let (label, tooltip) = if ask {
        ("Ask first", "Tools wait for your approval")
    } else {
        ("Auto-run", "Tools run without asking")
    };
    let trigger = atoms::toolbar_pill(cx, "permissions", IconName::Shield, label, tooltip);

    let current = app.approval_mode;
    let entity = entity.clone();
    menus::menu(
        "permissions-menu",
        Anchor::BottomLeft,
        trigger,
        move |_, _| {
            let entity_auto = entity.clone();
            let entity_ask = entity.clone();
            menus::column()
                .child(menus::heading("Tool permission"))
                .child(menus::action(
                    "perm-auto",
                    Some(IconName::Zap),
                    "Run without asking",
                    Some("Tools run the moment the model calls them.".into()),
                    current == ApprovalMode::Auto,
                    move |_, _, cx| {
                        entity_auto
                            .update(cx, |app, cx| app.set_approval_mode(ApprovalMode::Auto, cx));
                    },
                ))
                .child(menus::action(
                    "perm-ask",
                    Some(IconName::ShieldCheck),
                    "Ask before tools run",
                    Some("Every tool call waits for Allow or Deny.".into()),
                    current == ApprovalMode::Ask,
                    move |_, _, cx| {
                        entity_ask
                            .update(cx, |app, cx| app.set_approval_mode(ApprovalMode::Ask, cx));
                    },
                ))
                .into_any_element()
        },
    )
    .into_any_element()
}

/// The model the next turn runs on.
fn model_picker(
    app: &DesktopApp,
    cx: &mut Context<DesktopApp>,
    entity: &Entity<DesktopApp>,
) -> AnyElement {
    let label = short_model(&app.model);
    let trigger = atoms::toolbar_pill(cx, "model", IconName::Cpu, &label, "Model");

    let choices = app.model_choices.clone();
    let current = app.model.clone();
    let entity = entity.clone();
    menus::menu("model-menu", Anchor::BottomRight, trigger, move |_, _| {
        let mut column = menus::column().child(menus::heading("Model"));
        if choices.is_empty() {
            column = column.child(
                div()
                    .px(px(8.))
                    .py(px(7.))
                    .text_size(theme::TEXT_UI)
                    .text_color(theme::ink_tertiary())
                    .child("No models reported."),
            );
        }
        for choice in choices.clone() {
            let id = choice.id.clone();
            let detail = (choice.name != choice.id).then(|| choice.id.clone());
            let selected = choice.id == current;
            let entity = entity.clone();
            column = column.child(menus::action(
                format!("model-{}", choice.id),
                None,
                &choice.name,
                detail,
                selected,
                move |_, _, cx| {
                    let id = id.clone();
                    entity.update(cx, |app, cx| app.choose_model(id.clone(), cx));
                },
            ));
        }
        column.into_any_element()
    })
    .into_any_element()
}

/// The queued attachments, each removable before the message is sent.
fn attachment_row(app: &DesktopApp, entity: &Entity<DesktopApp>) -> AnyElement {
    let mut row = h_flex()
        .w_full()
        .min_w_0()
        .flex_wrap()
        .gap(px(6.))
        .pb(px(2.));
    for file in &app.attachments {
        row = row.child(attachment_chip(file, entity));
    }
    row.into_any_element()
}

fn attachment_chip(file: &Attachment, entity: &Entity<DesktopApp>) -> AnyElement {
    let path = file.path.clone();
    let entity = entity.clone();
    h_flex()
        .max_w(px(240.))
        .items_center()
        .gap(px(8.))
        .pl(px(9.))
        .pr(px(5.))
        .py(px(5.))
        .rounded(px(10.))
        .bg(theme::fill())
        .child(
            Icon::new(file.kind.icon())
                .text_color(theme::ink_secondary())
                .with_size(px(14.)),
        )
        .child(
            v_flex()
                .min_w_0()
                .gap(px(1.))
                .child(
                    div()
                        .truncate()
                        .text_size(theme::TEXT_CAPTION)
                        .text_color(theme::ink())
                        .child(file.name.clone()),
                )
                .child(atoms::caption(crate::model::format_size(file.size))),
        )
        .child(
            div()
                .id(format!("remove-{path}"))
                .flex_shrink_0()
                .flex()
                .items_center()
                .justify_center()
                .w(px(20.))
                .h(px(20.))
                .rounded(px(6.))
                .cursor_pointer()
                .hover(|style| style.bg(theme::fill_strong()))
                .child(
                    Icon::new(IconName::X)
                        .text_color(theme::ink_tertiary())
                        .with_size(px(13.)),
                )
                .on_click(move |_, _, cx| {
                    let path = path.clone();
                    entity.update(cx, |app, cx| app.remove_attachment(&path, cx));
                }),
        )
        .into_any_element()
}

/// The commands a half-typed command matches, if the field is still on the
/// command token.
fn command_matches(app: &DesktopApp, cx: &Context<DesktopApp>) -> Option<Vec<Command>> {
    let value = app.input.read(cx).value().to_string();
    let prefix = app.command_prefix.as_str();
    let rest = value.strip_prefix(prefix)?;
    if rest.contains(char::is_whitespace) {
        return None;
    }
    let token = rest.to_lowercase();
    let matches: Vec<Command> = app
        .commands
        .iter()
        .filter(|command| command.name.to_lowercase().starts_with(&token))
        .cloned()
        .collect();
    (!matches.is_empty()).then_some(matches)
}

/// The list of matching commands, floating above the capsule.
fn command_palette(
    matches: &[Command],
    prefix: String,
    entity: Entity<DesktopApp>,
    cx: &App,
) -> AnyElement {
    let mono = cx.theme().mono_font_family.clone();
    let mut list = v_flex()
        .absolute()
        .bottom_full()
        .left(px(-13.))
        .right(px(-13.))
        .mb(px(8.))
        .p(px(5.))
        .rounded(theme::RADIUS_CARD)
        .border_1()
        .border_color(theme::hairline())
        .bg(theme::surface())
        .shadow_lg();

    for command in matches {
        let trigger = command.trigger.clone();
        let name = command.name.clone();
        list = list.child(
            h_flex()
                .id(format!("command-{}", command.name))
                .w_full()
                .items_center()
                .gap(px(10.))
                .px(px(8.))
                .py(px(6.))
                .rounded(px(7.))
                .cursor_pointer()
                .hover(|style| style.bg(theme::hover()))
                .child(
                    div()
                        .flex_shrink_0()
                        .font_family(mono.clone())
                        .text_size(theme::TEXT_MICRO)
                        .text_color(theme::accent())
                        .child(format!("{prefix}{name}")),
                )
                .child(
                    div()
                        .flex_1()
                        .min_w_0()
                        .truncate()
                        .text_size(theme::TEXT_MICRO)
                        .text_color(theme::ink_tertiary())
                        .child("Bub command"),
                )
                .on_click({
                    let entity = entity.clone();
                    move |_, window, cx| {
                        let trigger = trigger.clone();
                        entity.update(cx, |app, cx| app.insert_command(&trigger, window, cx));
                    }
                }),
        );
    }

    list.into_any_element()
}

/// The model id without its provider prefix, which is what a reader recognises.
fn short_model(model: &str) -> String {
    if model.is_empty() {
        return "Model".to_string();
    }
    model.rsplit(':').next().unwrap_or(model).to_string()
}
