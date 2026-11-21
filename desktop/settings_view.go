package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/bubbuild/bub/desktop/internal/appdata"
)

// The settings page is one quiet typographic column: a small muted caption over
// an inset group of rows. The group keeps its shape — a hairline-bordered card
// with hairlines between its rows — but it sits on the page's own color rather
// than a gray fill, so grouping reads without turning the page into a stack of
// tinted boxes. Changing a control applies it and writes settings.json.

// settingsWidth is the reading measure of the settings column.
const settingsWidth = 620

// settingsState is what the settings page keeps between frames: the agent
// command being typed, which persists while the field has the focus.
type settingsState struct {
	agentCmd string
}

// settingsView builds the settings page, centered in the window's main area.
func (a *app) settingsView(c *ui.Context) {
	t := c.Theme()
	root := ui.Scroll(c).Fill().Background(t.Background)
	st := ui.Local(root, "settings", func() settingsState {
		return settingsState{agentCmd: a.settings.AgentCmd}
	})
	if st == nil {
		st = &settingsState{agentCmd: a.settings.AgentCmd}
	}
	root.Children(func() {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Column(c).Width(settingsWidth).MaxWidthPercent(94).Padding(tokens.Space5, 0).
				Children(func() {
					a.workspaceSection(c)
					a.appearanceSection(c)
					a.editorSection(c)
					a.aiSection(c, st)
					a.shortcutsSection(c)
					a.aboutSection(c)
				})
		})
	})
}

// settingsGroup is one named group: its muted caption, then its inset card of
// rows, with clear space from the group above. The card is a hairline on the
// page color, not a gray fill; a row draws the hairline that separates it from
// the row above, inset from the card's edge.
func (a *app) settingsGroup(c *ui.Context, title string, first bool, build func()) {
	t := c.Theme()
	top := float32(0)
	if !first {
		top = tokens.Space6
	}
	ui.Column(c).FillWidth().Children(func() {
		ui.Text(c, title).FontSize(tokens.FontCaption).FontWeight(tokens.WeightRegular).
			LetterSpacing(0.1).TextColor(t.TextMuted).Padding(top, 0, tokens.Space2, 0)
		ui.Column(c).FillWidth().Clip().Radius(tokens.RadiusLG).Background(t.Background).
			Border(1, t.Border).Children(build)
	})
}

// settingsRow is one item of a group: its name, its description under the
// name, and the control right-aligned in a fixed measure. divider draws the
// hairline that separates it from the item above.
func (a *app) settingsRow(c *ui.Context, divider bool, label, description string, control func()) {
	t := c.Theme()
	ui.Column(c).FillWidth().Children(func() {
		if divider {
			ui.Row(c).FillWidth().Padding(0, 14).Children(func() {
				ui.Box(c).Height(1).FillWidth().Background(t.Border)
			})
		}
		ui.Row(c).FillWidth().MinHeight(48).Gap(tokens.Space4).AlignItems(ui.Center).
			Padding(tokens.Space3, 14).Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, label).FontSize(tokens.FontBody).TextColor(t.Text)
				if description != "" {
					ui.Text(c, description).FontSize(tokens.FontCaption).
						TextColor(t.TextMuted).LineHeight(1.4)
				}
			})
			if control != nil {
				ui.Row(c).Width(200).Shrink(0).Justify(ui.End).AlignItems(ui.Center).
					Children(control)
			}
		})
	})
}

// workspaceSection says where the pages live and lets the writer move them.
func (a *app) workspaceSection(c *ui.Context) {
	a.settingsGroup(c, "Workspace", true, func() {
		path := strings.TrimSpace(a.settings.Workspace)
		if path == "" {
			path = "No workspace chosen yet."
		}
		a.settingsRow(c, false, "Location", path, func() {
			if ui.Button(c, "Choose…").Key("workspace-choose").Clicked() {
				a.chooseWorkspace()
			}
		})
	})
}

// chooseWorkspace asks for a folder off the main thread, then switches to it.
func (a *app) chooseWorkspace() {
	parent := a.win
	start := a.settings.Workspace
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:            parent,
			Title:             "Choose workspace",
			DefaultPath:       start,
			ButtonLabel:       "Choose",
			Directory:         true,
			CreateDirectories: true,
		})
		a.dispatch(func() {
			if err != nil {
				a.notifyError("Could not choose a workspace", err.Error(), nil)
				return
			}
			if len(paths) == 0 || strings.TrimSpace(paths[0]) == "" {
				return
			}
			a.settings.Workspace = paths[0]
			a.settingsChanged()
		})
	}()
}

