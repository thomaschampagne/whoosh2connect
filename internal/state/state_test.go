package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestOpenCreatesWithMode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "state.json")
	if _, err := Open(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o, want 600", perm)
	}
}

func TestPutPersistsAcrossReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Entry{ID: "a1", Hash: "h1", Status: StatusUploaded, GarminID: "g9"}
	if err := s.Put(want); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.ByID("a1")
	if !ok || got.Status != StatusUploaded || got.GarminID != "g9" || got.Hash != "h1" {
		t.Fatalf("reload got %+v ok=%v", got, ok)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt not set")
	}
}

func TestLookups(t *testing.T) {
	tests := []struct {
		name     string
		lookup   func(*Store) (Entry, bool)
		wantOK   bool
		wantStat Status
	}{
		{"id hit", func(s *Store) (Entry, bool) { return s.ByID("a1") }, true, StatusUploaded},
		{"id miss", func(s *Store) (Entry, bool) { return s.ByID("zz") }, false, ""},
		{"hash hit", func(s *Store) (Entry, bool) { return s.ByHash("h1") }, true, StatusUploaded},
		{"hash miss", func(s *Store) (Entry, bool) { return s.ByHash("zz") }, false, ""},
		{"empty hash never matches", func(s *Store) (Entry, bool) { return s.ByHash("") }, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustOpen(t)
			if err := s.Put(Entry{ID: "a1", Hash: "h1", Status: StatusUploaded}); err != nil {
				t.Fatal(err)
			}
			e, ok := tt.lookup(s)
			if ok != tt.wantOK || e.Status != tt.wantStat {
				t.Fatalf("got %+v ok=%v, want ok=%v status=%q", e, ok, tt.wantOK, tt.wantStat)
			}
		})
	}
}

func TestStaleUploadingResolvedOnOpen(t *testing.T) {
	tests := []struct {
		name       string
		age        time.Duration
		wantStatus Status
	}{
		{"fresh upload left alone", time.Minute, StatusUploading},
		{"old upload marked failed", 2 * staleAfter, StatusFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			old := file{Version: schemaVersion, Entries: map[string]Entry{
				"a1": {ID: "a1", Hash: "h1", Status: StatusUploading, UpdatedAt: time.Now().Add(-tt.age)},
			}}
			data, err := json.Marshal(old)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			s, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := s.ByID("a1")
			if got.Status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", got.Status, tt.wantStatus)
			}
		})
	}
}

func TestMigratesV1Map(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	v1 := `{"a1":"2026-01-02T03:04:05Z"}`
	if err := os.WriteFile(path, []byte(v1), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s.ByID("a1")
	if !ok || got.Status != StatusUploaded {
		t.Fatalf("migrated = %+v ok=%v", got, ok)
	}
	if got.UpdatedAt.Format(time.RFC3339) != "2026-01-02T03:04:05Z" {
		t.Errorf("UpdatedAt = %v", got.UpdatedAt)
	}
}

func TestCorruptFileIsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	const garbage = "not json"
	if err := os.WriteFile(path, []byte(garbage), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("expected parse error")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != garbage {
		t.Fatal("corrupt ledger was overwritten; user data lost")
	}
}

func TestEmptyPath(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func mustOpen(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTwoStoresDoNotLoseEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Put(Entry{ID: "a1", Status: StatusUploaded}); err != nil {
		t.Fatal(err)
	}
	if err := b.Put(Entry{ID: "b1", Status: StatusUploaded}); err != nil {
		t.Fatal(err)
	}
	final, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a1", "b1"} {
		if _, ok := final.ByID(id); !ok {
			t.Errorf("entry %s lost after concurrent writers", id)
		}
	}
}

func TestConcurrentWritersAcrossStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	const perStore = 15
	var wg sync.WaitGroup
	for s := 0; s < 2; s++ {
		st, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < perStore; i++ {
			wg.Add(1)
			go func(st *Store, id string) {
				defer wg.Done()
				if err := st.Put(Entry{ID: id, Status: StatusUploaded}); err != nil {
					t.Error(err)
				}
			}(st, fmt.Sprintf("s%d-%d", s, i))
		}
	}
	wg.Wait()
	final, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(final.byID); got != 2*perStore {
		t.Fatalf("entries = %d, want %d", got, 2*perStore)
	}
}

func TestPutDoesNotResolveOtherProcessUploading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	owner, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	other, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Put(Entry{ID: "x", Status: StatusUploading}); err != nil {
		t.Fatal(err)
	}
	if err := other.Put(Entry{ID: "y", Status: StatusUploaded}); err != nil {
		t.Fatal(err)
	}
	final, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if e, _ := final.ByID("x"); e.Status == StatusFailed {
		t.Fatal("in-flight upload of another process was marked failed")
	}
}

func TestLockFileMode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(Entry{ID: "a1", Status: StatusUploaded}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("lock perm = %o, want no group/other access", perm)
	}
}
