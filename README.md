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

Requires Go 1.24+.

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

### Flags

| Flag | Meaning |
| --- | --- |
| `--config <path>` | Path to the config YAML (default: XDG config dir) |
| `--mywhoosh-email <email>` | MyWhoosh account email |
| `--garmin-email <email>` | Garmin Connect email |
| `--device-preset <id>` | Device to spoof (e.g. `edge_540`, `edge_1030`) |
| `--device-name <name>` | Override the displayed device name |
| `--device-serial <n>` | Override the device serial (non-zero uint32) |
| `--no-tui` | Run headless and exit |
| `--version` | Print version and exit |

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
  name: ""          # optional override
  serial: 0         # 0 means "generate once and persist"
```

Environment variables:

| Variable | Purpose |
| --- | --- |
| `WHOOSH2CONNECT_CONFIG` | Config file path |
| `WHOOSH2CONNECT_MYWHOOSH_EMAIL` | MyWhoosh email |
| `WHOOSH2CONNECT_GARMIN_EMAIL` | Garmin email |
| `WHOOSH2CONNECT_MYWHOOSH_PASSWORD` | MyWhoosh password (headless) |
| `WHOOSH2CONNECT_GARMIN_PASSWORD` | Garmin password (headless) |
| `WHOOSH2CONNECT_DEVICE_PRESET` | Device preset ID |
| `WHOOSH2CONNECT_DEVICE_NAME` | Device name override |
| `WHOOSH2CONNECT_DEVICE_SERIAL` | Device serial override |

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

| Path | Contents |
| --- | --- |
| `~/.config/whoosh2connect/config.yaml` | Non-secret config (emails, device) |
| `~/.local/share/whoosh2connect/` | Data directory |
| `…/oauth1_token.json`, `…/oauth2_token.json` | Garmin tokens (mode `0600`) |
| `…/mywhoosh_token.json` | MyWhoosh token (mode `0600`) |
| `…/state.json` | Uploaded activity IDs for idempotence (mode `0600`) |

- Passwords are never written to disk, never passed as flags, and never logged.
- MFA codes are used once and discarded.
- Token files are created with mode `0600`.
- Uploads are idempotent: an activity already uploaded (locally recorded or
  rejected by Garmin as a duplicate) is not uploaded again.

## Disclaimer

Garmin Connect and MyWhoosh are unofficial, reverse-engineered integrations that
may change or break without notice. This project is not affiliated with Garmin or
MyWhoosh. Spoofing device identity and uploading data is your responsibility and
may conflict with the services' terms of service. Use at your own risk.

## Development

```sh
just test     # go test ./...
just vet      # go vet ./...
just lint     # golangci-lint run
just fmt      # gofmt -s -w .
```

See `AGENTS.md` for the project layout and contribution rules.
