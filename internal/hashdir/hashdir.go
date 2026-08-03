// Package hashdir produces a deterministic content hash for a directory tree,
// used to compare skill snapshots without byte-by-byte diffing.
package hashdir

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Hash returns "sha256:<hex>" over the directory's files: sorted relative
// paths and their contents. File modes and timestamps are ignored so the hash
// is stable across checkouts; .git directories are skipped.
func Hash(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)

	h := sha256.New()
	for _, f := range files {
		rel, err := filepath.Rel(dir, f)
		if err != nil {
			return "", err
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(rel), len(b))
		h.Write(b)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
