# agent-codex

`acx` is a CLI for syncing and governing AI agent skills across repos and agent
surfaces. It is not a registry and not a marketplace: it treats one canonical
skills repo as *upstream* and manages the loop between it and a fleet of
consuming repos — eval-gated propagation, drift detection, 3-way merge, and a
contribution-back path for edge learnings. The full design is in [PLAN.md](PLAN.md).

**Status: M0 spike.** The lockfile + ancestor-snapshot engine and the first
four commands work against a git or local-path upstream, projecting into the
Claude Code surface (`.claude/skills/`).

## Install

```sh
go build -o acx ./cmd/acx    # Go 1.24+
```

## Quickstart

In the canonical skills repo (one folder per skill, spec-compliant `SKILL.md`):

```sh
acx init          # scaffolds skills/example-skill/{SKILL.md,evals/}
```

In a consuming repo:

```sh
acx add deploy-checklist --from git@github.com:your-org/skills.git
# added deploy-checklist@1.4.0 -> .claude/skills/deploy-checklist (sha256:...)

acx status
# SKILL             STATE    VERSION  PATH
# deploy-checklist  aligned  1.4.0    .claude/skills/deploy-checklist

acx diff deploy-checklist            # local edits vs recorded ancestor
acx diff deploy-checklist --latest   # incoming upstream changes vs ancestor
```

Commit `skills.lock` and `.agent-codex/ancestors/` — they are the state that
makes drift computable on any checkout, offline, and they are the merge base
for the coming `acx update`.

## Drift states

`acx status` compares three content hashes per skill — working copy, recorded
ancestor (what you last synced), and upstream latest:

| State | Meaning |
|---|---|
| `aligned` | working == ancestor == latest — nothing to do |
| `behind` | upstream moved, local untouched — clean fast-forward |
| `drifted` | local edits, upstream unmoved — learnings to harvest |
| `diverged` | both moved — 3-way merge required |
| `missing` | in the lockfile but not on disk |

`status` exits non-zero when anything is not aligned, and `--json` emits
machine-readable rows, so the same binary gates CI. `--offline` skips the
upstream refresh and compares against last-synced content only.

## Layout

```
cmd/acx/            entry point
internal/cli/       command implementations (init, add, status, diff)
internal/lockfile/  skills.lock + ancestor snapshot locations
internal/drift/     the five-state drift resolver
internal/hashdir/   deterministic directory content hashing
internal/upstream/  source resolution (local path or cached git clone)
internal/skillmeta/ SKILL.md frontmatter parsing
```

## Roadmap

M0 (this) → M1 `update`/3-way merge + more surface adapters → M2 eval gate +
fleet propagation → M3 contribution-back → M4 governance. See [PLAN.md](PLAN.md).
