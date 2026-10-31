package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"

	"whoosh2connect/internal/device"
	"whoosh2connect/internal/mywhoosh"
	"whoosh2connect/internal/state"
)

func TestSecondUploadOfSameRideWhileInFlightIsSkipped(t *testing.T) {
	fit := fixture()
	mw := &fakeMyWhoosh{fits: map[string][]byte{"f-a1": fit}}
	started := make(chan struct{})
	release := make(chan struct{})
	g := &fakeGarmin{upload: func(string) error {
		close(started)
		<-release
		return nil
	}}
	svc := newService(t, mw, g)

	done := make(chan UploadResult, 1)
	go func() { done <- svc.Upload(context.Background(), mywhoosh.Activity{ID: "a1", ActivityFileID: "f-a1"}) }()
	<-started

	second := svc.Upload(context.Background(), mywhoosh.Activity{ID: "a1", ActivityFileID: "f-a1"})
	if second.Status != StatusSkipped || second.Reason != ReasonInFlight {
		t.Fatalf("second = %+v, want skipped in-flight", second)
	}
	close(release)
	if first := <-done; first.Status != StatusUploaded {
		t.Fatalf("first = %+v, want uploaded", first)
	}
	if g.uploads != 1 {
		t.Fatalf("uploads = %d, want 1", g.uploads)
	}
}

func TestLedgerWriteFailureAfterUploadStaysUploaded(t *testing.T) {
	dir := t.TempDir()
	st, err := state.Open(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	mw := &fakeMyWhoosh{fits: map[string][]byte{"f-a1": fixture()}}
	g := &fakeGarmin{}
	g.upload = func(string) error {
		// Block ledger writes after the POST: lock and temp files need dir write access.
		if err := os.Chmod(dir, 0o500); err != nil {
			return err
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
		return nil
	}
	svc := New(mw, g, st, testDeviceValue(), nil)
	res := svc.Upload(context.Background(), mywhoosh.Activity{ID: "a1", ActivityFileID: "f-a1"})
	if res.Status != StatusUploaded {
		t.Fatalf("status = %q, want uploaded (ride is on Garmin)", res.Status)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "ledger write failed") {
		t.Fatalf("err = %v, want ledger warning", res.Err)
	}
}

func TestWatchRideRemoteMatchSkips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ride.fit")
	if err := os.WriteFile(path, fixture(), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &fakeGarmin{}
	svc := newService(t, &fakeMyWhoosh{}, g)
	svc.Remote = &fakeRemote{found: true}
	res := svc.ProcessLocalFit(context.Background(), "local:abc", path)
	if res.Status != StatusSkipped || res.Reason != ReasonRemoteMatch {
		t.Fatalf("res = %+v, want remote match skip", res)
	}
	if g.uploads != 0 {
		t.Fatalf("uploads = %d, want 0", g.uploads)
	}
}

func TestUploadRecordsStartTime(t *testing.T) {
	g := &fakeGarmin{}
	svc := newService(t, &fakeMyWhoosh{fits: map[string][]byte{"f-a1": fixture()}}, g)
	start := time.Date(2026, 1, 8, 14, 10, 0, 0, time.UTC)
	act := mywhoosh.Activity{ID: "a1", ActivityFileID: "f-a1", StartDatetime: start.Format("2006-01-02T15:04:05.000Z")}
	if res := svc.Upload(context.Background(), act); res.Status != StatusUploaded {
		t.Fatalf("res = %+v", res)
	}
	e, ok := svc.Store.ByID("a1")
	if !ok || !e.Start.Equal(start) {
		t.Fatalf("ledger start = %v ok=%v, want %v", e.Start, ok, start)
	}
}

func TestVerify(t *testing.T) {
	start := time.Date(2026, 1, 8, 14, 10, 0, 0, time.UTC)
	tests := []struct {
		name     string
		entries  []state.Entry
		found    bool
		wantKind []string
	}{
		{"all present", []state.Entry{{ID: "a", Start: start, Status: state.StatusUploaded}}, true, nil},
		{"missing on garmin", []state.Entry{{ID: "a", Start: start, Status: state.StatusUploaded}}, false, []string{"uploaded but not found on Garmin"}},
		{"no start recorded", []state.Entry{{ID: "a", Status: state.StatusUploaded}}, true, []string{"no start time recorded, cannot verify"}},
		{"failed entries ignored", []state.Entry{{ID: "a", Start: start, Status: state.StatusFailed}}, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newService(t, &fakeMyWhoosh{}, &fakeGarmin{})
			svc.Remote = &fakeRemote{found: tt.found}
			for _, e := range tt.entries {
				if err := svc.Store.Put(e); err != nil {
					t.Fatal(err)
				}
			}
			findings, err := svc.Verify(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != len(tt.wantKind) {
				t.Fatalf("findings = %+v, want %d", findings, len(tt.wantKind))
			}
			for i, f := range findings {
				if f.Detail != tt.wantKind[i] {
					t.Errorf("detail = %q, want %q", f.Detail, tt.wantKind[i])
				}
			}
		})
	}
}

func TestUploadPendingStopsOnCancel(t *testing.T) {
	mw := &fakeMyWhoosh{fits: map[string][]byte{}}
	g := &fakeGarmin{}
	svc := newService(t, mw, g)
	var sessions []Session
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("a%d", i)
		mw.fits["f-"+id] = fixture()
		sessions = append(sessions, Session{Activity: mywhoosh.Activity{ID: id, ActivityFileID: "f-" + id}})
	}
	sessions[0].Uploaded = true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seen := 0
	svc.UploadPending(ctx, sessions, func(mywhoosh.Activity, UploadResult) {
		seen++
		if seen == 2 {
			cancel()
		}
	})
	if seen != 2 {
		t.Fatalf("reported %d, want 2 (uploaded skipped, stopped on cancel)", seen)
	}
}

// TestSync50NoGoroutineLeak syncs 50 fake rides and checks that no goroutines
// outlive the sync, then logs heap usage for the record.
func TestSync50NoGoroutineLeak(t *testing.T) {
	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	mw := &fakeMyWhoosh{fits: map[string][]byte{}}
	var sessions []Session
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("ride-%02d", i)
		mw.fits["f-"+id] = fixtureAt(base.Add(time.Duration(i) * time.Hour))
		sessions = append(sessions, Session{Activity: mywhoosh.Activity{ID: id, ActivityFileID: "f-" + id}})
	}
	svc := newService(t, mw, &fakeGarmin{})

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	uploaded := 0
	svc.UploadPending(context.Background(), sessions, func(_ mywhoosh.Activity, res UploadResult) {
		if res.Status == StatusUploaded {
			uploaded++
		}
	})
	runtime.GC()
	runtime.ReadMemStats(&after)
	if uploaded != 50 {
		t.Fatalf("uploaded = %d, want 50", uploaded)
	}
	t.Logf("heap before=%dKiB after=%dKiB goroutines=%d", before.HeapAlloc/1024, after.HeapAlloc/1024, runtime.NumGoroutine())
	goleak.VerifyNone(t)
}

