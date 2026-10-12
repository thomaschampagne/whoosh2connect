# whoosh2connect Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `whoosh2connect`, a local Go TUI that logs in to MyWhoosh, downloads a ride FIT, fixes it (averages/temperature/device spoof), and uploads it to Garmin Connect with MFA support.

**Architecture:** Small `internal/*` packages behind consumer-side interfaces, wired by `internal/app` and driven by a Bubble Tea `internal/tui`. Design: `docs/superpowers/specs/2026-10-12-whoosh2connect-design.md`.

**Tech Stack:** Go 1.24+, `muktihari/fit`, `dghubble/oauth1`, `google/uuid`, `charmbracelet/bubbletea`+`bubbles`+`lipgloss`, `gopkg.in/yaml.v3`.

**Spec:** `docs/superpowers/specs/2026-10-12-whoosh2connect-design.md`

## Global Constraints

- Module `whoosh2connect`, binary `whoosh2connect`.
- Config precedence flags > env > file > defaults. Config = YAML in XDG config dir.
- Tokens = files mode 0600. Never log secrets.
- Device serial: uint32, non-zero, generated once and persisted, never changes silently.
- Product IDs copied verbatim from Garmin FIT SDK constants. No guessed IDs.
- `go build`, `go vet`, `go test ./...`, golangci-lint clean.
- Conventional atomic commits; author = initial git author (Thomas Champagne).

---

### Task 0: Scaffold
**Files:** Create `go.mod`, `Makefile`, `.gitignore`, `internal/version/version.go`.
- [x] `go mod init whoosh2connect`, add deps, `make build` compiles empty `cmd`.
- [x] Commit `chore: scaffold module`.

### Task 1: config
**Files:** Create `internal/config/config.go`, `internal/config/config_test.go`.
**Produces:** `Default() Config`, `Paths{ConfigFile,DataDir}`, `Load(Overrides) (Config,error)`, `Save(path,Config) error`, `EnsureSerial(*Config) (uint32,error)`.
- [x] Tests: defaults; YAML file load; env overrides; flag overrides beat env; file beats defaults; serial preserved.
- [x] Implement. Commit `feat(config): ...`.

### Task 2: device
**Files:** Create `internal/device/presets.go`, `internal/device/device.go`, `internal/device/device_test.go`.
**Produces:** `Presets() []Preset`, `Lookup(id) (Preset,bool)`, `GenerateSerial() (uint32,error)`, `ValidateSerial(uint32) error`, `Device{Preset,Name,Serial}`, `Apply(*filedef.Activity, Device)`.
- [x] Tests: every preset sets manufacturer/product/serial; name override sets ProductName; invalid serial rejected. Build synthetic FIT, apply, decode back.
- [x] Implement. Commit `feat(device): ...`.

### Task 3: fit
**Files:** Create `internal/fit/fix.go`, `internal/fit/fix_test.go`.
**Produces:** `Stats`, `Fix(in io.Reader, out io.Writer, dev device.Device) (Stats,error)`.
- [x] Tests: synthetic MyWhoosh-like file → averages filled, temperature stripped, device spoofed, existing averages preserved. Port reference bugs faithfully.
- [x] Implement. Commit `feat(fit): ...`.

### Task 4: mywhoosh
**Files:** Create `internal/mywhoosh/client.go`, `internal/mywhoosh/token.go`, `internal/mywhoosh/client_test.go`.
**Produces:** `Activity`, `Client`, `New(baseURL,store,*http.Client)`, `Login`, `Resume`, `ListActivities`, `DownloadFit`.
- [x] Tests via httptest: login success/fail, pagination, presigned download, token cache 0600, resume.
- [x] Implement. Commit `feat(mywhoosh): ...`.

### Task 5: garmin tokens
**Files:** Create `internal/garmin/tokens.go`, `internal/garmin/tokens_test.go`.
**Produces:** `OAuth1Token`, `OAuth2Token`, `Expired`, `RefreshExpired`, `Bearer`, `SaveTokens(dir,o1,o2)`, `LoadTokens(dir)`.
- [x] Tests: round-trip, permissions 0600, expiry logic.
- [x] Commit `feat(garmin): token store`.

### Task 6: garmin SSO + MFA
**Files:** Create `internal/garmin/sso.go`, `internal/garmin/sso_test.go`.
**Produces:** `ErrMFARequired`, `ssoSession`, `login`, `submitMFA`, `getOAuth1Token`, `exchangeOAuth2`, injectable base hosts + http client for tests.
- [x] Tests via httptest: SUCCESSFUL path; MFA_REQUIRED path returns ErrMFARequired; verifyCode path; oauth1/oauth2 exchange parse.
- [x] Implement per garth flow (mobile endpoints). Commit `feat(garmin): mobile SSO and MFA`.

### Task 7: garmin client
**Files:** Create `internal/garmin/client.go`, `internal/garmin/client_test.go`.
**Produces:** `Client`, `New(tokenDir,domain,*http.Client)`, `Login`, `NeedsMFA`, `SubmitMFA`, `Resume`, `UploadFIT`.
- [x] Tests via httptest: upload multipart headers, 409 duplicate, detailedImportResult failures, refresh on expired.
- [x] Commit `feat(garmin): client`.

### Task 8: state
**Files:** Create `internal/state/state.go`, `internal/state/state_test.go`.
**Produces:** `Open(path) (*Store,error)`, `IsUploaded`, `MarkUploaded`, `UploadedAt`.
- [x] Tests: persistence, concurrency guard. Commit `feat(state): idempotence store`.

### Task 9: app
**Files:** Create `internal/app/app.go`, `internal/app/app_test.go`.
**Produces:** `Service` wrapping mywhoosh+garmin+fit+state+device; `LoginMyWhoosh`, `LoginGarmin`, `SubmitMFA`, `Sessions`, `Upload`.
- [x] Tests with fake clients + synthetic FIT: upload marks state; duplicate marks state; failure does not.
- [x] Commit `feat(app): orchestration service`.

### Task 10: tui
**Files:** Create `internal/tui/model.go`, `internal/tui/settings.go`, `internal/tui/styles.go`, `internal/tui/model_test.go`.
**Produces:** Bubble Tea `Model` with screens (sessions, settings, MFA, confirm), help bar, spinner, resize, light/dark; non-blocking via `tea.Cmd`.
- [x] Tests: Update transitions (fetch→list, upload→state, MFA input, settings save, confirm yes/no), View contains keybindings.
- [x] Commit `feat(tui): ...`.

### Task 11: main + docs + lint
**Files:** Create `cmd/whoosh2connect/main.go`, `AGENTS.md`, `README.md`; modify `Makefile`.
- [x] Flags, config load, keyring-less file tokens, launch TUI / `--no-tui`.
- [x] Docs per mission. `go build && go vet && go test ./... && golangci-lint run`.
- [x] Commit `docs: ...`, `feat(cmd): ...`.

## Self-Review
Every mission requirement maps to a task: MyWhoosh (T4), spoofing+presets+serial (T2,T3), Garmin upload (T7), MFA (T6), idempotence (T8), TUI (T10), config precedence (T1), docs (T11), quality gates (global).
