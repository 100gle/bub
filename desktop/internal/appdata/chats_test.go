package appdata_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/bubbuild/bub/desktop/internal/appdata"
)

// newTestChats returns a Chats pointing at a not-yet-created directory below a
// temporary directory, exercising the package's own MkdirAll.
func newTestChats(t *testing.T) *appdata.Chats {
	t.Helper()
	return &appdata.Chats{Dir: filepath.Join(t.TempDir(), "chats")}
}

func TestNewConversation(t *testing.T) {
	conv := appdata.NewConversation()
	if conv.ID == "" {
		t.Error("NewConversation ID is empty")
	}
	// A new conversation reads nothing until the writer attaches it.
	if conv.Page != "" || len(conv.References) != 0 {
		t.Errorf("a new conversation has context: page=%q references=%v", conv.Page, conv.References)
	}
	if conv.CreatedAt.IsZero() || conv.UpdatedAt.IsZero() {
		t.Errorf("timestamps not set: %+v", conv)
	}
	if conv.Messages == nil || len(conv.Messages) != 0 {
		t.Errorf("Messages = %#v, want empty non-nil slice", conv.Messages)
	}

	seen := make(map[string]bool, 50)
	for i := 0; i < 50; i++ {
		id := appdata.NewConversation().ID
		if seen[id] {
			t.Fatalf("duplicate conversation ID %q", id)
		}
		seen[id] = true
	}
}

