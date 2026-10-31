# ENDOF.md: handoff

Last checkpoint: 2026-10-31, after the "continue until all done" pass. Read this first. Design: `docs/superpowers/specs/2026-10-31-sync-ledger-design.md`. Audit with per-ID status: `docs/audit.md`.

## Status per goal

- Goal 1, idempotent sync: DONE except D6 (duration and distance match).
  - Ledger: JSON, statuses, FIT hash index, start time, cross-process lock, atomic writes, v1 migration, stale `uploading` after 15 min.
  - Guards, in order: ledger ID, in-flight set (per ride), Garmin remote match (start ±2 min, last 100 rides), FIT hash, `uploading` before POST, Garmin 409.
  - Dry-run: `--dry-run`. Verify: `--verify`. Both read only.
  - Watch mode uses the same pipeline; start time read from the FIT session, so the remote check applies there too.
- Goal 2, quality: DONE except items below.
  - Done: JSON injection fix, shared `fsutil` (atomic writes) and `httpx` (bounded bodies, short error snippets), `slog` file log with redaction, TUI context cancellation, upload pipeline moved to `app`, page cap, watch hash cache, config test isolation, goleak test on 50 rides (heap 504 to 508 KiB, 2 goroutines).
  - Open: S1 keyring (needs your approval), A3 (`Service` depends on concrete `*state.Store`), A8 (public token fields).
- Docs: README and AGENTS.md updated this pass. `docs/audit.md` has a status table.

## Commands

```
go vet ./...
go test -race ./...
golangci-lint run ./...
GOOS=windows go build -o /tmp/opencode/w.exe ./cmd/whoosh2connect
go run ./cmd/whoosh2connect --no-tui --dry-run
go run ./cmd/whoosh2connect --verify
```

## Decisions

- Ledger = JSON + `gofrs/flock` lock file. SQLite rejected: volume small.
- Remote match uses the Garmin activity list from `garth` test cassettes (no verified endpoint in `python-garminconnect` source; its list call lives in a module I did not read). Matching uses start time only.
- Remote error blocks upload. An unverifiable ride is never sent blind.
- Stale `uploading` resolved only after 15 min. Shorter would let an in-flight upload from another process be marked failed and re-uploaded.
- Upload confirmed but ledger write failed: status stays `uploaded`. Garmin has the ride; the ledger is repaired by the next verify or remote match.
- Log redaction is by key name. Values under other names are not redacted. Keep secret values out of log attributes.
- Config test isolated from `~/.config`. The earlier failure came from the developer's own config file, not from code.
- Assumption: design approved in chat ("json", "1", "go").

## Open questions and risks

- S1: credentials and tokens are plain 0600 JSON. Proposed fix: OS keyring (Windows Credential Manager, Secret Service on Fedora). Needs your approval before change.
- D6: `RideDuration` format is unknown. Need one real MyWhoosh response to compare durations safely.
- Remote match only sees the last 100 Garmin activities. Older duplicates rely on the ledger.
- Verify does up to 5 Garmin requests per uploaded ride. Slow with a large ledger. Acceptable for a manual command.
- Watch mode: `seen` map and the one-shot hash cache are per process. Restart re-checks the ledger, not Garmin, for rides outside the 100-ride window.

## Next concrete step

Ask Thomas: approve the keyring change (S1), and send one MyWhoosh activity JSON with `rideDuration` (D6). Otherwise the work is complete; remaining items are low priority (A3, A8).
