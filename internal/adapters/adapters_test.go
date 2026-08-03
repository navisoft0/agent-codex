package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/navisoft0/agent-codex/internal/skillmeta"
)

func skillDir(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	content := "---\nname: demo\ndescription: Demo skill.\nversion: 1.0.0\n---\n\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func ctx(t *testing.T, root, src string) Context {
	t.Helper()
	meta, err := skillmeta.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	return Context{Root: root, Skill: "demo", SrcDir: src, Meta: meta}
}

func TestCodexAdapter(t *testing.T) {
	root := t.TempDir()
	src := skillDir(t, "Body.\n")
	rel, err := codexAdapter{}.Project(ctx(t, root, src))
	if err != nil {
		t.Fatal(err)
	}
	if rel != ".codex/skills/demo" {
		t.Errorf("rel = %q", rel)
	}
	if _, err := os.Stat(filepath.Join(root, ".codex", "skills", "demo", "SKILL.md")); err != nil {
		t.Error(err)
	}
}

func TestCursorAdapter(t *testing.T) {
	root := t.TempDir()
	src := skillDir(t, "Body.\n")
	rel, err := cursorAdapter{}.Project(ctx(t, root, src))
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{`description: "Demo skill."`, "alwaysApply: false", "Body."} {
		if !strings.Contains(got, want) {
			t.Errorf("cursor rule missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "version: 1.0.0") {
		t.Error("cursor rule should not carry the SKILL.md frontmatter")
	}
}

func TestMarkerAdapterUpsert(t *testing.T) {
	root := t.TempDir()
	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("# My agents file\n\nhand-written intro\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := skillDir(t, "First body.\n")
	if _, err := (markerAdapter{file: "AGENTS.md"}).Project(ctx(t, root, src)); err != nil {
		t.Fatal(err)
	}
	// Re-project with changed content: section replaced, not duplicated.
	src2 := skillDir(t, "Second body.\n")
	if _, err := (markerAdapter{file: "AGENTS.md"}).Project(ctx(t, root, src2)); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "hand-written intro") {
		t.Error("existing content was clobbered")
	}
	if strings.Count(got, "<!-- acx:skill:demo:begin -->") != 1 {
		t.Errorf("expected exactly one managed section:\n%s", got)
	}
	if !strings.Contains(got, "Second body.") || strings.Contains(got, "First body.") {
		t.Errorf("section not replaced with latest content:\n%s", got)
	}
	if !strings.Contains(got, "## Skill: demo v1.0.0") {
		t.Errorf("missing section heading:\n%s", got)
	}
}

func TestKnownAndGet(t *testing.T) {
	for _, s := range []string{Primary, "codex", "cursor", "agents-md", "claude-md"} {
		if !Known(s) {
			t.Errorf("Known(%q) = false", s)
		}
	}
	if Known("copilot") {
		t.Error("copilot should be unknown for now")
	}
	if _, err := Get("nope"); err == nil {
		t.Error("Get(nope) should error")
	}
}
