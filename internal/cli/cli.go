// Package cli implements the acx command surface (M0: init, add, status, diff).
package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/navisoft0/agent-codex/internal/lockfile"
)

const usage = `acx — agent-skills sync & governance CLI (M0 spike)

Usage:
  acx init                          Scaffold a canonical upstream skills repo
  acx add <skill> --from <source>   Install a skill from an upstream (git URL or local path)
  acx status [--json] [--offline]   Drift report; exit 1 if anything is not aligned
  acx diff <skill> [--latest]       Diff working copy vs ancestor (--latest: ancestor vs upstream)

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
