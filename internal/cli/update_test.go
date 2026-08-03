package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const bodyTemplate = "---\nname: demo\ndescription: Demo skill.\nversion: %VER%\n---\n\n# Demo\n\nalpha\n\nmiddle stays\n\nomega\n"

func upstreamSkill(t *testing.T, up, ver, replace, with string) {
	t.Helper()
	content := strings.ReplaceAll(bodyTemplate, "%VER%", ver)
	if replace != "" {
		content = strings.ReplaceAll(content, replace, with)
	}
	write(t, filepath.Join(up, "skills", "demo", "SKILL.md"), content)
}

func lockVersion(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "skills.lock"))
	if err != nil {
		t.Fatal(err)
	}
	var lf struct {
		Skills map[string]struct {
			Version string `json:"version"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(b, &lf); err != nil {
		t.Fatal(err)
	}
	return lf.Skills["demo"].Version
}

func TestUpdateFastForwardMergeConflict(t *testing.T) {
	up := t.TempDir()
	upstreamSkill(t, up, "1.0.0", "", "")

	consumer := t.TempDir()
	t.Chdir(consumer)
	if _, code := capture(t, func() int { return Run([]string{"add", "demo", "--from", up}) }); code != 0 {
		t.Fatal("add failed")
	}
	install := filepath.Join(consumer, ".claude", "skills", "demo", "SKILL.md")

	// Upstream-only change: fast-forward.
	upstreamSkill(t, up, "1.1.0", "omega", "omega improved")
	out, code := capture(t, func() int { return Run([]string{"update"}) })
	if code != 0 || !strings.Contains(out, "fast-forwarded 1.0.0 -> 1.1.0") {
		t.Fatalf("fast-forward: code=%d out=%s", code, out)
	}
	if got := states(t)["demo"]; got.State != "aligned" {
		t.Fatalf("after fast-forward: %s, want aligned", got.State)
	}
	if v := lockVersion(t, consumer); v != "1.1.0" {
		t.Fatalf("lock version = %s, want 1.1.0", v)
	}

	// Local edit + non-overlapping upstream change: clean merge, edits kept.
	b, _ := os.ReadFile(install)
	write(t, install, strings.ReplaceAll(string(b), "alpha", "alpha LOCAL"))
	upstreamSkill(t, up, "1.2.0", "omega", "omega v2")
	out, code = capture(t, func() int { return Run([]string{"update"}) })
	if code != 0 || !strings.Contains(out, "merged 1.2.0 cleanly") {
		t.Fatalf("clean merge: code=%d out=%s", code, out)
	}
	merged, _ := os.ReadFile(install)
	if !strings.Contains(string(merged), "alpha LOCAL") || !strings.Contains(string(merged), "omega v2") {
		t.Fatalf("merge lost a side:\n%s", merged)
	}
	if got := states(t)["demo"]; got.State != "drifted" {
		t.Fatalf("after clean merge: %s, want drifted (local edit vs new ancestor)", got.State)
	}

	// Overlapping change: conflict markers, exit 1, ancestor still advances.
	upstreamSkill(t, up, "1.3.0", "alpha", "alpha UPSTREAM")
	out, code = capture(t, func() int { return Run([]string{"update"}) })
	if code != 1 || !strings.Contains(out, "conflicts in: SKILL.md") {
		t.Fatalf("conflict merge: code=%d out=%s", code, out)
	}
	conflicted, _ := os.ReadFile(install)
	if !strings.Contains(string(conflicted), "<<<<<<< local") {
		t.Fatalf("no conflict markers:\n%s", conflicted)
	}
	if v := lockVersion(t, consumer); v != "1.3.0" {
		t.Fatalf("lock version after conflict = %s, want 1.3.0", v)
	}

	// Missing working copy: update reinstalls.
	if err := os.RemoveAll(filepath.Dir(install)); err != nil {
		t.Fatal(err)
	}
	out, code = capture(t, func() int { return Run([]string{"update", "demo"}) })
	if code != 0 || !strings.Contains(out, "reinstalled") {
		t.Fatalf("reinstall: code=%d out=%s", code, out)
	}
	if got := states(t)["demo"]; got.State != "aligned" {
		t.Fatalf("after reinstall: %s, want aligned", got.State)
	}
}

func TestVerify(t *testing.T) {
	up := t.TempDir()
	upstreamSkill(t, up, "1.0.0", "", "")
	consumer := t.TempDir()
	t.Chdir(consumer)
	if _, code := capture(t, func() int { return Run([]string{"add", "demo", "--from", up}) }); code != 0 {
		t.Fatal("add failed")
	}

	if out, code := capture(t, func() int { return Run([]string{"verify"}) }); code != 0 {
		t.Fatalf("verify on clean install: code=%d out=%s", code, out)
	}

	// Tamper with the working copy.
	write(t, filepath.Join(consumer, ".claude", "skills", "demo", "SKILL.md"), "tampered\n")
	out, code := capture(t, func() int { return Run([]string{"verify", "--json"}) })
	if code != 1 || !strings.Contains(out, `"working": "modified"`) {
		t.Fatalf("verify after tamper: code=%d out=%s", code, out)
	}
	if !strings.Contains(out, `"ancestor": "match"`) {
		t.Fatalf("ancestor should still match: %s", out)
	}
}

func TestAddWithSurfacesAndProject(t *testing.T) {
	up := t.TempDir()
	upstreamSkill(t, up, "1.0.0", "", "")
	consumer := t.TempDir()
	t.Chdir(consumer)

	out, code := capture(t, func() int {
		return Run([]string{"add", "demo", "--from", up, "--surfaces", "claude-code,codex,cursor,agents-md"})
	})
	if code != 0 {
		t.Fatalf("add failed: %s", out)
	}
	for _, p := range []string{
		".claude/skills/demo/SKILL.md",
		".codex/skills/demo/SKILL.md",
		".cursor/rules/demo.mdc",
		"AGENTS.md",
	} {
		if _, err := os.Stat(filepath.Join(consumer, filepath.FromSlash(p))); err != nil {
			t.Errorf("missing projection %s: %v", p, err)
		}
	}

	// Unknown surface is rejected.
	if _, code := capture(t, func() int {
		return Run([]string{"add", "demo", "--from", up, "--surfaces", "warp-drive"})
	}); code == 0 {
		t.Error("unknown surface should fail")
	}

	// Edit working copy, re-project: AGENTS.md section follows the working copy.
	install := filepath.Join(consumer, ".claude", "skills", "demo", "SKILL.md")
	b, _ := os.ReadFile(install)
	write(t, install, strings.ReplaceAll(string(b), "alpha", "alpha EDITED"))
	if out, code := capture(t, func() int { return Run([]string{"project"}) }); code != 0 {
		t.Fatalf("project failed: %s", out)
	}
	agents, _ := os.ReadFile(filepath.Join(consumer, "AGENTS.md"))
	if !strings.Contains(string(agents), "alpha EDITED") {
		t.Errorf("AGENTS.md not re-rendered:\n%s", agents)
	}
}
