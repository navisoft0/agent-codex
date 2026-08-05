package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/navisoft0/shuhari/internal/lockfile"
	"github.com/navisoft0/shuhari/internal/scan"
)

// EnvScanCmd names an external scanner run in addition to the built-in
// heuristics (e.g. a Snyk or custom-rules invocation). It receives the skill
// directory as its argument; a non-zero exit is reported as a high finding.
const EnvScanCmd = "SHU_SCAN_CMD"

type scanRow struct {
	Skill    string         `json:"skill"`
	Findings []scan.Finding `json:"findings"`
}

// runScan checks skill content for prompt-injection patterns and risky
// payloads. Exit 1 when any high-severity finding exists, so CI can gate
// add/update PRs on it.
func runScan(args []string) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	pos, err := parse(fs, args)
	if err != nil {
		return 2
	}

	root, err := repoRoot()
	if err != nil {
		return fail(err)
	}
	targets, err := scanTargets(root, pos)
	if err != nil {
		return fail(err)
	}
	if len(targets) == 0 {
		fmt.Println("nothing to scan (no skills in " + lockfile.Name + " or skills/)")
		return 0
	}

	external := strings.Fields(os.Getenv(EnvScanCmd))
	exit := 0
	var rows []scanRow
	for _, name := range sortedKeys(targets) {
		dir := targets[name]
		findings, err := scan.Dir(dir)
		if err != nil {
			return fail(err)
		}
		if len(external) > 0 {
			cmd := exec.Command(external[0], append(external[1:], dir)...)
			if out, err := cmd.CombinedOutput(); err != nil {
				findings = append(findings, scan.Finding{
					Rule: "external-scanner", Severity: scan.High,
					Snippet: strings.TrimSpace(tail(string(out), 200)),
				})
			}
		}
		if scan.HighCount(findings) > 0 {
			exit = 1
		}
		rows = append(rows, scanRow{Skill: name, Findings: findings})
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rows); err != nil {
			return fail(err)
		}
		return exit
	}

	clean := true
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	for _, r := range rows {
		for _, f := range r.Findings {
			clean = false
			loc := f.File
			if f.Line > 0 {
				loc = fmt.Sprintf("%s:%d", f.File, f.Line)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.Skill, f.Severity, f.Rule, loc, f.Snippet)
		}
	}
	w.Flush()
	if clean {
		fmt.Printf("scanned %d skill(s): no findings\n", len(rows))
	}
	return exit
}

// scanTargets maps skill name -> directory, from the lockfile in a consuming
// repo or the canonical layout upstream.
func scanTargets(root string, names []string) (map[string]string, error) {
	targets := map[string]string{}
	lf, err := lockfile.Load(root)
	if err != nil {
		return nil, err
	}
	if len(names) > 0 {
		for _, n := range names {
			dir, err := skillWorkDir(root, n)
			if err != nil {
				return nil, err
			}
			targets[n] = dir
		}
		return targets, nil
	}
	for n, e := range lf.Skills {
		targets[n] = filepath.Join(root, filepath.FromSlash(e.Path))
	}
	if len(targets) == 0 {
		entries, _ := os.ReadDir(filepath.Join(root, "skills"))
		for _, ent := range entries {
			if ent.IsDir() {
				if dir := filepath.Join(root, "skills", ent.Name()); fileExists(filepath.Join(dir, "SKILL.md")) {
					targets[ent.Name()] = dir
				}
			}
		}
	}
	return targets, nil
}

// scanNotice is the hook add/update run after installing content: findings
// never block the install, they get surfaced for review.
func scanNotice(name, dir string) {
	findings, err := scan.Dir(dir)
	if err != nil || len(findings) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "shu: scan: %s has %d finding(s), %d high — run `shu scan %s` for details\n",
		name, len(findings), scan.HighCount(findings), name)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
