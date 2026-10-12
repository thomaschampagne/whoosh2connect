# whoosh2connect — Design

**Date:** 2026-10-12
**Status:** Approved (derived from `MISSION.md`)

## Intent

A local Go CLI/TUI that syncs MyWhoosh cycling sessions to Garmin Connect. MyWhoosh
exports FIT files that Garmin ignores for Training Effect / Training Load / VO2max
because averages are missing, temperature is bogus, and the device is unknown.
`whoosh2connect` downloads the ride from the MyWhoosh account, fixes the FIT
(averages, temperature, device identity), and uploads it to Garmin Connect.

## Why

Garmin only computes training metrics when it recognizes a known Garmin device,
so the FIT must be spoofed to a real Garmin product. The spoofed device must be
configurable and stable (one serial per install) so Garmin treats repeated rides
as coming from one device.

## Goals

- Log in to MyWhoosh, list sessions, download the `.fit`.
- Fix FIT faithfully to the Go reference (`fitfix.go`): fill missing session
  averages, strip record temperature, spoof device on `file_id` + `device_info`.
- Configurable device presets (Edge 540, Edge 1030, plus other Edge models from
  the Garmin FIT SDK product list), with name and serial overrides.
- Garmin login with MFA (2FA) support, token persistence, refresh, idempotent
  uploads.
- A keyboard-first Bubble Tea TUI: session list with status, MFA code entry,
  settings, confirmations, help bar, spinners, resize support.
- Idiomatic, tested, lint-clean Go.

## Non-goals

- No GUI toolkit (no Fyne).
- No Strava / AppleScript / PowerShell integration.
- No auto-detection of the local MyWhoosh install directory (we fetch from the
  account API instead). The Go reference's `findMyWhooshDir` path-based approach
  is intentionally not ported; the mission requires account login + download.
- No background daemon / scheduling.

## Anti-goals

- Hardcoded device identity.
- Serial that changes between runs.
- Logging or persisting raw passwords, MFA codes, or access tokens in plaintext
  to a world-readable location.
- Silent re-upload of already-synced sessions.

## Constraints

- Go 1.24+ (toolchain 1.27 available locally).
- Binary name `whoosh2connect`. Module name `whoosh2connect`.
- Small internal packages; interfaces at the consumer side; context propagation;
  wrapped errors (`%w`); no globals; no panics; gofmt/go vet/golangci-lint clean.
- Tests: table-driven; HTTP via `net/http/httptest`; no real credentials.
- Config precedence: **flags > env > file > defaults**.
- Config file: YAML in the XDG config dir.
- Tokens: file-based, mode `0600` (per user decision).
- Never log secrets.

## Decisions made with the user

- Garmin auth flow: **mobile SSO API** (the garth flow), because the Go
  reference's web-SSO flow cannot complete MFA. This is a local Go program; the
  flow is plain HTTPS and works standalone.
- Config format: **YAML**.
- Token storage: **file only, mode 0600**.
- Scope: build everything, TDD.

## Derived behavior (from reference reading — not guessed)

### MyWhoosh (from `go-ref/mywhoosh/client.go`)
- Login: `POST https://services.mywhoosh.com/http-service/api/login`, JSON body
  `{Username, Password, Platform:"Android", Action:1001, CorrelationId, DeviceId,
  Authorization:""}`. Response `{Success, Message, AccessToken, RefreshToken,
  WhooshId}`.
- List: `POST https://service14.mywhoosh.com/v2/rider/profile/activities` with
  `{"sortDate":"DESC","page":N}`, header `Authorization: Bearer <token>`,
  paginated by `data.totalPages`, results in `data.results`.
- Download: `POST https://service14.mywhoosh.com/v2/rider/profile/download-activity-file`
  with `{"fileId":"..."}` → `{data: "<presigned S3 url>"}` → GET the URL → bytes.
- Token cache: `mywhoosh_token.json` `{access_token, whoosh_id}`.

### FIT fix (from `go-ref/fitfix.go`)
- Decode with `filedef.NewListener`, `WithBroadcastOnly`, cast to
  `*filedef.Activity`.
- Collect power/HR/cadence samples, set every record `Temperature = 0x7F`.
- For each session, fill `AvgPower`/`AvgHeartRate`/`AvgCadence` only when the
  value is invalid (`0xFFFF`/`0xFF`) or `0`, using the mean of collected samples.
- Spoof `file_id` and every `device_info`: manufacturer Garmin, product, serial.
- Encode `activity.ToFIT(nil)` as FIT protocol V2.
- Invalid sentinels: `uint8 0xFF`, `uint16 0xFFFF`, `sint8 0x7F`.

### Garmin upload (from `go-ref/garmin/client.go`)
- `POST https://connectapi.<domain>/upload-service/upload`, multipart field
  `file`, headers `Authorization: Bearer`, `User-Agent: GCM-iOS-5.19.1.2`,
  `DI-Backend: connectapi.<domain>`, `NK: NT`.
- 409 → duplicate. Parse `detailedImportResult.failures`.

### Garmin mobile SSO + MFA (from garth `sso.py`)
- Consumer creds: `GET https://thegarth.s3.amazonaws.com/oauth_consumer.json`.
- `clientId = GCM_ANDROID_DARK`, UA `com.garmin.android.apps.connectmobile`
  (SSO pages use a browser-like UA to avoid Cloudflare).
- GET `https://sso.<domain>/mobile/sso/en/sign-in?clientId=...` (cookies).
- POST `https://sso.<domain>/mobile/api/login?clientId=...&locale=en-US&service=<mobileService>`
  JSON `{username,password,rememberMe:false,captchaToken:""}`.
  - `responseStatus.type == "SUCCESSFUL"` → `serviceTicketId`.
  - `responseStatus.type == "MFA_REQUIRED"` → `customerMfaInfo.mfaLastMethodUsed`.