func TestChatsSaveAndGet(t *testing.T) {
	c := newTestChats(t)
	at := time.Now().UTC().Add(-time.Minute)
	conv := appdata.NewConversation()
	conv.References = []string{"pages/hello.md", "pages/notes.md"}
	conv.Messages = []appdata.Message{
		{Role: appdata.RoleUser, Text: "Fix the intro\nand the rest", At: at},
		{Role: appdata.RoleAssistant, Text: "Done.", At: at.Add(time.Second)},
	}
	if err := c.Save(&conv); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if conv.ID == "" {
		t.Fatal("Save did not assign an ID")
	}
	if conv.Title != "Fix the intro" {
		t.Errorf("derived Title = %q, want %q", conv.Title, "Fix the intro")
	}
	if conv.CreatedAt.IsZero() || conv.UpdatedAt.IsZero() {
		t.Errorf("Save did not set timestamps: %+v", conv)
	}

	got, err := c.Get(conv.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != conv.ID || got.Title != conv.Title || !reflect.DeepEqual(got.References, conv.References) {
		t.Errorf("Get = %+v, want %+v", got, conv)
	}
	if got.Page != "" {
		t.Errorf("legacy Page = %q, want it migrated into References", got.Page)
	}
	if !got.CreatedAt.Equal(conv.CreatedAt) || !got.UpdatedAt.Equal(conv.UpdatedAt) {
		t.Errorf("timestamps changed: got %v/%v, want %v/%v",
			got.CreatedAt, got.UpdatedAt, conv.CreatedAt, conv.UpdatedAt)
	}
	if len(got.Messages) != 2 || got.Messages[0].Role != appdata.RoleUser ||
		got.Messages[0].Text != "Fix the intro\nand the rest" {
		t.Errorf("messages = %+v", got.Messages)
	}

	// An explicit title survives later saves.
	if err := c.Save(got); err != nil {
		t.Fatalf("Save second time: %v", err)
	}
	got.Title = "Kept"
	if err := c.Save(got); err != nil {
		t.Fatalf("Save explicit title: %v", err)
	}
	again, err := c.Get(conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Title != "Kept" {
		t.Errorf("explicit title overwritten: %q", again.Title)
	}

	data, err := os.ReadFile(filepath.Join(c.Dir, conv.ID+".json"))
	if err != nil {
		t.Fatalf("read conversation file: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("conversation JSON: %v", err)
	}
	for _, key := range []string{"id", "title", "references", "createdAt", "updatedAt", "messages"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("conversation JSON missing key %q", key)
		}
	}
	if _, ok := fields["page"]; ok {
		t.Error("conversation JSON still carries the migrated page key")
	}
	if !strings.Contains(string(data), `"role": "user"`) {
		t.Errorf("message JSON missing role: %s", data)
	}
}

func TestChatsSaveZeroFields(t *testing.T) {
	c := newTestChats(t)
	conv := appdata.Conversation{
		Messages: []appdata.Message{{Role: appdata.RoleUser, Text: "hello"}},
	}
	if err := c.Save(&conv); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if conv.ID == "" {
		t.Error("Save did not assign an ID")
	}
	if conv.CreatedAt.IsZero() || conv.UpdatedAt.IsZero() {
		t.Error("Save did not set timestamps")
	}
	if conv.Title != "hello" {
		t.Errorf("Title = %q, want %q", conv.Title, "hello")
	}
	if err := c.Save(nil); err == nil {
		t.Error("Save(nil) returned nil error")
	}
}

func TestChatsDeriveTitle(t *testing.T) {
	tests := []struct {
		name     string
		messages []appdata.Message
		want     string
	}{
		{
			name: "skips other roles",
			messages: []appdata.Message{
				{Role: appdata.RoleAssistant, Text: "hi"},
				{Role: appdata.RoleUser, Text: "  Real question  "},
			},
			want: "Real question",
		},
		{
			name: "first line only",
			messages: []appdata.Message{
				{Role: appdata.RoleUser, Text: "Line one\nLine two"},
			},
			want: "Line one",
		},
		{
			name: "skips empty",
			messages: []appdata.Message{
				{Role: appdata.RoleUser, Text: "   "},
				{Role: appdata.RoleUser, Text: "Second"},
			},
			want: "Second",
		},
		{
			name:     "no user messages",
			messages: []appdata.Message{{Role: appdata.RoleAssistant, Text: "hi"}},
			want:     "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestChats(t)
			conv := appdata.Conversation{Messages: tt.messages}
			if err := c.Save(&conv); err != nil {
				t.Fatalf("Save: %v", err)
			}
			if conv.Title != tt.want {
				t.Errorf("Title = %q, want %q", conv.Title, tt.want)
			}
		})
	}
}

func TestChatsDeriveTitleTruncates(t *testing.T) {
	c := newTestChats(t)
	long := strings.Repeat("word ", 30) + "end"
	conv := appdata.Conversation{Messages: []appdata.Message{{Role: appdata.RoleUser, Text: long}}}
	if err := c.Save(&conv); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !strings.HasSuffix(conv.Title, "\u2026") {
		t.Errorf("long title not truncated: %q", conv.Title)
	}
	if n := utf8.RuneCountInString(conv.Title); n > 81 {
		t.Errorf("truncated title is %d runes, want <= 81", n)
	}
}

func TestChatsListSortsByUpdatedAt(t *testing.T) {
	c := newTestChats(t)
	var saved []*appdata.Conversation
	for i := 0; i < 3; i++ {
		conv := appdata.NewConversation()
		conv.Messages = []appdata.Message{{Role: appdata.RoleUser, Text: "message"}}
		if err := c.Save(&conv); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
		saved = append(saved, &conv)
		time.Sleep(5 * time.Millisecond)
	}

	// Corrupt and irrelevant files must not hide the valid conversations.
	if err := os.WriteFile(filepath.Join(c.Dir, "bad.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(c.Dir, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(c.Dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}

	list, err := c.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("List returned %d conversations, want 3", len(list))
	}
	for i, want := range []*appdata.Conversation{saved[2], saved[1], saved[0]} {
		if list[i].ID != want.ID {
			t.Errorf("List[%d].ID = %q, want %q (newest first)", i, list[i].ID, want.ID)
		}
	}
	if len(list[0].Messages) != 1 {
		t.Errorf("List did not load messages: %+v", list[0].Messages)
	}
	if !list[0].UpdatedAt.Equal(saved[2].UpdatedAt) {
		t.Errorf("List[0].UpdatedAt = %v, want %v", list[0].UpdatedAt, saved[2].UpdatedAt)
	}
}

func TestChatsListCreatesMissingDir(t *testing.T) {
	c := newTestChats(t)
	list, err := c.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List = %v, want empty", list)
	}
	if info, err := os.Stat(c.Dir); err != nil || !info.IsDir() {
		t.Errorf("List did not create chats directory: info=%v err=%v", info, err)
	}
}

func TestChatsDelete(t *testing.T) {
	c := newTestChats(t)
	conv := appdata.NewConversation()
	if err := c.Save(&conv); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(conv.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := c.Get(conv.ID); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Get after Delete error = %v, want fs.ErrNotExist", err)
	}
	if err := c.Delete(conv.ID); err != nil {
		t.Errorf("Delete missing = %v, want nil", err)
	}
}

func TestChatsRejectsUnsafeIDs(t *testing.T) {
	c := newTestChats(t)
	for _, id := range []string{"", ".", "..", "../escape", `..\escape`, "a/b", ".hidden"} {
		if _, err := c.Get(id); !errors.Is(err, appdata.ErrInvalidID) {
			t.Errorf("Get(%q) error = %v, want ErrInvalidID", id, err)
		}
		if err := c.Delete(id); !errors.Is(err, appdata.ErrInvalidID) {
			t.Errorf("Delete(%q) error = %v, want ErrInvalidID", id, err)
		}
	}
	for _, id := range []string{".", "..", "../escape", `..\escape`, "a/b", ".hidden"} {
		conv := appdata.Conversation{ID: id}
		if err := c.Save(&conv); !errors.Is(err, appdata.ErrInvalidID) {
			t.Errorf("Save(%q) error = %v, want ErrInvalidID", id, err)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(c.Dir), "escape.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("unsafe save created a file outside the chats directory: %v", err)
	}
}

func TestChatsMissingConversation(t *testing.T) {
	c := newTestChats(t)
	if _, err := c.Get("does-not-exist"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Get missing error = %v, want fs.ErrNotExist", err)
	}
}

func TestChatsNormalizesMissingMessages(t *testing.T) {
	c := newTestChats(t)
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw := `{"id":"legacy","title":"Old","page":"","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(c.Dir, "legacy.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	conv, err := c.Get("legacy")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if conv.Messages == nil {
		t.Error("Messages is nil, want empty slice")
	}
}

// TestChatsMigrateTheBoundPage checks that a conversation written when a chat
// was tied to one page keeps that page, as its first reference, when it is
// read again — and that saving writes the new shape only.
func TestChatsMigrateTheBoundPage(t *testing.T) {
	c := newTestChats(t)
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw := `{"id":"legacy","title":"Old","page":"essays/The Quiet Web.md",` +
		`"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z","messages":[]}`
	if err := os.WriteFile(filepath.Join(c.Dir, "legacy.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	conv, err := c.Get("legacy")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if conv.Page != "" {
		t.Errorf("Page = %q, want it cleared", conv.Page)
	}
	if !reflect.DeepEqual(conv.References, []string{"essays/The Quiet Web.md"}) {
		t.Errorf("References = %v, want the bound page", conv.References)
	}
	if err := c.Save(conv); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(c.Dir, "legacy.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"page"`) || !strings.Contains(string(data), `"references"`) {
		t.Errorf("saved conversation has the old shape: %s", data)
	}
}

func TestOpenChats(t *testing.T) {
	isolateConfig(t)
	c, err := appdata.OpenChats()
	if err != nil {
		t.Fatalf("OpenChats: %v", err)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "Bub Desktop", "chats")
	if c.Dir != want {
		t.Errorf("Dir = %q, want %q", c.Dir, want)
	}
	if info, err := os.Stat(c.Dir); err != nil || !info.IsDir() {
		t.Errorf("OpenChats did not create directory: info=%v err=%v", info, err)
	}

	conv := appdata.NewConversation()
	conv.Messages = []appdata.Message{{Role: appdata.RoleUser, Text: "hi"}}
	if err := c.Save(&conv); err != nil {
		t.Fatalf("Save through OpenChats: %v", err)
	}
	list, err := c.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %v, %v; want one conversation", list, err)
	}
}
