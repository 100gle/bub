package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync/atomic"
	"unicode"

	"github.com/bubbuild/bub/desktop/internal/markdown"
)

const (
	// pageExt is the file extension of every page this package creates.
	pageExt = ".md"
	// maxSlugRunes bounds generated file names so they stay portable.
	maxSlugRunes = 80
	// maxNameAttempts bounds the collision search for generated names.
	maxNameAttempts = 1000
)

// ReadPage returns the raw Markdown of the page at rel.
func (w *Workspace) ReadPage(rel string) (string, error) {
	rel, err := w.pageRel(rel)
	if err != nil {
		return "", err
	}
	r, err := w.openRoot()
	if err != nil {
		return "", err
	}
	defer r.Close()
	data, err := r.ReadFile(rel)
	if err != nil {
		return "", fmt.Errorf("workspace: read page %q: %w", rel, err)
	}
	return string(data), nil
}

// WritePage replaces the page at rel with markdown, creating parent
// directories as needed. The write is atomic: readers see either the old or
// the new content, never a partial file.
func (w *Workspace) WritePage(rel, markdown string) error {
	rel, err := w.pageRel(rel)
	if err != nil {
		return err
	}
	r, err := w.openRoot()
	if err != nil {
		return err
	}
	defer r.Close()
	if dir := path.Dir(rel); dir != "." {
		if err := r.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("workspace: create directory %q: %w", dir, err)
		}
	}
	return writeAtomic(r, rel, []byte(markdown))
}

// CreatePage creates a page titled title in the directory dirRel ("" is the
// workspace root) and returns its workspace-relative path. The file name is
// Slug(title) plus ".md", with a "-2", "-3", ... suffix when that name is
// already taken. Content starts as "# title\n\n".
func (w *Workspace) CreatePage(dirRel, title string) (string, error) {
	dir, err := cleanDirRel(dirRel)
	if err != nil {
		return "", err
	}
	title = singleLine(title)
	if title == "" {
		title = "Untitled"
	}
	r, err := w.openRoot()
	if err != nil {
		return "", err
	}
	defer r.Close()
	if dir != "." {
		if err := r.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("workspace: create directory %q: %w", dir, err)
		}
	}
	rel, err := freePageRel(r, dir, Slug(title), pageExt, "")
	if err != nil {
		return "", err
	}
	if err := writeAtomic(r, rel, []byte("# "+title+"\n\n")); err != nil {
		return "", err
	}
	return rel, nil
}

// CreateFolder creates the directory parentRel/name ("" parent is the
// workspace root) and returns its workspace-relative path. Unlike page file
// names, folder names are kept human-readable; they must be plain names
// without path separators.
func (w *Workspace) CreateFolder(parentRel, name string) (string, error) {
	parent, err := cleanDirRel(parentRel)
	if err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if err := validFolderName(name); err != nil {
		return "", err
	}
	rel := path.Join(parent, name)
	r, err := w.openRoot()
	if err != nil {
		return "", err
	}
	defer r.Close()
	if parent != "." {
		if err := r.MkdirAll(parent, 0o755); err != nil {
			return "", fmt.Errorf("workspace: create directory %q: %w", parent, err)
		}
	}
	if err := r.Mkdir(rel, 0o755); err != nil {
		return "", fmt.Errorf("workspace: create folder %q: %w", rel, err)
	}
	return rel, nil
}

// Delete removes the page or directory at rel, recursively for directories.
// Deleting something that does not exist is not an error.
func (w *Workspace) Delete(rel string) error {
	clean, err := cleanRel(rel)
	if err != nil {
		return err
	}
	if clean == "." {
		return fmt.Errorf("%w: refusing to delete the workspace root", ErrInvalidPath)
	}
	r, err := w.openRoot()
	if err != nil {
		return err
	}
	defer r.Close()
	if err := r.RemoveAll(clean); err != nil {
		return fmt.Errorf("workspace: delete %q: %w", clean, err)
	}
	return nil
}

