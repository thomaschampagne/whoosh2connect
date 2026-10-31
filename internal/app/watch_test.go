package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"whoosh2connect/internal/garmin"
	"whoosh2connect/internal/watch"
)

type scriptedScanner struct {
	scans     [][]watch.Ride
	forgotten []string
	err       error
	stop      context.CancelFunc
}

func (s *scriptedScanner) Scan() ([]watch.Ride, error) {
	if s.err != nil {
		return nil, s.err
	}
	if len(s.scans) == 0 {
		if s.stop != nil {
			s.stop()
		}
		return nil, nil
	}
	r := s.scans[0]
	s.scans = s.scans[1:]
	return r, nil
}

func (s *scriptedScanner) Forget(hash string) { s.forgotten = append(s.forgotten, hash) }

type recordNotifier struct{ msgs []string }

func (r *recordNotifier) Notify(title, body string) error {
	r.msgs = append(r.msgs, title+": "+body)
	return nil
}

func rideAt(t *testing.T, name string) watch.Ride {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, fixture(), 0o600); err != nil {
		t.Fatal(err)
	}
	return watch.Ride{Path: p, File: name, Hash: "h-" + name}
}

func collect(t *testing.T, svc *Service, sc RideScanner, n *recordNotifier) []WatchEvent {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if s, ok := sc.(*scriptedScanner); ok {
		s.stop = cancel
	}
	out := make(chan WatchEvent)
	go svc.Watch(ctx, sc, time.Millisecond, n, out)
	var evs []WatchEvent
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-out:
			if !ok {
				return evs
			}
			evs = append(evs, ev)
			if ev.Err != nil {
				cancel()
			}
		case <-timeout:
			t.Fatal("watch did not finish")
		}
	}
}

func TestWatchEvents(t *testing.T) {
	tests := []struct {
		name       string
		upload     error
		wantStatus string
		wantNotify string
		wantForget bool
	}{
		{name: "upload succeeds", wantStatus: StatusUploaded, wantNotify: "Ride uploaded to Garmin: a.fit"},
		{name: "upload fails and is forgotten", upload: errors.New("boom"), wantStatus: StatusFailed, wantNotify: "Upload failed: a.fit: upload: boom", wantForget: true},
		{name: "duplicate on garmin", upload: garmin.ErrDuplicate, wantStatus: StatusDuplicate},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := &fakeGarmin{}
			if tc.upload != nil {
				g.upload = func(string) error { return tc.upload }
			}
			svc := newService(t, &fakeMyWhoosh{}, g)
			sc := &scriptedScanner{scans: [][]watch.Ride{{rideAt(t, "a.fit")}}}
			n := &recordNotifier{}

			evs := collect(t, svc, sc, n)
			if len(evs) != 1 {
				t.Fatalf("events = %d, want 1", len(evs))
			}
			if evs[0].File != "a.fit" || evs[0].Result.Status != tc.wantStatus {
				t.Errorf("event = %+v, want status %s", evs[0], tc.wantStatus)
			}
			if tc.wantNotify == "" {
				if len(n.msgs) != 0 {
					t.Errorf("notified %v, want none", n.msgs)
				}
			} else if len(n.msgs) != 1 || n.msgs[0] != tc.wantNotify {
				t.Errorf("notify = %v, want [%s]", n.msgs, tc.wantNotify)
			}
			if got := len(sc.forgotten) == 1; got != tc.wantForget {
				t.Errorf("forgotten = %v, want forget %v", sc.forgotten, tc.wantForget)
			}
		})
	}
}

func TestWatchScanErrorEmitsOnceAndStops(t *testing.T) {
	svc := newService(t, &fakeMyWhoosh{}, &fakeGarmin{})
	sc := &scriptedScanner{err: errors.New("disk gone"), stop: func() {}}
	evs := collect(t, svc, sc, &recordNotifier{})
	if len(evs) != 1 || evs[0].Err == nil {
		t.Fatalf("events = %+v, want one error event", evs)
	}
}

func TestWatchStopsOnCancel(t *testing.T) {
	svc := newService(t, &fakeMyWhoosh{}, &fakeGarmin{})
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan WatchEvent)
	done := make(chan struct{})
	go func() {
		svc.Watch(ctx, &scriptedScanner{}, time.Hour, &recordNotifier{}, out)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watch did not return after cancel")
	}
	if _, ok := <-out; ok {
		t.Fatal("out not closed")
	}
}
