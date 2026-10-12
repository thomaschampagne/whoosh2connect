// Package state tracks which MyWhoosh sessions have already been uploaded, so
// uploads are idempotent across runs.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store persists a map of uploaded activity IDs to upload timestamps.
type Store struct {
	mu       sync.Mutex
	path     string
	uploaded map[string]string
}

// Open loads the store from path, creating the file if it does not exist.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("state file path is empty")
	}
	s := &Store{path: path, uploaded: make(map[string]string)}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &s.uploaded); err != nil {
			return nil, fmt.Errorf("parse state %s: %w", path, err)
		}
	case os.IsNotExist(err):
		if err := s.persistLocked(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("read state %s: %w", path, err)
	}
	return s, nil
}

// IsUploaded reports whether id was marked as uploaded.
func (s *Store) IsUploaded(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.uploaded[id]
	return ok
}

// UploadedAt returns when id was marked uploaded.
func (s *Store) UploadedAt(id string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, ok := s.uploaded[id]
	if !ok {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// MarkUploaded records id as uploaded and persists the change.
func (s *Store) MarkUploaded(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploaded[id] = time.Now().Format(time.RFC3339)
	return s.persistLocked()
}

func (s *Store) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	data, err := json.MarshalIndent(s.uploaded, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}
