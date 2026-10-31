# README positioning and documentation accuracy — design

Date: 2026-11-01
Status: awaiting approval

## Intent

Turn `README.md` from a manual into a product page: a cyclist landing on the
repository must understand in about fifteen seconds what the tool does, believe
it solves their problem, and download the binary. At the same time, bring
`README.md` and `AGENTS.md` back in line with the code, because both drifted
after auto-watch, the ledger, releases and the Scoop bucket landed.

The product promise is narrow and true: MyWhoosh rides end up in Garmin Connect
with the training metrics Garmin refuses to compute for a foreign, malformed FIT
file. The README sells that promise; it does not sell a fitness platform.

## Why now

The current README opens with a technical definition ("logs into your MyWhoosh
account, downloads ride FIT files..."), puts installation below a "Why it
exists" essay, and documents neither the auto-watch flags nor the TUI keys. A
visitor who does not already want the tool has no reason to keep reading. The
release pipeline, Scoop bucket and 33 device presets are all shipped but
under-sold or wrongly listed.

## Goals

- A visitor understands the value in one screen: tagline, one-sentence promise,
  install command, proof.
- Download is reachable without scrolling past prose.
- Every documented flag, key, path, preset and environment variable matches the
  code.
- Technical reference stays complete, just demoted below the pitch.
- `AGENTS.md` matches the real layout and the real `just` recipes.

## Non-goals

- No new features, no code behavior change.
- No new binary assets (no committed screenshot or GIF).
- No French README, no docs site, no CHANGELOG.
- No rewrite of `MISSION.md`, `ENDOF.md` or `HANDOFF.md` (historical records).
- No claims the code cannot back: no "guaranteed metrics", no "works with every
  Garmin device".

## Anti-goals

- Hype that reads as a scam: fake testimonials, invented user counts, "100%
  safe", unqualified ToS language.
- A README that hides the disclaimer to look cleaner.
- Marketing copy that pushes the technical reference off the file entirely.

## Constraints

- English, direct, second person. Short sentences. No emoji headers.
- Truthful: every number and every metric name must exist in the code or the
  mission's definition of done.
- Single `README.md`. The technical reference lives in it, below the pitch.
- Device presets must be generated from `internal/device/presets.go`, not typed
  by hand, so the list cannot drift again.
- The TUI preview must come from the real model, not a hand-drawn mock.

## Approaches considered

1. **Hero-first** — badges, tagline, promise, install, proof, preview, then the
   reference. Optimised for the fifteen-second decision. Chosen.
2. **Story-first** — problem narrative, then proof, then install. Warmer, but
   install arrives late and a skimmer leaves first.
3. **Docs-first with a marketing header** — keep the current order, prepend a
   hero. Cheapest, least impact; still reads as a manual.

Approach 1 wins because the stated goal is conversion, not reading time, and
nothing in the current content needs to be lost — only reordered and corrected.

## Decisions

### Structure

1. Title, badge row (release, CI, MIT, Go), tagline.
2. Hero paragraph: what it does, in three sentences, ending on the metrics.
3. **Install** — Scoop on Windows, archive per OS, checksum note. One command
   per platform, no prose between the reader and the download.
4. **Quickstart** — three numbered steps: run, sign in, upload.
5. **What Garmin was missing** — before/after table: Training Effect, Training
   Load, VO2max, device identity, bogus temperature.
6. **The TUI** — real rendered preview plus the key list.
7. **Auto-watch** — `t` in the TUI, `--watch auto` headless, notifications,
   retry behavior, "no background service".
8. **Why you can trust it** — local-only, 0600 tokens, no password on disk,
   2FA, idempotent ledger, `--dry-run`, `--verify`.
9. **Reference** — flags (complete), config YAML, environment variables, device
   presets (generated), 2FA flow, files and security table, troubleshooting,
   development, disclaimer.

### Tagline

Primary: **Your MyWhoosh rides, on Garmin Connect — with the metrics that make
them count.**

Rejected: "Ride on MyWhoosh. Get credit on Garmin." (ambiguous), "The missing
link between MyWhoosh and Garmin" (generic, claims nothing).

### TUI preview

A throwaway Go test in `internal/tui` renders `Model.View()` with a fake
`Service` and fixed sessions at a set width, then writes the frame to a file.
The frame is pasted into the README in a fenced block. The test is deleted
afterwards, so the repository gains no asset and no golden file to maintain.
If the render proves unstable (colour codes, width), the fallback is a plain
code block built from the same model output, labelled as a preview.

### Accuracy fixes folded into the rewrite

README:
- Device presets: add the ten missing IDs (`edge_200`, `edge_800`, `edge_810`,
  `edge_1000_thai`, `edge_25_jpn`, `edge_explore_1000`, `edge_explore_820`,
  `edge_mtb`, `edge_remote`, `edge_touring`), generated from code.
- Flags: document `--watch`, `--watch-interval`, `--watch-stable`.
- Add the TUI key list (`?`, `u`, `a`, `t`, `r`, `s`, `l`, `m`, `w`, `o`).
- Go version: `1.27.2`, matching `go.mod`.
- Replace "two public endpoints" with the real ones: the Garmin OAuth consumer
  JSON on S3, and Garmin mobile SSO.
- Data paths for Windows (`%LOCALAPPDATA%`) and macOS (`Application Support`).
- Mention system notifications and the MIT license.

AGENTS:
- `just test` runs `go test -race ./...`.
- Layout: one `internal/garmin/` entry, add `internal/release/`, `bucket/`,
  `docs/`, `.github/`.
- Commands: add `just ci`, `sanity`, `release`, `scoop-manifest`, `clean`,
  `update-deps`.
- Verification block: `go test -race ./...` and `GOOS=windows go vet ./...`.

### Claims policy

Allowed: metrics the mission's definition of done requires (Training Effect,
Training Load, VO2max), device identity, temperature stripping, idempotence,
local-only storage, 2FA. Every one is backed by `internal/fit`, `internal/app`
or `internal/garmin`.

Not allowed: uptime, user numbers, "undetectable", "guaranteed", "official",
"safe". The disclaimer stays, in its own section before Development, and states
plainly that the integrations are unofficial, may break, and that spoofing is
the user's responsibility.

## What is left to the builder

- Exact wording of the hero paragraph and section intros.
- Whether the preview sits above or below the before/after table.
- Badge URLs and whether to include a Go version badge at all.

## Verification

- `gofmt -s -l .` clean, `go vet ./...`, `go test -race ./...`,
  `golangci-lint run`.
- Preset list in README compared against a generated list from
  `internal/device/presets.go`.
- Every flag in `cmd/whoosh2connect/main.go` appears in the README table.
- Every `just` recipe appears in AGENTS.md or is deliberately omitted.
- No broken relative links; release asset names match `just release`.
