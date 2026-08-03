package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/navisoft0/agent-codex/internal/fsutil"
	"github.com/navisoft0/agent-codex/internal/hashdir"
	"github.com/navisoft0/agent-codex/internal/lockfile"
	"github.com/navisoft0/agent-codex/internal/upstream"
)

// runHarvest packages local drift as a reviewable branch against the
// canonical upstream: clone, copy the working copy over the skill, commit
// with attribution, and (with --push) push so a PR can be opened. This is the
// contribution-back loop's capture step — learnings flow upstream through the
// same review discipline as code.
func runHarvest(args []string) int {
	fs := flag.NewFlagSet("harvest", flag.ContinueOnError)
	push := fs.Bool("push", false, "push the harvest branch to the upstream remote")
	branchFlag := fs.String("branch", "", "branch name (default acx/harvest/<skill>)")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprintln(os.Stderr, "usage: acx harvest <skill> [--push] [--branch <name>]")
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

	wdir := filepath.Join(root, filepath.FromSlash(entry.Path))
	if !fsutil.DirExists(wdir) {
		return fail(fmt.Errorf("working copy missing at %s", entry.Path))
	}
	working, err := hashdir.Hash(wdir)
	if err != nil {
		return fail(err)
	}
	ancestor := entry.Hash
	if adir := lockfile.AncestorDir(root, name); fsutil.DirExists(adir) {
		if h, err := hashdir.Hash(adir); err == nil {
			ancestor = h
		}
	}
	if working == ancestor {
		fmt.Printf("%s: working copy matches the synced ancestor — nothing to harvest\n", name)
		return 0
	}

	clone, err := os.MkdirTemp("", "acx-harvest-"+name+"-")
	if err != nil {
		return fail(err)
	}
	if out, err := git(root, "clone", entry.Source, clone); err != nil {
		os.RemoveAll(clone)
		return fail(fmt.Errorf("upstream %s is not cloneable: %v\n%s", entry.Source, err, out))
	}

	branch := *branchFlag
	if branch == "" {
		branch = "acx/harvest/" + name
	}
	if _, err := git(clone, "checkout", "-b", branch); err != nil {
		return fail(err)
	}

	dst, err := upstream.SkillDir(clone, name)
	if err != nil {
		dst = filepath.Join(clone, "skills", name) // new skill upstream
	}
	if err := fsutil.CopyDir(wdir, dst); err != nil {
		return fail(err)
	}

	consumer := filepath.Base(root)
	origin := ""
	if out, gerr := git(root, "config", "--get", "remote.origin.url"); gerr == nil {
		origin = out
	}
	msg := fmt.Sprintf("skills/%s: harvest local learnings from %s\n\n"+
		"Drift captured by acx relative to synced ancestor %s (version %s).\n"+
		"Source repo: %s\n",
		name, consumer, short(ancestor), versionOr(entry.Version, "unversioned"),
		versionOr(firstLine(origin), root))
	uname, uemail := gitIdentity(root)
	committed, err := commitAll(clone, uname, uemail, msg)
	if err != nil {
		return fail(err)
	}
	if !committed {
		os.RemoveAll(clone)
		fmt.Printf("%s: upstream already contains these changes — nothing to harvest\n", name)
		return 0
	}

	if *push {
		if _, err := git(clone, "push", "-u", "origin", branch); err != nil {
			return fail(err)
		}
		fmt.Printf("%s: pushed harvest branch %s to %s\n", name, branch, entry.Source)
		if url := githubCompareURL(entry.Source, branch); url != "" {
			fmt.Printf("open a PR: %s\n", url)
		}
		os.RemoveAll(clone)
		return 0
	}
	fmt.Printf("%s: prepared harvest branch %s in %s\n", name, branch, clone)
	fmt.Printf("review, then push: git -C %s push -u origin %s\n", clone, branch)
	return 0
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' || r == '\r' {
			return s[:i]
		}
	}
	return s
}
