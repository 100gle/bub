//! The agent panel.
//!
//! A workspace surface, closed until asked for with ⌘I. One segmented control
//! switches between two questions:
//!
//! - **Activity** — "what is it doing right now", and the default, because
//!   that is what a panel gets opened for mid-run.
//! - **Tools** — "what can it do here": the approval policy, and the tools,
//!   skills and commands the runtime reported.
//!
//! The old panel led with a five-row fact table — status, model, workspace,
//! session id, protocol version — then printed every tool as a name plus a
//! paragraph of prose with a "lazy" badge. Most of that was debug output
//! wearing a label: nobody acts on a protocol version, and a description is
//! not how anyone scans a list. What is left is the session line worth keeping,
//! the one switch that changes behaviour, and the capabilities as chips.

use gpui_kit::assets::IconName;
use gpui_kit::component::{
    h_flex, input::TextareaState, scroll::ScrollableElement as _, v_flex, Icon, Sizable as _,
    StyledExt as _,
};
use gpui_kit::prelude::FluentBuilder as _;
use gpui_kit::{
    div, px, AnyElement, App, Context, Entity, Hsla, InteractiveElement as _, IntoElement,
    ParentElement as _, StatefulInteractiveElement as _, Styled as _, Window,
};

use crate::app::DesktopApp;
use crate::model::{ActivityEntry, ApprovalMode, InspectorTab, Status};
use crate::theme;
use crate::ui::atoms;

impl DesktopApp {
    pub(crate) fn inspector(&mut self, cx: &mut Context<Self>) -> AnyElement {
        v_flex()
            .size_full()
            .min_h_0()
            .bg(theme::recess())
            .border_l_1()
            .border_color(theme::hairline())
            // The session line stays above the tabs: where the agent is
            // working is context for both halves, not a section of its own.
            .child(self.panel_header(cx))
            .child(match self.inspector_tab {
                InspectorTab::Activity => self.activity_pane(),
                InspectorTab::Tools => self.tools_pane(cx),
            })
            .into_any_element()
    }

    /// Session context plus the segmented control.
    fn panel_header(&self, cx: &mut Context<Self>) -> AnyElement {
        let mut strip = h_flex()
            .w_full()
            .gap(px(2.))
            .p(px(2.))
            .rounded(theme::RADIUS_CONTROL)
            .bg(theme::fill());

        for (label, tab) in [
            ("Activity", InspectorTab::Activity),
            ("Tools", InspectorTab::Tools),
        ] {
            let selected = self.inspector_tab == tab;
            strip = strip.child(
                div()
                    .id(label)
                    .flex_1()
                    .py(px(4.))
                    .rounded(px(7.))
                    .text_center()
                    .text_size(theme::TEXT_CAPTION)
                    .font_medium()
                    .when(selected, |this| {
                        this.bg(theme::surface())
                            .shadow_xs()
                            .text_color(theme::ink())
                    })
                    .when(!selected, |this| {
                        this.cursor_pointer()
                            .text_color(theme::ink_secondary())
                            .hover(|style| style.text_color(theme::ink()))
                    })
                    .child(label)
                    .on_click(cx.listener(move |this, _, _, cx| {
                        this.inspector_tab = tab;
                        cx.notify();
                    })),
            );
        }

        v_flex()
            .w_full()
            .flex_shrink_0()
            .gap(px(10.))
            .px(px(12.))
            .pt(px(22.))
            .pb(px(10.))
            .child(
                h_flex()
                    .w_full()
                    .items_center()
                    .gap(px(8.))
                    .child(atoms::dot(if self.connected {
                        theme::success()
                    } else {
                        theme::warning()
                    }))
                    .child(
                        div()
                            .flex_1()
                            .min_w_0()
                            .truncate()
                            .text_size(theme::TEXT_CAPTION)
                            .font_medium()
                            .text_color(theme::ink())
                            .child(if self.model.is_empty() {
                                "Connecting…".to_string()
                            } else {
                                self.model.clone()
                            }),
                    )
                    // Only the failure is written out. A green light next to the
                    // word "connected" is a sentence saying nothing.
                    .when(!self.connected, |this| {
                        this.child(
                            div()
                                .flex_shrink_0()
                                .text_size(theme::TEXT_MICRO)
                                .text_color(theme::warning())
                                .child("offline"),
                        )
                    }),
            )
            .child(
                div()
                    .w_full()
                    .min_w_0()
                    .truncate()
                    .text_size(theme::TEXT_MICRO)
                    .text_color(theme::ink_tertiary())
                    .child(if self.workspace.is_empty() {
                        "—".to_string()
                    } else {
                        self.workspace.clone()
                    }),
            )
            .child(strip)
            .into_any_element()
    }

