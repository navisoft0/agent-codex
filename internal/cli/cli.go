// Package cli implements the acx command surface.
package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/navisoft0/agent-codex/internal/lockfile"
)

// Version is stamped into release builds via
// -ldflags "-X github.com/navisoft0/agent-codex/internal/cli.Version=vX.Y.Z".
var Version = "v0.1.0"

const usage = `acx — agent-skills sync & governance CLI

Usage:
  acx init                          Scaffold a canonical upstream skills repo
  acx add <skill>[@constraint] --from <source>
                                    Install a skill (git URL or local path upstream);
                                      @1.4.0 exact, @^1.4 same-major, --pin content hash
                 [--surfaces list]    surfaces: claude-code, codex, cursor, agents-md, claude-md
  acx status [--json] [--offline]   Drift report; exit 1 if anything is not aligned
             [--fleet]                aggregate across every repo in fleet.json
  acx diff <skill> [--latest]       Diff working copy vs ancestor (--latest: ancestor vs upstream)
  acx update [<skill>...]           Pull upstream changes: fast-forward or 3-way merge that
                                      preserves local edits; exit 1 on conflicts
             [--ai-merge]             propose conflict resolutions via $ACX_AI_MERGE_CMD (never auto-applied)
  acx verify [--json]               Fail unless working tree and snapshots match skills.lock
  acx project [<skill>...]          (Re)render skills into their configured surfaces
  acx eval <skill> [--scaffold]     Run the skill's eval suite via a promptfoo-compatible runner
  acx harvest <skill> [--push]      Package local drift as an attributed branch against upstream
  acx propagate [--push]            Fleet fan-out: update every fleet.json repo, gate on evals,
                                      prepare (or push) a PR branch per repo
  acx reconcile [--json]            Cluster acx/harvest/* branches; flag overlapping learnings
  acx scan [<skill>...] [--json]    Flag prompt-injection patterns and risky payloads; exit 1 on high
  acx audit [<skill>...] [--json]   Content-level change log: who changed which skill, when

Drift states: aligned · behind · drifted · diverged · missing
`

// Run dispatches a command line and returns the process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	switch args[0] {
	case "init":
		return runInit(args[1:])
	case "add":
		return runAdd(args[1:])
	case "status":
		return runStatus(args[1:])
	case "diff":
		return runDiff(args[1:])
	case "update":
		return runUpdate(args[1:])
	case "verify":
		return runVerify(args[1:])
	case "project":
		return runProject(args[1:])
	case "eval":
		return runEval(args[1:])
	case "harvest":
		return runHarvest(args[1:])
	case "propagate":
		return runPropagate(args[1:])
	case "reconcile":
		return runReconcile(args[1:])
	case "scan":
		return runScan(args[1:])
	case "audit":
		return runAudit(args[1:])
	case "version", "--version", "-v":
		fmt.Println("acx", Version)
		return 0
	case "help", "-h", "--help":
		fmt.Print(usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "acx: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "acx:", err)
	return 1
}

// parse handles a flag set where flags may appear before or after positional
// arguments (Go's flag package stops at the first positional otherwise).
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	var pos []string
	rest := fs.Args()
	for len(rest) > 0 {
		pos = append(pos, rest[0])
		if err := fs.Parse(rest[1:]); err != nil {
			return nil, err
		}
		rest = fs.Args()
	}
	return pos, nil
}

// repoRoot walks up from the working directory to the nearest directory
// holding a skills.lock or a .git, defaulting to the working directory.
func repoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for d := cwd; ; {
		if _, err := os.Stat(filepath.Join(d, lockfile.Name)); err == nil {
			return d, nil
		}
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return cwd, nil
		}
		d = parent
	}
}

func short(hash string) string {
	if len(hash) > 19 {
		return hash[:19]
	}
	return hash
}
