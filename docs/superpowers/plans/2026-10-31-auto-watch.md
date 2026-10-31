# Auto-watch and notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user turn auto-watch on from the TUI. While it runs, every finished MyWhoosh ride is fixed, spoofed, and uploaded to Garmin, with a system notification.

**Architecture:** The watch loop moves from `cmd/whoosh2connect/main.go` into `internal/app` and emits events on a channel. The TUI consumes those events through a Bubble Tea subscription (one pending `tea.Cmd` that reads one event and re-arms). The CLI `--watch` flag and the TUI share that loop. A `notify` package wraps system notifications behind an interface.

**Tech Stack:** Go, Bubble Tea, Bubbles, `gopkg.in/yaml.v3`, `github.com/gen2brain/beeep` (new, to verify in Task 2).

**Spec:** `docs/superpowers/specs/2026-10-31-auto-watch-design.md`

## Global Constraints

- Idiomatic Go; interfaces defined at the consumer side; no globals; wrap errors with `%w`; lowercase error strings; no panics.
- Propagate `context.Context` through all network calls.
- Tests are table-driven; HTTP mocked with `net/http/httptest`; never real credentials.
- Never log or persist passwords, tokens, or MFA codes. Config and token files are mode `0600`.
- TUI: never block `Update`; do work in `tea.Cmd`.
- Before done: `gofmt -s -w .`, `go vet ./...`, `go test -race ./...`, `golangci-lint run`.
- Do not add dependencies without checking them first (Task 2 checks beeep).

---

### Task 1: Config keys for auto-watch

**Files:**
- Modify: `internal/config/config.go` (add `Watch` struct, field on `Config`)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: none
- Produces: `config.Watch{ Enabled bool \`yaml:"enabled"\`; Dir string \`yaml:"dir"\` }`, `config.Config.Watch Watch`. Dir `""` means `watch.DefaultDir()` is used by the caller.

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go` a table test that round-trips the new keys:

```go
func TestWatchSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	want := Default()
	want.Watch = Watch{Enabled: true, Dir: "/tmp/rides"}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, _, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Watch != want.Watch {
		t.Fatalf("watch = %+v, want %+v", got.Watch, want.Watch)
	}
}
```

If `loadFrom` does not exist, read the file with `os.ReadFile` and `yaml.Unmarshal` into a `Config` in the test instead. Check the existing test helpers first and reuse them.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run TestWatchSettingsRoundTrip -v`
Expected: FAIL, `undefined: Watch` or the field is missing.

- [ ] **Step 3: Write minimal implementation**

In `internal/config/config.go`:

```go
// Watch holds local ride-folder watching. Enabled is set by the TUI toggle.
type Watch struct {
	Enabled bool   `yaml:"enabled"`
	Dir     string `yaml:"dir"`
}
```

Add `Watch Watch \`yaml:"watch"\`` to `Config`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config -v`
Expected: PASS for all config tests.

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "feat(config): persist auto-watch enabled flag and folder"
```

---

### Task 2: Notification package

**Files:**
- Create: `internal/notify/notify.go`
- Create: `internal/notify/notify_test.go`
- Modify: `go.mod`, `go.sum` (via `go get`)

**Interfaces:**
- Consumes: none
- Produces: `notify.Notifier interface { Notify(title, body string) error }`, `notify.System() Notifier` (beeep-backed), `notify.Nop` (no-op value).

- [ ] **Step 1: Check the dependency before adding it**

Run: `go doc github.com/gen2brain/beeep` after `go get github.com/gen2brain/beeep@latest`. Record in the commit message whether it needs cgo on each OS. If beeep needs cgo on Windows or macOS and the release build disables cgo (see `justfile`), stop and report to the user before going on. Do not guess.

- [ ] **Step 2: Write the failing test**

```go
package notify

import "testing"

type recorder struct{ title, body string; err error }

func (r *recorder) Notify(title, body string) error {
	r.title, r.body = title, body
	return r.err
}

func TestNopNotifierNeverFails(t *testing.T) {
	if err := Nop.Notify("t", "b"); err != nil {
		t.Fatalf("Nop.Notify() = %v", err)
	}
}

func TestRecorderSatisfiesNotifier(t *testing.T) {
	var n Notifier = &recorder{}
	if err := n.Notify("Uploaded", "ride 1"); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/notify -v`
