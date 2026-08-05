#!/usr/bin/env bash
# Guided tour of shu: plays out the full lifecycle in a throwaway sandbox.
#
#   git clone https://github.com/navisoft0/shuhari
#   cd shuhari
#   bash docs/demo.sh
#
# Needs: git, and either Go 1.24+ or a prebuilt ./shu binary next to go.mod.
set -euo pipefail

say()  { printf '\n\033[1;36m▸ %s\033[0m\n' "$*"; }
run()  { printf '\033[1;33m$ %s\033[0m\n' "$*"; "$@" || true; }

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
SHU="$repo_root/shu"
if [ ! -x "$SHU" ]; then
  say "Building shu (one-time)"
  (cd "$repo_root" && go build -o shu ./cmd/shu)
fi

sandbox="$(mktemp -d -t shu-demo-XXXXXX)"
trap 'rm -rf "$sandbox"' EXIT
library="$sandbox/library"
project="$sandbox/checkout-service"
gitc() { git -C "$1" -c user.name=demo -c user.email=demo@acme.test "${@:2}"; }

say "ACT 1 — Acme's official playbook library (an ordinary git repo)"
mkdir -p "$library/skills/deploy-checklist"
cat > "$library/skills/deploy-checklist/SKILL.md" <<'EOF'
---
name: deploy-checklist
description: How to deploy services the Acme way.
version: 1.0.0
---

# Deploy checklist

## Before deploying

Notify the release channel before deploying.

Run the smoke tests.

## Rolling back

Revert the release tag and redeploy the previous version.
EOF
gitc "$library" init -q -b main
gitc "$library" add -A && gitc "$library" commit -qm "deploy-checklist v1.0.0"
echo "library ready at $library"

say "ACT 2 — The checkout team installs the playbook into their project"
mkdir -p "$project" && cd "$project"
run "$SHU" add deploy-checklist --from "$library" --surfaces claude-code,agents-md
run "$SHU" status

say "ACT 3 — The team adds a local nuance (their warehouse-scanner rule)"
sed -i.bak 's/Notify the release channel before deploying./Notify the release channel and the warehouse floor manager before deploying./' \
  .claude/skills/deploy-checklist/SKILL.md && rm -f .claude/skills/deploy-checklist/SKILL.md.bak
run "$SHU" status
echo "(drifted = customized locally; expected state, not an error)"

say "ACT 4 — Headquarters ships v1.1.0, improving a DIFFERENT section"
sed -i.bak -e 's/version: 1.0.0/version: 1.1.0/' \
  -e 's/Revert the release tag and redeploy the previous version./Revert the release tag, redeploy the previous version, and confirm health checks pass./' \
  "$library/skills/deploy-checklist/SKILL.md" && rm -f "$library/skills/deploy-checklist/SKILL.md.bak"
gitc "$library" add -A && gitc "$library" commit -qm "deploy-checklist v1.1.0"
run "$SHU" update
echo
echo "--- the merged file keeps BOTH the local nuance and the new rollback text ---"
grep -n "warehouse floor manager\|confirm health checks" .claude/skills/deploy-checklist/SKILL.md

say "ACT 5 — Headquarters ships v1.2.0, editing the SAME sentence the team customized"
sed -i.bak -e 's/version: 1.1.0/version: 1.2.0/' \
  -e 's/Notify the release channel before deploying./Notify the on-call engineer before deploying./' \
  "$library/skills/deploy-checklist/SKILL.md" && rm -f "$library/skills/deploy-checklist/SKILL.md.bak"
gitc "$library" add -A && gitc "$library" commit -qm "deploy-checklist v1.2.0"
run "$SHU" update
echo
echo "--- shu refused to guess: both versions are marked in the file ---"
sed -n '/<<<<<<</,/>>>>>>>/p' .claude/skills/deploy-checklist/SKILL.md

say "ACT 6 — A human blends the two in one edit, and the decision sticks"
python3 - <<'EOF' 2>/dev/null || perl -0pi -e 's/<<<<<<< local\n.*?=======\n.*?>>>>>>> latest/Notify the on-call engineer and the warehouse floor manager before deploying./s' .claude/skills/deploy-checklist/SKILL.md
import re, pathlib
p = pathlib.Path(".claude/skills/deploy-checklist/SKILL.md")
t = re.sub(r"<<<<<<< local\n.*?=======\n.*?>>>>>>> latest",
           "Notify the on-call engineer and the warehouse floor manager before deploying.",
           p.read_text(), flags=re.S)
p.write_text(t)
EOF
run "$SHU" status
echo "(back to plain 'drifted': the blend is now just their local nuance on top of v1.2.0)"

say "ACT 7 — The nuance turns out useful company-wide: send it upstream"
run "$SHU" harvest deploy-checklist --push --branch shu/harvest/deploy-checklist
echo
echo "--- the proposal branch in the library, with attribution ---"
gitc "$library" log -1 --format='%s%n%b' shu/harvest/deploy-checklist

say "ACT 8 — The safety net, in one command each"
run "$SHU" verify
echo "(working=modified is verify doing its job: the team's copy differs from the"
echo " official content on record — in CI this catches tampering; here it's the nuance)"
echo
run "$SHU" scan
echo
echo "(shu audit would show who changed which playbook, when — it reads the project's"
echo " git history, which this throwaway sandbox project doesn't have)"

say "Demo complete — sandbox was $sandbox (auto-removed)"
