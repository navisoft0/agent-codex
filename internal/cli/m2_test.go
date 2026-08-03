package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubRunner writes a promptfoo-shaped script that reports the given stats.
func stubRunner(t *testing.T, successes, failures int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub-runner.sh")
	exitCode := 0
	if failures > 0 {
		exitCode = 1
	}
	content := fmt.Sprintf(`#!/bin/sh
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--output" ]; then out="$a"; fi
  prev="$a"
done
printf '{"results":{"stats":{"successes":%d,"failures":%d}}}' > "$out"
exit %d
`, successes, failures, exitCode)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func gitc(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := git(dir, args...)
	if err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
	return out
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	gitc(t, dir, "init", "-q", "-b", "main")
	gitc(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-qm", "init")
}

func commitAllIn(t *testing.T, dir, msg string) {
	t.Helper()
	gitc(t, dir, "add", "-A")
	gitc(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", msg)
}

func TestEvalScaffoldAndRun(t *testing.T) {
	up := t.TempDir()
	upstreamSkill(t, up, "1.0.0", "", "")
	consumer := t.TempDir()
	t.Chdir(consumer)
	if _, code := capture(t, func() int { return Run([]string{"add", "demo", "--from", up}) }); code != 0 {
		t.Fatal("add failed")
	}

	// No suite yet: eval fails with a scaffold hint; scaffold creates one.
	if _, code := capture(t, func() int { return Run([]string{"eval", "demo", "--runner", "true"}) }); code == 0 {
		t.Error("eval without a suite should fail")
	}
	if out, code := capture(t, func() int { return Run([]string{"eval", "demo", "--scaffold"}) }); code != 0 {
		t.Fatalf("scaffold failed: %s", out)
	}
	if _, err := os.Stat(filepath.Join(consumer, ".claude", "skills", "demo", "evals", "evals.yaml")); err != nil {
		t.Fatal("scaffold did not write evals.yaml")
	}

	// Passing suite.
	pass := stubRunner(t, 3, 0)
	out, code := capture(t, func() int { return Run([]string{"eval", "demo", "--runner", pass}) })
	if code != 0 || !strings.Contains(out, "PASS — 3 passed, 0 failed") {
		t.Fatalf("pass run: code=%d out=%s", code, out)
	}

	// Failing suite.
	failr := stubRunner(t, 2, 1)
	out, code = capture(t, func() int { return Run([]string{"eval", "demo", "--runner", failr}) })
	if code != 1 || !strings.Contains(out, "FAIL — 2 passed, 1 failed") {
		t.Fatalf("fail run: code=%d out=%s", code, out)
	}
}

func TestHarvestPush(t *testing.T) {
	up := t.TempDir()
	upstreamSkill(t, up, "1.0.0", "", "")
	initGitRepo(t, up)
	commitAllIn(t, up, "skills")

	consumer := t.TempDir()
	t.Chdir(consumer)
	if _, code := capture(t, func() int { return Run([]string{"add", "demo", "--from", up}) }); code != 0 {
		t.Fatal("add failed")
	}

	// Nothing to harvest while aligned.
	out, code := capture(t, func() int { return Run([]string{"harvest", "demo"}) })
	if code != 0 || !strings.Contains(out, "nothing to harvest") {
		t.Fatalf("aligned harvest: code=%d out=%s", code, out)
	}

	// Local learning -> harvest --push lands an attributed branch upstream.
	install := filepath.Join(consumer, ".claude", "skills", "demo", "SKILL.md")
	b, _ := os.ReadFile(install)
	write(t, install, strings.ReplaceAll(string(b), "alpha", "alpha LEARNED"))
	out, code = capture(t, func() int {
		return Run([]string{"harvest", "demo", "--push", "--branch", "acx/harvest-test"})
	})
	if code != 0 || !strings.Contains(out, "pushed harvest branch") {
		t.Fatalf("harvest push: code=%d out=%s", code, out)
	}
	show := gitc(t, up, "show", "acx/harvest-test:skills/demo/SKILL.md")
	if !strings.Contains(show, "alpha LEARNED") {
		t.Fatalf("upstream branch missing the learning:\n%s", show)
	}
	log := gitc(t, up, "log", "-1", "--format=%B", "acx/harvest-test")
	if !strings.Contains(log, "harvest local learnings") || !strings.Contains(log, "ancestor") {
		t.Fatalf("commit message lacks attribution:\n%s", log)
	}
}

func TestPropagatePush(t *testing.T) {
	up := t.TempDir()
	upstreamSkill(t, up, "1.0.0", "", "")

	consumer := t.TempDir()
	t.Chdir(consumer)
	if _, code := capture(t, func() int { return Run([]string{"add", "demo", "--from", up}) }); code != 0 {
		t.Fatal("add failed")
	}
	initGitRepo(t, consumer)
	commitAllIn(t, consumer, "install skill")

	// Upstream ships 1.1.0 and lists the consumer in its fleet.
	upstreamSkill(t, up, "1.1.0", "omega", "omega v2")
	write(t, filepath.Join(up, "fleet.json"), `{"repos": [`+jsonString(consumer)+`]}`)
	initGitRepo(t, up)
	commitAllIn(t, up, "skills v1.1")

	t.Chdir(up)
	out, code := capture(t, func() int {
		return Run([]string{"propagate", "--push", "--branch", "acx/test-update"})
	})
	if code != 0 {
		t.Fatalf("propagate: code=%d out=%s", code, out)
	}
	if !strings.Contains(out, "fast-forwarded 1.0.0 -> 1.1.0") || !strings.Contains(out, "pushed acx/test-update") {
		t.Fatalf("propagate output:\n%s", out)
	}
	show := gitc(t, consumer, "show", "acx/test-update:.claude/skills/demo/SKILL.md")
	if !strings.Contains(show, "1.1.0") || !strings.Contains(show, "omega v2") {
		t.Fatalf("consumer branch not updated:\n%s", show)
	}
	lock := gitc(t, consumer, "show", "acx/test-update:skills.lock")
	if !strings.Contains(lock, "1.1.0") {
		t.Fatalf("lockfile not updated on branch:\n%s", lock)
	}

	// Second run: everything aligned, nothing to do.
	out, code = capture(t, func() int { return Run([]string{"propagate"}) })
	_ = out
	if code != 0 {
		t.Fatalf("idempotent propagate should exit 0: %s", out)
	}
}

func TestPropagateEvalGateBlocks(t *testing.T) {
	up := t.TempDir()
	upstreamSkill(t, up, "1.0.0", "", "")
	// Ship a (failing) eval suite with the skill.
	write(t, filepath.Join(up, "skills", "demo", "evals", "evals.yaml"), "tests: []\n")

	consumer := t.TempDir()
	t.Chdir(consumer)
	if _, code := capture(t, func() int { return Run([]string{"add", "demo", "--from", up}) }); code != 0 {
		t.Fatal("add failed")
	}
	initGitRepo(t, consumer)
	commitAllIn(t, consumer, "install")

	upstreamSkill(t, up, "1.1.0", "omega", "omega v2")
	write(t, filepath.Join(up, "fleet.json"), `{"repos": [`+jsonString(consumer)+`]}`)
	initGitRepo(t, up)
	commitAllIn(t, up, "v1.1")

	t.Chdir(up)
	failr := stubRunner(t, 1, 2)
	out, code := capture(t, func() int {
		return Run([]string{"propagate", "--push", "--runner", failr})
	})
	if code != 1 || !strings.Contains(out, "eval gate failed") {
		t.Fatalf("gate should block: code=%d out=%s", code, out)
	}
	if _, err := git(consumer, "rev-parse", "--verify", "acx/skills-update"); err == nil {
		t.Fatal("branch must not be pushed when the gate fails")
	}
}

func jsonString(s string) string {
	return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"`
}
