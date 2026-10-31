# Audit: whoosh2connect

Date: 2026-10-31. Scope: all Go packages, read in full for `internal/app`, `internal/state`, `internal/garmin`, `internal/mywhoosh`, `internal/watch`, `internal/config`, and `cmd/whoosh2connect`. Partially read for `internal/tui` (upload call sites only). Status column: FIXED (this session), OPEN, or VERIFY (not yet confirmed by reading or test).

## 1. Places where an activity can be uploaded twice

| # | Scenario | Status | Fix |
|---|----------|--------|-----|
| D1 | Same ride reached via API (activity ID key) and via watch mode (`local:<hash>` key). Two keys, two uploads. | FIXED | Ledger now also indexes spoofed-FIT SHA-256. Both paths produce identical spoofed bytes, so the hash hit skips the second. |
| D2 | Upload POST sent, response lost (timeout, reset). Entry stays `uploading`. | PARTIAL | `uploading` written before POST. On restart it becomes `failed` and is retried. Remote match (start ±2 min) runs before upload and catches it when the ride is in the last 100 activities. Older rides: still a duplicate risk. |
| D3 | Upload succeeded, then ledger write fails. Old code returned failed, ledger lacked the ride, next run re-uploads. | OPEN | Retry the ledger write with backoff, or rely on remote match. Needs a test with a failing persist. |
| D4 | Two processes share `state.json` (TUI and watch, or two CLI runs). Each holds its own in-memory map and rewrites the whole file. The later writer drops the other's entries. | OPEN, high | Lock file (`state.lock`, `flock`) plus reload-merge before each write. Must be done before running watch and TUI together. |
| D5 | TUI runs `svc.Upload` in goroutines (`tea.Cmd`). Two uploads of the same ride can run concurrently, each passing the ledger check before either writes `uploading`. | OPEN | Per-ID in-flight set in `app.Service` guarded by mutex. |
| D6 | Garmin 409 treated as duplicate only for exact matches. Garmin may accept a near-identical copy. | OPEN (by design) | Remote match covers start time. Duration and distance comparison not done. |
| D7 | Watch `seen` set is per process. Restart re-scans old FIT files. | FIXED for upload | Ledger hash check stops re-upload. Scan cost remains (see U2). |

## 2. Goroutines and resources without clear cancel or close

| # | Item | Status | Fix |
|---|------|--------|-----|
| G1 | TUI upload calls use `context.Background()` (`internal/tui/model.go` lines 161, 590). No way to cancel an upload on quit. | OPEN | Pass a context owned by the model, cancelled on quit. |
| G2 | Watch loop goroutine and ticker: cancel path via signal handler exists, but not reviewed in detail. | VERIFY | Read `watchLoop` in full, add a leak test with goleak. |
| G3 | HTTP response bodies: closed with `defer` in Garmin upload, Garmin SSO, MyWhoosh login and download. `ListActivities` closes per page. | FIXED in reviewed paths | Keep. Add a test that counts open bodies (later). |
| G4 | Temp FIT files created with `os.CreateTemp`. Old code deferred remove but also deferred close after remove. | FIXED | `uploadBytes` closes, writes, and removes in one path. |
| G5 | `Store` writes are synchronous inside a mutex. A slow disk blocks every ledger read. | OPEN, low | Acceptable for the volume. Note only. |

## 3. Unbounded in-memory structures

| # | Item | Status | Fix |
|---|------|--------|-----|
| U1 | `mywhoosh.ListActivities` loops over all pages and keeps every activity. No upper bound on pages. | OPEN | Cap pages (for example 50) and page size. |
| U2 | `watch.Scan` hashes every matching file every interval (10 s default). Cost grows with directory size. | OPEN | Cache hash by (path, size, mtime). Recompute only on change. |
| U3 | `watch.Watcher.seen` and `prev` maps grow with rides seen in this process. | OPEN, low | Bounded by rides per session. Acceptable. Note only. |
| U4 | HTTP bodies read with `io.ReadAll` without a size limit: Garmin upload, Garmin activity list (new), MyWhoosh login, list, download, and FIT download. | OPEN | Wrap with `io.LimitReader` (the new Garmin activity list already does, 10 MiB). |
| U5 | Ledger grows with every ride ever. | Acceptable | A few hundred entries per year. Note only. |