    /// The live trace of the current run.
    fn activity_pane(&self) -> AnyElement {
        if self.activity.is_empty() {
            return v_flex()
                .size_full()
                .items_center()
                .justify_center()
                .px(px(32.))
                .child(
                    div()
                        .text_center()
                        .text_size(theme::TEXT_CAPTION)
                        .line_height(px(18.))
                        .text_color(theme::ink_tertiary())
                        .child(if self.generating {
                            "Waiting for the first step…"
                        } else {
                            "Nothing has run yet. Ask something and the steps land here."
                        }),
                )
                .into_any_element();
        }

        v_flex()
            .id("activity-scroll")
            .flex_1()
            .min_h_0()
            .w_full()
            .px(px(12.))
            .pb(px(14.))
            .overflow_y_scrollbar()
            .children(self.activity.iter().map(activity_row))
            .into_any_element()
    }

    /// The policy that governs tool calls, and what the runtime discovered.
    fn tools_pane(&self, cx: &mut Context<Self>) -> AnyElement {
        let ask = self.approval_mode == ApprovalMode::Ask;

        v_flex()
            .id("tools-scroll")
            .flex_1()
            .min_h_0()
            .w_full()
            .gap(px(18.))
            .px(px(12.))
            .pb(px(16.))
            .overflow_y_scrollbar()
            .child(
                v_flex()
                    .w_full()
                    .gap(px(8.))
                    .child(atoms::group_title("Tool approval"))
                    .child(
                        h_flex()
                            .id("approval-toggle")
                            .w_full()
                            .items_center()
                            .gap(px(10.))
                            .cursor_pointer()
                            .on_click(cx.listener(move |this, _, _, cx| {
                                this.set_approval_mode(
                                    if ask {
                                        ApprovalMode::Auto
                                    } else {
                                        ApprovalMode::Ask
                                    },
                                    cx,
                                );
                            }))
                            .child(
                                div()
                                    .flex_1()
                                    .text_size(theme::TEXT_CAPTION)
                                    .text_color(theme::ink())
                                    .child("Ask before tools run"),
                            )
                            .child(atoms::switch_track(ask)),
                    )
                    .child(atoms::caption(if ask {
                        "Every tool call waits for Allow or Deny."
                    } else {
                        "Tools run without asking."
                    })),
            )
            .child(self.tool_cloud(cx))
            .child(self.skill_cloud(cx))
            .child(self.command_cloud(cx))
            .into_any_element()
    }

    /// Tools, each tagged by how the runtime exposes it, described on hover.
    fn tool_cloud(&self, cx: &mut Context<Self>) -> AnyElement {
        let mut cloud = h_flex().w_full().flex_wrap().gap(px(6.));
        for tool in &self.tools {
            let mut detail = tool.description.clone();
            if tool.deferred {
                if !detail.is_empty() {
                    detail.push_str("\n\n");
                }
                // Why this matters and the description alone cannot say: a
                // deferred tool is only advertised by name, so the model
                // cannot call it until something surfaces it.
                detail.push_str("Deferred: only its name is advertised until it is used.");
            }
            cloud = cloud.child(atoms::chip(
                cx,
                format!("tool-{}", tool.name),
                &tool.name,
                Some(exposure_icon(&tool.exposure)),
                Some(detail),
            ));
        }
        chip_section(
            format!("Tools ({})", self.tools.len()),
            cloud.into_any_element(),
            self.tools.is_empty(),
            "No tools reported.",
        )
    }

