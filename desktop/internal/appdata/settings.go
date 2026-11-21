// Package appdata stores Bub Desktop's user state: settings and saved AI
// conversations.
//
// Settings live at os.UserConfigDir()/Bub Desktop/settings.json and
// conversations at os.UserConfigDir()/Bub Desktop/chats/<id>.json. The package
// is UI-free: it depends only on the standard library and the workspace
// package.
package appdata

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bubbuild/bub/desktop/internal/workspace"
)

// appDirName is the application's directory below os.UserConfigDir().
const appDirName = "Bub Desktop"

// Theme values accepted by Settings.Theme.
const (
	ThemeSystem = "system"
	ThemeLight  = "light"
	ThemeDark   = "dark"
)

// Defaults for the numeric editor settings.
const (
	defaultEditorWidth    = 720
	defaultEditorFontSize = 16
)

// Settings is the app's persisted configuration.
type Settings struct {
	// Workspace is the root directory of the Markdown workspace.
	Workspace string `json:"workspace"`
	// AgentCmd overrides the command used to launch the agent. Empty means the
	// app resolves one later.
	AgentCmd string `json:"agentCmd"`
	// Theme is one of "system", "light" or "dark".
	Theme string `json:"theme"`
	// EditorWidth is the editor's text column width in pixels.
	EditorWidth int `json:"editorWidth"`
	// EditorFontSize is the editor's font size in pixels.
	EditorFontSize int `json:"editorFontSize"`
	// ShowOutline shows the page outline in the sidebar.
	ShowOutline bool `json:"showOutline"`
	// AIContextNeighbors includes neighboring blocks as AI context.
	AIContextNeighbors bool `json:"aiContextNeighbors"`
	// AIContextOutline includes the page outline as AI context.
	AIContextOutline bool `json:"aiContextOutline"`
	// Autosave writes edits to disk automatically.
	Autosave bool `json:"autosave"`
}

// DefaultSettings returns the settings used for a fresh install: the default
// workspace (~/Bub), an empty agent command, the system theme, a 720px wide
// editor at 16px, the outline shown, both AI context options on and autosave
// on.
func DefaultSettings() Settings {
	return Settings{
		Workspace:          defaultWorkspace(),
		AgentCmd:           "",
		Theme:              ThemeSystem,
		EditorWidth:        defaultEditorWidth,
		EditorFontSize:     defaultEditorFontSize,
		ShowOutline:        true,
		AIContextNeighbors: true,
		AIContextOutline:   true,
		Autosave:           true,
	}
}

// defaultWorkspace returns ~/Bub, falling back to a relative "Bub" when the
// home directory cannot be determined.
func defaultWorkspace() string {
	if root, err := workspace.DefaultRoot(); err == nil {
		return root
	}
	return "Bub"
}

// normalized fills in invalid or missing values with defaults.
func (s Settings) normalized() Settings {
	if s.Theme != ThemeLight && s.Theme != ThemeDark {
		s.Theme = ThemeSystem
	}
	if s.EditorWidth <= 0 {
		s.EditorWidth = defaultEditorWidth
	}
	if s.EditorFontSize <= 0 {
		s.EditorFontSize = defaultEditorFontSize
	}
	if strings.TrimSpace(s.Workspace) == "" {
		s.Workspace = defaultWorkspace()
	}
	return s
}

// Dir returns the application's data directory,
// os.UserConfigDir()/Bub Desktop. It does not create the directory.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("appdata: locate config directory: %w", err)
	}
	return filepath.Join(base, appDirName), nil
}

// LoadSettings reads settings from path. A missing file yields defaults and a
// nil error; malformed JSON yields defaults and the parse error. Fields absent
// from the file keep their default values, and invalid values are repaired.
func LoadSettings(path string) (Settings, error) {
	defaults := DefaultSettings()
	if path == "" {
		return defaults, fmt.Errorf("appdata: empty settings path")
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return defaults, nil
	}
	if err != nil {
		return defaults, fmt.Errorf("appdata: read settings %q: %w", path, err)
	}
	s := defaults
	if err := json.Unmarshal(data, &s); err != nil {
		return defaults, fmt.Errorf("appdata: parse settings %q: %w", path, err)
	}
	return s.normalized(), nil
}

// SaveSettings writes settings to path, creating parent directories and
// replacing the file atomically. Invalid values are normalized before writing.
func SaveSettings(path string, s Settings) error {
	if path == "" {
		return fmt.Errorf("appdata: empty settings path")
	}
	data, err := json.MarshalIndent(s.normalized(), "", "  ")
	if err != nil {
		return fmt.Errorf("appdata: encode settings: %w", err)
	}
	data = append(data, '\n')
	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("appdata: save settings %q: %w", path, err)
	}
	return nil
}
