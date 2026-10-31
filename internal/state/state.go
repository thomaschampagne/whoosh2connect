// Package state is the sync ledger. It records every ride the tool has touched,
// keyed by MyWhoosh activity ID and by the SHA-256 of the spoofed FIT, so a ride
// is uploaded to Garmin Connect at most once.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Status is the lifecycle state of one ride in the ledger.
type Status string

// Ledger statuses.
const (
	StatusDownloaded Status = "downloaded"
	StatusSpoofed    Status = "spoofed"
	StatusUploading  Status = "uploading"
	StatusUploaded   Status = "uploaded"
	StatusFailed     Status = "failed"
)

const schemaVersion = 2

// Entry is one ride in the ledger.
type Entry struct {
	ID        string    `json:"id"`
	Hash      string    `json:"hash,omitempty"`
	Status    Status    `json:"status"`
	GarminID  string    `json:"garmin_id,omitempty"`
	LastError string    `json:"last_error,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type file struct {
	Version int              `json:"version"`
	Entries map[string]Entry `json:"entries"`
}

// Store is a JSON-backed ledger safe for concurrent use.
type Store struct {
	mu     sync.Mutex
	path   string
	byID   map[string]Entry
	byHash map[string]string
}

// Open loads the ledger at path, migrating a v1 file and creating an empty one
// when none exists. A corrupt file is reported, never overwritten.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("state file path is empty")
	}
	s := &Store{path: path, byID: map[string]Entry{}, byHash: map[string]string{}}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := s.load(data); err != nil {
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

func (s *Store) load(data []byte) error {
	var f file
	if err := json.Unmarshal(data, &f); err == nil && f.Version == schemaVersion {
		for id, e := range f.Entries {
			s.index(id, e)
		}
		return s.resolveStale()
	}
	var v1 map[string]string
	if err := json.Unmarshal(data, &v1); err != nil {
		return err
	}
	for id, raw := range v1 {
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			at = time.Now()
		}
		s.index(id, Entry{ID: id, Status: StatusUploaded, UpdatedAt: at})
	}
	return s.persistLocked()
}

// resolveStale marks uploads interrupted by a crash as failed. They are not
// retried blindly; the remote check decides before any new upload.
func (s *Store) resolveStale() error {
	dirty := false
	for id, e := range s.byID {
		if e.Status == StatusUploading {
			e.Status = StatusFailed
			e.LastError = "interrupted before confirmation"
			e.UpdatedAt = time.Now()
			s.index(id, e)
			dirty = true
		}
	}
	if !dirty {
		return nil
	}
	return s.persistLocked()
}

func (s *Store) index(id string, e Entry) {
	e.ID = id
	s.byID[id] = e
	if e.Hash != "" {
		s.byHash[e.Hash] = id
	}
}

// ByID returns the entry for a MyWhoosh activity ID.
func (s *Store) ByID(id string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byID[id]
	return e, ok
}

// ByHash returns the entry whose spoofed FIT has the given SHA-256.
func (s *Store) ByHash(hash string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if hash == "" {
		return Entry{}, false
	}
	id, ok := s.byHash[hash]
	if !ok {
		return Entry{}, false
	}
	return s.byID[id], true
}

// Put inserts or replaces an entry and persists the ledger atomically.
func (s *Store) Put(e Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.UpdatedAt = time.Now()
	s.index(e.ID, e)
	return s.persistLocked()
}

func (s *Store) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	data, err := json.MarshalIndent(file{Version: schemaVersion, Entries: s.byID}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return writeAtomic(s.path, data)
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp state: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp state: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp state: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}
