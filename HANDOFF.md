# HANDOFF.md

## Handoff Summary: Since the Initial MISSION.md

This is a full working implementation of `whoosh2connect`. Below is what was added or changed since the starting brief.

## Overview

A local Go CLI/TUI that logs into MyWhoosh, lists and downloads rides as FIT files, fixes them (fill session averages, strip bogus temperature), spoofs a configurable Garmin Edge device, and uploads to Garmin Connect with MFA (mobile SSO). Features: idempotent uploads by activity ID, a redesigned TUI, and a cross-platform watch mode that detects newly finished MyWhoosh rides and auto-uploads them.

## Key Artifacts Created/Updated

### Core Modules

- `internal/config/` — YAML config + env (includes `WHOOSH2CONNECT_CONFIG`), flags precedence, defaults.
- `internal/device/` — Device presets derived from `muktihari/fit` typedef (automatic Edge list; regional variants filtered). Apply spoofs to file_id/device_info; persistent serial (non-zero uint32, generated once).
- `internal/fit/` — FIT fixer port (fills missing session averages, strips bogus temperature, applies device spoofing).
- `internal/mywhoosh/` — API client + token cache (0600).
- `internal/garmin/` — tokens (0600), mobile SSO + MFA (garth flow), upload with 409 duplicate handling.
- `internal/state/` — idempotent store keyed by activity ID (0600, race-safe).
- `internal/app/` — Service with Upload/ProcessLocalFit; shared upload path with duplicate detection and temp file cleanup.
- `internal/watch/` — Stable-scan directory watcher: ride considered done after N stable scans (size+mtime+content hash). Cross-platform default dirs (macOS/Windows), per-watcher seen-set for dedupe, `Forget(hash)` to retry failed uploads.
- `internal/tui/` — Bubble Tea UI: framed panels, header+stats+footer, aligned list with pagination, preview screen (enter), step progress in status, count-aware upload-all confirm, consistent keymap, theme (dark/light).
- `internal/version/` — build-time version/commit.

### CLI

- `cmd/whoosh2connect/main.go` — flags: `--config`, `--mywhoosh-email`, `--garmin-email`, `--device-preset`, `--device-name`, `--device-serial`, `--no-tui`, `--version`, `--watch <dir|auto>`, `--watch-interval <sec>`, `--watch-stable <n>`. Headless sync with MFA prompt on stdin; watch loop with graceful interrupt.

### Build/Test

- `justfile` replaces Makefile (build/test/vet/lint/fmt/tidy/clean/run). `just build` stamps version/commit. `just` is available.
- `go.mod` uses `github.com/muktihari/fit v0.28.4`, `github.com/charmbracelet/bubbletea v1.3.10`, `github.com/charmbracelet/bubbles v1.0.0`, `github.com/charmbracelet/lipgloss v1.1.0`, `github.com/google/uuid v1.6.0`, `github.com/dghubble/oauth1 v0.7.3`, `gopkg.in/yaml.v3 v3.0.1`.

### Docs

- `README.md` — install/usage (TUI default, `--no-tui`, watch mode), flags table, config precedence, env vars, device presets, MFA, security (tokens 0600, no creds persisted), paths (XDG), disclaimer.
- `AGENTS.md` — project layout, commands (`just`), conventions, agent rules.
- `docs/superpowers/plans/2026-10-12-whoosh2connect.md` — all tasks marked complete.
- `.superpowers/sdd/2026-10-12-whoosh2connect/progress.md` — SDD ledger entries.

### Tests

- Total 107 tests passing across 11 packages (config 8, device 11, fit 7, mywhoosh 9, garmin 12, state 4, app 10, tui 22, cmd 10, watch 5, version 0/0 trivial). All table-driven, HTTP mocked with httptest. No real credentials.

## Behavior Details

### Device Presets

Derived from `typedef.ListGarminProduct()` (muktihari/fit). Filter Edge; drop regional/special editions. Canonicalize keys (e.g. `edge1000` → `edge_1000`, `Edge_130` → `edge_130`, `edge_explore2` → `edge_explore_2`). String names used as display defaults. Lookup is exact by ID.

### Watch Mode (New)

- Default paths: macOS `~/Library/Containers/com.whoosh.whooshgame/Data/Library/Application Support/Epic/MyWhoosh/Content/Data`; Windows `%USERPROFILE%\AppData\Local\Packages\MyWhooshTechnologyService.*\LocalCache\Local\MyWhoosh\Content\Data`; others require explicit dir.
- Stability: file must remain identical (size+mtime) and produce same content hash for `--watch-stable` consecutive scans (default 2). Uses periodic scan (`--watch-interval` sec, default 10) — cross-platform, resilient to partial writes.
- Dedupe: hash-based `seen` set (lifetime of watcher); failed uploads call `Forget(hash)` to retry later. Uploads use `ProcessLocalFit` with key `local:<hash>`, recording to state store (idempotent). Spoofing always applied (fix+spoof+upload).
- Headless-only when watching (can run alongside MyWhoosh after rides finish).

### TUI Redesign

- Framed layout with header (title+version+device), stats bar (✓ synced, ○ pending, ● on Garmin), boxed scrollable list with aligned columns (time, title, status), preview panel (enter), footer status/help. Consistent keymap: `↑/↓/j/k` navigate, `enter` preview/open, `u` upload, `a` upload all (count-aware confirm), `s` settings, `r` refresh, `?` help, `esc/q` back/quit, `tab/↓/enter/up` settings nav, `ctrl+s` save.

### Security & Idempotence

- Tokens/state written with mode 0600. Passwords/MFA never persisted/logged. Duplicate uploads marked via local state or Garmin 409 → return `StatusDuplicate` and mark uploaded.

## Next Recommended Steps (If Continuing)

1. Flesh out TUI pagination edge cases (cursor movement across pages, pageSize configurable).
2. Add `--watch-glob` override (default `*.fit`) and maybe support `MyNewActivity-*.fit` explicitly.
3. Watch mode: optional MyWhoosh login not needed now (local files) — could add toggle if API ever required, but not necessary.
4. macOS/Linux `DefaultDir()` auto-detect: consider a small fallback search if container path differs across MyWhoosh versions.
5. Optional: show step progress (`download → fix → upload`) per activity in TUI (currently spinner text).

## References

- `mission.md` original brief.
- Reference repos: `jdelrue/mywoosh2garmin` (Go), `JayQueue/MyWhoosh2Garmin` (Python), `matin/garth` (Python).
- FIT SDK: `muktihari/fit@v0.28.4` (typedef product list).

## Verification (Last Run)

- `gofmt -s -w` clean; `go build ./...` OK; `go vet ./...` OK.
- `go test ./...` → 107 passed, 11 packages.
- `golangci-lint run` OK (v2.14.0).
- `just build` OK; binary prints version/commit.
