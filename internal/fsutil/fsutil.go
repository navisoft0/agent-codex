// Package fsutil holds small filesystem helpers shared by commands.
package fsutil

import (
	"io/fs"
	"os"
	"path/filepath"
)

// CopyDir replaces dst with a copy of src, skipping any .git directories.
func CopyDir(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if d.Name() == ".git" && p != src {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, info.Mode().Perm())
	})
}

// DirExists reports whether path exists and is a directory.
func DirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
