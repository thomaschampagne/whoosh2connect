package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScanReportsStableFileOnce(t *testing.T) {
	dir := t.TempDir()
	w, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "MyNewActivity-1.0.fit", "ride one")

	// First scan: watched but not yet stable.
	rides, err := w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(rides) != 0 {
		t.Fatalf("scan 1: %d rides, want 0 (needs two stable scans)", len(rides))
	}

	// Second scan with identical content: ride is done.
	rides, err = w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(rides) != 1 {
		t.Fatalf("scan 2: %d rides, want 1", len(rides))
	}
	r := rides[0]
	if r.Size != int64(len("ride one")) {
		t.Errorf("size = %d", r.Size)
	}
	if r.Hash == "" {
		t.Error("ride has no hash")
	}

	// Never reported again.
	rides, err = w.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(rides) != 0 {
		t.Fatalf("scan 3: %d rides, want 0 (dedupe)", len(rides))
	}
}

func TestScanTreatsInFlightWriteAsNotDone(t *testing.T) {
	dir := t.TempDir()
	w, err := New(Options{Dir: dir, StableScans: 3})
	if err != nil {
		t.Fatal(err)
	}
	path := writeFile(t, dir, "MyNewActivity-1.0.fit", "partial")

	for i := 1; i <= 2; i++ {
		// Content keeps changing: never stable, never reported.
		if err := os.WriteFile(path, []byte("partial "), 0o600); err != nil {
			t.Fatal(err)
		}
		rides, err := w.Scan()
		if err != nil {
			t.Fatal(err)
		}
		if len(rides) != 0 {
			t.Fatalf("scan %d: %d rides while file is mutating", i, len(rides))
		}
	}
}

func TestScanOverwriteIsNewRide(t *testing.T) {
	dir := t.TempDir()
	w, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "MyNewActivity-1.0.fit")
	if err := os.WriteFile(path, []byte("ride alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = mustStabilize(t, w)
	writeFile(t, dir, "MyNewActivity-1.0.fit", "ride bravo")
	rides := mustStabilize(t, w)
	if len(rides) != 1 {
		t.Fatalf("got %d rides after overwrite, want 1", len(rides))
	}
}

func TestScanIgnoresNonMatchingGlob(t *testing.T) {
	dir := t.TempDir()
	w, err := New(Options{Dir: dir, Glob: "MyNewActivity-*.fit"})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "other.fit", "not a ride")
	for i := 0; i < 3; i++ {
		rides, err := w.Scan()
		if err != nil {
			t.Fatal(err)
		}
		if len(rides) != 0 {
			t.Fatalf("scan %d: %d rides for non-matching file", i, len(rides))
		}
	}
}

func TestForgetReReportsRide(t *testing.T) {
	dir := t.TempDir()
	w, err := New(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "MyNewActivity-1.0.fit", "ride one")
	rides := mustStabilize(t, w)
	if len(rides) != 1 {
		t.Fatalf("got %d rides", len(rides))
	}
	w.Forget(rides[0].Hash)
	rides = mustStabilize(t, w)
	if len(rides) != 1 {
		t.Fatalf("after Forget: got %d rides, want 1 re-emitted", len(rides))
	}
}

func TestNewRequiresDir(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("expected error without dir")
	}
}

// mustStabilize scans until a ride emerges (or 5 scans), returning the rides.
func mustStabilize(t *testing.T, w *Watcher) []Ride {
	t.Helper()
	for i := 0; i < 5; i++ {
		rides, err := w.Scan()
		if err != nil {
			t.Fatal(err)
		}
		if len(rides) > 0 {
			return rides
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no ride after 5 scans")
	return nil
}