// appearanceSection follows the system appearance or holds one.
func (a *app) appearanceSection(c *ui.Context) {
	a.settingsGroup(c, "Appearance", false, func() {
		a.settingsRow(c, false, "Theme", "", func() {
			index := themeIndex(a.settings.Theme)
			seg := ui.Segmented(c.Key("theme"), &index, "System", "Light", "Dark").
				Label("Theme")
			if seg.Changed() {
				a.settings.Theme = themeName(index)
				a.settingsChanged()
			}
		})
	})
}

// editorSection sets the writing column's measure and text size.
func (a *app) editorSection(c *ui.Context) {
	a.settingsGroup(c, "Editor", false, func() {
		a.settingsRow(c, false, "Line width", "", func() {
			a.numberControl(c, "editor-width", "line width", a.settings.EditorWidth, 560, 960, 20,
				func(v int) {
					a.settings.EditorWidth = v
					a.settingsChanged()
				})
		})
		a.settingsRow(c, true, "Font size", "", func() {
			a.numberControl(c, "editor-size", "font size", a.settings.EditorFontSize, 13, 20, 1,
				func(v int) {
					a.settings.EditorFontSize = v
					a.settingsChanged()
				})
		})
	})
}

// numberControl is a quiet stepper: one bordered field holding minus, the
// value, and plus. The value cell carries its own surface and hairlines so the
// number reads as the field it is, instead of floating between two glyphs.
func (a *app) numberControl(c *ui.Context, key, name string, value, lo, hi, step int, set func(int)) {
	t := c.Theme()
	ui.Row(c).Height(tokens.RowHeight).Radius(tokens.RadiusSM).Border(1, t.Border).
		Background(t.Background).Clip().AlignItems(ui.Center).Children(func() {
		if stepControl(c, key+"-minus", "−", "Decrease "+name, value > lo) {
			set(clampInt(value-step, lo, hi))
		}
		ui.Box(c).Width(64).Height(tokens.RowHeight-2).Background(t.Surface).
			BorderWidth(0, 1, 0, 1).BorderColor(t.Border).
			AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("%d px", value)).FontSize(tokens.FontControl).TextColor(t.Text).
				TextAlign(ui.Center).SingleLine()
		})
		if stepControl(c, key+"-plus", "+", "Increase "+name, value < hi) {
			set(clampInt(value+step, lo, hi))
		}
	})
}

// stepControl is one arrow inside a number control, quiet until hovered and
// dim at the end of its range. The field around it owns the border.
func stepControl(c *ui.Context, key any, glyph, label string, enabled bool) bool {
	t := c.Theme()
	b := ui.Box(c).Key(key).Width(28).FillHeight().AlignItems(ui.Center).Justify(ui.Center).
		Focusable()
	color := t.TextMuted
	if !enabled {
		b.Disabled(true)
		color = t.Border
	} else if c.Preferences().Hover() && b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	clicked := b.Clicked()
	b.Children(func() {
		ui.Text(c, glyph).FontSize(14).TextColor(color).TextAlign(ui.Center)
	})
	b.Label(label)
	return clicked
}

// clampInt keeps a stepped value inside its range.
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// themeIndex is the segmented control's place for an appearance.
func themeIndex(name string) int {
	switch name {
	case appdata.ThemeLight:
		return 1
	case appdata.ThemeDark:
		return 2
	default:
		return 0
	}
}

// themeName is the appearance a segmented control chose.
func themeName(index int) string {
	switch index {
	case 1:
		return appdata.ThemeLight
	case 2:
		return appdata.ThemeDark
	default:
		return appdata.ThemeSystem
	}
}

