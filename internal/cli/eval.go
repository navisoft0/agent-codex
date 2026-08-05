package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/navisoft0/shuhari/internal/evals"
	"github.com/navisoft0/shuhari/internal/fsutil"
	"github.com/navisoft0/shuhari/internal/lockfile"
	"github.com/navisoft0/shuhari/internal/skillmeta"
	"github.com/navisoft0/shuhari/internal/upstream"
)

// skillWorkDir finds a skill's directory in either kind of repo: the working
// copy recorded in skills.lock (consuming repo) or the canonical layout
// (upstream repo).
func skillWorkDir(root, name string) (string, error) {
	lf, err := lockfile.Load(root)
	if err != nil {
		return "", err
	}
	if e, ok := lf.Skills[name]; ok {
		dir := filepath.Join(root, filepath.FromSlash(e.Path))
		if fsutil.DirExists(dir) {
			return dir, nil
		}
		return "", fmt.Errorf("skill %s working copy missing at %s", name, e.Path)
	}
	return upstream.SkillDir(root, name)
}

// runEval executes a skill's regression suite, or scaffolds one with
// --scaffold. Exit 1 means the suite failed — the same signal the propagate
// gate uses.
func runEval(args []string) int {
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	scaffold := fs.Bool("scaffold", false, "generate a starter eval suite instead of running")
	runnerFlag := fs.String("runner", "", "eval runner command (default: $"+evals.EnvRunner+" or npx promptfoo)")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "usage: shu eval <skill> [--scaffold] [--runner <cmd>]")
		return 2
	}
	name := pos[0]

	root, err := repoRoot()
	if err != nil {
		return fail(err)
	}
	dir, err := skillWorkDir(root, name)
	if err != nil {
		return fail(err)
	}

	if *scaffold {
		meta, _ := skillmeta.Load(dir)
		rel, err := evals.Scaffold(dir, name, meta.Description)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("scaffolded %s — replace the placeholder case with real regression prompts\n",
			filepath.ToSlash(filepath.Join(name, rel)))
		return 0
	}

	config := evals.FindConfig(dir)
	if config == "" {
		return fail(fmt.Errorf("%s has no eval suite (generate a starter with `shu eval %s --scaffold`)", name, name))
	}
	runner := evals.DefaultRunner()
	if *runnerFlag != "" {
		runner = strings.Fields(*runnerFlag)
	}

	res, err := evals.Run(runner, dir, config)
	if err != nil {
		return fail(err)
	}
	verdict := "PASS"
	if !res.Passed {
		verdict = "FAIL"
	}
	if res.HasStats {
		fmt.Printf("eval %s: %s — %d passed, %d failed (%s)\n", name, verdict, res.Successes, res.Failures, config)
	} else {
		fmt.Printf("eval %s: %s (%s; runner reported no stats, using exit code)\n", name, verdict, config)
	}
	if !res.Passed {
		return 1
	}
	return 0
}
