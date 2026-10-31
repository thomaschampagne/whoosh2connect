package logx

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactsSensitiveKeys(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		wantLeak bool
	}{
		{"password redacted", "password", "hunter2", false},
		{"access token redacted", "access_token", "abc123", false},
		{"mfa code redacted", "mfaCode", "123456", false},
		{"authorization redacted", "Authorization", "Bearer xyz", false},
		{"ride id kept", "id", "ride-42", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "log", "app.log")
			log, closer, err := Open(path, slog.LevelInfo)
			if err != nil {
				t.Fatal(err)
			}
			log.Info("event", tt.key, tt.value)
			_ = closer.Close()

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			leaked := strings.Contains(string(data), tt.value)
			if leaked != tt.wantLeak {
				t.Fatalf("value present = %v, want %v: %s", leaked, tt.wantLeak, data)
			}
		})
	}
}

func TestLogFileMode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	_, closer, err := Open(path, slog.LevelInfo)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closer.Close() }()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o, want 600", perm)
	}
}
