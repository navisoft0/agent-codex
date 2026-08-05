package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/navisoft0/shuhari/internal/lockfile"
	"github.com/navisoft0/shuhari/internal/upstream"
)

// runDiff shows what changed: by default the local working copy against the
// recorded ancestor (local edits), with --latest the ancestor against the
// upstream's current content (incoming changes).
func runDiff(args []string) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	latest := fs.Bool("latest", false, "diff ancestor vs upstream latest instead of working copy vs ancestor")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "usage: shu diff <skill> [--latest]")
		return 2
	}
	name := pos[0]

	root, err := repoRoot()
	if err != nil {
		return fail(err)
	}
	lf, err := lockfile.Load(root)
	if err != nil {
		return fail(err)
	}
	entry, ok := lf.Skills[name]
	if !ok {
		return fail(fmt.Errorf("skill %q is not in %s", name, lockfile.Name))
	}

	ancestor := lockfile.AncestorDir(root, name)
	var a, b, aLabel, bLabel string
	if *latest {
		base, err := upstream.Resolve(entry.Source, true)
		if err != nil && base == "" {
			return fail(err)
		}
		sdir, err := upstream.SkillDir(base, name)
		if err != nil {
			return fail(err)
		}
		a, b, aLabel, bLabel = ancestor, sdir, "ancestor", "latest"
	} else {
		a, b, aLabel, bLabel = ancestor, filepath.Join(root, filepath.FromSlash(entry.Path)), "ancestor", "local"
	}

	// Run from the repo root with root-relative paths where possible so the
	// diff headers stay readable.
	for _, p := range []*string{&a, &b} {
		if rel, err := filepath.Rel(root, *p); err == nil && !strings.HasPrefix(rel, "..") {
			*p = rel
		}
	}
	cmd := exec.Command("git", "diff", "--no-index",
		"--src-prefix="+aLabel+"/", "--dst-prefix="+bLabel+"/", "--", a, b)
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	err = cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return 0 // differences shown
	}
	if err != nil {
		return fail(err)
	}
	fmt.Printf("no differences between %s and %s for %s\n", aLabel, bLabel, name)
	return 0
}
