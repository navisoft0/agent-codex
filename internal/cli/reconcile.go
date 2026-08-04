package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

// reconcile clusters incoming harvest branches so a maintainer merges the
// union once instead of resolving the same learning serially. Two harvests of
// the same skill can be independent (different files, or distant regions of
// the same file) or overlapping (edits to the same lines relative to the
// shared base) — the report says which, per pair.

type harvestBranch struct {
	Branch string `json:"branch"`
	// files -> changed line ranges in the BASE version of each file, so
	// overlap between branches is computed against common ground.
	files map[string][]lineRange
}

type lineRange struct{ start, end int }

type overlapReport struct {
	A     string `json:"a"`
	B     string `json:"b"`
	File  string `json:"file"`
	Level string `json:"level"` // file | block
}

type reconcileCluster struct {
	Skill      string          `json:"skill"`
	Branches   []string        `json:"branches"`
	Overlaps   []overlapReport `json:"overlaps,omitempty"`
	Suggestion string          `json:"suggestion"`
}

// runReconcile inspects acx/harvest/* branches in the canonical repo.
func runReconcile(args []string) int {
	fs := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	baseFlag := fs.String("base", "", "base ref to diff against (default: origin/HEAD, then main)")
	if _, err := parse(fs, args); err != nil {
		return 2
	}

	root, err := repoRoot()
	if err != nil {
		return fail(err)
	}
	if _, err := git(root, "rev-parse", "--git-dir"); err != nil {
		return fail(fmt.Errorf("reconcile runs in the canonical upstream repo (a git repository)"))
	}
	// Best effort: see the freshest harvests; offline still works on what's local.
	_, _ = git(root, "fetch", "--prune", "origin")

	base := *baseFlag
	if base == "" {
		base = detectBase(root)
	}

	branches, err := harvestBranches(root)
	if err != nil {
		return fail(err)
	}
	if len(branches) == 0 {
		fmt.Println("no acx/harvest/* branches found — nothing to reconcile")
		return 0
	}

	// skill -> branches touching it, with their base-relative hunks.
	clusters := map[string][]harvestBranch{}
	for _, br := range branches {
		perSkill, err := branchSkillHunks(root, base, br)
		if err != nil {
			return fail(err)
		}
		for skill, hb := range perSkill {
			clusters[skill] = append(clusters[skill], hb)
		}
	}

	var out []reconcileCluster
	skills := make([]string, 0, len(clusters))
	for s := range clusters {
		skills = append(skills, s)
	}
	sort.Strings(skills)
	for _, skill := range skills {
		list := clusters[skill]
		c := reconcileCluster{Skill: skill}
		for _, hb := range list {
			c.Branches = append(c.Branches, hb.Branch)
		}
		sort.Strings(c.Branches)
		for i := 0; i < len(list); i++ {
			for j := i + 1; j < len(list); j++ {
				c.Overlaps = append(c.Overlaps, pairOverlaps(list[i], list[j])...)
			}
		}
		switch {
		case len(c.Branches) == 1:
			c.Suggestion = "single harvest — review and merge"
		case len(c.Overlaps) == 0:
			c.Suggestion = "independent learnings — merge in any order"
		default:
			c.Suggestion = "overlapping learnings — review these branches together and merge the union once"
		}
		out = append(out, c)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return fail(err)
		}
		return 0
	}
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "SKILL\tBRANCHES\tOVERLAP\tSUGGESTION")
	for _, c := range out {
		overlap := "none"
		if len(c.Overlaps) > 0 {
			var parts []string
			for _, o := range c.Overlaps {
				parts = append(parts, fmt.Sprintf("%s (%s)", o.File, o.Level))
			}
			overlap = strings.Join(dedupe(parts), ", ")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.Skill, strings.Join(c.Branches, ", "), overlap, c.Suggestion)
	}
	w.Flush()
	return 0
}

func detectBase(root string) string {
	if out, err := git(root, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if b := strings.TrimSpace(out); b != "" {
			return b
		}
	}
	for _, c := range []string{"origin/main", "main", "origin/master", "master"} {
		if _, err := git(root, "rev-parse", "--verify", c); err == nil {
			return c
		}
	}
	return "HEAD"
}

// harvestBranches lists local and remote acx/harvest/* refs, deduplicated by
// their harvest-relative name (remote copy preferred).
func harvestBranches(root string) ([]string, error) {
	out, err := git(root, "for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	seen := map[string]string{}
	for _, ref := range strings.Split(out, "\n") {
		ref = strings.TrimSpace(ref)
		i := strings.Index(ref, "acx/harvest/")
		if i < 0 || strings.HasSuffix(ref, "/HEAD") {
			continue
		}
		key := ref[i:]
		if prev, ok := seen[key]; !ok || strings.Contains(ref, "/") && !strings.Contains(prev, "/") {
			seen[key] = ref
		}
	}
	var refs []string
	for _, r := range seen {
		refs = append(refs, r)
	}
	sort.Strings(refs)
	return refs, nil
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+\d+(?:,\d+)? @@`)

// branchSkillHunks maps skill -> base-relative changed ranges for one branch.
func branchSkillHunks(root, base, branch string) (map[string]harvestBranch, error) {
	out, err := git(root, "diff", "--name-only", base+"..."+branch, "--", "skills")
	if err != nil {
		// A branch that doesn't share history with base: report as opaque.
		return nil, fmt.Errorf("diff %s...%s: %v", base, branch, err)
	}
	perSkill := map[string]harvestBranch{}
	for _, file := range strings.Split(strings.TrimSpace(out), "\n") {
		file = strings.TrimSpace(file)
		parts := strings.Split(file, "/")
		if len(parts) < 2 || parts[0] != "skills" {
			continue
		}
		skill := parts[1]
		hb, ok := perSkill[skill]
		if !ok {
			hb = harvestBranch{Branch: branch, files: map[string][]lineRange{}}
		}
		diff, err := git(root, "diff", "-U0", base+"..."+branch, "--", file)
		if err != nil {
			return nil, err
		}
		for _, ln := range strings.Split(diff, "\n") {
			if m := hunkHeader.FindStringSubmatch(ln); m != nil {
				start, _ := strconv.Atoi(m[1])
				count := 1
				if m[2] != "" {
					count, _ = strconv.Atoi(m[2])
				}
				end := start + count
				if count == 0 {
					end = start // pure insertion point
				}
				hb.files[file] = append(hb.files[file], lineRange{start, end})
			}
		}
		if len(hb.files[file]) == 0 {
			hb.files[file] = append(hb.files[file], lineRange{0, 0}) // whole-file change (add/delete/binary)
		}
		perSkill[skill] = hb
	}
	return perSkill, nil
}

// pairOverlaps compares two branches' base-relative ranges. Ranges within 2
// lines of each other count as block overlap — adjacent edits conflict in
// practice even when hunks don't strictly intersect.
func pairOverlaps(a, b harvestBranch) []overlapReport {
	var out []overlapReport
	for file, ar := range a.files {
		br, ok := b.files[file]
		if !ok {
			continue
		}
		level := "file"
		for _, ra := range ar {
			for _, rb := range br {
				if ra.start <= rb.end+2 && rb.start <= ra.end+2 {
					level = "block"
				}
			}
		}
		out = append(out, overlapReport{A: a.Branch, B: b.Branch, File: file, Level: level})
	}
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
