package workspace

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

// Node is one entry in the workspace tree: a page or a directory of pages.
type Node struct {
	// Name is the display name: a page's title, or the directory's name.
	Name string
	// Rel is the slash-separated path relative to the workspace root.
	Rel string
	// IsDir reports whether the node is a directory.
	IsDir bool
	// ModTime is the modification time of the page or directory.
	ModTime time.Time
	// Children holds a directory's entries, sorted directories first and then
	// pages, each case-insensitively by name. It is nil for pages.
	Children []*Node
}

// Tree lists the workspace recursively. Only Markdown pages and the
// directories that contain them appear; empty directories, dotfiles and
// dot-directories are omitted. Directories come before pages and names sort
// case-insensitively.
func (w *Workspace) Tree() ([]*Node, error) {
	r, err := w.openRoot()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return walk(r, ".")
}

// walk lists the directory dirRel, which is "." for the workspace root.
func walk(r *os.Root, dirRel string) ([]*Node, error) {
	f, err := r.Open(dirRel)
	if err != nil {
		return nil, fmt.Errorf("workspace: list %q: %w", dirRel, err)
	}
	entries, readErr := f.ReadDir(-1)
	closeErr := f.Close()
	if readErr != nil {
		return nil, fmt.Errorf("workspace: list %q: %w", dirRel, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("workspace: list %q: %w", dirRel, closeErr)
	}

	var dirs, pages []*Node
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		rel := path.Join(dirRel, name)
		if entry.IsDir() {
			children, err := walk(r, rel)
			if err != nil {
				return nil, err
			}
			if len(children) == 0 {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return nil, fmt.Errorf("workspace: stat %q: %w", rel, err)
			}
			dirs = append(dirs, &Node{
				Name:     name,
				Rel:      rel,
				IsDir:    true,
				ModTime:  info.ModTime(),
				Children: children,
			})
			continue
		}
		if !entry.Type().IsRegular() || !strings.EqualFold(path.Ext(name), ".md") {
			continue
		}
		data, err := r.ReadFile(rel)
		if err != nil {
			return nil, fmt.Errorf("workspace: read %q: %w", rel, err)
		}
		title := TitleOf(string(data))
		if title == "" {
			title = strings.TrimSuffix(name, path.Ext(name))
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("workspace: stat %q: %w", rel, err)
		}
		pages = append(pages, &Node{
			Name:    title,
			Rel:     rel,
			ModTime: info.ModTime(),
		})
	}

	sortNodes(dirs)
	sortNodes(pages)
	return append(dirs, pages...), nil
}

// sortNodes sorts nodes by case-insensitive name, breaking ties with the exact
// name so ordering is deterministic.
func sortNodes(nodes []*Node) {
	sort.SliceStable(nodes, func(i, j int) bool {
		a, b := strings.ToLower(nodes[i].Name), strings.ToLower(nodes[j].Name)
		if a != b {
			return a < b
		}
		return nodes[i].Name < nodes[j].Name
	})
}

// Pages lists the pages of a tree in reading order, folders dropped: the
// order the sidebar shows them in, which a mention or a picker searches.
func Pages(nodes []*Node) []*Node {
	var out []*Node
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.IsDir {
			out = append(out, Pages(node.Children)...)
			continue
		}
		out = append(out, node)
	}
	return out
}
