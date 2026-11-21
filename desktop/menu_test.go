package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo"
)

func TestMenuTemplate(t *testing.T) {
	a, _ := newTestApp(t)
	template := menuTemplate(a)
	if len(template) != 7 {
		t.Fatalf("menu has %d top-level items, want 7", len(template))
	}
	wantRoles := []struct {
		pos  int
		role mygo.MenuRole
	}{
		{0, mygo.RoleAppMenu},
		{2, mygo.RoleEditMenu},
		{3, mygo.RoleFormatMenu},
		{6, mygo.RoleWindowMenu},
	}
	for _, want := range wantRoles {
		if template[want.pos].Role != want.role {
			t.Errorf("item %d role = %q, want %q", want.pos, template[want.pos].Role, want.role)
		}
	}
	if template[1].Label != "File" || template[4].Label != "View" || template[5].Label != "Bub" {
		t.Errorf("custom menu labels = %q, %q, %q, want File, View, Bub", template[1].Label, template[4].Label, template[5].Label)
	}

	file := template[1].Submenu
	if len(file) != 6 {
		t.Fatalf("File has %d items, want 6", len(file))
	}
	wantFile := []struct {
		label string
		accel string
	}{
		{"New Page", "CmdOrCtrl+N"},
		{"New AI Chat", "CmdOrCtrl+Shift+N"},
		{"---", ""},
		{"Save", "CmdOrCtrl+S"},
		{"---", ""},
		{"Close", ""},
	}
	for i, want := range wantFile {
		if want.label == "---" {
			if file[i].Type != mygo.MenuItemSeparator {
				t.Errorf("File item %d is not a separator: %+v", i, file[i])
			}
			continue
		}
		if want.label == "Close" {
			if file[i].Role != mygo.RoleClose {
				t.Errorf("File last item role = %q, want close", file[i].Role)
			}
			continue
		}
		if file[i].Label != want.label || file[i].Accelerator != want.accel {
			t.Errorf("File item %d = %q %q, want %q %q", i, file[i].Label, file[i].Accelerator, want.label, want.accel)
		}
		if file[i].Click == nil {
			t.Errorf("File item %q has no Click", file[i].Label)
		}
	}

	view := template[4].Submenu
	var viewLabels []string
	for _, item := range view {
		if item.Type == mygo.MenuItemSeparator {
			viewLabels = append(viewLabels, "---")
			continue
		}
		viewLabels = append(viewLabels, item.Label)
	}
	wantView := "Search,Toggle Sidebar,Toggle AI Chat,Toggle Contents,---,Settings…"
	if got := strings.Join(viewLabels, ","); got != wantView {
		t.Errorf("View menu = %q, want %q", got, wantView)
	}

	bub := template[5].Submenu
	var labels []string
	for _, item := range bub {
		if item.Type == mygo.MenuItemSeparator {
			labels = append(labels, "---")
			continue
		}
		labels = append(labels, item.Label)
	}
	wantBub := "Rewrite Block,Shorten Block,Expand Block,Continue Block,Translate Block,---,Ask Bub…,Chat with Bub,---,Reconnect to Bub"
	if got := strings.Join(labels, ","); got != wantBub {
		t.Errorf("Bub menu = %q, want %q", got, wantBub)
	}
	if bub[0].Accelerator != "CmdOrCtrl+Shift+R" || bub[6].Accelerator != "CmdOrCtrl+Shift+K" {
		t.Errorf("Bub accelerators = %q, %q, want CmdOrCtrl+Shift+R and CmdOrCtrl+Shift+K", bub[0].Accelerator, bub[6].Accelerator)
	}
	for _, item := range bub {
		if item.Type != mygo.MenuItemSeparator && item.Click == nil {
			t.Errorf("Bub item %q has no Click", item.Label)
		}
	}
}