- MFA: POST `https://sso.<domain>/mobile/api/mfa/verifyCode?...` JSON
  `{mfaMethod, mfaVerificationCode, rememberMyBrowser:false, reconsentList:[],
  mfaSetup:false}` → `serviceTicketId`.
- GET `https://sso.<domain>/portal/sso/embed` best-effort (LB cookie).
- OAuth1: signed GET `https://connectapi.<domain>/oauth-service/oauth/preauthorized?ticket=<t>&login-url=<mobileService>&accepts-mfa-tokens=true`
  → form-encoded `oauth_token`, `oauth_token_secret`, optional `mfa_token`.
- OAuth2: signed POST `https://connectapi.<domain>/oauth-service/oauth/exchange/user/2.0`
  form `audience=GARMIN_CONNECT_MOBILE_ANDROID_DI` (+ `mfa_token` when present)
  → JSON token with `expires_in`, `refresh_token_expires_in`.
- `mobileService = https://mobile.integration.<domain>/gcm/android`.

### Device product IDs (from Garmin FIT SDK, `muktihari/fit` typedef)
The preset table uses the SDK constants verbatim, e.g. Edge 500=1036, Edge 510=1561,
Edge 520=2067, Edge 530=3121, Edge 520 Plus=3112, Edge 820=2530, Edge 830=3122,
Edge 1000=1836, Edge 1030=2713, Edge 1030 Plus=3570, Edge 1040=3843, Edge 130=2909,
Edge 130 Plus=3558, Edge 540=4061, Edge 840=4062, Edge Explore=3011,
Edge Explore 2=4169, Edge 1050=4440, Edge 550=4633, Edge 850=4634. Manufacturer
Garmin = 1.

## Architecture

```
cmd/whoosh2connect/        thin main: flags, config, launch TUI (or headless)
internal/config/           YAML + env + flags, defaults, XDG paths
internal/device/           presets, serial gen/validate, apply to FIT
internal/fit/              port of fitfix.go (Fix), Stats
internal/mywhoosh/         API client (login/list/download) + token cache
internal/garmin/           tokens (0600), mobile SSO + MFA, client (upload/refresh)
internal/state/            idempotence store (uploaded activity IDs)
internal/app/              orchestration service tying it together
internal/tui/              Bubble Tea model, views, settings, MFA screen
internal/version/          version string
```

### Key interfaces (consumer-side)

- `fit`: `func Fix(in io.Reader, out io.Writer, dev device.Device) (Stats, error)`.
- `mywhoosh`:
  ```go
  type Client struct{ ... }
  func New(baseURL string, store *Store, hc *http.Client) *Client
  func (c *Client) Login(ctx, email, password string) error
  func (c *Client) Resume(ctx) error
  func (c *Client) ListActivities(ctx) ([]Activity, error)
  func (c *Client) DownloadFit(ctx, fileID string) ([]byte, error)
  ```
  Base URL injectable so tests point at `httptest`.
- `garmin`:
  ```go
  type Client struct{ ... }
  func New(tokenDir, domain string, hc *http.Client) *Client
  func (c *Client) Login(ctx, email, password string) error   // -> ErrMFARequired
  func (c *Client) NeedsMFA() bool
  func (c *Client) SubmitMFA(ctx, code string) error
  func (c *Client) Resume(ctx) error
  func (c *Client) UploadFIT(ctx, path string) error
  ```
- `state`: `func Open(path string) (*Store, error)`, `IsUploaded(id) bool`,
  `MarkUploaded(id) error`.
- `app`: `type Service struct{...}` with `Login`, `Sessions`, `Upload`.

### Device application

`device.Device{Preset Preset, Name string, Serial uint32}`. `Apply(activity)`
sets `Manufacturer`, `Product`, `SerialNumber` on `file_id` and each
`device_info`; when `Name` is non-empty it also sets `ProductName` on both.

### Config model

```yaml
mywhoosh:
  email: ""
garmin:
  email: ""
device:
  preset: edge_540
  name: ""       # optional override
  serial: 0      # 0 => use persisted serial
```
Env: `WHOOSH2CONNECT_MYWHOOSH_EMAIL`, `WHOOSH2CONNECT_GARMIN_EMAIL`,
`WHOOSH2CONNECT_DEVICE_PRESET`, `WHOOSH2CONNECT_DEVICE_NAME`,
`WHOOSH2CONNECT_DEVICE_SERIAL`, `WHOOSH2CONNECT_CONFIG`.
Flags: `--config`, `--mywhoosh-email`, `--garmin-email`, `--device-preset`,
`--device-name`, `--device-serial`, `--no-tui` (headless sync).

Serial persistence: on first `Save`, if `device.serial == 0`, generate a random
non-zero uint32 and persist it; thereafter it is stable.

## Error handling

- All public functions take `context.Context` where I/O is involved.
- Errors wrapped with `%w`; sentinel `ErrMFARequired` for the two-phase flow.
- Upload treats HTTP 409 and `duplicate` as success-already-present.

## Security

- Passwords and MFA codes are only held in memory, never logged.
- Token files created with `os.OpenFile(..., 0600)`.
- No secrets in error messages or logs.

## Testing seams

- `mywhoosh` and `garmin` HTTP against `httptest` servers (base URL / http.Client
  injected).
- `fit` and `device` against synthetic FIT files built with `filedef` (as the
  reference test does).
- `config` against temp dirs and env via `t.Setenv`.
- `state` against temp files.
- `tui` model Update/View tested as pure functions (no real terminal).

## What is left to the builder

- Exact TUI visual styling (theme, colors) within the mission's UX requirements.
- Choice of random serial source (crypto/rand).
- Minor helper decomposition.
