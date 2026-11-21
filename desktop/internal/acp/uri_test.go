package acp

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFileURI(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name  string
		file  string
		holds string // the escape the URI must carry for the file's name
	}{
		{"plain", "a.md", "a.md"},
		{"space", "My Notes.md", "%20"},
		{"hash", "a#b.md", "%23"},
		{"query", "a?b.md", "%3F"},
		{"unicode", "中文.md", "%E4%B8%AD%E6%96%87"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FileURI(filepath.Join(dir, tt.file))
			if !strings.HasPrefix(got, "file:///") {
				t.Fatalf("FileURI = %q, want a file:// URI", got)
			}
			if !strings.Contains(got, tt.holds) {
				t.Errorf("FileURI = %q, want it to hold %q", got, tt.holds)
			}
			// Nothing may read as a fragment, a query or a raw space.
			if strings.ContainsAny(strings.TrimPrefix(got, "file://"), "#? ") {
				t.Errorf("FileURI = %q leaves a raw #, ? or space", got)
			}
		})
	}
}

func TestFileURIMakesPathsAbsolute(t *testing.T) {
	got := FileURI("relative.md")
	want := "file:///" + strings.TrimPrefix(filepath.ToSlash(mustAbs(t, "relative.md")), "/")
	if got != want {
		t.Errorf("FileURI(relative.md) = %q, want %q", got, want)
	}
}

// mustAbs resolves path against the working directory.
func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("Abs(%q): %v", path, err)
	}
	return abs
}
