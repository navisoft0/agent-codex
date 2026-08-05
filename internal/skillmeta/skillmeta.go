// Package skillmeta reads the YAML frontmatter of a SKILL.md file. Only the
// flat scalar keys the spec requires are parsed; unknown keys are ignored.
package skillmeta

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Meta is the subset of SKILL.md frontmatter shu cares about.
type Meta struct {
	Name        string
	Description string
	Version     string
}

// Parse extracts frontmatter from SKILL.md contents.
func Parse(b []byte) (Meta, error) {
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return Meta{}, errors.New("SKILL.md: missing frontmatter (expected leading ---)")
	}
	var m Meta
	for _, ln := range lines[1:] {
		if strings.TrimSpace(ln) == "---" {
			return m, nil
		}
		k, v, ok := strings.Cut(ln, ":")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch strings.TrimSpace(k) {
		case "name":
			m.Name = v
		case "description":
			m.Description = v
		case "version":
			m.Version = v
		}
	}
	return Meta{}, errors.New("SKILL.md: unterminated frontmatter")
}

// Body returns the markdown body after the frontmatter block, or the whole
// input when no frontmatter is present.
func Body(b []byte) string {
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	lines := strings.Split(s, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return s
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.TrimLeft(strings.Join(lines[i+1:], "\n"), "\n")
		}
	}
	return s
}

// Load reads and parses dir/SKILL.md.
func Load(dir string) (Meta, error) {
	b, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return Meta{}, err
	}
	return Parse(b)
}
