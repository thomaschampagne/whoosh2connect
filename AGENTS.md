# AGENTS.md

Guidance for agents and contributors working on `whoosh2connect`.

## What this is

A local Go CLI/TUI that downloads MyWhoosh cycling sessions as FIT files, fixes
them so Garmin Connect computes training metrics, spoofs a configurable Garmin
Edge device, and uploads the result to Garmin Connect (with two-factor auth).
See `MISSION.md` for the original brief and `README.md` for user-facing docs.

## Layout

```
cmd/whoosh2connect/   thin main: flags, wiring, headless mode, watch loop, TUI launch
internal/config/      YAML + env + flags (precedence: flags > env > file > defaults), XDG paths
internal/device/      Edge presets (product IDs from the Garmin FIT SDK), serial gen/validate, apply to FIT
internal/fit/         port of fitfix.go: fill session averages, strip temperature, spoof device
internal/mywhoosh/    MyWhoosh API client (login, list, download, page cap) + token cache
internal/garmin/      token store (0600), mobile SSO + MFA, upload client, activity list for remote match
internal/state/       sync ledger (JSON, cross-process lock, atomic writes)
internal/fsutil/      atomic 0600 file writes
internal/httpx/       bounded response reads, short error snippets
internal/logx/        slog file logger with secret redaction
internal/watch/       local FIT directory watcher (hash cache, stable scans)
internal/notify/      system notifications behind a small interface
internal/app/         upload pipeline shared by API and watch paths; ledger guards; verify
internal/tui/         Bubble Tea model, views, settings, MFA screen
internal/release/     Scoop manifest rendering tests
internal/version/     build-time version/commit
bucket/               Scoop manifest, regenerated on every release
docs/                 design docs, plans, audit
.github/workflows/    CI (just ci) and release (semantic-release + bucket job)
```
## Agent communication style

Reply in caveman mode (terse, fragments, no filler/preamble/postamble). Exact values for code, paths, names. Code, comments, README, AGENTS.md written normally, not caveman.

## Superpowers

- Superpowers skills installed. Before ANY task, check for relevant skill and invoke it. Mandatory, not optional.
- New feature or behavior change: `brainstorming` first. No code before design approved.
- After design: `using-git-worktrees`, then `writing-plans`, then `subagent-driven-development` or `executing-plans`.
- Implementation: `test-driven-development`. RED, GREEN, REFACTOR. Write failing test first, watch fail.
- Bug or failing test: `systematic-debugging`. Root cause before fix.
- Before claiming done: `verification-before-completion`. Run command, show output. Evidence over claim.
- Between tasks: `requesting-code-review`. Critical issues block.
- Branch finished: `finishing-a-development-branch`.
- Principles: YAGNI, DRY, simplicity first.
- If skill seem skipped, say so and invoke it
- Think in caveman mode

## Commands

```
just build     # dist/whoosh2connect with version/commit ldflags
just test      # go test -race ./...
just vet       # go vet ./... and GOOS=windows go vet ./...
just lint      # golangci-lint run
just fmt       # gofmt -s -w .
just fmt-check # check-only formatting (what CI runs)
just tidy      # go mod tidy
just ci        # check-only: fmt-check, vet, lint, test, build
just sanity    # local: fmt, test, vet, build
just release 1.2.3                  # all platforms into dist/ + checksums.txt
just scoop-manifest 1.2.3 <sha256>  # render bucket/whoosh2connect.json
just clean     # rm -rf bin dist
just update-deps
just run -- --flags  # build then run dist/whoosh2connect
go run ./cmd/whoosh2connect --no-tui --dry-run   # preview sync decisions
go run ./cmd/whoosh2connect --verify            # ledger vs Garmin, read only
```

Always run `gofmt -s -w .`, `go vet ./...`, `GOOS=windows go vet ./...`,
`go test -race ./...`, and `golangci-lint run` before considering work done.

## Conventions

- Idiomatic Go; small packages; **interfaces defined at the consumer side**.
- Wrap errors with `%w`; lowercase error strings; no panics; no globals.
- Propagate `context.Context` through all network calls.
- Tests are table-driven; HTTP is mocked with `net/http/httptest`. **Never use
  real credentials in tests.**
- Never log or persist passwords, tokens, or MFA codes. Tokens are files with
  mode `0600`; passwords live only in memory or env vars.
- TUI: never block the `Update` loop — do work in `tea.Cmd` functions.

## Rules for agents

- **Do not invent endpoints, headers, product IDs, or auth flows.** Derive them
  from the reference repos and the Garmin FIT SDK. If unsure, flag it, do not
  guess.
- Reference repos: `github.com/jdelrue/mywoosh2garmin` (Go, main guideline),
  `github.com/JayQueue/MyWhoosh2Garmin` (Python, MFA reference),
  `github.com/matin/garth` (Python Garmin SSO reference).
- Device product IDs come verbatim from `muktihari/fit`
  `profile/typedef` (`GarminProduct…`). Keep `internal/fit` faithful to
  `fitfix.go` behavior.
- Commit using atomic conventional commits and the repository's initial git
  author.
- Agent chat replies are terse; code, comments, and docs are written normally.
