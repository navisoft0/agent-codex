package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const exampleSkillMD = `---
name: example-skill
description: Example skill scaffolded by shu init. Replace this description with when-to-use guidance so agents trigger it correctly.
version: 0.1.0
---

# Example skill

Replace this body with the skill's instructions. Keep supporting material in
references/ and executable helpers in scripts/; put regression prompts in
evals/ so version bumps can be gated on behavior.
`

const exampleEvals = `# Regression suite for example-skill (promptfoo-compatible).
# Each version bump should be non-inferior to the current version on this suite.
prompts:
  - "Demonstrate the behavior this skill is supposed to produce."
tests:
  - assert:
      - type: contains
        value: "expected behavior"
`

// runInit scaffolds a canonical upstream skills repo: a skills/ directory
// with one example skill laid out per the agentskills.io convention.
func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	if _, err := parse(fs, args); err != nil {
		return 2
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	skillsDir := filepath.Join(cwd, "skills")
	if _, err := os.Stat(skillsDir); err == nil {
		fmt.Println("skills/ already exists — adopting it as the canonical layout; nothing to scaffold")
		return 0
	}
	example := filepath.Join(skillsDir, "example-skill")
	if err := os.MkdirAll(filepath.Join(example, "evals"), 0o755); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(example, "SKILL.md"), []byte(exampleSkillMD), 0o644); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(example, "evals", "evals.yaml"), []byte(exampleEvals), 0o644); err != nil {
		return fail(err)
	}
	fmt.Println("scaffolded canonical upstream:")
	fmt.Println("  skills/example-skill/SKILL.md")
	fmt.Println("  skills/example-skill/evals/evals.yaml")
	fmt.Println("next: rename example-skill, commit, and `shu add <skill> --from <this repo>` in a consuming repo")
	return 0
}
