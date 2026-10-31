# Auto-watch and notifications from the TUI: design

Date: 2026-10-31. Status: draft, awaiting approval.

## Intent

Users finish a MyWhoosh ride and should not have to start an upload. Turn the
existing watch mode (`--watch auto`, `internal/watch`) on or off from the TUI.
While it is on, every finished ride is fixed, spoofed, and uploaded to Garmin
Connect, and the user gets a system notification.

## Goals

- Toggle auto-watch from the TUI with one key, and show its state in the header or footer.
- The state is saved in the config file, so it survives restarts.
- Watch runs in the TUI process only. No service, no task scheduler, no tray.
- System notification on each successful upload, and on each failure or when MFA is needed.
- Reuse the existing pipeline (`internal/app`) and watcher. No second upload path.

## Non-goals

- Running when the TUI is closed.
- A tray icon or any background process.
- Changing the watch algorithm (stable scans, hash dedupe, ledger).

## Anti-goals

- Blocking the Bubble Tea `Update` loop. Watch runs in a `tea.Cmd`, results come back as messages.
- Showing a blocking MFA prompt from a notification. The user opens the TUI to type the code.
- Persisting Garmin passwords so that watch can log in alone.

## Constraints

- AGENTS.md: idiomatic Go, interfaces at the consumer side, no globals, `%w`, context propagation, table-driven tests, httptest mocks, no real credentials.
- Garmin tokens are reused and refreshed. If the refresh fails, watch stops and notifies. It does not prompt on stdin.
- Watch currently needs `--garmin-email`, so the TUI must read it from config or the login screen.

## Approach (one, chosen)

1. **Config**: add `watch` (`off` | directory or `auto`) and `auto_upload bool` to the YAML. The TUI toggle writes this file atomically (`fsutil`).
2. **Watch loop in the app**: move the loop out of `cmd/whoosh2connect/main.go` (`watchLoop`) into `internal/app` as a `Watch(ctx, ...)` that emits events (`Uploaded`, `Failed`, `MFARequired`). CLI `--watch` and the TUI both use it. This also closes audit item A1.
3. **TUI**: a toggle key (proposed `a`) starts or cancels the watch context in a `tea.Cmd`. Each event becomes a `tea.Msg` that updates the list status (new/uploaded/failed) and the footer.
4. **Notifications**: a small `internal/notify` package with an interface `Notifier { Notify(title, body string) error }`. Default implementation: `github.com/gen2brain/beeep`. Failures are logged and never stop the watch.
5. **Startup**: if `auto_upload` is true and the Garmin token is valid, watch starts with the TUI.

## Open items (to verify, not guessed)

- `beeep` is a new dependency. Check cgo/platform needs before adding it (AGENTS: flag, do not assume).
- MyWhoosh folder paths in `DefaultDir()` come from the reference repo. Confirm on real Windows and macOS.
- Confirm that the FIT file is complete when first seen. Stable scans cover the usual case.

## Testing

- `internal/app`: table-driven tests for the watch loop with a fake watcher and fake service. Cover success, failure, MFA-required, and cancel.
- `internal/notify`: test the interface with a fake. Real `beeep` is not called in tests.
- `internal/tui`: the toggle sends the right command and updates state. Test the message flow, not rendering.
- `-race` on all packages.
