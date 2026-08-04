package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/navisoft0/agent-codex/internal/lockfile"
)

type auditFile struct {
	Path      string `json:"path"`
	Additions string `json:"additions"` // "-" for binary
	Deletions string `json:"deletions"`
}

type auditEntry struct {
	Skill   string      `json:"skill"`
	Commit  string      `json:"commit"`
	Author  string      `json:"author"`
	Email   string      `json:"email"`
	Date    string      `json:"date"`
	Subject string      `json:"subject"`
	Files   []auditFile `json:"files,omitempty"`
}

// runAudit is the content-level change log: who changed which skill's prose,
// when, commit by commit — derived from git history over each skill's path,
// so it works in consuming repos (working copies) and canonical repos
// (skills/) alike.
func runAudit(args []string) int {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	limit := fs.Int("limit", 30, "max commits per skill")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}

	root, err := repoRoot()
	if err != nil {
		return fail(err)
	}
	if _, err := git(root, "rev-parse", "--git-dir"); err != nil {
		return fail(fmt.Errorf("audit needs git history and %s is not a git repository", root))
	}
	targets, err := scanTargets(root, pos)
	if err != nil {
		return fail(err)
	}
	if len(targets) == 0 {
		fmt.Println("nothing to audit (no skills in " + lockfile.Name + " or skills/)")
		return 0
	}

	var entries []auditEntry
	for _, name := range sortedKeys(targets) {
		rel, err := filepath.Rel(root, targets[name])
		if err != nil {
			return fail(err)
		}
		out, err := git(root, "log", "-n", strconv.Itoa(*limit), "--numstat",
			"--format=%x01%H%x00%an%x00%ae%x00%aI%x00%s", "--", rel)
		if err != nil {
			return fail(err)
		}
		entries = append(entries, parseAuditLog(name, out)...)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(entries); err != nil {
			return fail(err)
		}
		return 0
	}
	if len(entries) == 0 {
		fmt.Println("no committed history for any managed skill yet")
		return 0
	}
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "SKILL\tDATE\tAUTHOR\tCOMMIT\tCHANGE\tSUBJECT")
	for _, e := range entries {
		change := ""
		var add, del int
		for _, f := range e.Files {
			if a, err := strconv.Atoi(f.Additions); err == nil {
				add += a
			}
			if d, err := strconv.Atoi(f.Deletions); err == nil {
				del += d
			}
		}
		change = fmt.Sprintf("+%d/-%d in %d file(s)", add, del, len(e.Files))
		date := e.Date
		if len(date) >= 10 {
			date = date[:10]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%.12s\t%s\t%s\n", e.Skill, date, e.Author, e.Commit, change, e.Subject)
	}
	w.Flush()
	return 0
}

// parseAuditLog reads `git log --numstat --format=\x01H\x00an\x00ae\x00aI\x00s`.
func parseAuditLog(skill, out string) []auditEntry {
	var entries []auditEntry
	for _, block := range strings.Split(out, "\x01") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		head := strings.Split(lines[0], "\x00")
		if len(head) < 5 {
			continue
		}
		e := auditEntry{
			Skill: skill, Commit: head[0], Author: head[1], Email: head[2],
			Date: head[3], Subject: head[4],
		}
		for _, ln := range lines[1:] {
			parts := strings.SplitN(strings.TrimSpace(ln), "\t", 3)
			if len(parts) == 3 {
				e.Files = append(e.Files, auditFile{Path: parts[2], Additions: parts[0], Deletions: parts[1]})
			}
		}
		entries = append(entries, e)
	}
	return entries
}