## 4. Security

| # | Item | Status | Fix |
|---|------|--------|-----|
| S1 | Tokens (Garmin OAuth1 and OAuth2, MyWhoosh access token) stored as plain JSON, mode 0600. Not encrypted at rest. | OPEN, ASK FIRST | Propose OS keyring (Windows Credential Manager, Secret Service on Fedora). The task says ask before changing. Not changed. |
| S2 | Garmin token files written with `os.WriteFile` directly, not temp + rename. A crash can leave a truncated token file. | OPEN | Use the same atomic writer as the ledger. |
| S3 | `DownloadFit` built JSON with `fmt.Sprintf` (injection). | FIXED | `json.Marshal` plus a test with quotes in the ID. |
| S4 | Error messages include up to 300 bytes of HTTP response body (`truncate`). A login error body could echo account data. | OPEN | Redact known fields, or log only status code and the first 80 characters. |
| S5 | No structured logging. Errors go to stderr via `fmt`. No file log under the data dir. Redaction does not exist. | OPEN | Add `log/slog` with a file handler under the XDG data dir and a redacting handler for tokens and passwords. |
| S6 | `InsecureSkipVerify` | VERIFY | Grep the tree. None seen in the files read. |
| S7 | Path handling: watch directory is user-supplied and globbed. Activity IDs are never used as file names. Temp names are generated. | OK | Keep. Validate that the watch glob stays inside the directory (no `..`). |
| S8 | File permissions: state and config written 0600, directories 0700. | OK | State is now atomic. Config save is not atomic (`os.WriteFile`). Make it atomic too. |
| S9 | Credentials: passwords read from env in headless mode (`WHOOSH2CONNECT_*_PASSWORD`). Environment variables are visible to same-user processes. | Accepted | Document in README. Prefer interactive prompt. |
| S10 | Shell execution: none. No `exec.Command` in reviewed code. | OK | Keep. |

## 5. Architecture smells

| # | Item | Status | Fix |
|---|------|--------|-----|
| A1 | Upload orchestration duplicated: headless loop in `cmd/main.go`, upload-all loop in `internal/tui/model.go` (line 590), watch loop in `main.go`. Each decides its own status text. | OPEN | Move the loop into `app` (one `SyncOne`/`SyncAll` API). TUI and CLI only render results. |
| A2 | Logic in TUI: upload-all sequencing and confirm counting live in `tui/model.go`. | OPEN | Move sequencing to `app`. |
| A3 | `app.Service` holds `*state.Store` (concrete type). Tests need a real file. | FIXED in part | `Service` still takes `*state.Store`. Next: define a `Ledger` interface on the consumer side. |
| A4 | `garmin.Options.Hosts` is a test hook inside production options. Test code reaches into `hosts` through the same struct. | OPEN, low | Keep unexported. Tests in the same package already use it. Acceptable. |
| A5 | `truncate` duplicated in `mywhoosh` and `garmin`. | OPEN, low | Move to a small shared helper or keep per package. Decide when refactoring errors. |
| A6 | Global state: none reviewed. Device presets are a read-only table. | OK | Keep. |
| A7 | Config precedence (flags > env > file > defaults) is clear and tested. | OK | Keep. |
| A8 | Garmin client exposes `Domain`, `TokenDir`, `OAuth1`, `OAuth2` fields publicly. Callers can mutate tokens. | OPEN, low | Make token fields unexported when refactoring. |

## Fixes done in this session

- Ledger v2 (JSON, atomic, hash index, stale recovery, v1 migration).
- Upload guards: ID, hash, remote match, `uploading` before POST, failure recorded.
- Garmin remote match via activities list (ported from garth cassettes).
- `--dry-run` decisions without side effects.
- `DownloadFit` JSON injection fixed.

## Proposed order for the next work

1. D4 (two processes, file lock). Data loss risk, blocks running watch and TUI together.
2. D5 (per-ID in-flight guard).
3. U4 and U1 (body limits, page cap).
4. S2 and S8 (atomic writes for tokens and config).
5. S5 (slog with redaction), S4.
6. A1 and A2 (move sync loops into `app`).
7. U2 (hash cache for watch).
8. Verify command, watch remote check, README, AGENTS.md.
9. S1: keyring. Ask Thomas first.
