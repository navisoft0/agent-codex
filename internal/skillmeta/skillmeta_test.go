package skillmeta

import "testing"

func TestParse(t *testing.T) {
	m, err := Parse([]byte("---\nname: deploy-checklist\ndescription: \"Use when deploying: covers rollback.\"\nversion: 1.4.0\n---\n\n# Body\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "deploy-checklist" {
		t.Errorf("name = %q", m.Name)
	}
	if m.Description != "Use when deploying: covers rollback." {
		t.Errorf("description = %q", m.Description)
	}
	if m.Version != "1.4.0" {
		t.Errorf("version = %q", m.Version)
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := Parse([]byte("# no frontmatter\n")); err == nil {
		t.Error("expected error for missing frontmatter")
	}
	if _, err := Parse([]byte("---\nname: x\n")); err == nil {
		t.Error("expected error for unterminated frontmatter")
	}
}
