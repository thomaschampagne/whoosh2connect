package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	tests := []struct {
		name     string
		existing []byte
		data     []byte
	}{
		{"creates new file", nil, []byte("secret")},
		{"replaces existing file", []byte("old"), []byte("new")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "token.json")
			if tt.existing != nil {
				if err := os.WriteFile(path, tt.existing, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := WriteFileAtomic(path, tt.data, 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(tt.data) {
				t.Fatalf("content = %q, want %q", got, tt.data)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" {
				if perm := info.Mode().Perm(); perm != 0o600 {
					t.Errorf("perm = %o, want 600", perm)
				}
			}
			entries, _ := os.ReadDir(filepath.Dir(path))
			if len(entries) != 1 {
				t.Errorf("temp files left behind: %d entries", len(entries))
			}
		})
	}
}

func TestWriteFileAtomicMissingDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "token.json")
	if err := WriteFileAtomic(path, []byte("x"), 0o600); err == nil {
		t.Fatal("expected error for missing directory")
	}
}
