// Package upstream resolves a skill source — a local directory or a git URL —
// to a readable local directory, caching git clones under the user cache dir.
package upstream

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Resolve returns a local directory for source. A source that exists on disk
// is used in place; anything else is treated as a git URL, cloned shallowly
// into the user cache on first use and pulled on later uses when refresh is
// set. If a refresh fails (e.g. offline), the cached directory is returned
// together with the error so callers can degrade to last-known content.
func Resolve(source string, refresh bool) (string, error) {
	if fi, err := os.Stat(source); err == nil && fi.IsDir() {
		return filepath.Abs(source)
	}

	ucd, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(source))
	dir := filepath.Join(ucd, "shuhari", hex.EncodeToString(sum[:])[:12])

	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return "", err
		}
		if out, err := exec.Command("git", "clone", "--depth", "1", source, dir).CombinedOutput(); err != nil {
			os.RemoveAll(dir)
			return "", fmt.Errorf("clone %s: %v\n%s", source, err, out)
		}
		return dir, nil
	}
	if refresh {
		if out, err := exec.Command("git", "-C", dir, "pull", "--ff-only").CombinedOutput(); err != nil {
			return dir, fmt.Errorf("refresh %s: %v\n%s", source, err, out)
		}
	}
	return dir, nil
}

// SkillDir locates a skill inside a resolved upstream: skills/<name> per the
// canonical layout, falling back to <name> at the root. A skill directory is
// one that contains SKILL.md.
func SkillDir(base, name string) (string, error) {
	for _, c := range []string{filepath.Join(base, "skills", name), filepath.Join(base, name)} {
		if _, err := os.Stat(filepath.Join(c, "SKILL.md")); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("skill %q not found under %s (looked in skills/%s and %s)", name, base, name, name)
}
