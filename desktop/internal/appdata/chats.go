package appdata

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// maxTitleRunes bounds conversation titles derived from the first user message.
const maxTitleRunes = 80

// Message roles used by the app.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

// ErrInvalidID reports a conversation ID that is not a safe file name.
var ErrInvalidID = errors.New("appdata: invalid conversation id")

// Message is one turn in a saved conversation.
type Message struct {
	// Role is the speaker: "user", "assistant" or "system".
	Role string `json:"role"`
	// Text is the message body.
	Text string `json:"text"`
	// Attachments are the files the turn sent as resource links, by path.
	Attachments []string `json:"attachments,omitempty"`
	// At is when the message was created.
	At time.Time `json:"at"`
}

// Conversation is a saved AI conversation. A conversation reads the pages the
// writer attaches to it; it is not tied to one page, and a page may hold any
// number of conversations.
type Conversation struct {
	// ID is the stable identifier and file name stem.
	ID string `json:"id"`
	// Title is the display title. It is derived from the first user message
	// when empty.
	Title string `json:"title"`
	// References are the workspace-relative pages this conversation reads as
	// context. Every turn carries them as resource links, so the context is
	// what the writer attached and not whatever page happens to be open.
	References []string `json:"references,omitempty"`
	// Page is the single page older conversations were bound to. It is kept
	// only so those files keep their context: reading and saving migrate it
	// into References and clear it.
	Page string `json:"page,omitempty"`
	// CreatedAt is when the conversation was first saved.
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt is when the conversation was last saved.
	UpdatedAt time.Time `json:"updatedAt"`
	// Messages are the conversation turns in order.
	Messages []Message `json:"messages"`
}

// Chats stores conversations as one JSON file per conversation.
type Chats struct {
	// Dir is the directory holding <id>.json files.
	Dir string
}

// OpenChats opens the app's conversation store, creating its directory.
func OpenChats() (*Chats, error) {
	base, err := Dir()
	if err != nil {
		return nil, err
	}
	c := &Chats{Dir: filepath.Join(base, "chats")}
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("appdata: create chats directory %q: %w", c.Dir, err)
	}
	return c, nil
}

// NewConversation returns a new, unsaved conversation with a unique
// time-and-random ID. It references no page: the app attaches the pages and
// files a conversation reads.
func NewConversation() Conversation {
	now := time.Now().UTC()
	return Conversation{
		ID:        newConversationID(),
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []Message{},
	}
}

// List returns every saved conversation, sorted by UpdatedAt descending and
// then by ID descending. Entire conversations, including messages, are loaded;
// unreadable or malformed files are skipped so one bad file cannot hide the
// rest.
func (c *Chats) List() ([]*Conversation, error) {
	dir, err := c.ensureDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("appdata: list chats %q: %w", dir, err)
	}
	convs := make([]*Conversation, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") || !strings.EqualFold(filepath.Ext(name), ".json") {
			continue
		}
		conv, err := readConversation(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if conv.ID == "" {
			conv.ID = strings.TrimSuffix(name, filepath.Ext(name))
		}
		convs = append(convs, conv)
	}
	sort.SliceStable(convs, func(i, j int) bool {
		if !convs[i].UpdatedAt.Equal(convs[j].UpdatedAt) {
			return convs[i].UpdatedAt.After(convs[j].UpdatedAt)
		}
		return convs[i].ID > convs[j].ID
	})
	return convs, nil
}

// Get returns the conversation with the given ID.
func (c *Chats) Get(id string) (*Conversation, error) {
	path, err := c.path(id)
	if err != nil {
		return nil, err
	}
	conv, err := readConversation(path)
	if err != nil {
		return nil, fmt.Errorf("appdata: get conversation %q: %w", id, err)
	}
	if conv.ID == "" {
		conv.ID = id
	}
	return conv, nil
}

