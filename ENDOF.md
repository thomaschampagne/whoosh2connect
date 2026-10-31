# ENDOF.md: handoff

Last checkpoint: 2026-10-31. Read this first, then `docs/superpowers/specs/2026-10-31-sync-ledger-design.md`.

## Status per goal

- Goal 1, idempotent sync. Partial.
  - Done: JSON ledger (`internal/state`) with statuses, hash index, atomic 0600 writes, v1 migration, stale `uploading` resolved to `failed` on open, corrupt file never overwritten.
  - Done: `internal/app` uses ledger. Dedup by ID and by spoofed-FIT hash. `uploading` written before POST. 409 recorded as uploaded. Remote-check hook (`RemoteChecker`, nil = off). Remote error = no upload.
  - Not done: real Garmin remote checker (blocked on verified endpoint). Dry-run mode. Verify command. Watch mode has no remote check (no start time handy; needs FIT session parse).
- Goal 2, quality. Not started except where noted above. Missing: `docs/audit.md`, security fixes (`DownloadFit` builds JSON with `fmt.Sprintf`, use `json.Marshal`), watch `seen` map growth, goroutine/leak check, README, AGENTS.md update.

## Commands

```
go vet ./...
go test -race ./internal/app/ ./internal/state/
go test ./...                 # config test fails, see Risks
golangci-lint run ./...
GOOS=windows go build -o /tmp/opencode/w.exe ./cmd/whoosh2connect
just build && just run -- --no-tui
```

## Decisions

- Ledger = JSON, not SQLite. Volume is small; no new dependency. Why: approved by Thomas.
- Remote check not implemented. Reason: no Garmin activity-list endpoint exists in the reference repos (checked `jdelrue/mywoosh2garmin/garmin/client.go` and `matin/garth`). AGENTS.md forbids guessing endpoints.
- Remote error blocks upload. Why: an unverifiable ride must not go up blind.
- Stale `uploading` becomes `failed` on open, not auto-retried. Why: the lost POST may have landed.
- Design approved in chat ("json", "1", "go"). Assumption: that counts as design approval.

## Open questions and risks

- Need a verified Garmin activity-list endpoint, or HAR capture from Garmin Connect web. Ask Thomas.
- `internal/config` test `TestInvalidEnvSerialIgnored` fails on the original commit too. Not caused by this work. Investigate before release.
- Remote match plan: start time +/- 2 min, then duration and distance. Units of `RideDuration` and `Distance` unknown. Do not guess; check against real API response.
- Garmin ID not parsed from upload response. `GarminID` stays empty.

## Next concrete step

1. Ask Thomas for the Garmin activity-list endpoint (HAR capture or approval to use a named endpoint).
2. Meanwhile: fix `DownloadFit` JSON building with `json.Marshal` (security, no endpoint needed), then add dry-run planner in `internal/app` that returns reason per activity, then CLI `--dry-run` in `cmd/whoosh2connect`.
