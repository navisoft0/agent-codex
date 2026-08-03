// Package lockfile reads and writes skills.lock, the per-repo record of which
// skills are installed, from where, and the exact upstream content last synced
// (the ancestor hash that makes 3-way merge possible later).
package lockfile

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Name is the lockfile's filename at the consuming repo's root.
const Name = "skills.lock"

// AncestorsDir is where full ancestor snapshots live, one subdirectory per
// skill. Snapshots are committed alongside the lockfile so drift is computable
// on any checkout without network access.
const AncestorsDir = ".agent-codex/ancestors"

// Entry records one installed skill.
type Entry struct {
	// Source is the upstream as given to `acx add`: a git URL or local path.
	Source string `json:"source"`
	// Version is the skill's frontmatter version at sync time, if declared.
	Version string `json:"version,omitempty"`
	// Hash is the upstream content hash at sync time (the ancestor hash).
	Hash string `json:"hash"`
	// Path is the install location relative to the repo root, slash-separated.
	Path string `json:"path"`
	// Surfaces lists the agent surfaces this skill is projected into. Empty
	// means just the primary (claude-code) working copy.
	Surfaces []string `json:"surfaces,omitempty"`
}

// File is the parsed lockfile.
type File struct {
	Version int              `json:"version"`
	Skills  map[string]Entry `json:"skills"`
}

// Load reads root/skills.lock. A missing lockfile yields an empty File.
func Load(root string) (*File, error) {
	b, err := os.ReadFile(filepath.Join(root, Name))
	if errors.Is(err, fs.ErrNotExist) {
		return &File{Version: 1, Skills: map[string]Entry{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	if f.Skills == nil {
		f.Skills = map[string]Entry{}
	}
	return &f, nil
}

// Save writes the lockfile back to root/skills.lock.
func (f *File) Save(root string) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, Name), append(b, '\n'), 0o644)
}

// AncestorDir returns the snapshot directory for a skill under root.
func AncestorDir(root, skill string) string {
	return filepath.Join(root, filepath.FromSlash(AncestorsDir), skill)
}
