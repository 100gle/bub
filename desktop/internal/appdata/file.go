package appdata

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeFileAtomic writes data to path by writing a temporary file in the same
// directory and renaming it into place. Readers therefore see either the old
// or the new file, never a partial one.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %q: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file in %q: %w", dir, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace %q: %w", path, err)
	}
	return nil
}
