package cli

import (
	"flag"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/navisoft0/shuhari/internal/adapters"
	"github.com/navisoft0/shuhari/internal/fsutil"
	"github.com/navisoft0/shuhari/internal/lockfile"
	"github.com/navisoft0/shuhari/internal/skillmeta"
)

// projectSkill renders one skill's working copy into every configured
// non-primary surface and returns the repo-relative paths written.
func projectSkill(root, name string, e lockfile.Entry) ([]string, error) {
	surfaces := e.Surfaces
	if len(surfaces) == 0 {
		return nil, nil // primary only
	}
	src := filepath.Join(root, filepath.FromSlash(e.Path))
	if !fsutil.DirExists(src) {
		return nil, fmt.Errorf("cannot project %s: working copy missing at %s", name, e.Path)
	}
	meta, _ := skillmeta.Load(src)

	var written []string
	for _, s := range surfaces {
		if s == adapters.Primary {
			continue
		}
		ad, err := adapters.Get(s)
		if err != nil {
			return written, err
		}
		rel, err := ad.Project(adapters.Context{Root: root, Skill: name, SrcDir: src, Meta: meta})
		if err != nil {
			return written, err
		}
		written = append(written, rel)
	}
	return written, nil
}

// runProject (re)renders skills into their configured surfaces, e.g. after
// hand-resolving a merge or when a projection file was deleted.
func runProject(args []string) int {
	fs := flag.NewFlagSet("project", flag.ContinueOnError)
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}

	root, err := repoRoot()
	if err != nil {
		return fail(err)
	}
	lf, err := lockfile.Load(root)
	if err != nil {
		return fail(err)
	}

	names := pos
	if len(names) == 0 {
		for n := range lf.Skills {
			names = append(names, n)
		}
		sort.Strings(names)
	}

	any := false
	for _, name := range names {
		e, ok := lf.Skills[name]
		if !ok {
			return fail(fmt.Errorf("skill %q is not in %s", name, lockfile.Name))
		}
		written, err := projectSkill(root, name, e)
		if err != nil {
			return fail(err)
		}
		for _, p := range written {
			any = true
			fmt.Printf("%s: projected -> %s\n", name, p)
		}
	}
	if !any {
		fmt.Println("nothing to project (no non-primary surfaces configured; see `shu add --surfaces`)")
	}
	return 0
}
