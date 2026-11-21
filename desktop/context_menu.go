package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/bubbuild/bub/desktop/internal/block"
)

// blockContextMenu builds the editor's context menu: the system's
// editing commands, then the block AI actions on the selected block.
func (a *app) blockContextMenu(m *ui.Menu) {
	m.EditItems()
	m.Separator()
	m.Submenu("Bub", func(m *ui.Menu) {
		// The menu's function runs once to collect its items and again
		// after one was chosen, so the target is derived each run: the
		// page may have moved the blocks in between.
		target := a.contextTarget()
		disabled := target == nil
		for _, action := range []struct {
			label string
			kind  actionKind
		}{
			{"Rewrite", actionRewrite},
			{"Shorten", actionShorten},
			{"Expand", actionExpand},
			{"Continue", actionContinue},
			{"Translate", actionTranslate},
		} {
			if m.Item(action.label).Disabled(disabled).Chosen() && target != nil {
				a.startAction(a.actionFor(action.kind, "", *target))
			}
		}
		m.Separator()
		if m.Item("Ask Bub…").Disabled(disabled).Chosen() && target != nil {
			a.dismissPopovers()
			a.ask = &askState{target: *target}
		}
	})
}

// contextTarget is the block the menu acts on: the one holding the
// selection's start, or the caret's block when nothing is selected.
func (a *app) contextTarget() *block.Block {
	start, _ := a.doc.Selection()
	return a.blockAt(start)
}