// Rename gives the page at rel a new title. It renames the file to the
// slugified title (keeping its extension) and rewrites the page's first
// Markdown heading, adding one when the page has none. The new
// workspace-relative path is returned.
func (w *Workspace) Rename(rel, title string) (string, error) {
	rel, err := w.pageRel(rel)
	if err != nil {
		return "", err
	}
	title = singleLine(title)
	if title == "" {
		return "", fmt.Errorf("%w: empty title", ErrInvalidPath)
	}
	r, err := w.openRoot()
	if err != nil {
		return "", err
	}
	defer r.Close()
	info, err := r.Lstat(rel)
	if err != nil {
		return "", fmt.Errorf("workspace: rename page %q: %w", rel, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%w: %q is a directory", ErrInvalidPath, rel)
	}
	data, err := r.ReadFile(rel)
	if err != nil {
		return "", fmt.Errorf("workspace: read page %q: %w", rel, err)
	}
	ext := path.Ext(rel)
	if ext == "" {
		ext = pageExt
	}
	dir := path.Dir(rel)
	newRel, err := freePageRel(r, dir, Slug(title), ext, rel)
	if err != nil {
		return "", err
	}
	if newRel != rel {
		if err := r.Rename(rel, newRel); err != nil {
			return "", fmt.Errorf("workspace: rename page %q to %q: %w", rel, newRel, err)
		}
	}
	if err := writeAtomic(r, newRel, []byte(markdown.ReplaceTitle(string(data), title))); err != nil {
		return "", err
	}
	return newRel, nil
}

// TitleOf returns the text of the first heading of markdown, or "" when it has
// none. The document is parsed with goldmark, so a heading is the one CommonMark
// sees: a heading inside a fenced block, an indented block or a quotation is not
// the page's, and a leading YAML front matter block is skipped.
func TitleOf(source string) string {
	return markdown.Title(source)
}

// Slug turns a page title into a safe file name. Unicode letters, numbers and
// combining marks are kept (lowercased); every other run is collapsed to a
// single "-"; path separators and punctuation therefore cannot survive. The
// result is at most 80 runes, never starts or ends with "-", and falls back to
// "untitled".
func Slug(title string) string {
	var b strings.Builder
	dashed := true
	runes := 0
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) {
			if runes >= maxSlugRunes {
				continue
			}
			b.WriteRune(r)
			runes++
			dashed = false
			continue
		}
		if !dashed && runes > 0 {
			b.WriteByte('-')
			dashed = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "untitled"
	}
	return slug
}

// freePageRel picks an unused page path: dir/base+ext, else dir/base-2+ext,
// and so on. An existing file at keep (the page being renamed) counts as free.
func freePageRel(r *os.Root, dir, base, ext, keep string) (string, error) {
	for attempt := 1; attempt <= maxNameAttempts; attempt++ {
		name := base + ext
		if attempt > 1 {
			name = fmt.Sprintf("%s-%d%s", base, attempt, ext)
		}
		rel := path.Join(dir, name)
		if rel == keep {
			return rel, nil
		}
		_, err := r.Lstat(rel)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return rel, nil
		case err != nil:
			return "", fmt.Errorf("workspace: check %q: %w", rel, err)
		}
	}
	return "", fmt.Errorf("workspace: no free page name for %q after %d attempts", base, maxNameAttempts)
}

// validFolderName rejects names that are empty, hidden, or contain path
// separators.
func validFolderName(name string) error {
	switch {
	case name == "" || name == "." || name == "..":
		return fmt.Errorf("%w: invalid folder name %q", ErrInvalidPath, name)
	case strings.HasPrefix(name, "."):
		return fmt.Errorf("%w: folder name %q is hidden", ErrInvalidPath, name)
	case strings.ContainsAny(name, `/\`):
		return fmt.Errorf("%w: folder name %q contains a path separator", ErrInvalidPath, name)
	case strings.ContainsRune(name, 0):
		return fmt.Errorf("%w: folder name contains NUL", ErrInvalidPath)
	}
	return nil
}

// singleLine flattens a title to one line by collapsing all whitespace,
// including newlines and tabs, to single spaces.
func singleLine(title string) string {
	return strings.Join(strings.Fields(title), " ")
}

// writeAtomic writes data to rel through a hidden temporary file in the same
// directory and renames it into place.
func writeAtomic(r *os.Root, rel string, data []byte) error {
	dir := path.Dir(rel)
	tmp, f, err := createTemp(r, dir)
	if err != nil {
		return err
	}
	discard := func() {
		_ = f.Close()
		_ = r.Remove(tmp)
	}
	if _, err := f.Write(data); err != nil {
		discard()
		return fmt.Errorf("workspace: write page %q: %w", rel, err)
	}
	if err := f.Close(); err != nil {
		_ = r.Remove(tmp)
		return fmt.Errorf("workspace: write page %q: %w", rel, err)
	}
	if err := r.Rename(tmp, rel); err != nil {
		_ = r.Remove(tmp)
		return fmt.Errorf("workspace: replace page %q: %w", rel, err)
	}
	return nil
}

// createTemp creates a unique hidden file in dir (relative to root).
func createTemp(r *os.Root, dir string) (string, *os.File, error) {
	for attempt := 0; attempt < 10; attempt++ {
		name := ".tmp-" + randomID()
		rel := name
		if dir != "." {
			rel = path.Join(dir, name)
		}
		f, err := r.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		switch {
		case err == nil:
			return rel, f, nil
		case errors.Is(err, fs.ErrExist):
			continue
		default:
			return "", nil, fmt.Errorf("workspace: create temporary file in %q: %w", dir, err)
		}
	}
	return "", nil, fmt.Errorf("workspace: could not create a temporary file in %q", dir)
}

// randomID returns a short random hexadecimal identifier.
func randomID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%x-%x", os.Getpid(), fallbackID.Add(1))
	}
	return hex.EncodeToString(buf[:])
}

// fallbackID keeps generated names unique if crypto/rand is unavailable.
var fallbackID atomic.Uint64
