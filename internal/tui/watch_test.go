package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"whoosh2connect/internal/app"
	"whoosh2connect/internal/notify"
	"whoosh2connect/internal/watch"
)

type fakeScanner struct{}

func (fakeScanner) Scan() ([]watch.Ride, error) { return nil, nil }
func (fakeScanner) Forget(string)               {}

// Watch blocks until ctx is cancelled, then closes out like the real loop.
func (f *fakeService) Watch(ctx context.Context, sc app.RideScanner, interval time.Duration, n notify.Notifier, out chan<- app.WatchEvent) {
	defer close(out)
	<-ctx.Done()
}

func keyT(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func newWatchModel(t *testing.T, saved *[]bool) Model {
	t.Helper()
	return New(Options{
		Service: &fakeService{},
		Context: context.Background(),
		Watch: WatchOptions{
			Interval:   time.Millisecond,
			Notifier:   notify.Nop,
			NewScanner: func(string) (app.RideScanner, error) { return fakeScanner{}, nil },
			Save: func(on bool) error {
				*saved = append(*saved, on)
				return nil
			},
		},
	})
}

func TestToggleWatchStartsAndStops(t *testing.T) {
	var saved []bool
	m := newWatchModel(t, &saved)
	m.screen = screenList

	cmd := upd(t, &m, keyT("t"))
	if cmd == nil {
		t.Fatal("t should return a start command")
	}
	upd(t, &m, cmd())
	if m.watch == nil {
		t.Fatal("watch should be running after started message")
	}
	if len(saved) != 1 || !saved[0] {
		t.Fatalf("saved = %v, want [true]", saved)
	}

	upd(t, &m, keyT("t"))
	if m.watch != nil {
		t.Fatal("watch should stop on second t")
	}
	if len(saved) != 2 || saved[1] {
		t.Fatalf("saved = %v, want [true false]", saved)
	}
}

func TestWatchEventUpdatesStatus(t *testing.T) {
	tests := []struct {
		name    string
		ev      app.WatchEvent
		want    string
		running bool
	}{
		{
			name:    "uploaded",
			ev:      app.WatchEvent{File: "a.fit", Result: app.UploadResult{Status: app.StatusUploaded}},
			want:    "uploaded",
			running: true,
		},
		{
			name:    "stream error stops watch",
			ev:      app.WatchEvent{Err: errors.New("disk gone")},
			want:    "disk gone",
			running: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var saved []bool
			m := newWatchModel(t, &saved)
			run := &watchRun{}
			m.watch = run
			upd(t, &m, watchEventMsg{run: run, ev: tc.ev})
			if !strings.Contains(m.status, tc.want) {
				t.Errorf("status = %q, want it to contain %q", m.status, tc.want)
			}
			if (m.watch != nil) != tc.running {
				t.Errorf("running = %v, want %v", m.watch != nil, tc.running)
			}
		})
	}
}

func TestStaleWatchEventIgnored(t *testing.T) {
	var saved []bool
	m := newWatchModel(t, &saved)
	m.watch = &watchRun{}
	before := m.status
	upd(t, &m, watchEventMsg{run: &watchRun{}, ev: app.WatchEvent{File: "old.fit", Result: app.UploadResult{Status: app.StatusUploaded}}})
	if m.status != before {
		t.Errorf("stale event changed status to %q", m.status)
	}
}
