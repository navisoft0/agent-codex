package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

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
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 || *from == "" {
		fmt.Fprintln(os.Stderr, "usage: acx add <skill> --from <source> [--to <dir>]")
		return 2
	}
	name := pos[0]

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
	lf.Skills[name] = lockfile.Entry{
		Source:  *from,
		Version: meta.Version,
		Hash:    hash,
		Path:    filepath.ToSlash(rel),
	}
	if err := lf.Save(root); err != nil {
		return fail(err)
	}

	if meta.Version != "" {
		fmt.Printf("added %s@%s -> %s (%s)\n", name, meta.Version, rel, short(hash))
	} else {
		fmt.Printf("added %s -> %s (%s)\n", name, rel, short(hash))
	}
	fmt.Println("commit skills.lock and " + lockfile.AncestorsDir + "/ so drift stays computable on every checkout")
	return 0
}
