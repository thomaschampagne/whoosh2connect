# whoosh2connect

[![Release](https://img.shields.io/github/v/release/thomaschampagne/whoosh2connect?sort=semver)](https://github.com/thomaschampagne/whoosh2connect/releases)
[![CI](https://github.com/thomaschampagne/whoosh2connect/actions/workflows/ci.yml/badge.svg)](https://github.com/thomaschampagne/whoosh2connect/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Your MyWhoosh rides, on Garmin Connect — with the metrics that make them count.**

`whoosh2connect` takes the rides you already finished on MyWhoosh, repairs the
FIT file Garmin Connect would otherwise ignore, and uploads it as a real Garmin
Edge. Training Effect, Training Load and VO2max show up the way they do after an
outdoor ride: no export, no manual upload, no lost training history.

One binary. Runs on your machine. Nothing leaves it except the requests needed
to log in and sync.

## Install

**Windows (Scoop)**

```
scoop bucket add whoosh2connect https://github.com/thomaschampagne/whoosh2connect
scoop install whoosh2connect/whoosh2connect
```

**Any OS (direct download)**

Grab the archive for your system from
[Releases](https://github.com/thomaschampagne/whoosh2connect/releases):

| Platform            | Asset                                                                |
| ------------------- | -------------------------------------------------------------------- |
| Windows             | `whoosh2connect-windows-amd64.zip`, or the bare `whoosh2connect.exe` |
| Linux x86-64        | `whoosh2connect-linux-amd64.tar.gz`                                  |
| macOS Apple Silicon | `whoosh2connect-darwin-arm64.tar.gz`                                 |
| macOS Intel         | `whoosh2connect-darwin-amd64.tar.gz`                                 |

Check the download against `checksums.txt`, then confirm the version:

```
whoosh2connect --version
```

From source needs Go 1.27.2 (see `go.mod`): `just build` writes
`dist/whoosh2connect` with the version and commit stamped in, or use
`go install ./cmd/whoosh2connect`.

## Quickstart

1. Run `whoosh2connect`.
2. Press `s`, fill in your MyWhoosh and Garmin emails and passwords, then
   `ctrl+s`. Garmin two-factor codes are asked for in the app.
3. Press `a` to sync every new ride, or `u` on a single ride.

Headless, for a script or a scheduler:

```
export WHOOSH2CONNECT_MYWHOOSH_PASSWORD=...
export WHOOSH2CONNECT_GARMIN_PASSWORD=...
whoosh2connect --no-tui --mywhoosh-email you@example.com --garmin-email you@example.com
```

## What Garmin was missing

Garmin only computes training metrics for a FIT file that comes from a
recognised Garmin device, with sane session averages and no bogus sensor data.
A MyWhoosh export fails all three: no session averages, a fake temperature, an
unknown device. `whoosh2connect` fixes the file before it is uploaded.

| Before                       | After                                              |
| ---------------------------- | -------------------------------------------------- |
| Unknown device, no metrics   | Uploads as the Edge you pick (Edge 540 by default) |
| No Training Effect           | Training Effect computed                           |
| No Training Load             | Training Load counted                              |
| VO2max untouched             | VO2max updated                                     |
| Fake temperature in the file | Temperature stripped                               |

The device is not hardcoded. 32 Edge presets ship with the binary, and the
serial is generated once, persisted, and kept stable so Garmin sees one
consistent device ride after ride.

## The TUI

```
╭────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│ whoosh2connect  dev                                                                               device: edge_540 │
│ ────────────────────────────────────────────────────────────────────────────────────────────────────────────────── │
│   ✓ 1 synced   ○ 2 pending                                                                                         │
│                                                                                                                    │
│   ➤ 28 Oct 17:42 Cog 8 · Watopia                                                                                ○  │
│     26 Oct 07:15 Tempo · Beach Loop                                                                             ✓  │
│     24 Oct 18:05 Sprint · Coastal Run                                                                           ○  │
│   3 ride(s) · 1 synced · 2 pending                                                                                 │
│                                                                                                                    │
│                                                                                                                    │
│                                                                                                                    │
│                                                                                                                    │
│   last: synced Tempo · Beach Loop                                                                                  │
│   ? help · q quit · ↑/↓ move · enter preview · u upload · a upload-all · t auto-watch · w save FIT · o spoof dir … │
╰────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

| Key                    | Action                                                   |
| ---------------------- | -------------------------------------------------------- |
| `↑` / `↓` or `k` / `j` | Move through the ride list                               |
| `enter`                | Preview a ride                                           |
| `u`                    | Upload the selected ride                                 |
| `a`                    | Upload every new ride                                    |
| `t`                    | Toggle auto-watch                                        |
| `r`                    | Refresh the MyWhoosh ride list                           |
| `s`                    | Settings: emails, passwords, device preset, name, serial |
| `l`                    | Log panel                                                |
| `m`                    | Toggle mouse capture (off lets you select and copy text) |
| `w`                    | Save the spoofed FIT file next to the binary             |
| `o`                    | Open the spoofed FIT folder                              |
| `?`                    | Help                                                     |
| `q` or `ctrl+c`        | Quit                                                     |

## Auto-watch

Press `t` and the app scans your MyWhoosh ride folder every 10 seconds. When a
file stops changing, it is fixed, spoofed, uploaded, and announced with a system
notification. A failed upload is retried on a later scan. Press `t` again to
stop. The setting is saved and watching restarts with the TUI.

The same loop runs headless:

```
whoosh2connect --watch auto --garmin-email you@example.com
```

`--watch auto` discovers the folder on Windows and macOS; on Linux, pass the
directory explicitly. Use `--watch-interval` and `--watch-stable` to tune the
scan. Watching runs only while `whoosh2connect` is open: there is no background
service.

## Why you can trust it

- **Local by design.** It talks to MyWhoosh and Garmin, nothing else. No
  telemetry, no account of ours, no cloud relay.
- **Passwords stay off disk.** The TUI keeps them in memory; headless mode reads
  them from the environment. They are never written to the config, never passed
  as flags, never logged.
- **Tokens are files with mode `0600`**, refreshed automatically, so 2FA is a
  once-in-a-while thing, not every sync.
- **Uploads are idempotent.** Each ride is checked against the local ledger (by
  MyWhoosh ID and FIT hash) and against Garmin (start time within 2 minutes)
  before anything is sent. Garmin rejecting duplicates is the last guard.
- **You can check before you commit.** `--dry-run` prints what would upload and
  why, `--verify` compares the ledger with Garmin and changes nothing.

## Reference

### Flags

| Flag                       | Meaning                                                           |
| -------------------------- | ----------------------------------------------------------------- |
| `--config <path>`          | Path to the config YAML (default: XDG config dir)                 |
| `--mywhoosh-email <email>` | MyWhoosh account email                                            |
| `--garmin-email <email>`   | Garmin Connect email                                              |
| `--device-preset <id>`     | Device to spoof (e.g. `edge_540`, `edge_1030`)                    |
| `--device-name <name>`     | Override the displayed device name                                |
| `--device-serial <n>`      | Override the device serial (non-zero uint32)                      |
| `--watch <dir\|auto>`      | Watch a local MyWhoosh folder and auto-upload                     |
| `--watch-interval <s>`     | Seconds between scans in watch mode (default 10)                  |
| `--watch-stable <n>`       | Consecutive stable scans before a ride counts as done (default 2) |
| `--no-tui`                 | Run headless and exit                                             |
| `--dry-run`                | Headless: print what would upload or skip                         |
| `--verify`                 | Check uploaded rides against Garmin, upload nothing               |
| `--version`                | Print version and exit                                            |

### Configuration

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
watch:
  enabled: false # set by the TUI auto-watch toggle
  dir: "" # empty means the platform default MyWhoosh folder
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

### Device presets

Product IDs are taken verbatim from the Garmin FIT SDK. Pick any of:

`edge_1000`, `edge_1000_thai`, `edge_1030`, `edge_1030_plus`, `edge_1040`,
`edge_1050`, `edge_130`, `edge_130_plus`, `edge_20`, `edge_200`, `edge_25`,
`edge_25_jpn`, `edge_500`, `edge_510`, `edge_520`, `edge_520_plus`, `edge_530`,
`edge_540`, `edge_550`, `edge_800`, `edge_810`, `edge_820`, `edge_830`,
`edge_840`, `edge_850`, `edge_explore`, `edge_explore_1000`, `edge_explore_2`,
`edge_explore_820`, `edge_mtb`, `edge_remote`, `edge_touring`.

The default is `edge_540`. You can override the displayed name with
`--device-name` / `device.name`, and the serial with `--device-serial` /
`device.serial`.

**The serial is generated once, persisted to the config file, and kept stable.**
Garmin treats uploads as coming from one consistent device only if the serial
does not change between rides.

> ⚠️ The device you spoof affects how Garmin Connect classifies the activity and
> which metrics it computes (for example, power-based metrics require a device
> that supports them). Choose a device that matches the data your ride contains.

### Dry-run and verify

Preview what a headless sync would do, without downloading or uploading:

```
whoosh2connect --no-tui --dry-run --mywhoosh-email you@example.com --garmin-email you@example.com
```

Each ride prints `would upload <id>` or `would skip <id> (reason)`. Reasons are
`ledger hit: activity id` (already in the ledger) or `garmin match: start time`
(Garmin already has a ride starting within 2 minutes). A ride that passes this
preview can still be skipped during a real run when its FIT content hash is
already in the ledger.

Check the ledger against Garmin Connect, uploading nothing:

```
whoosh2connect --verify --garmin-email you@example.com
```

Each uploaded ride that Garmin does not show prints `mismatch <id>`, and the
command exits with status 1. Rides recorded before start times were stored
print `no start time recorded`.

### Two-factor authentication (2FA)

Garmin login uses the same mobile SSO flow as the Garmin Connect app, so 2FA is
supported:

1. `whoosh2connect` logs in with your email and password.
2. If Garmin requires a code, the TUI shows a verification screen.
3. Enter the code Garmin sent you; the code is submitted and never stored.
4. OAuth tokens are saved to disk (mode `0600`) and refreshed automatically, so
   subsequent runs skip both password and code until they expire.

### Files and security

| Path                                                   | Contents                                                                       |
| ------------------------------------------------------ | ------------------------------------------------------------------------------ |
| `~/.config/whoosh2connect/config.yaml`                 | Non-secret config (emails, device, watch)                                      |
| Linux: `~/.local/share/whoosh2connect/`                | Data directory                                                                 |
| macOS: `~/Library/Application Support/whoosh2connect/` | Data directory                                                                 |
| Windows: `%LOCALAPPDATA%\whoosh2connect\`              | Data directory                                                                 |
| `…/oauth1_token.json`, `…/oauth2_token.json`           | Garmin tokens (mode `0600`)                                                    |
| `…/mywhoosh_token.json`                                | MyWhoosh token (mode `0600`)                                                   |
| `…/state.json`                                         | Sync ledger: ride IDs, FIT hashes, statuses (mode `0600`)                      |
| `…/state.json.lock`                                    | Lock file serialising ledger writes across processes                           |
| `whoosh2connect.log` (next to the executable)          | Structured log, overwritten on each start, secrets redacted (mode `0600`)      |
| `spoofed/<id>.fit` (next to the executable)            | Spoofed FIT, saved only when you press `w` (mode `0600`); `o` opens the folder |

- Passwords are never written to disk, never passed as flags, and never logged.
- MFA codes are used once and discarded.
- Token files are created with mode `0600`.
- `whoosh2connect` sends data only to MyWhoosh, to Garmin, and to the two public
  endpoints those logins require: the Garmin OAuth consumer JSON on S3 and the
  Garmin mobile SSO host.

## Troubleshooting

- **The ride uploads but Garmin shows no metrics.** Check the device preset: a
  preset without power support will not unlock power-based metrics. Pick the
  Edge model closest to the data your ride contains.
- **A ride is skipped as already synced.** The ledger matched it by MyWhoosh ID,
  FIT hash, or Garmin start time. Run `--dry-run` to see which.
- **Something failed.** The log sits next to the binary as
  `whoosh2connect.log`; secrets are redacted, so it is safe to share.
- **Garmin asked for a code.** Enter it on the verification screen. It is used
  once and never stored.

## Disclaimer

Garmin Connect and MyWhoosh are unofficial, reverse-engineered integrations that
may change or break without notice. This project is not affiliated with Garmin or
MyWhoosh. Spoofing device identity and uploading data is your responsibility and
may conflict with the services' terms of service. Use at your own risk.

## Development

```sh
just test     # go test -race ./...
just vet      # go vet ./... and GOOS=windows go vet ./...
just lint     # golangci-lint run
just fmt      # gofmt -s -w .
just ci       # check-only: fmt-check, vet, test, build (what CI runs)
just release 1.2.3   # build all platforms into dist/ (normally run by semantic-release)
go test -race ./...   # race detector, before calling work done
```

See `AGENTS.md` for the project layout and contribution rules.

Released under the [MIT licence](LICENSE).
