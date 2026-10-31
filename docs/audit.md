# Audit: whoosh2connect

Date: 2026-10-31. Scope: all Go packages, read in full for `internal/app`, `internal/state`, `internal/garmin`, `internal/mywhoosh`, `internal/watch`, `internal/config`, and `cmd/whoosh2connect`. Partially read for `internal/tui` (upload call sites only). Status column: FIXED (this session), OPEN, or VERIFY (not yet confirmed by reading or test).

## 1. Places where an activity can be uploaded twice

| # | Scenario | Status | Fix |
|---|----------|--------|-----|
| D1 | Same ride reached via API (activity ID key) and via watch mode (`local:<hash>` key). Two keys, two uploads. | FIXED | Ledger now also indexes spoofed-FIT SHA-256. Both paths produce identical spoofed bytes, so the hash hit skips the second. |
| D2 | Upload POST sent, response lost (timeout, reset). Entry stays `uploading`. | PARTIAL | `uploading` written before POST. Entries older than 15 min become `failed` on Open and are retried after the remote check. Younger entries are left alone, so another process mid-upload is safe. Remote match (start ±2 min) runs before upload and catches it when the ride is in the last 100 activities. Older rides: still a duplicate risk. |
| D3 | Upload succeeded, then ledger write fails. Old code returned failed, ledger lacked the ride, next run re-uploads. | OPEN | Retry the ledger write with backoff, or rely on remote match. Needs a test with a failing persist. |
| D4 | Two processes share `state.json`. Each write overwrote the other. | FIXED | Every `Put` takes `state.json.lock` (gofrs/flock), reloads, applies, writes atomically. Open no longer resolves in-flight uploads (see D2). |
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
| S6 | `InsecureSkipVerify` | OK | Grep of the whole tree: no match. |
| S7 | Path handling: watch directory is user-supplied and globbed. Activity IDs are never used as file names. Temp names are generated. | OK | Keep. Validate that the watch glob stays inside the directory (no `..`). |
| S8 | File permissions: state and config written 0600, directories 0700. | OK | State is now atomic. Config save is not atomic (`os.WriteFile`). Make it atomic too. |
| S9 | Credentials: passwords read from env in headless mode (`WHOOSH2CONNECT_*_PASSWORD`). Environment variables are visible to same-user processes. | Accepted | Document in README. Prefer interactive prompt. |
| S10 | Shell execution: none. No `exec.Command` in reviewed code. | OK | Keep. |

## 5. Architecture smells

| # | Item | Status | Fix |
|---|------|--------|-----|
| A1 | Upload orchestration duplicated: headless loop in `cmd/main.go`, upload-all loop in `internal/tui/model.go` (line 590), watch loop in `main.go`. Each decides its own status text. | OPEN | PARTIAL: watch loop now in `app.Watch`, shared by CLI and TUI. Headless and upload-all loops still duplicate. |
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

## Status update (end of this pass)

| ID | Status | Note |
|----|--------|------|
| D1 | FIXED | Hash index shared by API and watch paths. |
| D2 | PARTIAL | Stale `uploading` resolved after 15 min. Remote match catches the last 100 rides only. |
| D3 | FIXED | Upload confirmed but ledger write failed: status stays `uploaded`, warning in `Err`. Test: read-only ledger dir. |
| D4 | FIXED | Cross-process lock, reload before every write. |
| D5 | FIXED | In-flight set per ride ID. Test: concurrent upload of the same ride is skipped. |
| D6 | CLOSED, by design | `rideDuration` format is `HH:MM:SS.mmm` (checked on one real sample: `totalWorkout` in kJ matches watts × duration). Garmin list gives `duration` in seconds. Not used as a match criterion: a small duration difference would cause a missed match and a duplicate upload. Start time stays the only remote key. |
| D7 | FIXED | Ledger hash check survives restarts. |
| G1 | FIXED | TUI uploads use a context cancelled on exit. |
| G2 | VERIFIED | Watch loop only waits on `ctx.Done()` or a timer. No goroutines spawned. |
| G3 | FIXED | Bodies closed in every reviewed path. |
| G4 | FIXED | Temp FIT removed in all paths. |
| G5 | ACCEPTED | Synchronous ledger writes, volume is small. |
| U1 | FIXED | Activity pages capped at 50. Test: request count equals cap. |
| U2 | FIXED | Hash cache keyed by path, invalidated on size or mtime change, pruned. |
| U3 | ACCEPTED | Per-process maps, bounded by rides per session. |
| U4 | FIXED | `httpx.ReadBody` caps every response at 10 MiB. |
| U5 | ACCEPTED | Ledger grows slowly. |
| S1 | OPEN, ASK FIRST | Tokens still plain 0600 JSON. Keyring change needs approval. |
| S2 | FIXED | Token files written atomically (`fsutil.WriteFileAtomic`). |
| S3 | FIXED | JSON built with `json.Marshal`. |
| S4 | FIXED | MyWhoosh login error no longer echoes the body. Other error snippets capped at 120 bytes. |
| S5 | FIXED | `slog` JSON log at `whoosh2connect.log` next to the executable, overwritten each run, mode 0600, keys containing password, token, secret, mfa, code, authorization, cookie redacted. |
| S6 | OK | No `InsecureSkipVerify`. |
| S7 | OK | Activity IDs never become file names. |
| S8 | FIXED | Config save is atomic. |
| S9 | ACCEPTED | Headless passwords come from env. Documented in README. |
| S10 | OK | No shell execution. |
| A1 | PARTIAL | Headless and TUI share `app` pipeline. Headless still has its own print loop (thin). |
| A2 | FIXED | TUI upload-all calls `UploadPending` in `app`. |
| A3 | OPEN, low | `Service` still takes concrete `*state.Store`. |
| A4 | ACCEPTED | Test hook in `garmin.Options`. |
| A5 | ACCEPTED | Small duplicate helpers. |
| A8 | OPEN, low | Garmin token fields public. |
