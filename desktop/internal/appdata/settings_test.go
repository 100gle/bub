package appdata_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bubbuild/bub/desktop/internal/appdata"
)

// isolateConfig points the home and config directories at a temporary
// directory so tests never touch the real user configuration.
func isolateConfig(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData"))
	return home
}

func TestDir(t *testing.T) {
	isolateConfig(t)
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	got, err := appdata.Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	want := filepath.Join(base, "Bub Desktop")
	if got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestDefaultSettings(t *testing.T) {
	home := isolateConfig(t)
	got := appdata.DefaultSettings()
	want := appdata.Settings{
		Workspace:          filepath.Join(home, "Bub"),
		AgentCmd:           "",
		Theme:              appdata.ThemeSystem,
		EditorWidth:        720,
		EditorFontSize:     16,
		ShowOutline:        true,
		AIContextNeighbors: true,
		AIContextOutline:   true,
		Autosave:           true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DefaultSettings() = %+v, want %+v", got, want)
	}
}

func TestLoadSettingsMissingFile(t *testing.T) {
	isolateConfig(t)
	path := filepath.Join(t.TempDir(), "settings.json")
	got, err := appdata.LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings missing: %v", err)
	}
	if want := appdata.DefaultSettings(); !reflect.DeepEqual(got, want) {
		t.Errorf("LoadSettings missing = %+v, want defaults %+v", got, want)
	}
}

func TestLoadSettingsMalformed(t *testing.T) {
	isolateConfig(t)
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := appdata.LoadSettings(path)
	if err == nil {
		t.Fatal("LoadSettings malformed returned nil error")
	}
	if want := appdata.DefaultSettings(); !reflect.DeepEqual(got, want) {
		t.Errorf("LoadSettings malformed = %+v, want defaults %+v", got, want)
	}
}

func TestLoadSettingsPartial(t *testing.T) {
	isolateConfig(t)
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark","autosave":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := appdata.LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	want := appdata.DefaultSettings()
	want.Theme = appdata.ThemeDark
	want.Autosave = false
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadSettings partial = %+v, want %+v", got, want)
	}
}

func TestLoadSettingsNormalizes(t *testing.T) {
	isolateConfig(t)
	path := filepath.Join(t.TempDir(), "settings.json")
	raw := `{"theme":"neon","editorWidth":-5,"editorFontSize":0,"workspace":"   "}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := appdata.LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if got.Theme != appdata.ThemeSystem {
		t.Errorf("Theme = %q, want %q", got.Theme, appdata.ThemeSystem)
	}
	if got.EditorWidth != 720 || got.EditorFontSize != 16 {
		t.Errorf("editor sizes = %dx%d, want 720x16", got.EditorWidth, got.EditorFontSize)
	}
	wantWorkspace := filepath.Join(os.Getenv("HOME"), "Bub")
	if got.Workspace != wantWorkspace {
		t.Errorf("Workspace = %q, want default %q", got.Workspace, wantWorkspace)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	isolateConfig(t)
	path := filepath.Join(t.TempDir(), "nested", "dir", "settings.json")
	want := appdata.Settings{
		Workspace:          filepath.Join(t.TempDir(), "workspace"),
		AgentCmd:           "bub acp --workspace .",
		Theme:              appdata.ThemeDark,
		EditorWidth:        880,
		EditorFontSize:     18,
		ShowOutline:        false,
		AIContextNeighbors: false,
		AIContextOutline:   false,
		Autosave:           false,
	}
	if err := appdata.SaveSettings(path, want); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	got, err := appdata.LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "}\n") {
		t.Errorf("settings file does not end with a newline: %q", data)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("settings JSON: %v", err)
	}
	for _, key := range []string{
		"workspace", "agentCmd", "theme", "editorWidth", "editorFontSize",
		"showOutline", "aiContextNeighbors", "aiContextOutline", "autosave",
	} {
		if _, ok := fields[key]; !ok {
			t.Errorf("settings JSON missing key %q", key)
		}
	}
}

func TestSaveSettingsEmptyPath(t *testing.T) {
	if err := appdata.SaveSettings("", appdata.DefaultSettings()); err == nil {
		t.Fatal("SaveSettings(\"\") returned nil error")
	}
	if _, err := appdata.LoadSettings(""); err == nil {
		t.Fatal("LoadSettings(\"\") returned nil error")
	}
}

func TestLoadSettingsKeepsExplicitFalse(t *testing.T) {
	isolateConfig(t)
	path := filepath.Join(t.TempDir(), "settings.json")
	raw := `{"showOutline":false,"aiContextNeighbors":false,"aiContextOutline":false,"autosave":false}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := appdata.LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if got.ShowOutline || got.AIContextNeighbors || got.AIContextOutline || got.Autosave {
		t.Errorf("explicit false values were lost: %+v", got)
	}
	if got.EditorWidth != 720 || got.Theme != appdata.ThemeSystem {
		t.Errorf("missing values did not default: %+v", got)
	}
}
