# whoosh2connect

Sync your MyWhoosh indoor cycling rides to Garmin Connect — with working training
metrics.

`whoosh2connect` logs into your MyWhoosh account, downloads ride FIT files, fixes
the things that make Garmin Connect ignore them for Training Effect / Training
Load / VO2max, spoofs a real Garmin Edge device identity, and uploads the result
to Garmin Connect. It is a local, single-binary Go CLI/TUI. Nothing is sent
anywhere except to MyWhoosh, Garmin, and the two public endpoints those services
require.

## Why it exists

Garmin Connect only computes training metrics when a FIT file comes from a
recognised Garmin device, with sane session averages and no bogus temperature
data. MyWhoosh exports are missing session averages, carry a fake temperature,
and identify as an unknown device — so Garmin either ignores them or shows an
activity stripped of metrics. This tool fixes all three before uploading.

## Install

### From releases

Download the archive for your system from the
[Releases](https://github.com/thomaschampagne/whoosh2connect/releases) page:

- Windows: `whoosh2connect-windows-amd64.zip`, or the bare `whoosh2connect.exe`
- Linux: `whoosh2connect-linux-amd64.tar.gz`
- macOS: `whoosh2connect-darwin-arm64.tar.gz` (Apple Silicon) or
  `whoosh2connect-darwin-amd64.tar.gz` (Intel)

Check the download against `checksums.txt`. Run `whoosh2connect --version` to see
the release version; the TUI header shows it too.

### From Scoop (Windows)

This repository is its own Scoop bucket:

```
scoop bucket add whoosh2connect https://github.com/thomaschampagne/whoosh2connect
scoop install whoosh2connect/whoosh2connect
```

The manifest in `bucket/` is regenerated on every release.

### From source

Requires Go 1.27 (see `go.mod`).

```sh
just build          # builds bin/whoosh2connect with version/commit stamped in
# or
go install ./cmd/whoosh2connect
```
## Usage

Run the TUI (default):

```sh
whoosh2connect
```

Or run a one-shot headless sync:

```sh
whoosh2connect --no-tui \
  --mywhoosh-email you@example.com \
  --garmin-email you@example.com
```

### Dry-run and verify

Preview what a headless sync would do, without downloading or uploading:

```sh
whoosh2connect --no-tui --dry-run --mywhoosh-email you@example.com --garmin-email you@example.com
```

Each ride prints `would upload <id>` or `would skip <id> (reason)`. Reasons are
`ledger hit: activity id` (already in the ledger) or `garmin match: start time`
(Garmin already has a ride starting within 2 minutes). A ride that passes this
preview can still be skipped during a real run when its FIT content hash is
already in the ledger.

Check the ledger against Garmin Connect, uploading nothing:

```sh
whoosh2connect --verify --garmin-email you@example.com
```

Each uploaded ride that Garmin does not show prints `mismatch <id>`, and the
command exits with status 1. Rides recorded before start times were stored
print `no start time recorded`.

### Flags

| Flag                       | Meaning                                           |
| -------------------------- | ------------------------------------------------- |
| `--config <path>`          | Path to the config YAML (default: XDG config dir) |
| `--mywhoosh-email <email>` | MyWhoosh account email                            |
| `--garmin-email <email>`   | Garmin Connect email                              |
| `--device-preset <id>`     | Device to spoof (e.g. `edge_540`, `edge_1030`)    |
| `--device-name <name>`     | Override the displayed device name                |
| `--device-serial <n>`      | Override the device serial (non-zero uint32)      |
| `--no-tui`                 | Run headless and exit                             |
| `--dry-run`                | Headless: print what would upload or skip         |
| `--verify`                 | Check uploaded rides against Garmin, upload nothing |
| `--version`                | Print version and exit                            |

Passwords are never taken from the config file or flags. In the TUI you enter
them on the settings screen (held in memory only). Headless mode reads them from
the environment:

```sh
export WHOOSH2CONNECT_MYWHOOSH_PASSWORD=...
export WHOOSH2CONNECT_GARMIN_PASSWORD=...
```

## Configuration

Config is resolved with the precedence **flags > environment > file > defaults**.
The file is YAML in the XDG config directory
(`$XDG_CONFIG_HOME/whoosh2connect/config.yaml`, falling back to
`~/.config/whoosh2connect/config.yaml`):

```yaml
mywhoosh:
  email: you@example.com
garmin:
  email: you@example.com
device:
  preset: edge_540
  name: "" # optional override
  serial: 0 # 0 means "generate once and persist"
```

Environment variables:

| Variable                           | Purpose                      |
| ---------------------------------- | ---------------------------- |
| `WHOOSH2CONNECT_CONFIG`            | Config file path             |
| `WHOOSH2CONNECT_MYWHOOSH_EMAIL`    | MyWhoosh email               |
| `WHOOSH2CONNECT_GARMIN_EMAIL`      | Garmin email                 |
| `WHOOSH2CONNECT_MYWHOOSH_PASSWORD` | MyWhoosh password (headless) |
| `WHOOSH2CONNECT_GARMIN_PASSWORD`   | Garmin password (headless)   |
| `WHOOSH2CONNECT_DEVICE_PRESET`     | Device preset ID             |
| `WHOOSH2CONNECT_DEVICE_NAME`       | Device name override         |
| `WHOOSH2CONNECT_DEVICE_SERIAL`     | Device serial override       |

## Device presets

Product IDs are taken verbatim from the Garmin FIT SDK. Pick any of:

`edge_20`, `edge_25`, `edge_130`, `edge_130_plus`, `edge_500`, `edge_510`,
`edge_520`, `edge_520_plus`, `edge_530`, `edge_540`, `edge_550`, `edge_820`,
`edge_830`, `edge_840`, `edge_850`, `edge_1000`, `edge_1030`, `edge_1030_plus`,
`edge_1040`, `edge_1050`, `edge_explore`, `edge_explore_2`.

The default is `edge_540`. You can override the displayed name with
`--device-name` / `device.name`, and the serial with `--device-serial` /
`device.serial`.

**The serial is generated once, persisted to the config file, and kept stable.**
Garmin treats uploads as coming from one consistent device only if the serial
does not change between rides.

> ⚠️ The device you spoof affects how Garmin Connect classifies the activity and
> which metrics it computes (for example, power-based metrics require a device
> that supports them). Choose a device that matches the data your ride contains.

## Two-factor authentication (2FA)

Garmin login uses the same mobile SSO flow as the Garmin Connect app, so 2FA is
supported:

1. `whoosh2connect` logs in with your email and password.
2. If Garmin requires a code, the TUI shows a verification screen.
3. Enter the code Garmin sent you; the code is submitted and never stored.
4. OAuth tokens are saved to disk (mode `0600`) and refreshed automatically, so
   subsequent runs skip both password and code until they expire.

## Files and security

| Path                                         | Contents                                            |
| -------------------------------------------- | --------------------------------------------------- |
| `~/.config/whoosh2connect/config.yaml`       | Non-secret config (emails, device)                  |
| `~/.local/share/whoosh2connect/`             | Data directory                                      |
| `…/oauth1_token.json`, `…/oauth2_token.json` | Garmin tokens (mode `0600`)                         |
| `…/mywhoosh_token.json`                      | MyWhoosh token (mode `0600`)                        |
| `…/state.json`                               | Sync ledger: ride IDs, FIT hashes, statuses (mode `0600`) |
| `…/state.json.lock`                          | Lock file serialising ledger writes across processes |
| `whoosh2connect.log` (next to the executable)  | Structured log, overwritten on each start, secrets redacted (mode `0600`) |
| `spoofed/<id>.fit` (next to the executable)   | Spoofed FIT, saved only when you press `w` on a ride (mode `0600`). Open the folder with `o`. |

- Passwords are never written to disk, never passed as flags, and never logged.
- MFA codes are used once and discarded.
- Token files are created with mode `0600`.
- Uploads are idempotent: each ride is checked against the local ledger (by MyWhoosh
  ID and by FIT hash), then against Garmin Connect (start time within 2 minutes).
  A ride found by any of these is not uploaded again. Garmin rejects duplicates
  as a further guard.

## Disclaimer

Garmin Connect and MyWhoosh are unofficial, reverse-engineered integrations that
may change or break without notice. This project is not affiliated with Garmin or
MyWhoosh. Spoofing device identity and uploading data is your responsibility and
may conflict with the services' terms of service. Use at your own risk.

## Development

```sh
just test     # go test -race ./...
just vet      # go vet ./...
just lint     # golangci-lint run
just fmt      # gofmt -s -w .
just ci       # check-only: fmt-check, vet, test, build (what CI runs)
just release 1.2.3   # build all platforms into dist/ (normally run by semantic-release)
go test -race ./...   # race detector, before calling work done
```

See `AGENTS.md` for the project layout and contribution rules.
