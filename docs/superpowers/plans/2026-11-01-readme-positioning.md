# README Positioning and Documentation Accuracy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn `README.md` into a product page that converts a visitor in fifteen seconds, and make `README.md` and `AGENTS.md` match the shipped code.

**Architecture:** Docs only. Capture one real TUI frame from the existing Bubble Tea model with a throwaway test, rewrite the README around it (hero → install → proof → reference), then correct `AGENTS.md`. No production code changes.

**Tech Stack:** Go 1.27.2, Bubble Tea model in `internal/tui`, Markdown, `just` recipes.

**Spec:** `docs/superpowers/specs/2026-11-01-readme-positioning-design.md`

## Global Constraints

- English, direct, second person, short sentences. No emoji headers.
- Every claim must be backed by code or by the mission's definition of done.
- Forbidden words: "guaranteed", "undetectable", "100% safe", "official", invented user numbers.
- No binary assets committed. The TUI preview is text, captured from the real model.
- 32 device presets, generated from `internal/device/presets.go` (the spec said 33; the real count is 32 — use 32).
- Go floor: `1.27.2`.
- Single `README.md`; technical reference stays inside it, below the pitch.
- Disclaimer kept, before Development.

---

### Task 1: Capture a real TUI frame

**Files:**
- Create (temporary): `internal/tui/zz_preview_tmp_test.go`
- Output: `/tmp/opencode/tui-preview.txt`

**Interfaces:**
- Consumes: `tui.New(Options) Model`, `Model.View() string`, in-package test helpers `fakeService`, `session(id string)`, `activity(id string)`, and unexported fields `m.screen`, `m.width`, `m.height`, `m.sessions`.
- Produces: a plain-text frame in `/tmp/opencode/tui-preview.txt` that Task 2 pastes into the README.

- [ ] **Step 1: Write the throwaway test**

```go
package tui

import (
	"os"
	"testing"

	"whoosh2connect/internal/app"
)

func TestPreviewFrameTmp(t *testing.T) {
	f := &fakeService{sessions: []app.Session{
		session("1"),
		{Activity: activity("2"), Uploaded: true},
		session("3"),
	}}
	m := New(Options{Service: f, Settings: Settings{
		MywhooshEmail: "you@example.com",
		GarminEmail:   "you@example.com",
		DevicePreset:  "edge_540",
	}})
	m.width, m.height = 100, 28
	m.screen = screenList
	m.sessions = f.sessions
	m.results["1"] = "new"
	m.results["2"] = "uploaded"

	frame := m.View()
	if err := os.WriteFile("/tmp/opencode/tui-preview.txt", []byte(frame), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("\n%s", frame)
}
```

- [ ] **Step 2: Run it and confirm the frame renders**

Run: `go test ./internal/tui -run TestPreviewFrameTmp -v`
Expected: `PASS`, and the log shows a bordered frame containing `whoosh2connect`, `Ride 1`, `edge_540`, `upload all`.

- [ ] **Step 3: Read the captured frame**

Run: `cat /tmp/opencode/tui-preview.txt`
Expected: about 28 lines, ANSI colour codes stripped or absent in the test terminal.

- [ ] **Step 4: Delete the throwaway test and confirm a clean tree**

Run: `rm internal/tui/zz_preview_tmp_test.go && git status --short`
Expected: only `docs/superpowers/specs/2026-11-01-readme-positioning-design.md` (and this plan) untracked. No `.go` change.

No commit in this task: it produces an artifact outside the repo.

---

### Task 2: Rewrite README.md

**Files:**
- Modify: `README.md` (full rewrite)

**Interfaces:**
- Consumes: the frame from Task 1.
- Produces: `README.md` with the structure the spec fixes.

- [ ] **Step 1: Write sections 1-8 (pitch half)**

Paste this content verbatim, inserting the Task 1 frame where marked.

