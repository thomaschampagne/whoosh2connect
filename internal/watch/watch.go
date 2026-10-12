// Package watch detects finished MyWhoosh rides in the local FIT directory.
//
// MyWhoosh writes ride recordings locally just after a ride, clobbering a
// file named like "MyNewActivity-<version>.fit" in the app's data directory.
// A ride is considered finished when its content hash is stable across
// consecutive scans, which tolerates the file still being written.
package watch

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Ride is a locally-detected, finished ride.
type Ride struct {
	Path string
	File string
	Hash string // sha256 of the raw file bytes, used as the dedupe key
	Size int64
}

// Options configures a Watcher.
type Options struct {
	Dir         string
	Glob        string
	Interval    time.Duration
	StableScans int
}

// Watcher reports new finished rides in a directory.
type Watcher struct {
	opts  Options
	seen  map[string]bool
	prev  map[string]fileInfo
	stats Stats
}

// Stats counts rides handed to the caller.
type Stats struct {
	Scanned int
	New     int
}

type fileInfo struct {
	path  string
	size  int64
	mtime int64
	count int
}

// New validates options and applies defaults.
func New(o Options) (*Watcher, error) {
	if o.Dir == "" {
		return nil, fmt.Errorf("watch dir is required")
	}
	if o.Glob == "" {
		o.Glob = "*.fit"
	}
	if o.Interval <= 0 {
		o.Interval = 10 * time.Second
	}
	if o.StableScans <= 0 {
		o.StableScans = 2
	}
	return &Watcher{opts: o, seen: map[string]bool{}, prev: map[string]fileInfo{}}, nil
}

// Interval returns the configured scan interval.
func (w *Watcher) Interval() time.Duration { return w.opts.Interval }

// Dir returns the watched directory.
func (w *Watcher) Dir() string { return w.opts.Dir }

// Stat returns counters since the watcher was created.
func (w *Watcher) Stat() Stats { return w.stats }

// Scan looks for finished rides. A file is a finished ride when its content
// hash has been unchanged (same size and mtime) for StableScans consecutive
// scans. Each ride is reported exactly once per watcher lifetime.
func (w *Watcher) Scan() ([]Ride, error) {
	matches, err := filepath.Glob(filepath.Join(w.opts.Dir, w.opts.Glob))
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", w.opts.Glob, err)
	}
	w.stats.Scanned++

	cur := map[string]fileInfo{}
	for _, path := range matches {
		st, err := os.Stat(path)
		if err != nil {
			continue
		}
		hash, err := hashFile(path)
		if err != nil {
			continue
		}
		cur[hash] = fileInfo{path: path, size: st.Size(), mtime: st.ModTime().UnixNano()}
	}

	rides := []Ride{}
	for hash, inf := range cur {
		if w.seen[hash] {
			continue
		}
		if prev, ok := w.prev[hash]; ok && prev.size == inf.size && prev.mtime == inf.mtime {
			inf.count = prev.count + 1
		} else {
			inf.count = 1
		}
		if inf.count >= w.opts.StableScans {
			w.seen[hash] = true
			w.stats.New++
			rides = append(rides, Ride{Path: inf.path, File: filepath.Base(inf.path), Hash: hash, Size: inf.size})
			continue
		}
		cur[hash] = inf
	}
	w.prev = cur
	return rides, nil
}

// Forget drops a ride hash from the done set so it is reported again once it
// re-stabilizes. Used to retry rides whose upload failed.
func (w *Watcher) Forget(hash string) {
	delete(w.seen, hash)
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// DefaultDir returns the platform's known MyWhoosh local ride directory.
// Locations come from the JayQueue/MyWhoosh2Garmin reference project.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		d := filepath.Join(home, "Library", "Containers", "com.whoosh.whooshgame",
			"Data", "Library", "Application Support", "Epic", "MyWhoosh", "Content", "Data")
		if isDir(d) {
			return d, nil
		}
		return "", fmt.Errorf("MyWhoosh data directory not found at %s", d)
	case "windows":
		packages := filepath.Join(home, "AppData", "Local", "Packages")
		entries, err := os.ReadDir(packages)
		if err != nil {
			return "", fmt.Errorf("read Packages: %w", err)
		}
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "MyWhooshTechnologyService.") {
				d := filepath.Join(packages, e.Name(), "LocalCache", "Local", "MyWhoosh", "Content", "Data")
				if isDir(d) {
					return d, nil
				}
			}
		}
		return "", fmt.Errorf("no MyWhooshTechnologyService.* package under %s", packages)
	default:
		return "", fmt.Errorf("no known MyWhoosh local directory on %s; pass --watch <dir>", runtime.GOOS)
	}
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
