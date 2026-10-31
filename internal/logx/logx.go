// Package logx builds the structured file logger. Values under sensitive keys
// (passwords, tokens, MFA codes, authorization headers) are replaced before
// they reach the file.
package logx

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const redacted = "[redacted]"

var sensitiveKeys = []string{"password", "token", "secret", "mfa", "code", "authorization", "cookie"}

// Open appends JSON log lines to path, creating the file with mode 0600 and
// its directory with mode 0700. The returned closer must be called on exit.
func Open(path string, level slog.Level) (*slog.Logger, io.Closer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, nil, fmt.Errorf("create log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log %s: %w", path, err)
	}
	h := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level, ReplaceAttr: redact})
	return slog.New(h), f, nil
}

func redact(_ []string, a slog.Attr) slog.Attr {
	key := strings.ToLower(a.Key)
	for _, s := range sensitiveKeys {
		if strings.Contains(key, s) {
			return slog.String(a.Key, redacted)
		}
	}
	return a
}
