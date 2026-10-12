# AGENTS.md

Guidance for agents and contributors working on `whoosh2connect`.

## What this is

A local Go CLI/TUI that downloads MyWhoosh cycling sessions as FIT files, fixes
them so Garmin Connect computes training metrics, spoofs a configurable Garmin
Edge device, and uploads the result to Garmin Connect (with two-factor auth).
See `MISSION.md` for the original brief and `README.md` for user-facing docs.

## Layout

```
cmd/whoosh2connect/   thin main: flags, wiring, headless mode, TUI launch
internal/config/      YAML + env + flags (precedence: flags > env > file > defaults), XDG paths
internal/device/      Edge presets (product IDs from the Garmin FIT SDK), serial gen/validate, apply to FIT
internal/fit/         port of fitfix.go: fill session averages, strip temperature, spoof device
internal/mywhoosh/    MyWhoosh API client (login, list, download) + token cache
internal/garmin/      token store (0600), mobile SSO + MFA, client (upload, refresh)
internal/state/       idempotence store of uploaded activity IDs
internal/app/         orchestration: download → fix → upload → mark, session listing
internal/tui/         Bubble Tea model, views, settings, MFA screen
internal/version/     build-time version/commit
```

## Commands

```
just build     # build bin/whoosh2connect with version ldflags
just test      # go test ./...
just vet       # go vet ./...
just lint      # golangci-lint run
just fmt       # gofmt -s -w .
just tidy      # go mod tidy
just run -- --flags  # go run ./cmd/whoosh2connect
```

Always run `gofmt -s -w`, `go vet ./...`, `go test ./...`, and
`golangci-lint run` before considering work done.

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
