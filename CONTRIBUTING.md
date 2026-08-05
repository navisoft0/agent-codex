# Contributing to shuhari

Thanks for helping build `shu`.

## Building and testing

```sh
go build -o shu ./cmd/shu   # Go 1.24+, no other dependencies
go test ./...               # full suite; needs git on PATH
gofmt -l . && go vet ./...  # both must come back clean
```

`bash docs/demo.sh` runs the guided end-to-end tour in a throwaway sandbox —
useful for sanity-checking a change against the real command flow.

## Ground rules

- **Stdlib only.** The binary stays dependency-free; integrations (promptfoo,
  scanners, AI merge proposers) are reached by exec-ing external commands,
  never by importing them.
- **Never clobber, never guess.** Anything that touches a user's skill content
  must preserve local edits or stop with an explicit conflict. Silent
  resolution of prose is a bug even when it happens to be right.
- **Every mutation is a git commit or a PR.** No state outside the repos.
- **Degrade offline.** Commands should do something useful without network
  (lockfile + ancestor snapshots are local by design).

## Sending changes

Ordinary GitHub PRs. Keep commits focused, run the checks above, and for
behavior changes add a test alongside the existing suites (`internal/merge`,
`internal/cli`, ...). For anything user-visible, update README.md and the
usage text in `internal/cli/cli.go` together.
