package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/navisoft0/agent-codex/internal/fsutil"
	"github.com/navisoft0/agent-codex/internal/hashdir"
	"github.com/navisoft0/agent-codex/internal/lockfile"
)

type verifyRow struct {
	Skill    string `json:"skill"`
	OK       bool   `json:"ok"`
	Working  string `json:"working"`  // match | modified | missing
	Ancestor string `json:"ancestor"` // match | modified | missing
}

// runVerify is the tamper check: the working tree and the ancestor snapshots
// must both hash to exactly what skills.lock records. Run it in CI after
// checkout — a mismatch means someone changed skill content without going
// through add/update, or edited a snapshot by hand.
func runVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
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
	rows := make([]verifyRow, 0, len(names))
	for _, name := range names {
		e := lf.Skills[name]
		check := func(dir string) string {
			if !fsutil.DirExists(dir) {
				return "missing"
			}
			h, err := hashdir.Hash(dir)
			if err != nil || h != e.Hash {
				return "modified"
			}
			return "match"
		}
		row := verifyRow{
			Skill:    name,
			Working:  check(filepath.Join(root, filepath.FromSlash(e.Path))),
			Ancestor: check(lockfile.AncestorDir(root, name)),
		}
		row.OK = row.Working == "match" && row.Ancestor == "match"
		if !row.OK {
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
	fmt.Fprintln(w, "SKILL\tOK\tWORKING\tANCESTOR")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%t\t%s\t%s\n", r.Skill, r.OK, r.Working, r.Ancestor)
	}
	w.Flush()
	return exit
}
