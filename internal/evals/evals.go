// Package evals runs a skill's regression suite through a promptfoo-compatible
// runner. acx never interprets prompts itself — it locates the config, execs
// the runner, and reads back the stats — so any provider promptfoo supports
// (or any drop-in compatible tool) works.
package evals

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// EnvRunner overrides the default runner command, split on whitespace.
const EnvRunner = "ACX_EVAL_RUNNER"

// DefaultRunner returns the runner command: $ACX_EVAL_RUNNER if set,
// otherwise promptfoo via npx.
func DefaultRunner() []string {
	if v := strings.TrimSpace(os.Getenv(EnvRunner)); v != "" {
		return strings.Fields(v)
	}
	return []string{"npx", "--yes", "promptfoo@latest"}
}

// FindConfig locates the eval config inside a skill directory, or returns ""
// when the skill has no suite.
func FindConfig(skillDir string) string {
	for _, c := range []string{
		filepath.Join("evals", "promptfooconfig.yaml"),
		filepath.Join("evals", "evals.yaml"),
		filepath.Join("evals", "evals.yml"),
	} {
		if _, err := os.Stat(filepath.Join(skillDir, c)); err == nil {
			return c
		}
	}
	matches, _ := filepath.Glob(filepath.Join(skillDir, "evals", "*.yaml"))
	sort.Strings(matches)
	if len(matches) > 0 {
		rel, err := filepath.Rel(skillDir, matches[0])
		if err == nil {
			return rel
		}
	}
	return ""
}

// Result is what came back from a runner invocation.
type Result struct {
	Passed    bool
	Successes int
	Failures  int
	HasStats  bool
	Output    string // combined runner output, for diagnostics
}

// Run executes `<runner> eval -c <config> --output <tmp.json>` inside the
// skill directory. The runner's JSON output supplies pass/fail stats when it
// parses; the exit code is the fallback signal.
func Run(runner []string, skillDir, config string) (Result, error) {
	if len(runner) == 0 {
		return Result{}, errors.New("no eval runner configured")
	}
	tmp, err := os.CreateTemp("", "acx-eval-*.json")
	if err != nil {
		return Result{}, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	args := append(append([]string{}, runner[1:]...), "eval", "-c", config, "--output", tmp.Name())
	cmd := exec.Command(runner[0], args...)
	cmd.Dir = skillDir
	out, runErr := cmd.CombinedOutput()

	res := Result{Passed: runErr == nil, Output: string(out)}
	if runErr != nil {
		var ee *exec.ExitError
		if !errors.As(runErr, &ee) {
			return res, fmt.Errorf("eval runner %s: %v\n%s", runner[0], runErr, out)
		}
	}
	if b, err := os.ReadFile(tmp.Name()); err == nil && len(b) > 0 {
		var doc struct {
			Results struct {
				Stats *stats `json:"stats"`
			} `json:"results"`
			Stats *stats `json:"stats"`
		}
		if json.Unmarshal(b, &doc) == nil {
			s := doc.Results.Stats
			if s == nil {
				s = doc.Stats
			}
			if s != nil {
				res.HasStats = true
				res.Successes = s.Successes
				res.Failures = s.Failures
				res.Passed = s.Failures == 0
			}
		}
	}
	return res, nil
}

type stats struct {
	Successes int `json:"successes"`
	Failures  int `json:"failures"`
}

// Scaffold writes a starter suite for a skill that has none, returning the
// repo-relative config path. It refuses to overwrite an existing config.
func Scaffold(skillDir, name, description string) (string, error) {
	if c := FindConfig(skillDir); c != "" {
		return "", fmt.Errorf("eval config already exists: %s", c)
	}
	rel := filepath.Join("evals", "evals.yaml")
	path := filepath.Join(skillDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	content := fmt.Sprintf(`# Regression suite for %s (promptfoo-compatible).
# Each version bump should be non-inferior to the current version on this suite.
description: %q
prompts:
  - "{{input}}"
tests:
  - description: "replace with a real case demonstrating this skill's behavior"
    vars:
      input: "A task where the skill should change the outcome."
    assert:
      - type: contains
        value: "expected behavior"
`, name, name+" regression evals")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return rel, nil
}
