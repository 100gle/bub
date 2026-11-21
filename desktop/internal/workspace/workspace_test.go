package workspace_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bubbuild/bub/desktop/internal/workspace"
)

// mustOpen opens a fresh workspace rooted at a temporary directory.
func mustOpen(t *testing.T) *workspace.Workspace {
	t.Helper()
	w, err := workspace.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return w
}

// writeFile writes a file, creating parent directories.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func TestSlug(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"plain words", "The Quiet Web", "the-quiet-web"},
		{"punctuation", "Hello,   World!", "hello-world"},
		{"path separators", "a/b\\c", "a-b-c"},
		{"traversal", "../../etc/passwd", "etc-passwd"},
		{"unicode letters", "Über Café", "über-café"},
		{"cjk", "你好，世界", "你好-世界"},
		{"empty", "", "untitled"},
		{"only punctuation", "!!!", "untitled"},
		{"leading and trailing dashes", "-leading-and-trailing-", "leading-and-trailing"},
		{"underscores", "a__b", "a-b"},
		{"dots", "v1.2 Notes", "v1-2-notes"},
		{"extension", "file.md", "file-md"},
		{"hashes", "C# notes", "c-notes"},
		{"whitespace", " \t\n ", "untitled"},
		{"long", strings.Repeat("A", 200), strings.Repeat("a", 80)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := workspace.Slug(tt.title); got != tt.want {
				t.Errorf("Slug(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestTitleOf(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     string
	}{
		{"h1", "# The Quiet Web\n\nBody", "The Quiet Web"},
		{"later heading", "intro\n\n## Sub\n", "Sub"},
		{"no heading", "plain text only\n", ""},
		{"empty", "", ""},
		{"closing hashes", "## Spaced ##\n", "Spaced"},
		{"hash in text", "# C#\n", "C#"},
		{"seven hashes", "####### nope\n", ""},
		{"no space after hashes", "#nope\n", ""},
		{"four space indent", "    # code, not a heading\n", ""},
		{"inside backtick fence", "```\n# not a title\n```\n# Real\n", "Real"},
		{"inside tilde fence", "~~~~\n# not a title\n~~~~\n# Real\n", "Real"},
		{"unclosed fence hides heading", "```\n# nope\n``\n# Real\n", ""},
		{"longer closing fence", "```\n# nope\n````\n# Real\n", "Real"},
		{"front matter", "---\ntitle: YAML\n---\n# Body\n", "Body"},
		{"unterminated front matter", "---\ntitle: YAML\n# Body\n", "Body"},
		{"bom", "\ufeff# BOM title\n", "BOM title"},
		{"empty heading", "# \n", ""},
		{"crlf", "## Windows Title\r\n", "Windows Title"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := workspace.TitleOf(tt.markdown); got != tt.want {
				t.Errorf("TitleOf(%q) = %q, want %q", tt.markdown, got, tt.want)
			}
		})
	}
}

func TestOpenCreatesRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "nested", "workspace")
	w, err := workspace.Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if w.Root != root {
		t.Errorf("Root = %q, want %q", w.Root, root)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("root directory not created: info=%v err=%v", info, err)
	}

	if _, err := workspace.Open(""); !errors.Is(err, workspace.ErrInvalidPath) {
		t.Errorf("Open(\"\") error = %v, want ErrInvalidPath", err)
	}

	file := filepath.Join(base, "file")
	writeFile(t, file, "x")
	if _, err := workspace.Open(file); err == nil {
		t.Error("Open on a regular file succeeded, want error")
	}
}

func TestDefaultRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME-based default is not used on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := workspace.DefaultRoot()
	if err != nil {
		t.Fatalf("DefaultRoot: %v", err)
	}
	if want := filepath.Join(home, "Bub"); got != want {
		t.Errorf("DefaultRoot() = %q, want %q", got, want)
	}
}