// Save writes conv to disk, assigning an ID and CreatedAt when missing and
// always updating UpdatedAt. When Title is empty it is derived from the first
// user message.
func (c *Chats) Save(conv *Conversation) error {
	if conv == nil {
		return fmt.Errorf("appdata: cannot save a nil conversation")
	}
	if conv.ID == "" {
		conv.ID = newConversationID()
	}
	path, err := c.path(conv.ID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if conv.CreatedAt.IsZero() {
		conv.CreatedAt = now
	}
	conv.UpdatedAt = now
	if strings.TrimSpace(conv.Title) == "" {
		conv.Title = deriveTitle(conv.Messages)
	}
	if conv.Messages == nil {
		conv.Messages = []Message{}
	}
	migratePageReference(conv)
	data, err := json.MarshalIndent(conv, "", "  ")
	if err != nil {
		return fmt.Errorf("appdata: encode conversation %q: %w", conv.ID, err)
	}
	data = append(data, '\n')
	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("appdata: save conversation %q: %w", conv.ID, err)
	}
	return nil
}

// Delete removes the conversation with the given ID. Deleting a conversation
// that does not exist is not an error.
func (c *Chats) Delete(id string) error {
	path, err := c.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("appdata: delete conversation %q: %w", id, err)
	}
	return nil
}

// ensureDir returns the chats directory, creating it when needed.
func (c *Chats) ensureDir() (string, error) {
	if c == nil || strings.TrimSpace(c.Dir) == "" {
		return "", fmt.Errorf("appdata: chats directory is empty")
	}
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return "", fmt.Errorf("appdata: create chats directory %q: %w", c.Dir, err)
	}
	return c.Dir, nil
}

// path returns the file path for the conversation id after validating it.
func (c *Chats) path(id string) (string, error) {
	if err := validID(id); err != nil {
		return "", err
	}
	dir, err := c.ensureDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".json"), nil
}

// readConversation decodes a conversation file.
func readConversation(path string) (*Conversation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var conv Conversation
	if err := json.Unmarshal(data, &conv); err != nil {
		return nil, fmt.Errorf("appdata: parse conversation %q: %w", path, err)
	}
	if conv.Messages == nil {
		conv.Messages = []Message{}
	}
	migratePageReference(&conv)
	return &conv, nil
}

// migratePageReference moves the single page an older conversation was bound
// to into its reference list, so a saved chat keeps the context it was written
// with, and clears the old field.
func migratePageReference(conv *Conversation) {
	if conv == nil {
		return
	}
	page := strings.TrimSpace(conv.Page)
	conv.Page = ""
	if page == "" || containsString(page, conv.References) {
		return
	}
	conv.References = append(conv.References, page)
}

// containsString reports whether values holds want.
func containsString(want string, values []string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// validID reports whether id can be used as a conversation file name.
func validID(id string) error {
	switch {
	case id == "" || id == "." || id == "..":
		return fmt.Errorf("%w: %q", ErrInvalidID, id)
	case strings.HasPrefix(id, "."):
		return fmt.Errorf("%w: %q is hidden", ErrInvalidID, id)
	case strings.ContainsAny(id, `/\`):
		return fmt.Errorf("%w: %q contains a path separator", ErrInvalidID, id)
	case strings.ContainsRune(id, 0):
		return fmt.Errorf("%w: %q contains NUL", ErrInvalidID, id)
	}
	return nil
}

// deriveTitle builds a title from the first non-empty user message, using its
// first line and truncating long text.
func deriveTitle(messages []Message) string {
	for _, msg := range messages {
		if msg.Role != RoleUser {
			continue
		}
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			continue
		}
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = strings.TrimSpace(text[:i])
		}
		return truncateRunes(text, maxTitleRunes)
	}
	return ""
}

// truncateRunes shortens s to at most n runes, adding an ellipsis when it had
// to cut.
func truncateRunes(s string, n int) string {
	count := 0
	for i := range s {
		if count == n {
			return strings.TrimSpace(s[:i]) + "\u2026"
		}
		count++
	}
	return s
}

// newConversationID returns a unique, file-name-safe ID built from the UTC time
// and random bytes.
func newConversationID() string {
	suffix := ""
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err == nil {
		suffix = hex.EncodeToString(buf[:])
	} else {
		suffix = fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
	}
	return time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + suffix
}
