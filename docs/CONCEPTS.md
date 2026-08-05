# acx, explained in plain English

No jargon assumed. Ten minutes, and the rest of the docs will make sense.

## The problem

A **skill** is an instruction file that teaches an AI agent how to do
something your way — "here's how we deploy," "here's how we use monday.com."
Skills are valuable, so they spread: copied into other projects, shared with
teammates.

The moment a skill is copied, the copy is on its own. Improve the original —
the copies don't hear about it. A teammate fixes something in their copy —
the fix helps only them. Multiply by ten projects and a dozen skills and you
get the familiar mess: nobody knows which version is current, and hard-won
lessons are scattered across folders where they help exactly one person.

## The idea

Pick one git repo to be **the library** — the official home of your skills.
(An ordinary repo; a folder per skill. Nothing special is installed in it.)

Then `acx`, a small command-line tool on each person's machine, manages the
relationship between that library and every copy taken from it. The key
trick: **when acx installs a skill into a project, it also records a receipt
— exactly what was received, from where, at which version.** Everything else
follows from that receipt, because acx can always compare three things:

1. what the library has **now**
2. what this project **originally received** (the receipt)
3. what's in the project **today**

## The five states

Comparing those three, every installed skill is in exactly one state:

| State | In plain words |
|---|---|
| `aligned` | Your copy matches the library. Nothing to do. |
| `behind` | The library improved; you haven't. An update will apply cleanly. |
| `drifted` | *You* changed your copy; the library hasn't moved. Your edits are potential contributions. |
| `diverged` | Both changed. An update will need to merge the two. |
| `missing` | The receipt says you have this skill, but the folder is gone. |

`acx status` prints this for every skill in a project. `drifted` is not an
error — customizing your copy is normal and supported forever.

## The loop

**Pull** — `acx add monday --from github.com/you/skills` installs a skill and
writes the receipt. Your existing GitHub access controls who can do this.

**Update** — `acx update` brings in the library's latest. Because of the
receipt, acx knows which lines are *your* edits and which are *the update's*:
non-overlapping changes are combined automatically; if both sides touched the
same line, acx stops, marks that one spot with both versions, and lets a
human choose. It never guesses, and it never throws your edits away.

**Contribute** — `acx harvest monday` packages your local edits as a proposed
change to the library — an ordinary pull request, labeled with who made it
and what it's based on. The maintainer reviews it like any other change;
nothing enters the library without review. If several people propose
overlapping fixes, `acx reconcile` groups them so the maintainer merges the
lesson once.

**Fan out** — from the library, `acx propagate` pushes a new release to every
listed team repo as a reviewable update, running each skill's test cases
first (skills can carry example tasks that prove they work). A failing update
simply isn't proposed.

## Safety nets

- `acx verify` — proves a project's copies match the receipts exactly (catches
  tampering; run it in CI).
- `acx scan` — checks incoming skill content for red flags: hidden
  instructions like "don't tell the user," scripts that download and run
  code. Runs automatically on every install/update.
- `acx audit` — the history: who changed which skill, when.
- Pinning — `acx add monday@1.2.0` (or `--pin`) freezes a copy; updates
  report "held back" instead of moving it until you say so.
- Rolling back — everything acx does is plain files committed to *your* repo,
  so undoing an update is a normal git revert, and automated updates arrive
  as pull requests you can simply decline.

## What runs where

- **The library repo:** just your skill folders (plus, optionally, a list of
  team repos and a GitHub automation for fan-out). No acx state lives here.
- **Each project:** the installed copies, the receipt file (`skills.lock`),
  and the snapshots (`.agent-codex/`) — all committed, all plain files.
- **acx itself:** one binary on laptops and in CI. No server, no accounts.
  Remove it and every file remains readable.

## Mini-glossary

| Term you'll see | What it means here |
|---|---|
| **upstream / library / canonical repo** | The one official skills repo |
| **consuming repo / downstream** | A project that installed skills from it |
| **lockfile (`skills.lock`)** | The receipt: what you installed, from where |
| **ancestor / snapshot** | The saved copy of exactly what you received — what makes safe merging possible |
| **drift** | Any difference between your copy and what you received |
| **3-way merge** | Combining your edits and the library's update by comparing both against the ancestor |
| **vendored** | The skill's files are physically in your repo (not fetched at runtime) |
| **projection / surface** | A copy auto-formatted for one specific tool (Claude Code, Cursor…) |
| **harvest** | Turning local edits into a proposed library change |
| **fleet** | All the repos a library propagates to |
| **eval** | A test case shipped with a skill proving it works |