func testDeviceValue() device.Device {
	return device.Device{Preset: device.Presets()[0], Serial: 1234}
}

func TestUploadWithoutStartTimeFailsClosed(t *testing.T) {
	g := &fakeGarmin{}
	svc := newService(t, &fakeMyWhoosh{fits: map[string][]byte{"f-a1": fixture()}}, g)
	svc.Remote = &fakeRemote{}
	res := svc.Upload(context.Background(), mywhoosh.Activity{ID: "a1", ActivityFileID: "f-a1"})
	if res.Status != StatusFailed {
		t.Fatalf("status = %q, want failed (remote check impossible)", res.Status)
	}
	if g.uploads != 0 {
		t.Fatalf("uploads = %d, want 0", g.uploads)
	}
}

func TestPlanWithoutStartTimeSkips(t *testing.T) {
	svc := newService(t, &fakeMyWhoosh{}, &fakeGarmin{})
	svc.Remote = &fakeRemote{}
	d, err := svc.Plan(context.Background(), mywhoosh.Activity{ID: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Action != ActionSkip || d.Reason != ReasonNoStart {
		t.Fatalf("decision = %+v, want skip no-start", d)
	}
}

func TestFailureKeepsKnownHash(t *testing.T) {
	st, err := state.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Put(state.Entry{ID: "a1", Hash: "abc", Status: state.StatusUploading}); err != nil {
		t.Fatal(err)
	}
	svc := New(&fakeMyWhoosh{}, &fakeGarmin{}, st, testDeviceValue(), nil)
	res := svc.run(context.Background(), "a1", time.Time{}, func(context.Context) ([]byte, error) {
		return nil, errors.New("download timeout")
	})
	if res.Status != StatusFailed {
		t.Fatalf("status = %q, want failed", res.Status)
	}
	e, _ := st.ByID("a1")
	if e.Hash != "abc" {
		t.Fatalf("hash = %q, want abc kept", e.Hash)
	}
}