Expected: FAIL, `undefined: Notifier`.

- [ ] **Step 4: Write minimal implementation**

```go
// Package notify shows short system notifications. Failures are reported to
// the caller and never stop the caller's work.
package notify

import "github.com/gen2brain/beeep"

type Notifier interface {
	Notify(title, body string) error
}

type nop struct{}

func (nop) Notify(string, string) error { return nil }

// Nop discards notifications. Use it in tests and when notifications are off.
var Nop Notifier = nop{}

type system struct{}

func (system) Notify(title, body string) error {
	return beeep.Notify(title, body, "")
}

// System returns a Notifier that shows native desktop notifications.
func System() Notifier { return system{} }
```

Note: "no globals" in AGENTS.md. `Nop` is a sentinel value, the same as `io.Discard`. Keep it and mention it in the review.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/notify -v && go vet ./internal/notify`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/notify go.mod go.sum
git commit -m "feat(notify): system notifications behind a small interface"
```

---

### Task 3: Watch loop in `internal/app`

**Files:**
- Create: `internal/app/watch.go`
- Create: `internal/app/watch_test.go`
- Modify: `cmd/whoosh2connect/main.go` (later, Task 5)

**Interfaces:**
- Consumes: `watch.Ride` from `internal/watch` (`Path`, `File`, `Hash`), `app.Service.ProcessLocalFit`, `app.UploadResult`, `notify.Notifier`.
- Produces:

```go
// RideScanner is the slice of *watch.Watcher the loop needs.
type RideScanner interface {
	Scan() ([]watch.Ride, error)
	Forget(hash string)
}

// WatchEvent reports one ride processed by the watch loop.
type WatchEvent struct {
	File   string
	Result UploadResult
	Err    error // non-nil when the scan itself failed; the loop stops after sending it
}

// Watch scans every interval until ctx is cancelled, processing each finished
// ride and sending one WatchEvent per ride on out. It closes out on return.
func (s *Service) Watch(ctx context.Context, sc RideScanner, interval time.Duration, notifier notify.Notifier, out chan<- WatchEvent)
```

Note: the `watch` package imports nothing from `app`, so there is no cycle. Check that `app` may import `watch`; it currently does not, so this is a new edge. Also check `internal/notify` has no cycle.

- [ ] **Step 1: Write the failing test**

Table-driven test with fakes. The scanner returns scripted rides; `Service` runs against a fake Garmin via `app.New` and httptest-free fakes already used in `pipeline_test.go` (reuse those helpers, read them first).

```go
func TestWatchEmitsOneEventPerRide(t *testing.T) {
	tests := []struct {
		name       string
		scans      [][]watch.Ride
		uploadErr  error
		wantStatus []string
		wantNotify int
	}{
		{
			name:       "upload succeeds",
			scans:      [][]watch.Ride{{{File: "a.fit", Path: "/x/a.fit", Hash: "h1"}}},
			wantStatus: []string{StatusUploaded},
			wantNotify: 1,
		},
		{
			name:       "upload fails and is forgotten",
			scans:      [][]watch.Ride{{{File: "a.fit", Path: "/x/a.fit", Hash: "h1"}}},
			uploadErr:  errors.New("boom"),
			wantStatus: []string{StatusFailed},
			wantNotify: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// build service with fakes from pipeline_test.go, fake scanner that
			// returns tc.scans[i] on the i-th call and then blocks on ctx
			// run Watch in a goroutine with a cancelable ctx, read events from out,
			// assert statuses and notification count, cancel, wait for close.
		})
	}
}
```

Fill the subtest body using the fakes in `pipeline_test.go`. The fake scanner records `Forget` calls; assert `Forget("h1")` is called on failure only. Also add a case for ctx cancel: `Watch` returns and closes `out` within 1 second.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/app -run TestWatch -v`
Expected: FAIL, `undefined: Watch` / `RideScanner`.

- [ ] **Step 3: Write minimal implementation**

```go
package app

