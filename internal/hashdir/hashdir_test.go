package hashdir

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHashDeterministicAndSensitive(t *testing.T) {
	a := t.TempDir()
	write(t, a, "SKILL.md", "---\nname: x\n---\nbody\n")
	write(t, a, "references/notes.md", "notes\n")

	b := t.TempDir()
	write(t, b, "references/notes.md", "notes\n")
	write(t, b, "SKILL.md", "---\nname: x\n---\nbody\n")

	ha, err := Hash(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := Hash(b)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Errorf("same content, different hashes: %s vs %s", ha, hb)
	}

	write(t, b, "SKILL.md", "---\nname: x\n---\nchanged\n")
	hb2, err := Hash(b)
	if err != nil {
		t.Fatal(err)
	}
	if hb2 == hb {
		t.Error("content change did not change hash")
	}
}

func TestHashSkipsGitDir(t *testing.T) {
	a := t.TempDir()
	write(t, a, "SKILL.md", "body\n")
	ha, err := Hash(a)
	if err != nil {
		t.Fatal(err)
	}
	write(t, a, ".git/config", "noise\n")
	hb, err := Hash(a)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Error(".git contents affected the hash")
	}
}
