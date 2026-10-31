# Sync ledger design (Goal 1: no duplicate uploads)

Status: approved in chat (ledger = JSON; remote-match deferred until a Garmin endpoint is verified).

## Intent

Each MyWhoosh ride reaches Garmin Connect exactly once. Re-running never creates a second copy, even after a crash or lost response.

## Goals

- Local ledger keyed by MyWhoosh activity ID and by SHA-256 of the spoofed FIT.
- Status per entry: `downloaded`, `spoofed`, `uploading`, `uploaded`, `failed`.
- Write `uploading` before the upload POST, then `uploaded` or `failed`.
- Atomic writes (temp file + rename), mode 0600.
- Migrate the existing `state.json` (map of ID to RFC3339 time) to `uploaded` entries.
- Dedup by ID and by hash before any upload.
- Dry-run prints action and reason per activity.

## Non-goals (this slice)

- Remote Garmin match (start time, duration, distance). Behind an interface, no implementation until the endpoint is verified.
- Verify command against Garmin. Follows the remote match.
- SQLite. Rejected: volume is small, JSON keeps dependencies flat.

## Anti-goals

- Never delete or rewrite user ledger data outside the atomic write path.
- Never guess a Garmin endpoint.

## Approaches

1. JSON file, atomic write. Chosen.
2. SQLite (modernc). Rejected: extra dependency for a few hundred rows.

## Decisions taken by the builder

- Ledger file stays `state.json` in the data dir. Schema version field added; v1 files are read as map of IDs.
- `uploading` entries found on startup are resolved as `failed` with reason `stale`, not re-uploaded silently. Remote match will decide later.
- Watch-mode key becomes the spoofed-FIT hash; ID-keyed entries stay separate and both are checked.

## Open risks

- Without a remote check, a lost response on the upload POST can still produce a duplicate. Garmin 409 is the only backstop.