func TestTree(t *testing.T) {
	w := mustOpen(t)
	writeFile(t, filepath.Join(w.Root, "a.md"), "# Apple\n")
	writeFile(t, filepath.Join(w.Root, "b.md"), "plain text\n")
	writeFile(t, filepath.Join(w.Root, "notes.txt"), "not a page\n")
	writeFile(t, filepath.Join(w.Root, ".hidden.md"), "# Hidden\n")
	writeFile(t, filepath.Join(w.Root, ".secret", "x.md"), "# Secret\n")
	writeFile(t, filepath.Join(w.Root, "Zeta", "one.md"), "# One\n")
	writeFile(t, filepath.Join(w.Root, "alpha", "two.md"), "# Two\n")
	writeFile(t, filepath.Join(w.Root, "alpha", "nested", "three.md"), "# Three\n")
	if err := os.MkdirAll(filepath.Join(w.Root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(w.Root, "textonly", "readme.txt"), "x\n")

	top, err := w.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	var got []string
	for _, n := range top {
		got = append(got, n.Name)
	}
	want := []string{"alpha", "Zeta", "Apple", "b"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Tree top level = %v, want %v", got, want)
	}
	if len(top) != len(want) {
		t.Fatalf("Tree returned %d top-level nodes, want %d", len(top), len(want))
	}
	if !top[0].IsDir || !top[1].IsDir {
		t.Error("directories must come before pages")
	}
	if top[2].Rel != "a.md" || top[2].Name != "Apple" {
		t.Errorf("page node = %+v, want Rel a.md Name Apple", top[2])
	}
	if top[3].Name != "b" {
		t.Errorf("page without heading Name = %q, want file stem %q", top[3].Name, "b")
	}
	if top[0].ModTime.IsZero() || top[2].ModTime.IsZero() {
		t.Error("ModTime should be set")
	}

	alpha := top[0]
	if len(alpha.Children) != 2 {
		t.Fatalf("alpha children = %d, want 2", len(alpha.Children))
	}
	if !alpha.Children[0].IsDir || alpha.Children[0].Rel != "alpha/nested" {
		t.Errorf("alpha first child = %+v, want nested directory", alpha.Children[0])
	}
	if alpha.Children[1].Rel != "alpha/two.md" || alpha.Children[1].Name != "Two" {
		t.Errorf("alpha page = %+v, want Rel alpha/two.md Name Two", alpha.Children[1])
	}
}

func TestReadWritePage(t *testing.T) {
	w := mustOpen(t)
	if err := w.WritePage("notes/page.md", "# Hi\n\nBody\n"); err != nil {
		t.Fatalf("WritePage: %v", err)
	}
	got, err := w.ReadPage("notes/page.md")
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if got != "# Hi\n\nBody\n" {
		t.Errorf("ReadPage = %q", got)
	}
	if err := w.WritePage("notes/page.md", "# Hi again\n"); err != nil {
		t.Fatalf("WritePage overwrite: %v", err)
	}
	got, err = w.ReadPage("notes/page.md")
	if err != nil {
		t.Fatalf("ReadPage after overwrite: %v", err)
	}
	if got != "# Hi again\n" {
		t.Errorf("ReadPage after overwrite = %q", got)
	}

	entries, err := os.ReadDir(filepath.Join(w.Root, "notes"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "page.md" {
		t.Errorf("directory entries = %v, want [page.md] (no temp files)", entries)
	}

	if _, err := w.ReadPage("missing.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadPage missing error = %v, want fs.ErrNotExist", err)
	}
	if _, err := w.ReadPage("notes"); err == nil {
		t.Error("ReadPage on a directory succeeded, want error")
	}
}

func TestCreatePage(t *testing.T) {
	t.Run("root and heading", func(t *testing.T) {
		w := mustOpen(t)
		rel, err := w.CreatePage("", "Hello World")
		if err != nil {
			t.Fatalf("CreatePage: %v", err)
		}
		if rel != "hello-world.md" {
			t.Errorf("rel = %q, want hello-world.md", rel)
		}
		got, err := w.ReadPage(rel)
		if err != nil {
			t.Fatal(err)
		}
		if got != "# Hello World\n\n" {
			t.Errorf("content = %q, want %q", got, "# Hello World\n\n")
		}
	})

	t.Run("collision suffix", func(t *testing.T) {
		w := mustOpen(t)
		want := []string{"hello-world.md", "hello-world-2.md", "hello-world-3.md"}
		for i, wantRel := range want {
			rel, err := w.CreatePage("", "Hello World")
			if err != nil {
				t.Fatalf("CreatePage %d: %v", i, err)
			}
			if rel != wantRel {
				t.Errorf("CreatePage %d rel = %q, want %q", i, rel, wantRel)
			}
		}
	})

	t.Run("subdirectory", func(t *testing.T) {
		w := mustOpen(t)
		rel, err := w.CreatePage("notes", "Hello World")
		if err != nil {
			t.Fatalf("CreatePage: %v", err)
		}
		if rel != "notes/hello-world.md" {
			t.Errorf("rel = %q, want notes/hello-world.md", rel)
		}
	})

	t.Run("empty title", func(t *testing.T) {
		w := mustOpen(t)
		rel, err := w.CreatePage("", "   ")
		if err != nil {
			t.Fatalf("CreatePage: %v", err)
		}
		if rel != "untitled.md" {
			t.Errorf("rel = %q, want untitled.md", rel)
		}
		got, _ := w.ReadPage(rel)
		if got != "# Untitled\n\n" {
			t.Errorf("content = %q", got)
		}
	})

	t.Run("multiline title", func(t *testing.T) {
		w := mustOpen(t)
		rel, err := w.CreatePage("", "  Multi\tline\nTitle ")
		if err != nil {
			t.Fatalf("CreatePage: %v", err)
		}
		if rel != "multi-line-title.md" {
			t.Errorf("rel = %q, want multi-line-title.md", rel)
		}
		got, _ := w.ReadPage(rel)
		if got != "# Multi line Title\n\n" {
			t.Errorf("content = %q", got)
		}
	})

	t.Run("title with slash", func(t *testing.T) {
		w := mustOpen(t)
		rel, err := w.CreatePage("", "A/B")
		if err != nil {
			t.Fatalf("CreatePage: %v", err)
		}
		if rel != "a-b.md" {
			t.Errorf("rel = %q, want a-b.md", rel)
		}
	})

	t.Run("escape rejected", func(t *testing.T) {
		w := mustOpen(t)
		if _, err := w.CreatePage("..", "Nope"); !errors.Is(err, workspace.ErrInvalidPath) {
			t.Errorf("CreatePage(..) error = %v, want ErrInvalidPath", err)
		}
	})
}

func TestCreateFolder(t *testing.T) {
	w := mustOpen(t)
	rel, err := w.CreateFolder("", "Drafts")
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if rel != "Drafts" {
		t.Errorf("rel = %q, want Drafts", rel)
	}
	info, err := os.Stat(filepath.Join(w.Root, "Drafts"))
	if err != nil || !info.IsDir() {
		t.Fatalf("folder not created: info=%v err=%v", info, err)
	}

	nested, err := w.CreateFolder("Drafts", "Sub")
	if err != nil {
		t.Fatalf("CreateFolder nested: %v", err)
	}
	if nested != "Drafts/Sub" {
		t.Errorf("nested rel = %q, want Drafts/Sub", nested)
	}

	if _, err := w.CreateFolder("", "Drafts"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("duplicate CreateFolder error = %v, want fs.ErrExist", err)
	}

	for _, name := range []string{"", ".", "..", ".hidden", "a/b", `a\b`} {
		if _, err := w.CreateFolder("", name); !errors.Is(err, workspace.ErrInvalidPath) {
			t.Errorf("CreateFolder(%q) error = %v, want ErrInvalidPath", name, err)
		}
	}
	if _, err := w.CreateFolder("../escape", "x"); !errors.Is(err, workspace.ErrInvalidPath) {
		t.Errorf("CreateFolder traversing parent error = %v, want ErrInvalidPath", err)
	}
}

func TestDelete(t *testing.T) {
	w := mustOpen(t)
	writeFile(t, filepath.Join(w.Root, "a.md"), "# A\n")
	writeFile(t, filepath.Join(w.Root, "dir", "b.md"), "# B\n")

	if err := w.Delete("a.md"); err != nil {
		t.Fatalf("Delete file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(w.Root, "a.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("page still exists after delete: %v", err)
	}
	if err := w.Delete("dir"); err != nil {
		t.Fatalf("Delete dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(w.Root, "dir")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("folder still exists after delete: %v", err)
	}
	if err := w.Delete("missing.md"); err != nil {
		t.Errorf("Delete missing = %v, want nil", err)
	}
	for _, rel := range []string{".", ""} {
		if err := w.Delete(rel); !errors.Is(err, workspace.ErrInvalidPath) {
			t.Errorf("Delete(%q) = %v, want ErrInvalidPath", rel, err)
		}
	}
}

func TestRename(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		w := mustOpen(t)
		if err := w.WritePage("page.md", "# Old\n\nBody\n"); err != nil {
			t.Fatal(err)
		}
		rel, err := w.Rename("page.md", "New Title")
		if err != nil {
			t.Fatalf("Rename: %v", err)
		}
		if rel != "new-title.md" {
			t.Errorf("rel = %q, want new-title.md", rel)
		}
		got, err := w.ReadPage(rel)
		if err != nil {
			t.Fatal(err)
		}
		if got != "# New Title\n\nBody\n" {
			t.Errorf("content = %q", got)
		}
		if _, err := os.Stat(filepath.Join(w.Root, "page.md")); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("old page still exists: %v", err)
		}
	})

	t.Run("collision", func(t *testing.T) {
		w := mustOpen(t)
		writeFile(t, filepath.Join(w.Root, "page.md"), "# Old\n")
		writeFile(t, filepath.Join(w.Root, "new-title.md"), "# Existing\n")
		rel, err := w.Rename("page.md", "New Title")
		if err != nil {
			t.Fatalf("Rename: %v", err)
		}
		if rel != "new-title-2.md" {
			t.Errorf("rel = %q, want new-title-2.md", rel)
		}
		existing, _ := w.ReadPage("new-title.md")
		if existing != "# Existing\n" {
			t.Errorf("existing page was modified: %q", existing)
		}
	})

	t.Run("adds heading when missing", func(t *testing.T) {
		w := mustOpen(t)
		writeFile(t, filepath.Join(w.Root, "notes.md"), "Body text\n")
		rel, err := w.Rename("notes.md", "Notes")
		if err != nil {
			t.Fatalf("Rename: %v", err)
		}
		if rel != "notes.md" {
			t.Errorf("rel = %q, want notes.md", rel)
		}
		got, _ := w.ReadPage(rel)
		if got != "# Notes\n\nBody text\n" {
			t.Errorf("content = %q", got)
		}
	})

	t.Run("adds heading after front matter", func(t *testing.T) {
		w := mustOpen(t)
		writeFile(t, filepath.Join(w.Root, "page.md"), "---\ntags: [a]\n---\nBody\n")
		rel, err := w.Rename("page.md", "Titled")
		if err != nil {
			t.Fatalf("Rename: %v", err)
		}
		got, _ := w.ReadPage(rel)
		want := "---\ntags: [a]\n---\n# Titled\n\nBody\n"
		if got != want {
			t.Errorf("content = %q, want %q", got, want)
		}
	})

	t.Run("keeps heading level", func(t *testing.T) {
		w := mustOpen(t)
		writeFile(t, filepath.Join(w.Root, "page.md"), "## Old\n\nBody\n")
		rel, err := w.Rename("page.md", "New")
		if err != nil {
			t.Fatalf("Rename: %v", err)
		}
		got, _ := w.ReadPage(rel)
		if got != "## New\n\nBody\n" {
			t.Errorf("content = %q", got)
		}
	})

	t.Run("errors", func(t *testing.T) {
		w := mustOpen(t)
		writeFile(t, filepath.Join(w.Root, "page.md"), "# Old\n")
		folder, err := w.CreateFolder("", "Folder")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Rename("page.md", "  "); !errors.Is(err, workspace.ErrInvalidPath) {
			t.Errorf("empty title error = %v, want ErrInvalidPath", err)
		}
		if _, err := w.Rename(folder, "Renamed"); !errors.Is(err, workspace.ErrInvalidPath) {
			t.Errorf("rename directory error = %v, want ErrInvalidPath", err)
		}
		if _, err := w.Rename("../page.md", "X"); !errors.Is(err, workspace.ErrInvalidPath) {
			t.Errorf("rename traversal error = %v, want ErrInvalidPath", err)
		}
		if _, err := w.Rename("missing.md", "X"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("rename missing error = %v, want fs.ErrNotExist", err)
		}
	})
}

func TestAbs(t *testing.T) {
	w := mustOpen(t)
	got, err := w.Abs("")
	if err != nil || got != w.Root {
		t.Errorf("Abs(\"\") = %q, %v; want %q, nil", got, err, w.Root)
	}
	got, err = w.Abs("a/b.md")
	want := filepath.Join(w.Root, "a", "b.md")
	if err != nil || got != want {
		t.Errorf("Abs(a/b.md) = %q, %v; want %q, nil", got, err, want)
	}
	for _, rel := range []string{"../x.md", "/etc/passwd", `a\b.md`} {
		if _, err := w.Abs(rel); !errors.Is(err, workspace.ErrInvalidPath) {
			t.Errorf("Abs(%q) error = %v, want ErrInvalidPath", rel, err)
		}
	}

	literal := &workspace.Workspace{Root: w.Root}
	if got, err := literal.Abs("p.md"); err != nil || got != filepath.Join(w.Root, "p.md") {
		t.Errorf("literal Workspace.Abs = %q, %v", got, err)
	}
}

func TestPathTraversalRejected(t *testing.T) {
	w := mustOpen(t)
	outside := filepath.Join(filepath.Dir(w.Root), "escape.md")
	paths := []struct {
		name string
		rel  string
	}{
		{"parent", "../escape.md"},
		{"grandparent", "../../escape.md"},
		{"absolute", "/tmp/escape.md"},
		{"nested traversal", "a/../../escape.md"},
		{"windows separator", `..\escape.md`},
		{"windows drive", `C:\escape.md`},
		{"empty", ""},
	}
	for _, tt := range paths {
		rel := tt.rel
		t.Run(tt.name, func(t *testing.T) {
			if _, err := w.ReadPage(rel); !errors.Is(err, workspace.ErrInvalidPath) {
				t.Errorf("ReadPage(%q) error = %v, want ErrInvalidPath", rel, err)
			}
			if err := w.WritePage(rel, "# Pwned\n"); !errors.Is(err, workspace.ErrInvalidPath) {
				t.Errorf("WritePage(%q) error = %v, want ErrInvalidPath", rel, err)
			}
			if err := w.Delete(rel); !errors.Is(err, workspace.ErrInvalidPath) {
				t.Errorf("Delete(%q) error = %v, want ErrInvalidPath", rel, err)
			}
			if _, err := w.Rename(rel, "Pwned"); !errors.Is(err, workspace.ErrInvalidPath) {
				t.Errorf("Rename(%q) error = %v, want ErrInvalidPath", rel, err)
			}
		})
	}
	if _, err := os.Stat(outside); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("file was created outside the workspace: %v", err)
	}
}