```markdown
# whoosh2connect

[![Release](https://img.shields.io/github/v/release/thomaschampagne/whoosh2connect?sort=semver)](https://github.com/thomaschampagne/whoosh2connect/releases)
[![CI](https://github.com/thomaschampagne/whoosh2connect/actions/workflows/ci.yml/badge.svg)](https://github.com/thomaschampagne/whoosh2connect/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Your MyWhoosh rides, on Garmin Connect — with the metrics that make them count.**

`whoosh2connect` takes the rides you already finished on MyWhoosh, repairs the
FIT file Garmin Connect would otherwise ignore, and uploads it as a real Garmin
Edge. Training Effect, Training Load and VO2max show up the way they do after an
outdoor ride: no export, no manual upload, no lost training history.

One binary. Runs on your machine. Nothing leaves it except the requests to
MyWhoosh and Garmin.

## Install

**Windows (Scoop)**

```
scoop bucket add whoosh2connect https://github.com/thomaschampagne/whoosh2connect
scoop install whoosh2connect/whoosh2connect
```

**Any OS (direct download)**

Grab the archive for your system from
[Releases](https://github.com/thomaschampagne/whoosh2connect/releases):

| Platform | Asset |
| --- | --- |
| Windows | `whoosh2connect-windows-amd64.zip`, or the bare `whoosh2connect.exe` |
| Linux x86-64 | `whoosh2connect-linux-amd64.tar.gz` |
| macOS Apple Silicon | `whoosh2connect-darwin-arm64.tar.gz` |
| macOS Intel | `whoosh2connect-darwin-amd64.tar.gz` |

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

| Before | After |
| --- | --- |
| Unknown device, no metrics | Uploads as the Edge you pick (Edge 540 by default) |
| No Training Effect | Training Effect computed |
| No Training Load | Training Load counted |
| VO2max untouched | VO2max updated |
| Fake temperature in the file | Temperature stripped |

The device is not hardcoded. 32 Edge presets ship with the binary, and the
serial is generated once, persisted, and kept stable so Garmin sees one
consistent device ride after ride.

## The TUI

<FRAME>

| Key | Action |
| --- | --- |
| `↑` / `↓` or `k` / `j` | Move through the ride list |
| `enter` | Preview a ride |
| `u` | Upload the selected ride |
| `a` | Upload every new ride |
| `t` | Toggle auto-watch |
| `r` | Refresh the MyWhoosh ride list |
| `s` | Settings: emails, passwords, device preset, name, serial |
| `l` | Log panel |
| `m` | Toggle mouse capture (off lets you select and copy text) |
| `w` | Save the spoofed FIT file next to the binary |
| `o` | Open the spoofed FIT folder |
| `?` | Help |
| `q` or `ctrl+c` | Quit |

## Auto-watch

Press `t` and the app scans your MyWhoosh ride folder every 10 seconds. When a
file stops changing, it is fixed, spoofed, uploaded, and announced with a system
notification. A failed upload is retried on a later scan. Press `t` again to
stop. The setting is saved and watching restarts with the TUI.

The same loop runs headless:

```
whoosh2connect --watch auto --garmin-email you@example.com
```

`--watch auto` discovers the platform folder; pass a path instead of `auto` to
watch somewhere else, and use `--watch-interval` / `--watch-stable` to tune the
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
```

- [ ] **Step 2: Write the reference half (section 9)**

Keep the existing tables and prose from the current README, with these exact
edits:

1. Replace the "Flags" table body with:

```markdown
| Flag | Meaning |
| --- | --- |
| `--config <path>` | Path to the config YAML (default: XDG config dir) |
| `--mywhoosh-email <email>` | MyWhoosh account email |
| `--garmin-email <email>` | Garmin Connect email |
| `--device-preset <id>` | Device to spoof (e.g. `edge_540`, `edge_1030`) |
| `--device-name <name>` | Override the displayed device name |
| `--device-serial <n>` | Override the device serial (non-zero uint32) |
| `--watch <dir\|auto>` | Watch a local MyWhoosh folder and auto-upload |
| `--watch-interval <s>` | Seconds between scans in watch mode (default 10) |
| `--watch-stable <n>` | Consecutive stable scans before a ride counts as done (default 2) |
| `--no-tui` | Run headless and exit |
| `--dry-run` | Headless: print what would upload or skip |
| `--verify` | Check uploaded rides against Garmin, upload nothing |
| `--version` | Print version and exit |
```