import (
	"context"
	"fmt"
	"time"

	"whoosh2connect/internal/notify"
	"whoosh2connect/internal/watch"
)

type RideScanner interface {
	Scan() ([]watch.Ride, error)
	Forget(hash string)
}

type WatchEvent struct {
	File   string
	Result UploadResult
	Err    error
}

func (s *Service) Watch(ctx context.Context, sc RideScanner, interval time.Duration, n notify.Notifier, out chan<- WatchEvent) {
	defer close(out)
	for {
		rides, err := sc.Scan()
		if err != nil {
			send(ctx, out, WatchEvent{Err: fmt.Errorf("watch: %w", err)})
			return
		}
		for _, r := range rides {
			res := s.ProcessLocalFit(ctx, "local:"+r.Hash, r.Path)
			if res.Status == StatusFailed {
				sc.Forget(r.Hash)
			}
			notifyResult(n, r.File, res, s.Log)
			send(ctx, out, WatchEvent{File: r.File, Result: res})
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func send(ctx context.Context, out chan<- WatchEvent, ev WatchEvent) {
	select {
	case out <- ev:
	case <-ctx.Done():
	}
}

func notifyResult(n notify.Notifier, file string, res UploadResult, log *slog.Logger) {
	switch res.Status {
	case StatusUploaded:
		_ = n.Notify("Ride uploaded to Garmin", file)
	case StatusFailed:
		_ = n.Notify("Upload failed", file+": "+res.Err.Error())
	}
	if err := ...; err != nil { log.Warn(...) }
}
```

Replace the `_ =` with a logged warning (`s.Log.Warn("notify failed", "err", err)`) so the notify error is not silently dropped. `res.Err` can be nil on failure only if the code is wrong. Guard with `DescribeUpload()` instead of `res.Err.Error()`.

Note: `send` on a cancelled ctx drops the event. Acceptable: the ride is already in the ledger.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -race ./internal/app -run TestWatch -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/app/watch.go internal/app/watch_test.go
git commit -m "feat(app): watch loop emits events and notifies, shared by CLI and TUI"
```

---

### Task 4: TUI toggle and event subscription

**Files:**
- Modify: `internal/tui/model.go` (Options, Model fields, Update cases, `Service` interface unchanged)
- Create: `internal/tui/watch.go` (commands, msgs, subscription)
- Modify: `internal/tui/views.go` (footer/header state)
- Test: `internal/tui/watch_test.go`

**Interfaces:**
- Consumes: `app.WatchEvent`, `app.RideScanner`, `notify.Notifier`, `config.Watch`.
- Produces, in `Options`:

```go
// Watch controls auto-watch from the TUI.
Watch WatchOptions

type WatchOptions struct {
	Enabled bool                                  // initial state from config
	Dir     string                                // resolved folder, "" if not found
	NewScanner func(dir string) (app.RideScanner, error)
	Notifier   notify.Notifier
	Interval   time.Duration
	Save       func(enabled bool) error          // persists the toggle
}
```

Keep the scanner factory injectable so tests do not touch the real disk.

- [ ] **Step 1: Check key conflicts**

Already checked: `a` is taken (`internal/tui/model.go:472`, "upload all pending"), `w` is "save spoofed", and `t` is free in `internal/tui`. Plan uses `t` for auto-watch. Re-check with `grep -n 'case "t"' internal/tui/*.go` before editing, and update the footer help text in `views.go`.

- [ ] **Step 2: Write the failing test**

```go
func TestToggleWatchStartsAndStops(t *testing.T) {
	saved := []bool{}
	m := New(Options{
		Service: &fakeService{},
		Context: context.Background(),
		Watch: WatchOptions{
			Dir:        t.TempDir(),
			Interval:   time.Millisecond,
			Notifier:   notify.Nop,
			NewScanner: func(string) (app.RideScanner, error) { return &fakeScanner{}, nil },
			Save:       func(on bool) error { saved = append(saved, on); return nil },
		},
	})
	m = boot(t, m).(Model) // reuse helper from model_test.go
	m.screen = screenList

	cmd := upd(t, &m, keyMsg("t"))
	if !m.watching { t.Fatal("watching should be true after t") }
	if cmd == nil { t.Fatal("expected a watch subscription cmd") }
	if len(saved) != 1 || !saved[0] { t.Fatalf("saved = %v, want [true]", saved) }

	upd(t, &m, keyMsg("t"))
	if m.watching { t.Fatal("watching should be false after second t") }
	if len(saved) != 2 || saved[1] { t.Fatalf("saved = %v, want [true false]", saved) }
}

func TestWatchEventUpdatesResults(t *testing.T) {
	m := New(Options{Service: &fakeService{}, Context: context.Background()})
	m.watching = true
	upd(t, &m, watchEventMsg{ev: app.WatchEvent{File: "a.fit", Result: app.UploadResult{Status: app.StatusUploaded}}})
	if m.status == "" { t.Fatal("status should show the last watch event") }
}
```

Check `keyMsg` and `fakeService` exist in `model_test.go`. Add `keyMsg` only if it does not. `fakeScanner` is new; define it in `watch_test.go`.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/tui -run 'TestToggleWatch|TestWatchEvent' -v`
Expected: FAIL, undefined `WatchOptions`, `watching`, `watchEventMsg`.

- [ ] **Step 4: Write minimal implementation**

`internal/tui/watch.go`:

```go
package tui

// watchEventMsg carries one event from the watch loop into Update.
type watchEventMsg struct{ ev app.WatchEvent }

// watchStartedMsg is sent when the loop channel is ready.
type watchStartedMsg struct{ ch <-chan app.WatchEvent; cancel context.CancelFunc }

// startWatchCmd builds the scanner off the Update loop and starts the service loop.
func (m Model) startWatchCmd() tea.Cmd { ... }

// nextWatchEventCmd blocks on one event. Update re-arms it after each message.
func nextWatchEventCmd(ch <-chan app.WatchEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return watchStoppedMsg{}
		}
		return watchEventMsg{ev: ev}
	}
}
```

In `Update`, on `watchEventMsg`: update `m.results[ev.File]` and `m.status` through the existing `DescribeUpload` text, log it, and return `nextWatchEventCmd(m.watchCh)`. On `watchStoppedMsg`: set `m.watching = false`, log. Also on `watchErrMsg` (scan error): log and stop.

The `case "t"` key: if off, call `startWatchCmd` and set `m.watching = true`. If on, call `m.watchCancel()` and set `m.watching = false`. Call `m.watchSave(enabled)` and log an error if it fails.

Do not block: the scanner factory runs inside the Cmd, not in Update.

Store `watchCancel context.CancelFunc` and `watchCh <-chan app.WatchEvent` on `Model`. Model is a value type; keep them as pointer fields or the closure-captured values, because Bubble Tea copies the model each update. Pointer to a small struct `*watchRun` is the cleanest.

Footer text: add `t auto-watch on|off` to the help line and show a `● watching` marker in the header when `m.watching`.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -race ./internal/tui -v`
Expected: PASS, including existing tests.

- [ ] **Step 6: Commit**

```bash
git add internal/tui
git commit -m "feat(tui): t toggles auto-watch with live status and notifications"
```

---

### Task 5: Wire CLI and startup

**Files:**
- Modify: `cmd/whoosh2connect/main.go`
- Test: `cmd/whoosh2connect/main_test.go`

**Interfaces:**
- Consumes: `app.Service.Watch`, `tui.WatchOptions`, `watch.New`, `watch.DefaultDir`, `config.Watch`, `notify.System`.
- Produces: none (wiring).

- [ ] **Step 1: Write the failing test**

Table test on a helper `resolveWatchDir(cfg config.Config, flag string) (string, error)`:

```go
func TestResolveWatchDir(t *testing.T) {
	tests := []struct {
		name, flag, cfgDir string
		wantErr            bool
	}{
		{"flag wins", t.TempDir(), "/nope", false},
		{"config dir used", "", t.TempDir(), false},
		{"missing dir errors", "", filepath.Join(t.TempDir(), "gone"), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Watch.Dir = tc.cfgDir
			got, err := resolveWatchDir(cfg, tc.flag)
			if (err != nil) != tc.wantErr { t.Fatalf("err = %v, wantErr %v", err, tc.wantErr) }
			if err == nil && got == "" { t.Fatal("empty dir") }
		})
	}
}
```

Note: the "flag wins" case uses a temp dir for the flag. Confirm the flag path is checked before `cfgDir`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/whoosh2connect -run TestResolveWatchDir -v`
Expected: FAIL, undefined `resolveWatchDir`.

- [ ] **Step 3: Write minimal implementation**

- Extract `resolveWatchDir(cfg, flag)`: flag first, then `cfg.Watch.Dir`, then `watch.DefaultDir()`. `"auto"` is treated as "use `watch.DefaultDir()`". Check the directory exists.
- Replace the body of `watchLoop` with a call to `svc.Watch(ctx, w, interval, notify.System(), ch)` and a loop that prints each event to stdout. Keep the existing login and MFA prompt. Keep the `--watch` flag and its behavior.
- In the TUI branch, pass `Watch: tui.WatchOptions{ Enabled: cfg.Watch.Enabled, Dir: resolved-or-"", NewScanner: ..., Notifier: notify.System(), Interval: 10s, Save: func(on bool) error { cfg.Watch.Enabled = on; return config.Save(paths.ConfigFile, cfg) } }`.
- Startup: if `cfg.Watch.Enabled` and Garmin is already logged in (`svc.LoginGarmin` resumes tokens without MFA), the TUI starts the watch on bootstrap. If the login needs MFA, the TUI shows the MFA screen first and starts the watch afterwards. Implement this in the TUI bootstrap path, not in `main.go`.
- Remove the duplicate `watchLoop` code once the CLI uses `svc.Watch`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -race ./cmd/whoosh2connect -v`
Expected: PASS, including the existing `TestWatch*` tests.

- [ ] **Step 5: Commit**

```bash
git add cmd/whoosh2connect
git commit -m "refactor(cmd): watch uses app.Watch, TUI starts it when enabled"
```

---

### Task 6: Docs and final verification

**Files:**
- Modify: `README.md` (watch section, the `t` key, notifications, config keys)
- Modify: `AGENTS.md` (layout line for `internal/notify`, mention `app/watch.go`)
- Modify: `docs/audit.md` (mark A1 as fixed)

**Interfaces:** none.

- [ ] **Step 1: Update docs**

Add to README: the `t` key, the `watch.enabled` and `watch.dir` config keys, and that the watch runs only while the app is open. Add `internal/notify/` to the layout in AGENTS.md. Mark audit A1 FIXED.

- [ ] **Step 2: Run the full verification**

Run:

```bash
gofmt -s -w .
go vet ./...
go test -race ./...
golangci-lint run
```

Expected: every command exits 0. Paste the tail of each output into the report.

- [ ] **Step 3: Manual smoke check (not automated)**

Run: `go run ./cmd/whoosh2connect`, press `t`, confirm the header shows `watching`. Drop a FIT into the watch folder and confirm one notification. Record what you observed on Linux only. Windows and macOS paths are unverified; say so in the report.

- [ ] **Step 4: Commit**

```bash
git add README.md AGENTS.md docs/audit.md
git commit -m "docs: auto-watch toggle, config keys, notifications"
```

---

## Self-Review

- **Spec coverage:** toggle in TUI (Task 4), persisted state (Task 1, Task 5), watch in TUI process only (Tasks 3-5), notifications on success and failure (Tasks 2-3), MFA needed notification: **not covered**. The spec says notify on MFA required. Add to Task 3 if the loop can see `garmin.ErrMFARequired` from `ProcessLocalFit`; otherwise log it and report it in the TUI. Decide before executing.
- **Placeholder scan:** Task 2 step 1 and Task 4 step 1 are checks, not placeholders. Task 3 test body and Task 5 `TUI bootstrap` describe the intent but not full code; the executor must read `pipeline_test.go` and `model_test.go` first. Resolve before running.
- **Type consistency:** `RideScanner`, `WatchEvent`, `WatchOptions`, `watchEventMsg`, and `config.Watch` are named once each and used with the same names.
- **Import cycle:** `app` now imports `watch` and `notify`. Neither imports `app`. Verified by reading `watch.go` imports.
