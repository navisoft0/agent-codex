package merge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func readBack(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const base = "alpha\none\ntwo\nthree\nomega\n"

func TestCleanMerge(t *testing.T) {
	work := setup(t, map[string]string{"SKILL.md": "ALPHA LOCAL\none\ntwo\nthree\nomega\n"})
	anc := setup(t, map[string]string{"SKILL.md": base})
	latest := setup(t, map[string]string{"SKILL.md": "alpha\none\ntwo\nthree\nOMEGA UPSTREAM\n"})

	conflicts, err := Dirs(work, anc, latest)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %v", conflicts)
	}
	got := readBack(t, work, "SKILL.md")
	if got != "ALPHA LOCAL\none\ntwo\nthree\nOMEGA UPSTREAM\n" {
		t.Errorf("merge result:\n%s", got)
	}
}

func TestConflict(t *testing.T) {
	work := setup(t, map[string]string{"SKILL.md": "alpha local\none\ntwo\nthree\nomega\n"})
	anc := setup(t, map[string]string{"SKILL.md": base})
	latest := setup(t, map[string]string{"SKILL.md": "alpha upstream\none\ntwo\nthree\nomega\n"})

	conflicts, err := Dirs(work, anc, latest)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || conflicts[0] != "SKILL.md" {
		t.Fatalf("conflicts = %v, want [SKILL.md]", conflicts)
	}
	got := readBack(t, work, "SKILL.md")
	for _, marker := range []string{"<<<<<<< local", "alpha local", "alpha upstream", ">>>>>>> latest"} {
		if !strings.Contains(got, marker) {
			t.Errorf("merged file missing %q:\n%s", marker, got)
		}
	}
}

func TestAddsAndDeletes(t *testing.T) {
	work := setup(t, map[string]string{
		"SKILL.md":  base,
		"stale.md":  "old\n",            // deleted upstream, untouched locally -> removed
		"local.md":  "mine\n",           // local-only addition -> kept
		"edited.md": "edited locally\n", // deleted upstream, edited locally -> conflict, kept
	})
	anc := setup(t, map[string]string{
		"SKILL.md":  base,
		"stale.md":  "old\n",
		"edited.md": "original\n",
	})
	latest := setup(t, map[string]string{
		"SKILL.md":          base,
		"references/new.md": "fresh upstream\n", // added upstream -> installed
	})

	conflicts, err := Dirs(work, anc, latest)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || conflicts[0] != "edited.md" {
		t.Fatalf("conflicts = %v, want [edited.md]", conflicts)
	}
	if _, err := os.Stat(filepath.Join(work, "stale.md")); !os.IsNotExist(err) {
		t.Error("stale.md should have been removed")
	}
	if readBack(t, work, "local.md") != "mine\n" {
		t.Error("local.md should be untouched")
	}
	if readBack(t, work, "edited.md") != "edited locally\n" {
		t.Error("edited.md local content should be kept on delete/edit conflict")
	}
	if readBack(t, work, filepath.Join("references", "new.md")) != "fresh upstream\n" {
		t.Error("upstream-added file should be installed")
	}
}

func TestDeletedLocallyChangedUpstream(t *testing.T) {
	work := setup(t, map[string]string{"SKILL.md": base})
	anc := setup(t, map[string]string{"SKILL.md": base, "gone.md": "v1\n"})
	latest := setup(t, map[string]string{"SKILL.md": base, "gone.md": "v2\n"})

	conflicts, err := Dirs(work, anc, latest)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || conflicts[0] != "gone.md" {
		t.Fatalf("conflicts = %v, want [gone.md]", conflicts)
	}
	if readBack(t, work, "gone.md") != "v2\n" {
		t.Error("upstream-changed file should be restored when deleted locally")
	}
}
