package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/navisoft0/agent-codex/internal/adapters"
	"github.com/navisoft0/agent-codex/internal/fsutil"
	"github.com/navisoft0/agent-codex/internal/hashdir"
	"github.com/navisoft0/agent-codex/internal/lockfile"
	"github.com/navisoft0/agent-codex/internal/skillmeta"
	"github.com/navisoft0/agent-codex/internal/upstream"
)

// runAdd installs a skill from an upstream into this repo: copies it to the
// Claude Code surface (.claude/skills/<name> by default), records a full
// ancestor snapshot, and writes the lockfile entry.
func runAdd(args []string) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	from := fs.String("from", "", "upstream source: git URL or local path (required)")
	to := fs.String("to", "", "install dir relative to repo root (default .claude/skills/<skill>)")
	surfacesFlag := fs.String("surfaces", adapters.Primary,
		"comma-separated surfaces to project into: "+strings.Join(adapters.Names(), ", "))
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 || *from == "" {
		fmt.Fprintln(os.Stderr, "usage: acx add <skill> --from <source> [--to <dir>] [--surfaces <list>]")
		return 2
	}
	name := pos[0]

	var surfaces []string
	for _, s := range strings.Split(*surfacesFlag, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !adapters.Known(s) {
			return fail(fmt.Errorf("unknown surface %q (known: %s)", s, strings.Join(adapters.Names(), ", ")))
		}
		surfaces = append(surfaces, s)
	}

	root, err := repoRoot()
	if err != nil {
		return fail(err)
	}
	base, err := upstream.Resolve(*from, true)
	if err != nil && base == "" {
		return fail(err)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "acx: warning: could not refresh upstream, installing from cached copy")
	}
	src, err := upstream.SkillDir(base, name)
	if err != nil {
		return fail(err)
	}

	rel := *to
	if rel == "" {
		rel = filepath.Join(".claude", "skills", name)
	}
	if err := fsutil.CopyDir(src, filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		return fail(err)
	}
	if err := fsutil.CopyDir(src, lockfile.AncestorDir(root, name)); err != nil {
		return fail(err)
	}

	hash, err := hashdir.Hash(src)
	if err != nil {
		return fail(err)
	}
	meta, _ := skillmeta.Load(src) // a missing version is allowed; the hash is authoritative

	lf, err := lockfile.Load(root)
	if err != nil {
		return fail(err)
	}
	entry := lockfile.Entry{
		Source:   *from,
		Version:  meta.Version,
		Hash:     hash,
		Path:     filepath.ToSlash(rel),
		Surfaces: surfaces,
	}
	lf.Skills[name] = entry
	if err := lf.Save(root); err != nil {
		return fail(err)
	}

	if meta.Version != "" {
		fmt.Printf("added %s@%s -> %s (%s)\n", name, meta.Version, rel, short(hash))
	} else {
		fmt.Printf("added %s -> %s (%s)\n", name, rel, short(hash))
	}
	written, err := projectSkill(root, name, entry)
	if err != nil {
		return fail(err)
	}
	for _, p := range written {
		fmt.Printf("projected -> %s\n", p)
	}
	fmt.Println("commit skills.lock and " + lockfile.AncestorsDir + "/ so drift stays computable on every checkout")
	return 0
}