2. Replace the device preset list with the generated list, exactly:

```markdown
`edge_1000`, `edge_1000_thai`, `edge_1030`, `edge_1030_plus`, `edge_1040`,
`edge_1050`, `edge_130`, `edge_130_plus`, `edge_20`, `edge_200`, `edge_25`,
`edge_25_jpn`, `edge_500`, `edge_510`, `edge_520`, `edge_520_plus`, `edge_530`,
`edge_540`, `edge_550`, `edge_800`, `edge_810`, `edge_820`, `edge_830`,
`edge_840`, `edge_850`, `edge_explore`, `edge_explore_1000`, `edge_explore_2`,
`edge_explore_820`, `edge_mtb`, `edge_remote`, `edge_touring`.
```

3. Replace the files table with:

```markdown
| Path | Contents |
| --- | --- |
| `~/.config/whoosh2connect/config.yaml` | Non-secret config (emails, device, watch) |
| Linux: `~/.local/share/whoosh2connect/` | Data directory |
| macOS: `~/Library/Application Support/whoosh2connect/` | Data directory |
| Windows: `%LOCALAPPDATA%\whoosh2connect\` | Data directory |
| `…/oauth1_token.json`, `…/oauth2_token.json` | Garmin tokens (mode `0600`) |
| `…/mywhoosh_token.json` | MyWhoosh token (mode `0600`) |
| `…/state.json` | Sync ledger: ride IDs, FIT hashes, statuses (mode `0600`) |
| `…/state.json.lock` | Lock file serialising ledger writes across processes |
| `whoosh2connect.log` (next to the executable) | Structured log, overwritten on each start, secrets redacted (mode `0600`) |
| `spoofed/<id>.fit` (next to the executable) | Spoofed FIT, saved only when you press `w` (mode `0600`); `o` opens the folder |
```

4. Fix the security claim sentence to name the real endpoints:

```markdown
`whoosh2connect` sends data only to MyWhoosh, to Garmin, and to the two public
endpoints those logins require: the Garmin OAuth consumer JSON on S3 and the
Garmin mobile SSO host.
```

5. Add a short troubleshooting subsection before Development:

```markdown
## Troubleshooting

- **The ride uploads but Garmin shows no metrics.** Check the device preset: a
  preset without power support will not unlock power-based metrics. Pick the
  Edge model closest to the data your ride contains.
- **A ride is skipped as already synced.** The ledger matched it by MyWhoosh ID,
  FIT hash, or Garmin start time. Run `--dry-run` to see which.
- **Something failed.** The log sits next to the binary as
  `whoosh2connect.log`; secrets are redacted, so it is safe to share.
- **Garmin asked for a code.** Enter the code on the verification screen. It is
  used once and never stored.
```

6. Keep `## Disclaimer` before `## Development`, with the existing text, and add
   the MIT licence line to Development:

```markdown
Released under the [MIT licence](LICENSE).
```

- [ ] **Step 3: Check for forbidden claims and stale values**

Run: `grep -nEi 'guarantee|undetectable|100% safe|official|Go 1\.27[^.]' README.md`
Expected: no match.

- [ ] **Step 4: Check every flag is documented**

