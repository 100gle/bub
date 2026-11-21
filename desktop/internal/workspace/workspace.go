// Package workspace stores a writing workspace as plain Markdown files on disk.
//
// A workspace is a directory chosen by the user. Every page is a Markdown file
// somewhere below that directory and every directory groups pages. Page paths
// handed to and returned from this package are slash-separated paths relative
// to the workspace root, such as "notes/hello.md".
//
// The package is deliberately strict about paths: absolute paths, ".."
// components and symlinks that point outside the root are rejected. All file
// system access goes through os.Root, which enforces the same rules.
package workspace

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrInvalidPath reports a path or name that cannot be used inside a
// workspace. It is wrapped by the errors returned from this package.
var ErrInvalidPath = errors.New("workspace: invalid path")

// Workspace is a directory of Markdown pages.
type Workspace struct {
	// Root is the absolute path of the workspace directory.
	Root string
}

// Open opens the workspace rooted at root, creating the directory when it does
// not exist yet. A relative root is resolved against the current directory.
func Open(root string) (*Workspace, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: empty workspace root", ErrInvalidPath)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("workspace: resolve root %q: %w", root, err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("workspace: create root %q: %w", abs, err)
	}
	// Reject roots that exist but are not directories.
	r, err := os.OpenRoot(abs)
	if err != nil {
		return nil, fmt.Errorf("workspace: open root %q: %w", abs, err)
	}
	if err := r.Close(); err != nil {
		return nil, fmt.Errorf("workspace: close root %q: %w", abs, err)
	}
	return &Workspace{Root: abs}, nil
}

// openRoot opens the workspace root for a single operation.
func (w *Workspace) openRoot() (*os.Root, error) {
	if w == nil || strings.TrimSpace(w.Root) == "" {
		return nil, fmt.Errorf("%w: no workspace root", ErrInvalidPath)
	}
	r, err := os.OpenRoot(w.Root)
	if err != nil {
		return nil, fmt.Errorf("workspace: open %q: %w", w.Root, err)
	}
	return r, nil
}

// cleanRel validates rel as a slash-separated path below the workspace root and
// returns its cleaned form. "" and "." both denote the root itself.
func cleanRel(rel string) (string, error) {
	switch {
	case strings.ContainsRune(rel, 0):
		return "", fmt.Errorf("%w: path contains NUL", ErrInvalidPath)
	case strings.Contains(rel, `\`):
		return "", fmt.Errorf("%w: path %q must use forward slashes", ErrInvalidPath, rel)
	case filepath.IsAbs(rel), path.IsAbs(rel):
		return "", fmt.Errorf("%w: path %q is absolute", ErrInvalidPath, rel)
	case filepath.VolumeName(rel) != "":
		return "", fmt.Errorf("%w: path %q has a volume name", ErrInvalidPath, rel)
	case len(rel) >= 2 && rel[1] == ':':
		return "", fmt.Errorf("%w: path %q has a drive letter", ErrInvalidPath, rel)
	}
	clean := path.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: path %q escapes the workspace", ErrInvalidPath, rel)
	}
	return clean, nil
}

// pageRel validates rel as a path to a page, rejecting the workspace root
// itself and other directory-only spellings.
func (w *Workspace) pageRel(rel string) (string, error) {
	clean, err := cleanRel(rel)
	if err != nil {
		return "", err
	}
	if clean == "." {
		return "", fmt.Errorf("%w: %q is not a file", ErrInvalidPath, rel)
	}
	return clean, nil
}

// cleanDirRel validates rel as a directory path inside the workspace. "" and
// "." denote the root.
func cleanDirRel(rel string) (string, error) {
	return cleanRel(rel)
}

// Abs returns the absolute path of the workspace entry rel, for binding a page
// or folder to its file. "" and "." return the workspace root. The path is
// validated like every other operation, but the entry does not have to exist.
func (w *Workspace) Abs(rel string) (string, error) {
	if w == nil || strings.TrimSpace(w.Root) == "" {
		return "", fmt.Errorf("%w: no workspace root", ErrInvalidPath)
	}
	clean, err := cleanRel(rel)
	if err != nil {
		return "", err
	}
	root := w.Root
	if !filepath.IsAbs(root) {
		abs, err := filepath.Abs(root)
		if err != nil {
			return "", fmt.Errorf("workspace: resolve root %q: %w", root, err)
		}
		root = abs
	}
	if clean == "." {
		return root, nil
	}
	return filepath.Join(root, filepath.FromSlash(clean)), nil
}

// DefaultRoot returns the default workspace root, ~/Bub.
func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("workspace: locate home directory: %w", err)
	}
	return filepath.Join(home, "Bub"), nil
}