func TestSymlinkEscapeRejected(t *testing.T) {
	w := mustOpen(t)
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.md"), "# Secret\n")

	if err := os.Symlink(outside, filepath.Join(w.Root, "link")); err != nil {
		t.Skipf("cannot create symlinks: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(w.Root, "page.md")); err != nil {
		t.Skipf("cannot create symlinks: %v", err)
	}

	if _, err := w.ReadPage("link/secret.md"); err == nil {
		t.Error("ReadPage through escaping symlink succeeded, want error")
	}
	if err := w.WritePage("link/new.md", "# New\n"); err == nil {
		t.Error("WritePage through escaping symlink succeeded, want error")
	}
	if _, err := w.ReadPage("page.md"); err == nil {
		t.Error("ReadPage of escaping symlink file succeeded, want error")
	}

	tree, err := w.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	if len(tree) != 0 {
		t.Errorf("Tree = %v, want symlinks omitted", tree)
	}

	if err := w.Delete("link"); err != nil {
		t.Fatalf("Delete symlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "secret.md")); err != nil {
		t.Errorf("deleting a symlink removed its target: %v", err)
	}
}

func TestTreeModTimeAndSortTies(t *testing.T) {
	w := mustOpen(t)
	writeFile(t, filepath.Join(w.Root, "b.md"), "# Banana\n")
	writeFile(t, filepath.Join(w.Root, "A.md"), "# apple\n")
	writeFile(t, filepath.Join(w.Root, "zoo", "x.md"), "# Zoo\n")
	writeFile(t, filepath.Join(w.Root, "Alpha", "y.md"), "# Alpha\n")

	tree, err := w.Tree()
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	var names []string
	for _, n := range tree {
		names = append(names, n.Name)
	}
	want := []string{"Alpha", "zoo", "apple", "Banana"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("Tree names = %v, want %v", names, want)
	}
	for _, n := range tree {
		if n.ModTime.IsZero() || n.ModTime.After(time.Now().Add(time.Minute)) {
			t.Errorf("node %q has implausible ModTime %v", n.Name, n.ModTime)
		}
	}
}

func TestPagesDropsFoldersInReadingOrder(t *testing.T) {
	nodes := []*workspace.Node{
		{Name: "Beta", Rel: "beta.md"},
		{Name: "Notes", Rel: "notes", IsDir: true, Children: []*workspace.Node{
			{Name: "Alpha", Rel: "notes/alpha.md"},
			{Name: "Deep", Rel: "notes/deep", IsDir: true, Children: []*workspace.Node{
				{Name: "Gamma", Rel: "notes/deep/gamma.md"},
			}},
		}},
	}
	var got []string
	for _, page := range workspace.Pages(nodes) {
		if page.IsDir {
			t.Fatalf("a folder came back as a page: %q", page.Rel)
		}
		got = append(got, page.Rel)
	}
	want := []string{"beta.md", "notes/alpha.md", "notes/deep/gamma.md"}
	if !slices.Equal(got, want) {
		t.Errorf("pages = %v, want %v", got, want)
	}
	if got := workspace.Pages(nil); got != nil {
		t.Errorf("an empty tree listed %v", got)
	}
}
