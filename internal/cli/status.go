package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/navisoft0/agent-codex/internal/drift"
	"github.com/navisoft0/agent-codex/internal/fsutil"
	"github.com/navisoft0/agent-codex/internal/hashdir"
	"github.com/navisoft0/agent-codex/internal/lockfile"
	"github.com/navisoft0/agent-codex/internal/upstream"
)

type statusRow struct {
	Skill   string `json:"skill"`
	State   string `json:"state"`
	Version string `json:"version,omitempty"`
	Path    string `json:"path"`
	Source  string `json:"source"`
	Note    string `json:"note,omitempty"`
}

// runStatus computes the drift state of every skill in the lockfile by
// comparing working copy, ancestor snapshot, and upstream latest. Exits 1 if
// anything is not aligned so CI can gate on it.
func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	offline := fs.Bool("offline", false, "skip upstream refresh; compare against last-synced content only")
	if _, err := parse(fs, args); err != nil {
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
	if len(lf.Skills) == 0 {
		if *jsonOut {
			fmt.Println("[]")
		} else {
			fmt.Println("no skills under management (empty or missing " + lockfile.Name + ")")
		}
		return 0
	}

	names := make([]string, 0, len(lf.Skills))
	for n := range lf.Skills {
		names = append(names, n)
	}
	sort.Strings(names)

	exit := 0
	rows := make([]statusRow, 0, len(names))
	for _, name := range names {
		e := lf.Skills[name]
		row := statusRow{Skill: name, Version: e.Version, Path: e.Path, Source: e.Source}

		working := ""
		if wdir := filepath.Join(root, filepath.FromSlash(e.Path)); fsutil.DirExists(wdir) {
			if working, err = hashdir.Hash(wdir); err != nil {
				return fail(err)
			}
		}

		ancestor := e.Hash
		if adir := lockfile.AncestorDir(root, name); fsutil.DirExists(adir) {
			if h, err := hashdir.Hash(adir); err == nil {
				ancestor = h
			}
		}

		latest := ancestor
		if !*offline {
			base, err := upstream.Resolve(e.Source, true)
			switch {
			case base == "":
				row.Note = "upstream unreachable; comparing against last sync"
			case err != nil:
				row.Note = "upstream refresh failed; using cached copy"
				fallthrough
			default:
				if sdir, serr := upstream.SkillDir(base, name); serr == nil {
					if h, herr := hashdir.Hash(sdir); herr == nil {
						latest = h
					}
				} else {
					row.Note = "skill no longer present upstream"
				}
			}
		}

		row.State = string(drift.Compute(working, ancestor, latest))
		if row.State != string(drift.Aligned) {
			exit = 1
		}
		rows = append(rows, row)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rows); err != nil {
			return fail(err)
		}
		return exit
	}

	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "SKILL\tSTATE\tVERSION\tPATH\tNOTE")
	for _, r := range rows {
		v := r.Version
		if v == "" {
			v = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Skill, r.State, v, r.Path, r.Note)
	}
	w.Flush()
	return exit
}