    fn skill_cloud(&self, cx: &mut Context<Self>) -> AnyElement {
        let mut cloud = h_flex().w_full().flex_wrap().gap(px(6.));
        for skill in &self.skills {
            let mut detail = skill.description.clone();
            if !skill.source.is_empty() {
                if !detail.is_empty() {
                    detail.push_str("\n\n");
                }
                detail.push_str(&format!("Source: {}", skill.source));
            }
            cloud = cloud.child(atoms::chip(
                cx,
                format!("skill-{}", skill.name),
                &skill.name,
                None,
                Some(detail),
            ));
        }
        chip_section(
            format!("Skills ({})", self.skills.len()),
            cloud.into_any_element(),
            self.skills.is_empty(),
            "No skills discovered.",
        )
    }

    /// Commands, which are useful precisely because they can be run.
    fn command_cloud(&self, cx: &mut Context<Self>) -> AnyElement {
        let mut clouds = h_flex().w_full().flex_wrap().gap(px(6.));
        for command in &self.commands {
            let input = self.input.clone();
            let value = command.clone();
            clouds = clouds.child(
                atoms::chip(cx, format!("command-{command}"), command, None, None)
                    .cursor_pointer()
                    .on_click(move |_, window, cx| {
                        insert(&input, value.clone(), window, cx);
                    }),
            );
        }

        chip_section(
            format!("Commands ({})", self.commands.len()),
            clouds.into_any_element(),
            self.commands.is_empty(),
            "No commands reported.",
        )
    }
}

/// The icon for how a tool is exposed to the model.
fn exposure_icon(exposure: &str) -> IconName {
    match exposure {
        "command" => IconName::Terminal,
        "code" => IconName::FileCode,
        _ => IconName::Wrench,
    }
}

/// Put a command into the composer, leaving the caret ready to continue.
fn insert(input: &Entity<TextareaState>, value: String, window: &mut Window, cx: &mut App) {
    input.update(cx, |state, cx| {
        state.set_value(value, window, cx);
        state.focus(window, cx);
    });
}

fn chip_section(title: String, body: AnyElement, empty: bool, empty_label: &str) -> AnyElement {
    v_flex()
        .w_full()
        .gap(px(8.))
        .child(atoms::group_title(title))
        .child(if empty {
            atoms::caption(empty_label).into_any_element()
        } else {
            body
        })
        .into_any_element()
}

/// How a step's outcome is coloured.
///
/// `done` is the resting state of most rows, so it does not get the success
/// green: only an outcome worth noticing keeps its own colour, and the rest
/// stay quiet.
fn outcome_color(status: Status) -> Hsla {
    if status == Status::Done {
        theme::ink_tertiary()
    } else {
        status.color()
    }
}

/// One step of the current run.
fn activity_row(entry: &ActivityEntry) -> AnyElement {
    let color = outcome_color(entry.status);

    h_flex()
        .w_full()
        .items_start()
        .gap(px(8.))
        .py(px(5.))
        .child(Icon::new(entry.icon).text_color(color).with_size(px(12.)))
        .child(
            v_flex()
                .flex_1()
                .min_w_0()
                .gap(px(2.))
                .child(
                    h_flex()
                        .w_full()
                        .items_center()
                        .gap(px(8.))
                        .child(
                            div()
                                .flex_1()
                                .min_w_0()
                                .truncate()
                                .text_size(theme::TEXT_CAPTION)
                                .text_color(theme::ink())
                                .child(entry.label.clone()),
                        )
                        .child(
                            div()
                                .flex_shrink_0()
                                .text_size(theme::TEXT_MICRO)
                                .text_color(color)
                                .child(entry.status.label()),
                        ),
                )
                .when(!entry.detail.is_empty(), |this| {
                    this.child(
                        div()
                            .w_full()
                            .truncate()
                            .text_size(theme::TEXT_MICRO)
                            .text_color(theme::ink_tertiary())
                            .child(entry.detail.clone()),
                    )
                }),
        )
        .into_any_element()
}
