package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCreatesAndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.IsUploaded("a1") {
		t.Fatal("should not be uploaded")
	}
	if err := s.MarkUploaded("a1"); err != nil {
		t.Fatal(err)
	}
	if !s.IsUploaded("a1") {
		t.Fatal("should be uploaded after mark")
	}
	if _, ok := s.UploadedAt("a1"); !ok {
		t.Fatal("no uploaded time")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("empty state file")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 600", info.Mode().Perm())
	}

	// Reload and verify persistence.
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.IsUploaded("a1") {
		t.Fatal("state not persisted across reload")
	}
}

func TestEmptyPath(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestConcurrentMark(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			_ = s.MarkUploaded("act" + string(rune('a'+i%4)))
		}(i)
	}
	for i := 0; i < 20; i++ {
		<-done
	}
	for _, id := range []string{"acta", "actb", "actc", "actd"} {
		if !s.IsUploaded(id) {
			t.Errorf("expected %s recorded", id)
		}
	}
}