// aiSection is what Bub reads, how it is started, and whether edits save.
func (a *app) aiSection(c *ui.Context, st *settingsState) {
	t := c.Theme()
	a.settingsGroup(c, "AI", false, func() {
		a.settingsRow(c, false, "Context from nearby blocks", "", func() {
			on := a.settings.AIContextNeighbors
			sw := ui.Switch(c.Key("ai-neighbors"), &on).
				Label("Toggle context from nearby blocks")
			if sw.Changed() {
				a.settings.AIContextNeighbors = on
				a.settingsChanged()
			}
		})
		a.settingsRow(c, true, "Page outline in context", "", func() {
			on := a.settings.AIContextOutline
			sw := ui.Switch(c.Key("ai-outline"), &on).
				Label("Toggle page outline in context")
			if sw.Changed() {
				a.settings.AIContextOutline = on
				a.settingsChanged()
			}
		})
		// A command is long: it takes the row's whole width under its
		// label, so it is read whole.
		ui.Column(c).FillWidth().Children(func() {
			ui.Row(c).FillWidth().Padding(0, 14).Children(func() {
				ui.Box(c).Height(1).FillWidth().Background(t.Border)
			})
			ui.Column(c).FillWidth().Padding(tokens.Space3, 14).Gap(7).Children(func() {
				ui.Text(c, "Agent command").FontSize(tokens.FontBody).TextColor(t.Text)
				ui.Text(c, "Empty detects bub acp or uv run bub acp; BUB_ACP_CMD overrides both.").
					FontSize(tokens.FontCaption).TextColor(t.TextMuted).LineHeight(1.4)
				field := ui.TextInput(c.Key("agent-command"), &st.agentCmd).FillWidth().
					FontSize(tokens.FontBody).Background(t.Background).Label("Agent command")
				field.OnTextKey(func(k ui.TextKey) bool {
					if k.Key != ui.KeyEnter || k.Mods != 0 {
						return false
					}
					a.commitAgentCommand(st, k.Text())
					return true
				})
				// Typed changes persist when the field gives the focus back,
				// so a command is one write to settings.json, not one per key.
				if st.agentCmd != a.settings.AgentCmd && !field.Focused() {
					a.commitAgentCommand(st, st.agentCmd)
				}
			})
		})
		a.settingsRow(c, true, "Autosave", "", func() {
			on := a.settings.Autosave
			sw := ui.Switch(c.Key("autosave"), &on).Label("Toggle autosave")
			if sw.Changed() {
				a.settings.Autosave = on
				a.settingsChanged()
			}
		})
	})
}

// commitAgentCommand stores the field's text and saves the settings.
func (a *app) commitAgentCommand(st *settingsState, text string) {
	if text == a.settings.AgentCmd {
		return
	}
	st.agentCmd = text
	a.settings.AgentCmd = text
	a.settingsChanged()
}

// shortcutsSection is a still reference of the keyboard: the row is a label
// and its keycaps, not a control.
func (a *app) shortcutsSection(c *ui.Context) {
	shortcuts := []struct {
		keys   string
		action string
	}{
		{"⌘N", "New page"},
		{"⌘S", "Save"},
		{"⌘K", "Search"},
		{"⇧⌘R", "Rewrite block"},
		{"⇧⌘K", "Ask Bub"},
		{"⌘J", "AI chat"},
		{"⇧⌘O", "Contents"},
		{"⌘\\", "Sidebar"},
		{"⌘,", "Settings"},
		{"/", "Block menu"},
	}
	a.settingsGroup(c, "Shortcuts", false, func() {
		for i, shortcut := range shortcuts {
			i, shortcut := i, shortcut
			a.settingsRow(c, i > 0, shortcut.action, "", func() {
				keycap(c, shortcut.keys)
			})
		}
	})
}

// aboutSection names the build, the agent it talks to, and where settings go.
func (a *app) aboutSection(c *ui.Context) {
	t := c.Theme()
	a.settingsGroup(c, "About", false, func() {
		a.settingsRow(c, false, "Version", "", func() {
			ui.Text(c, appVersion).FontSize(tokens.FontControl).TextColor(t.TextMuted).SingleLine()
		})
		a.settingsRow(c, true, "Bub agent", a.agentState(), func() {
			reconnect := ui.Button(c, "Reconnect").Key("agent-reconnect").
				Disabled(a.connecting)
			if reconnect.Clicked() {
				a.reconnect()
			}
		})
		a.settingsRow(c, true, "Preferences file", a.settingsFile(), nil)
	})
}

// agentState is the connection as the About row says it.
func (a *app) agentState() string {
	switch {
	case a.client != nil:
		return "Connected"
	case a.connecting:
		return "Connecting…"
	case strings.TrimSpace(a.connectErr) != "":
		return "Not connected — " + a.connectErr
	default:
		return "Not connected"
	}
}

// settingsFile names where preferences are written.
func (a *app) settingsFile() string {
	path := strings.TrimSpace(a.settingsPath)
	if path == "" {
		return "settings.json"
	}
	return "settings.json — " + path
}
