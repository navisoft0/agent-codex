// Package merge implements the file-level 3-way merge behind `shu update`:
// the recorded ancestor snapshot is the base, the working copy is ours, the
// upstream latest is theirs. Non-overlapping changes combine; overlapping
// changes are written back with git-style conflict markers, never resolved
// silently.
package merge

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// Dirs merges latestDir into workDir using ancDir as the common ancestor,
// file by file. It returns the relative paths that need manual resolution:
// content conflicts carry git conflict markers in the working file; add/delete
// conflicts keep the safer side (never silently dropping local edits).
func Dirs(workDir, ancDir, latestDir string) ([]string, error) {
	rels := map[string]bool{}
	for _, d := range []string{workDir, ancDir, latestDir} {
		files, err := list(d)
		if err != nil {
			return nil, err
		}
		for _, r := range files {
			rels[r] = true
		}
	}
	sorted := make([]string, 0, len(rels))
	for r := range rels {
		sorted = append(sorted, r)
	}
	sort.Strings(sorted)

	var conflicts []string
	for _, rel := range sorted {
		w, wok, err := read(workDir, rel)
		if err != nil {
			return conflicts, err
		}
		a, aok, err := read(ancDir, rel)
		if err != nil {
			return conflicts, err
		}
		l, lok, err := read(latestDir, rel)
		if err != nil {
			return conflicts, err
		}
		wp := filepath.Join(workDir, rel)

		switch {
		case !lok && !aok:
			// Local-only addition (or long gone everywhere): keep as is.
		case !lok && !wok:
			// Deleted on both sides: nothing to do.
		case !lok:
			// Deleted upstream. Safe to delete only if the local copy is
			// untouched since the ancestor.
			if bytes.Equal(w, a) {
				if err := os.Remove(wp); err != nil {
					return conflicts, err
				}
			} else {
				conflicts = append(conflicts, rel) // edited locally; kept
			}
		case !wok && !aok:
			// Added upstream: install it.
			if err := write(wp, l); err != nil {
				return conflicts, err
			}
		case !wok:
			// Deleted locally. The deletion stands unless upstream changed
			// the file since the ancestor — then restore latest and flag it.
			if !bytes.Equal(a, l) {
				if err := write(wp, l); err != nil {
					return conflicts, err
				}
				conflicts = append(conflicts, rel)
			}
		default:
			// Present on both sides.
			switch {
			case aok && bytes.Equal(a, l):
				// Upstream unchanged: local edits (if any) stand.
			case bytes.Equal(w, l):
				// Both ended up identical.
			case aok && bytes.Equal(w, a):
				// Local unchanged: take upstream.
				if err := write(wp, l); err != nil {
					return conflicts, err
				}
			default:
				conflicted, err := mergeFile(wp, filepath.Join(ancDir, rel), latestDir, rel, aok)
				if err != nil {
					return conflicts, err
				}
				if conflicted {
					conflicts = append(conflicts, rel)
				}
			}
		}
	}
	return conflicts, nil
}

// mergeFile 3-way merges one file in place via git merge-file. A file added
// on both sides has no ancestor; an empty base gives a full-file conflict.
func mergeFile(workPath, basePath, latestDir, rel string, haveBase bool) (bool, error) {
	if !haveBase {
		tmp, err := os.CreateTemp("", "shu-empty-base-*")
		if err != nil {
			return false, err
		}
		tmp.Close()
		defer os.Remove(tmp.Name())
		basePath = tmp.Name()
	}
	cmd := exec.Command("git", "merge-file", "-p",
		"-L", "local", "-L", "ancestor", "-L", "latest",
		workPath, basePath, filepath.Join(latestDir, rel))
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb

	conflicted := false
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		// merge-file exits with the number of conflicts (1..127); anything
		// else is a real error.
		if errors.As(err, &ee) && ee.ExitCode() >= 1 && ee.ExitCode() <= 127 {
			conflicted = true
		} else {
			return false, fmt.Errorf("git merge-file %s: %v\n%s", workPath, err, errb.String())
		}
	}
	return conflicted, os.WriteFile(workPath, out.Bytes(), 0o644)
}

func list(dir string) ([]string, error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
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
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	return files, err
}

func read(dir, rel string) ([]byte, bool, error) {
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

func write(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
