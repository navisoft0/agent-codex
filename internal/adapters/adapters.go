// Package adapters projects a skill's canonical working copy into the native
// format of each agent surface. Projections are generated artifacts: they are
// rendered from the working copy, never edited in place.
package adapters

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/navisoft0/agent-codex/internal/fsutil"
	"github.com/navisoft0/agent-codex/internal/skillmeta"
)

// Primary is the surface `acx add` installs into. It is the working copy
// itself — the thing drift is measured on — not a generated projection.
const Primary = "claude-code"

// Context carries what an adapter needs to render one skill.
type Context struct {
	Root   string // consuming repo root
	Skill  string // skill name
	SrcDir string // the skill's working-copy directory (the primary install)
	Meta   skillmeta.Meta
}

// Adapter renders a skill into one surface and returns the repo-relative
// path it wrote.
type Adapter interface {
	Project(ctx Context) (string, error)
}

var registry = map[string]Adapter{
	"codex":     codexAdapter{},
	"cursor":    cursorAdapter{},
	"agents-md": markerAdapter{file: "AGENTS.md"},
	"claude-md": markerAdapter{file: "CLAUDE.md"},
}

// Get returns the adapter for a surface name. Primary has no adapter.
func Get(name string) (Adapter, error) {
	if a, ok := registry[name]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("unknown surface %q (known: %s)", name, strings.Join(Names(), ", "))
}

// Known reports whether name is a recognized surface, including the primary.
func Known(name string) bool {
	_, ok := registry[name]
	return ok || name == Primary
}

// Names lists all recognized surface names, primary first.
func Names() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return append([]string{Primary}, names...)
}

// codexAdapter mirrors the full skill directory into Codex's skills layout.
type codexAdapter struct{}

func (codexAdapter) Project(c Context) (string, error) {
	rel := filepath.Join(".codex", "skills", c.Skill)
	if err := fsutil.CopyDir(c.SrcDir, filepath.Join(c.Root, rel)); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// cursorAdapter renders the skill body as a Cursor project rule (.mdc).
type cursorAdapter struct{}

func (cursorAdapter) Project(c Context) (string, error) {
	b, err := os.ReadFile(filepath.Join(c.SrcDir, "SKILL.md"))
	if err != nil {
		return "", err
	}
	rel := filepath.Join(".cursor", "rules", c.Skill+".mdc")
	content := fmt.Sprintf("---\ndescription: %q\nalwaysApply: false\n---\n\n%s",
		c.Meta.Description, skillmeta.Body(b))
	path := filepath.Join(c.Root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// markerAdapter maintains a managed section inside a shared instructions file
// (AGENTS.md or CLAUDE.md), delimited by acx markers so re-projection
// replaces exactly its own section and nothing else.
type markerAdapter struct {
	file string
}

func (m markerAdapter) Project(c Context) (string, error) {
	b, err := os.ReadFile(filepath.Join(c.SrcDir, "SKILL.md"))
	if err != nil {
		return "", err
	}
	title := c.Skill
	if c.Meta.Version != "" {
		title += " v" + c.Meta.Version
	}
	content := "## Skill: " + title + "\n\n" +
		strings.TrimRight(skillmeta.Body(b), "\n") + "\n"
	if err := UpsertSection(filepath.Join(c.Root, m.file), c.Skill, content); err != nil {
		return "", err
	}
	return m.file, nil
}
