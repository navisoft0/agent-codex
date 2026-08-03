package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/navisoft0/agent-codex/internal/drift"
	"github.com/navisoft0/agent-codex/internal/fsutil"
	"github.com/navisoft0/agent-codex/internal/hashdir"
	"github.com/navisoft0/agent-codex/internal/lockfile"
	"github.com/navisoft0/agent-codex/internal/merge"
	"github.com/navisoft0/agent-codex/internal/skillmeta"
	"github.com/navisoft0/agent-codex/internal/upstream"
)

// runUpdate pulls upstream changes into the working copy. Behind fast-forwards
// cleanly; diverged goes through the 3-way merge with the ancestor snapshot as
// base, preserving local edits and writing conflict markers when they overlap.
// The ancestor and lockfile advance to latest either way, so a post-conflict
// resolution reads as local customization of the new base.
func runUpdate(args []string) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}

	root, err := repoRoot()
	if err != nil {
		return fail(err)
	}
	return updateAt(root, pos)
}

// updateAt runs the update engine against an explicit repo root, so
// propagate can drive it inside fleet clones.
func updateAt(root string, pos []string) int {
	lf, err := lockfile.Load(root)
	if err != nil {
		return fail(err)
	}
	if len(lf.Skills) == 0 {
		fmt.Println("no skills under management (empty or missing " + lockfile.Name + ")")
		return 0
	}

	names := pos
	if len(names) == 0 {
		for n := range lf.Skills {
			names = append(names, n)
		}
		sort.Strings(names)
	} else {
		for _, n := range names {
			if _, ok := lf.Skills[n]; !ok {
				return fail(fmt.Errorf("skill %q is not in %s", n, lockfile.Name))
			}
		}
	}

	exit := 0
	for _, name := range names {
		e := lf.Skills[name]
		base, rerr := upstream.Resolve(e.Source, true)
		if rerr != nil && base == "" {
			return fail(rerr)
		}
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "acx: warning: %s: upstream refresh failed, using cached copy\n", name)
		}
		sdir, err := upstream.SkillDir(base, name)
		if err != nil {
			return fail(err)
		}
		latestHash, err := hashdir.Hash(sdir)
		if err != nil {
			return fail(err)
		}

		wdir := filepath.Join(root, filepath.FromSlash(e.Path))
		ancDir := lockfile.AncestorDir(root, name)
		working := ""
		if fsutil.DirExists(wdir) {
			if working, err = hashdir.Hash(wdir); err != nil {
				return fail(err)
			}
		}
		ancestor := e.Hash
		if fsutil.DirExists(ancDir) {
			if h, err := hashdir.Hash(ancDir); err == nil {
				ancestor = h
			}
		}

		meta, _ := skillmeta.Load(sdir)
		oldVersion := e.Version
		sync := func() error {
			if err := fsutil.CopyDir(sdir, ancDir); err != nil {
				return err
			}
			e.Hash = latestHash
			e.Version = meta.Version
			lf.Skills[name] = e
			return nil
		}

		switch drift.Compute(working, ancestor, latestHash) {
		case drift.Aligned:
			fmt.Printf("%s: aligned, nothing to do\n", name)
			continue
		case drift.Drifted:
			fmt.Printf("%s: local edits only, nothing incoming (kept; `acx harvest` lands in M2)\n", name)
			continue
		case drift.Missing:
			if err := fsutil.CopyDir(sdir, wdir); err != nil {
				return fail(err)
			}
			if err := sync(); err != nil {
				return fail(err)
			}
			fmt.Printf("%s: was missing, reinstalled %s\n", name, versionOr(meta.Version, short(latestHash)))
		case drift.Behind:
			if err := fsutil.CopyDir(sdir, wdir); err != nil {
				return fail(err)
			}
			if err := sync(); err != nil {
				return fail(err)
			}
			fmt.Printf("%s: fast-forwarded %s -> %s\n", name,
				versionOr(oldVersion, "last sync"), versionOr(meta.Version, short(latestHash)))
		case drift.Diverged:
			conflicts, err := merge.Dirs(wdir, ancDir, sdir)
			if err != nil {
				return fail(err)
			}
			if err := sync(); err != nil {
				return fail(err)
			}
			if len(conflicts) > 0 {
				exit = 1
				fmt.Printf("%s: merged %s, conflicts in: %s — resolve the <<<<<<< markers, then commit\n",
					name, versionOr(meta.Version, short(latestHash)), strings.Join(conflicts, ", "))
			} else {
				fmt.Printf("%s: merged %s cleanly, local edits preserved\n",
					name, versionOr(meta.Version, short(latestHash)))
			}
		}

		written, err := projectSkill(root, name, lf.Skills[name])
		if err != nil {
			return fail(err)
		}
		for _, p := range written {
			fmt.Printf("%s: projected -> %s\n", name, p)
		}
	}

	if err := lf.Save(root); err != nil {
		return fail(err)
	}
	return exit
}

func versionOr(v, alt string) string {
	if v != "" {
		return v
	}
	return alt
}
