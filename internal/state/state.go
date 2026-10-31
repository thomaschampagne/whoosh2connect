// Package state is the sync ledger. It records every ride the tool has touched,
// keyed by MyWhoosh activity ID and by the SHA-256 of the spoofed FIT, so a ride
// is uploaded to Garmin Connect at most once.
//
// Several processes (TUI, CLI, watch) may share one ledger file. Every write
// takes an exclusive lock, reloads the file, applies the change, and writes
// atomically, so no process overwrites another's entries.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/gofrs/flock"

	"whoosh2connect/internal/fsutil"
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

const (
	schemaVersion = 2
	staleAfter    = 15 * time.Minute
)

// Entry is one ride in the ledger.
type Entry struct {
	ID        string    `json:"id"`
	Hash      string    `json:"hash,omitempty"`
	Status    Status    `json:"status"`
	GarminID  string    `json:"garmin_id,omitempty"`
	LastError string    `json:"last_error,omitempty"`
	Start     time.Time `json:"start,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type file struct {
	Version int              `json:"version"`
	Entries map[string]Entry `json:"entries"`
}

// Store is a JSON-backed ledger safe for concurrent use within a process and
// across processes sharing the same file.
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
	s := &Store{path: path}
	s.setEntries(map[string]Entry{})
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		entries, legacy, err := decode(data)
		if err != nil {
			return nil, fmt.Errorf("parse state %s: %w", path, err)
		}
		s.setEntries(entries)
		if err := s.withLock(func() error {
			if legacy {
				if err := s.persistLocked(); err != nil {
					return err
				}
			}
			return s.resolveStale()
		}); err != nil {
			return nil, err
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

// decode parses a v2 ledger, or a v1 map of ID to RFC3339 time (legacy = true).
func decode(data []byte) (entries map[string]Entry, legacy bool, err error) {
	var f file
	if json.Unmarshal(data, &f) == nil && f.Version == schemaVersion {
		if f.Entries == nil {
			f.Entries = map[string]Entry{}
		}
		return f.Entries, false, nil
	}
	var v1 map[string]string
	if err := json.Unmarshal(data, &v1); err != nil {
		return nil, false, err
	}
	entries = make(map[string]Entry, len(v1))
	for id, raw := range v1 {
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			at = time.Now()
		}
		entries[id] = Entry{ID: id, Status: StatusUploaded, UpdatedAt: at}
	}
	return entries, true, nil
}

func (s *Store) setEntries(entries map[string]Entry) {
	s.byID = make(map[string]Entry, len(entries))
	s.byHash = make(map[string]string, len(entries))
	for id, e := range entries {
		s.index(id, e)
	}
}

// resolveStale marks uploads that never reported back as failed. An upload is
// stale only after staleAfter, far longer than the Garmin HTTP timeout, so an
// upload still in flight in another process is never touched. Stale entries
// are not retried blindly; the remote check decides before any new upload.
// Runs under the file lock after a reload.
func (s *Store) resolveStale() error {
	dirty := false
	now := time.Now()
	for id, e := range s.byID {
		if e.Status == StatusUploading && now.Sub(e.UpdatedAt) > staleAfter {
			e.Status = StatusFailed
			e.LastError = "interrupted before confirmation"
			e.UpdatedAt = now
			s.index(id, e)
			dirty = true
		}
	}
	if !dirty {
		return nil
	}
	return s.persistLocked()
}

// withLock runs fn holding the cross-process file lock, with the in-memory
// view reloaded from disk first.
func (s *Store) withLock(fn func() error) error {
	lock := flock.New(s.path + ".lock")
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("lock state: %w", err)
	}
	defer func() { _ = lock.Unlock() }()
	if err := s.reloadLocked(); err != nil {
		return err
	}
	return fn()
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

// Put inserts or replaces an entry. It holds the file lock across reload,
// update, and write, so entries written by other processes are kept.
func (s *Store) Put(e Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withLock(func() error {
		e.UpdatedAt = time.Now()
		s.index(e.ID, e)
		return s.persistLocked()
	})
}

func (s *Store) reloadLocked() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		s.setEntries(map[string]Entry{})
		return nil
	}
	if err != nil {
		return fmt.Errorf("read state %s: %w", s.path, err)
	}
	entries, _, err := decode(data)
	if err != nil {
		return fmt.Errorf("parse state %s: %w", s.path, err)
	}
	s.setEntries(entries)
	return nil
}

func (s *Store) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	data, err := json.MarshalIndent(file{Version: schemaVersion, Entries: s.byID}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return fsutil.WriteFileAtomic(s.path, data, 0o600)
}

// Refresh reloads the ledger from disk under the cross-process lock, so reads
// see entries written by other processes.
func (s *Store) Refresh() error {
	return s.withLock(func() error { return nil })
}

// Entries returns a snapshot of every ride in the ledger, sorted by ID.
func (s *Store) Entries() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, len(s.byID))
	for _, e := range s.byID {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
