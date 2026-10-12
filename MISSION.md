# Mission

Build `whoosh2connect`: Go CLI/TUI app that uploads MyWhoosh cycling sessions to Garmin Connect.

# Reference repos (read first, reuse logic)

- Go (main guideline): https://github.com/jdelrue/mywoosh2garmin
- Python (reference for Garmin 2FA/MFA only): https://github.com/JayQueue/MyWhoosh2Garmin

Do not invent endpoints, headers, product IDs or flows. Read repo code, derive behavior from it. If unsure, flag it, do not guess.

# Skills

Use the Superpowers plugin skills. Follow them strictly: brainstorming, planning, TDD, systematic debugging, verification before completion. Read each relevant SKILL before acting.

# Functional requirements

1. MyWhoosh: login, list sessions, download .fit file. Follow Go repo.

2. Device spoofing: port `fitfix.go` from the Go repo into the new project. Keep behavior intact. Goal: Garmin Connect must compute training load, training effect, etc. from the uploaded .fit. Keep logic faithful. Add unit tests with sample .fit data.
   - Device must be configurable, not hardcoded. Support presets (at least Edge 540, Edge 1030, plus other Edge models found in Garmin FIT SDK product list). Each preset = manufacturer, product ID, default name.
   - User can override device name and serial number.
   - Serial number: validate (uint32, non-zero). Default: stable value generated once and persisted, so Garmin sees one consistent device. Never change silently between uploads.
   - Config via config file (YAML or TOML in XDG config dir), env vars, CLI flags. Precedence: flags > env > file > defaults.
   - Derive product IDs from the Go repo and Garmin FIT SDK. Do not guess IDs.
   - Apply chosen device consistently across all relevant FIT messages (file_id, device_info, etc.) as fitfix.go does.
   - Unit tests: each preset, name override, serial override, invalid serial.

3. Garmin: login + upload fixed .fit. Follow Go repo.

4. Garmin 2FA/MFA: Go repo does NOT support it. Python repo does. Study Python implementation, reimplement in Go: detect MFA challenge, prompt code in TUI, submit, obtain tokens. Persist tokens securely (OS keyring preferred, else file mode 0600). Reuse and refresh tokens to avoid repeated login/MFA.

5. Idempotence: do not re-upload sessions already uploaded (local state file).

# UI

- TUI with Bubble Tea + Lip Gloss (+ Bubbles components if useful). No Fyne.
- Act as expert UX/UI designer: clear layout, keyboard-first, help bar with keybindings, consistent color theme, light/dark terminal support, loading spinners, progress, explicit error states, MFA code input screen, session list with status (new/uploaded/failed), confirmation before upload.
- Settings screen: select device preset from list, edit device name and serial number, inline validation, save to config. Show current device in header or status bar.
- Handle window resize. No blocking in Update loop (use tea.Cmd).

# Code quality

- Idiomatic Go: small packages (internal/mywhoosh, internal/garmin, internal/fit, internal/tui, internal/config), interfaces at consumer side, context propagation, wrapped errors (%w), no globals, no panics, gofmt, go vet, golangci-lint clean.
- Tests: table-driven, HTTP mocked via httptest. No real credentials in tests.
- Security: never log passwords, tokens, MFA codes. Credentials via env/prompt/keyring.
- Go modules (module name matches repo), pinned deps, Makefile (build, test, lint).
- Binary name: `whoosh2connect`.

# Docs

- Write AGENTS.md: project layout, commands, conventions, rules for agents.
- Write README.md: purpose, install, usage, 2FA flow, config, security notes, disclaimer (Garmin API unofficial, may break; user responsibility for ToS).
- README: document device presets, name/serial override, config precedence, and warning that spoofing choice affects how Garmin Connect treats the activity.

# Agent communication style

Reply in caveman mode (terse, fragments, no filler/preamble/postamble). Exact values for code, paths, names. Code, comments, README, AGENTS.md written normally, not caveman.

# Definition of done

- `go build`, `go vet`, `go test ./...`, lint all pass.
- MFA flow works end to end against Garmin.
- Uploaded activity on Garmin shows training load + training effect.
- Uploaded activity on Garmin shows chosen device name/model and serial.
- AGENTS.md and README.md present.
- Summarize what was verified and what was not.

# Git History

- Commit your development using meaningfull & clear atomic conventional commits. Use initial git author
