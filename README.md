# agent-codex

`acx` is a CLI for syncing and governing AI agent skills across repos and agent
surfaces. It is not a registry and not a marketplace: it treats one canonical
skills repo as *upstream* and manages the loop between it and a fleet of
consuming repos — eval-gated propagation, drift detection, 3-way merge, and a
contribution-back path for edge learnings. The full design is in [PLAN.md](PLAN.md).

**Status: M1 ("Copier for skills").** The lockfile + ancestor-snapshot engine,
drift detection, 3-way-merge updates, tamper verification, and multi-surface
projection all work against a git or local-path upstream.

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
acx add deploy-checklist --from git@github.com:your-org/skills.git \
        --surfaces claude-code,codex,agents-md
# added deploy-checklist@1.4.0 -> .claude/skills/deploy-checklist (sha256:...)
# projected -> .codex/skills/deploy-checklist
# projected -> AGENTS.md

acx status
# SKILL             STATE    VERSION  PATH
# deploy-checklist  aligned  1.4.0    .claude/skills/deploy-checklist

acx diff deploy-checklist            # local edits vs recorded ancestor
acx diff deploy-checklist --latest   # incoming upstream changes vs ancestor

acx update
# deploy-checklist: fast-forwarded 1.4.0 -> 1.5.0        (no local edits)
# deploy-checklist: merged 1.5.0 cleanly, local edits preserved
# deploy-checklist: merged 1.5.0, conflicts in: SKILL.md — resolve the <<<<<<< markers…

acx verify                           # CI gate: working tree + snapshots match skills.lock
acx project                          # re-render surfaces after hand edits or resolution
```

Commit `skills.lock` and `.agent-codex/ancestors/` — they are the state that
makes drift computable on any checkout, offline, and the ancestor snapshots
are the merge base `acx update` uses to preserve local customization.

## Updates never clobber

`update` decides per skill from its drift state: `behind` fast-forwards,
`drifted` is left alone (those are your local learnings — harvesting them
upstream lands in M2), and `diverged` gets a file-level 3-way merge (ancestor
as base) that combines non-overlapping changes and writes git-style conflict
markers when they overlap, exiting 1 so nothing ships unresolved. Add/delete
conflicts always keep the side with local edits. After an update the ancestor
advances to the new upstream content, so whatever you keep locally reads as
customization of the new base.

## Surfaces (adapter pattern)

One canonical working copy per skill (the primary, `.claude/skills/`),
projected into each configured surface. Projections are generated artifacts —
re-rendered by `add`, `update`, and `project`, never edited in place:

| Surface | Renders to |
|---|---|
| `claude-code` | `.claude/skills/<name>/` (the working copy itself) |
| `codex` | `.codex/skills/<name>/` (full mirror) |
| `cursor` | `.cursor/rules/<name>.mdc` (body as a project rule) |
| `agents-md` | managed `<!-- acx:skill:… -->` section in `AGENTS.md` |
| `claude-md` | managed section in `CLAUDE.md` |

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
internal/cli/       command implementations (init, add, status, diff, update, verify, project)
internal/lockfile/  skills.lock + ancestor snapshot locations
internal/drift/     the five-state drift resolver
internal/merge/     file-level 3-way merge (ancestor as base, conflict markers on overlap)
internal/adapters/  surface adapters: codex, cursor, agents-md, claude-md
internal/hashdir/   deterministic directory content hashing
internal/upstream/  source resolution (local path or cached git clone)
internal/skillmeta/ SKILL.md frontmatter parsing
```

## Roadmap

M0+M1 (this) → M2 eval gate + fleet propagation (`eval`, `propagate`,
`harvest`) → M3 contribution-back reconciliation → M4 governance. See
[PLAN.md](PLAN.md).
