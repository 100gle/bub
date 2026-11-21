package block

import (
	"reflect"
	"testing"
)

// kindsOf returns the kinds of commands, in order.
func kindsOf(commands []Command) []CommandKind {
	kinds := make([]CommandKind, 0, len(commands))
	for _, c := range commands {
		kinds = append(kinds, c.Kind)
	}
	return kinds
}

func TestCatalog(t *testing.T) {
	all := Commands()
	wantKinds := []CommandKind{
		CommandText,
		CommandHeading1,
		CommandHeading2,
		CommandHeading3,
		CommandBulletedList,
		CommandNumberedList,
		CommandTodoList,
		CommandQuote,
		CommandCodeBlock,
	}
	if got := kindsOf(all); !reflect.DeepEqual(got, wantKinds) {
		t.Fatalf("Commands() kinds = %v, want %v", got, wantKinds)
	}
	wantHints := map[CommandKind]string{
		CommandText:         "",
		CommandHeading1:     "#",
		CommandHeading2:     "##",
		CommandHeading3:     "###",
		CommandBulletedList: "-",
		CommandNumberedList: "1.",
		CommandTodoList:     "[]",
		CommandQuote:        ">",
		CommandCodeBlock:    "</>",
	}
	for _, c := range all {
		if c.Label == "" {
			t.Errorf("command %d has no label", c.Kind)
		}
		if len(c.Keywords) == 0 {
			t.Errorf("command %q has no keywords", c.Label)
		}
		if want := wantHints[c.Kind]; c.Hint != want {
			t.Errorf("command %q hint = %q, want %q", c.Label, c.Hint, want)
		}
		got, ok := CommandFor(c.Kind)
		if !ok || !reflect.DeepEqual(got, c) {
			t.Errorf("CommandFor(%d) = %#v, %v, want %#v, true", c.Kind, got, ok, c)
		}
	}
	if _, ok := CommandFor(CommandKind(99)); ok {
		t.Error("CommandFor(unknown) = true, want false")
	}
}

func TestFilterCommands(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []CommandKind
	}{
		{
			name:  "empty query offers all",
			query: "",
			want:  kindsOf(Commands()),
		},
		{
			name:  "spaces only offers all",
			query: "   ",
			want:  kindsOf(Commands()),
		},
		{
			name:  "label fragment",
			query: "head",
			want:  []CommandKind{CommandHeading1, CommandHeading2, CommandHeading3},
		},
		{
			name:  "heading label",
			query: "heading",
			want:  []CommandKind{CommandHeading1, CommandHeading2, CommandHeading3},
		},
		{
			name:  "keyword h1 ignores case",
			query: "H1",
			want:  []CommandKind{CommandHeading1},
		},
		{
			name:  "lists",
			query: "list",
			want:  []CommandKind{CommandBulletedList, CommandNumberedList, CommandTodoList},
		},
		{
			name:  "todo keyword",
			query: "todo",
			want:  []CommandKind{CommandTodoList},
		},
		{
			name:  "quote label",
			query: "quote",
			want:  []CommandKind{CommandQuote},
		},
		{
			name:  "code keyword",
			query: "snippet",
			want:  []CommandKind{CommandCodeBlock},
		},
		{
			name:  "text label",
			query: "TEXT",
			want:  []CommandKind{CommandText},
		},
		{
			name:  "nothing matches",
			query: "zzz",
			want:  []CommandKind{},
		},
		{
			name:  "label matches come before keyword matches",
			query: "h",
			want:  []CommandKind{CommandHeading1, CommandHeading2, CommandHeading3, CommandText, CommandTodoList},
		},
		{
			name:  "check keyword",
			query: "check",
			want:  []CommandKind{CommandTodoList},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterCommands(tt.query)
			if !reflect.DeepEqual(kindsOf(got), tt.want) {
				t.Fatalf("FilterCommands(%q) kinds = %v, want %v", tt.query, kindsOf(got), tt.want)
			}
		})
	}
}

func TestFilterCommandsDoesNotChangeTheCatalog(t *testing.T) {
	got := FilterCommands("")
	if len(got) == 0 {
		t.Fatal("FilterCommands(\"\") = no commands")
	}
	got[0].Label = "changed"
	if command, _ := CommandFor(CommandText); command.Label == "changed" {
		t.Fatal("changing a result changed the catalog")
	}
	if want := "Text"; Commands()[0].Label != want {
		t.Fatalf("catalog label = %q, want %q", Commands()[0].Label, want)
	}
}
