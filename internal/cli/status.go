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

// statusRowsAt computes the drift state of every skill locked at root. The
// int result is the would-be exit code: 1 when anything is not aligned.
func statusRowsAt(root string, offline bool) ([]statusRow, int, error) {
	lf, err := lockfile.Load(root)
	if err != nil {
		return nil, 1, err
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
				return nil, 1, err
			}
		}

		ancestor := e.Hash
		if adir := lockfile.AncestorDir(root, name); fsutil.DirExists(adir) {
			if h, err := hashdir.Hash(adir); err == nil {
				ancestor = h
			}
		}

		latest := ancestor
		if !offline {
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
		if row.State == string(drift.Behind) && e.Constraint != "" {
			row.Note = "constraint @" + shortConstraint(e.Constraint) + " may hold this back on update"
		}
		if row.State != string(drift.Aligned) {
			exit = 1
		}
		rows = append(rows, row)
	}
	return rows, exit, nil
}

// runStatus prints the drift report for this repo, or with --fleet for every
// repo in fleet.json — the data source for fleet drift dashboards.
func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	offline := fs.Bool("offline", false, "skip upstream refresh; compare against last-synced content only")
	fleet := fs.Bool("fleet", false, "report across every repo in fleet.json (run from the canonical repo)")
	if _, err := parse(fs, args); err != nil {
		return 2
	}

	root, err := repoRoot()
	if err != nil {
		return fail(err)
	}
	if *fleet {
		return fleetStatus(root, *jsonOut, *offline)
	}

	rows, exit, err := statusRowsAt(root, *offline)
	if err != nil {
		return fail(err)
	}
	if len(rows) == 0 {
		if *jsonOut {
			fmt.Println("[]")
		} else {
			fmt.Println("no skills under management (empty or missing " + lockfile.Name + ")")
		}
		return 0
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
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Skill, r.State, versionOr(r.Version, "-"), r.Path, r.Note)
	}
	w.Flush()
	return exit
}

type fleetStatusEntry struct {
	Repo   string      `json:"repo"`
	Error  string      `json:"error,omitempty"`
	Skills []statusRow `json:"skills,omitempty"`
}

// fleetStatus clones each fleet repo shallowly and aggregates its drift rows.
func fleetStatus(root string, jsonOut, offline bool) int {
	m, err := loadFleet(root, "fleet.json")
	if err != nil {
		return fail(err)
	}
	exit := 0
	entries := make([]fleetStatusEntry, 0, len(m.Repos))
	for _, src := range m.Repos {
		entry := fleetStatusEntry{Repo: src}
		clone, err := os.MkdirTemp("", "acx-fleet-")
		if err != nil {
			return fail(err)
		}
		if out, cerr := git(root, "clone", "--depth", "1", src, clone); cerr != nil {
			entry.Error = "clone failed: " + firstLine(out)
			exit = 1
		} else if rows, code, serr := statusRowsAt(clone, offline); serr != nil {
			entry.Error = serr.Error()
			exit = 1
		} else {
			entry.Skills = rows
			if code != 0 {
				exit = 1
			}
		}
		os.RemoveAll(clone)
		entries = append(entries, entry)
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(entries); err != nil {
			return fail(err)
		}
		return exit
	}

	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "REPO\tSKILL\tSTATE\tVERSION\tNOTE")
	for _, e := range entries {
		switch {
		case e.Error != "":
			fmt.Fprintf(w, "%s\t-\terror\t-\t%s\n", e.Repo, e.Error)
		case len(e.Skills) == 0:
			fmt.Fprintf(w, "%s\t-\tempty\t-\tno skills under management\n", e.Repo)
		default:
			for _, r := range e.Skills {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.Repo, r.Skill, r.State, versionOr(r.Version, "-"), r.Note)
			}
		}
	}
	w.Flush()
	return exit
}

func shortConstraint(c string) string {
	if len(c) > 19 {
		return c[:19] + "…"
	}
	return c
}
