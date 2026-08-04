package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/navisoft0/agent-codex/internal/evals"
	"github.com/navisoft0/agent-codex/internal/lockfile"
)

// fleetManifest lists the consuming repos an upstream propagates to.
// It lives as fleet.json at the canonical repo's root.
type fleetManifest struct {
	Repos []string `json:"repos"`
}

func loadFleet(root, manifestPath string) (fleetManifest, error) {
	var m fleetManifest
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(manifestPath)))
	if err != nil {
		return m, fmt.Errorf("no fleet manifest: %v (create %s with {\"repos\": [\"<path-or-git-url>\", ...]})", err, manifestPath)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("parse %s: %v", manifestPath, err)
	}
	return m, nil
}

// runPropagate is the fleet fan-out: clone every repo in fleet.json, run the
// same update engine inside each clone, gate on evals when a runner is
// available, and turn clean results into a branch (pushed with --push) ready
// for a PR. Repos with conflicts or failing evals are reported and skipped —
// nothing merges without review, nothing ships that regresses the suite.
func runPropagate(args []string) int {
	fs := flag.NewFlagSet("propagate", flag.ContinueOnError)
	push := fs.Bool("push", false, "push each prepared branch to its repo's remote")
	branch := fs.String("branch", "acx/skills-update", "branch name for prepared updates")
	manifestPath := fs.String("manifest", "fleet.json", "fleet manifest path relative to repo root")
	runnerFlag := fs.String("runner", "", "eval runner for the gate (default: $"+evals.EnvRunner+"; gate skipped if unset)")
	if _, err := parse(fs, args); err != nil {
		return 2
	}

	root, err := repoRoot()
	if err != nil {
		return fail(err)
	}
	m, err := loadFleet(root, *manifestPath)
	if err != nil {
		return fail(err)
	}
	if len(m.Repos) == 0 {
		fmt.Println("fleet manifest lists no repos")
		return 0
	}

	runner := strings.Fields(*runnerFlag)
	if len(runner) == 0 && strings.TrimSpace(os.Getenv(evals.EnvRunner)) != "" {
		runner = evals.DefaultRunner()
	}

	exit := 0
	for _, src := range m.Repos {
		fmt.Printf("=== %s ===\n", src)
		clone, err := os.MkdirTemp("", "acx-propagate-")
		if err != nil {
			return fail(err)
		}
		if out, cerr := git(root, "clone", src, clone); cerr != nil {
			fmt.Fprintf(os.Stderr, "acx: clone failed, skipping: %v\n%s", cerr, out)
			os.RemoveAll(clone)
			exit = 1
			continue
		}

		code := updateAt(clone, nil, false)
		if out, serr := git(clone, "status", "--porcelain"); serr != nil {
			return fail(serr)
		} else if strings.TrimSpace(out) == "" {
			fmt.Println("up to date, nothing to propagate")
			os.RemoveAll(clone)
			continue
		}
		if code != 0 {
			exit = 1
			fmt.Printf("merge conflicts — needs a manual `acx update` in this repo; working clone kept at %s\n", clone)
			continue
		}

		if ok, err := evalGate(clone, runner); err != nil {
			return fail(err)
		} else if !ok {
			exit = 1
			fmt.Printf("eval gate failed — update NOT propagated; working clone kept at %s\n", clone)
			continue
		}

		if _, err := git(clone, "checkout", "-b", *branch); err != nil {
			return fail(err)
		}
		uname, uemail := gitIdentity(root)
		msg := "Update skills to latest upstream\n\nPrepared by acx propagate; drift report and merge were clean" +
			gateNote(runner) + ".\n"
		if _, err := commitAll(clone, uname, uemail, msg); err != nil {
			return fail(err)
		}

		if *push {
			if _, err := git(clone, "push", "-u", "origin", *branch); err != nil {
				return fail(err)
			}
			fmt.Printf("pushed %s\n", *branch)
			if url := githubCompareURL(src, *branch); url != "" {
				fmt.Printf("open a PR: %s\n", url)
			}
			os.RemoveAll(clone)
		} else {
			fmt.Printf("prepared %s in %s\n", *branch, clone)
			fmt.Printf("review, then push: git -C %s push -u origin %s\n", clone, *branch)
		}
	}
	return exit
}

// evalGate runs the suite of every locked skill that has one. No runner
// configured means the gate is skipped with a notice (most wild skills have
// no suites yet; the gate hardens as suites appear).
func evalGate(root string, runner []string) (bool, error) {
	lf, err := lockfile.Load(root)
	if err != nil {
		return false, err
	}
	names := make([]string, 0, len(lf.Skills))
	for n := range lf.Skills {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		dir := filepath.Join(root, filepath.FromSlash(lf.Skills[name].Path))
		config := evals.FindConfig(dir)
		if config == "" {
			continue
		}
		if len(runner) == 0 {
			fmt.Printf("%s has an eval suite but no runner is configured — gate skipped (set $%s or --runner)\n",
				name, evals.EnvRunner)
			continue
		}
		res, err := evals.Run(runner, dir, config)
		if err != nil {
			return false, err
		}
		if res.HasStats {
			fmt.Printf("eval %s: %d passed, %d failed\n", name, res.Successes, res.Failures)
		}
		if !res.Passed {
			return false, nil
		}
	}
	return true, nil
}

func gateNote(runner []string) string {
	if len(runner) == 0 {
		return " (eval gate skipped: no runner configured)"
	}
	return "; eval gate passed"
}
