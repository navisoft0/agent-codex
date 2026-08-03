package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// capture runs f with stdout redirected and returns what it printed.
func capture(t *testing.T, f func() int) (string, int) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := f()
	w.Close()
	os.Stdout = old
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b), code
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func states(t *testing.T) map[string]statusRow {
	t.Helper()
	out, _ := capture(t, func() int { return Run([]string{"status", "--json"}) })
	var rows []statusRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("status --json produced invalid JSON: %v\n%s", err, out)
	}
	m := map[string]statusRow{}
	for _, r := range rows {
		m[r.Skill] = r
	}
	return m
}

// TestAddStatusDiffLifecycle drives the full M0 loop against a local-path
// upstream: install, then walk the skill through every drift state.
func TestAddStatusDiffLifecycle(t *testing.T) {
	up := t.TempDir()
	upSkill := filepath.Join(up, "skills", "demo", "SKILL.md")
	write(t, upSkill, "---\nname: demo\ndescription: Demo skill.\nversion: 1.0.0\n---\n\nOriginal body.\n")

	consumer := t.TempDir()
	t.Chdir(consumer)

	if _, code := capture(t, func() int { return Run([]string{"add", "demo", "--from", up}) }); code != 0 {
		t.Fatalf("add exited %d", code)
	}
	install := filepath.Join(consumer, ".claude", "skills", "demo", "SKILL.md")
	if _, err := os.Stat(install); err != nil {
		t.Fatalf("skill not installed at Claude Code surface: %v", err)
	}
	if _, err := os.Stat(filepath.Join(consumer, "skills.lock")); err != nil {
		t.Fatalf("lockfile not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(consumer, ".agent-codex", "ancestors", "demo", "SKILL.md")); err != nil {
		t.Fatalf("ancestor snapshot not written: %v", err)
	}

	if got := states(t)["demo"]; got.State != "aligned" || got.Version != "1.0.0" {
		t.Fatalf("after add: state=%s version=%s, want aligned 1.0.0", got.State, got.Version)
	}
	if _, code := capture(t, func() int { return Run([]string{"status"}) }); code != 0 {
		t.Fatalf("status exited %d while aligned, want 0", code)
	}

	// Upstream moves, local untouched -> behind.
	write(t, upSkill, "---\nname: demo\ndescription: Demo skill.\nversion: 1.1.0\n---\n\nImproved body.\n")
	if got := states(t)["demo"]; got.State != "behind" {
		t.Fatalf("after upstream edit: state=%s, want behind", got.State)
	}
	if _, code := capture(t, func() int { return Run([]string{"status"}) }); code != 1 {
		t.Fatal("status should exit 1 when not aligned")
	}

	// Local edit on top -> diverged.
	write(t, install, "---\nname: demo\ndescription: Demo skill.\nversion: 1.0.0\n---\n\nLocal learnings.\n")
	if got := states(t)["demo"]; got.State != "diverged" {
		t.Fatalf("after both edits: state=%s, want diverged", got.State)
	}

	// Upstream back to the synced content, local edit remains -> drifted.
	write(t, upSkill, "---\nname: demo\ndescription: Demo skill.\nversion: 1.0.0\n---\n\nOriginal body.\n")
	if got := states(t)["demo"]; got.State != "drifted" {
		t.Fatalf("after upstream revert: state=%s, want drifted", got.State)
	}

	// diff shows the local edit against the ancestor.
	out, code := capture(t, func() int { return Run([]string{"diff", "demo"}) })
	if code != 0 {
		t.Fatalf("diff exited %d", code)
	}
	if want := "Local learnings."; !strings.Contains(out, want) {
		t.Errorf("diff output missing %q:\n%s", want, out)
	}

	// Offline status compares against the ancestor only -> drifted still.
	if out, _ := capture(t, func() int { return Run([]string{"status", "--json", "--offline"}) }); !strings.Contains(out, "drifted") {
		t.Errorf("offline status should report drifted:\n%s", out)
	}

	// Working copy removed -> missing.
	if err := os.RemoveAll(filepath.Join(consumer, ".claude", "skills", "demo")); err != nil {
		t.Fatal(err)
	}
	if got := states(t)["demo"]; got.State != "missing" {
		t.Fatalf("after removal: state=%s, want missing", got.State)
	}
}

func TestInitScaffold(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, code := capture(t, func() int { return Run([]string{"init"}) }); code != 0 {
		t.Fatalf("init exited %d", code)
	}
	for _, p := range []string{"skills/example-skill/SKILL.md", "skills/example-skill/evals/evals.yaml"} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	// Re-running adopts the existing layout instead of overwriting.
	if _, code := capture(t, func() int { return Run([]string{"init"}) }); code != 0 {
		t.Error("re-init should succeed as a no-op")
	}
}