Run: `grep -o '\-\-[a-z-]*' cmd/whoosh2connect/main.go | sort -u`
Expected: every flag in the output appears in the README flags table.

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs(readme): lead with the promise, fix stale reference"
```

---

### Task 3: Correct AGENTS.md

**Files:**
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: nothing from Task 2.
- Produces: accurate layout, commands and verification block.

- [ ] **Step 1: Replace the layout block**

```markdown
```
cmd/whoosh2connect/   thin main: flags, wiring, headless mode, watch loop, TUI launch
internal/config/      YAML + env + flags (precedence: flags > env > file > defaults), XDG paths
internal/device/      Edge presets (product IDs from the Garmin FIT SDK), serial gen/validate, apply to FIT
internal/fit/         port of fitfix.go: fill session averages, strip temperature, spoof device
internal/mywhoosh/    MyWhoosh API client (login, list, download, page cap) + token cache
internal/garmin/      token store (0600), mobile SSO + MFA, upload client, activity list for remote match
internal/state/       sync ledger (JSON, cross-process lock, atomic writes)
internal/fsutil/      atomic 0600 file writes
internal/httpx/       bounded response reads, short error snippets
internal/logx/        slog file logger with secret redaction
internal/watch/       local FIT directory watcher (hash cache, stable scans)
internal/notify/      system notifications behind a small interface
internal/app/         upload pipeline shared by API and watch paths; ledger guards; verify
internal/tui/         Bubble Tea model, views, settings, MFA screen
internal/release/     Scoop manifest rendering tests
internal/version/     build-time version/commit
bucket/               Scoop manifest, regenerated on every release
docs/                 design docs, plans, audit
.github/workflows/    CI (just ci) and release (semantic-release + bucket job)
```
```

- [ ] **Step 2: Replace the commands block**

```markdown
```
just build     # dist/whoosh2connect with version/commit ldflags
just test      # go test -race ./...
just vet       # go vet ./... and GOOS=windows go vet ./...
just lint      # golangci-lint run
just fmt       # gofmt -s -w .
just fmt-check # check-only formatting (what CI runs)
just tidy      # go mod tidy
just ci        # check-only: fmt-check, vet, lint, test, build
just sanity    # local: fmt, test, vet, build
just release 1.2.3                  # all platforms into dist/ + checksums.txt
just scoop-manifest 1.2.3 <sha256>  # render bucket/whoosh2connect.json
just clean     # rm -rf bin dist
just update-deps
just run -- --flags  # build then run dist/whoosh2connect
go run ./cmd/whoosh2connect --no-tui --dry-run   # preview sync decisions
go run ./cmd/whoosh2connect --verify            # ledger vs Garmin, read only
```
```

- [ ] **Step 3: Fix the verification paragraph**

```markdown
Always run `gofmt -s -w .`, `go vet ./...`, `GOOS=windows go vet ./...`,
`go test -race ./...`, and `golangci-lint run` before considering work done.
```

- [ ] **Step 4: Commit**

```bash
git add AGENTS.md
git commit -m "docs(agents): match layout and just recipes to the code"
```

---

### Task 4: Verify

**Files:**
- No source changes.

- [ ] **Step 1: Formatting, vet, tests, lint**

Run: `gofmt -s -l . && go vet ./... && GOOS=windows go vet ./... && go test -race ./... && golangci-lint run`
Expected: `gofmt` prints nothing, vet silent, tests `ok`, lint `0 issues`.

- [ ] **Step 2: Preset list matches the code**

Run: `go run ./cmd/whoosh2connect --device-preset nope 2>&1` and compare the README list against `internal/device/presets.go` output (32 IDs).
Expected: identical sets, no ID in one and not the other.

- [ ] **Step 3: Links and assets**

Run: `grep -oE '\]\([^)]+\)' README.md | sort -u`
Expected: only `LICENSE`, the release page, the CI workflow, and the Scoop URL; every relative path exists.

- [ ] **Step 4: Commit the design and plan**

```bash
git add docs/superpowers/specs/2026-11-01-readme-positioning-design.md docs/superpowers/plans/2026-11-01-readme-positioning.md
git commit -m "docs: design and plan for README positioning"
```
