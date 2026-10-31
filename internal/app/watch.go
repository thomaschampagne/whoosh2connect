package app

import (
	"context"
	"fmt"
	"time"

	"whoosh2connect/internal/notify"
	"whoosh2connect/internal/watch"
)

// RideScanner is the slice of *watch.Watcher the watch loop needs.
type RideScanner interface {
	Scan() ([]watch.Ride, error)
	Forget(hash string)
}

// WatchEvent reports one ride handled by the watch loop. Err is set only when
// the folder scan itself failed; the loop stops after sending that event.
type WatchEvent struct {
	File   string
	Result UploadResult
	Err    error
}

// Watch scans every interval, processes each finished ride, and sends one
// WatchEvent per ride on out. It returns and closes out when ctx is cancelled
// or a scan fails.
func (s *Service) Watch(ctx context.Context, sc RideScanner, interval time.Duration, n notify.Notifier, out chan<- WatchEvent) {
	defer close(out)
	for {
		rides, err := sc.Scan()
		if err != nil {
			send(ctx, out, WatchEvent{Err: fmt.Errorf("scan: %w", err)})
			return
		}
		for _, r := range rides {
			res := s.ProcessLocalFit(ctx, "local:"+r.Hash, r.Path)
			if res.Status == StatusFailed {
				sc.Forget(r.Hash)
			}
			s.notifyResult(n, r.File, res)
			send(ctx, out, WatchEvent{File: r.File, Result: res})
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func send(ctx context.Context, out chan<- WatchEvent, ev WatchEvent) {
	select {
	case out <- ev:
	case <-ctx.Done():
	}
}

func (s *Service) notifyResult(n notify.Notifier, file string, res UploadResult) {
	var err error
	switch res.Status {
	case StatusUploaded:
		err = n.Notify("Ride uploaded to Garmin", file)
	case StatusFailed:
		err = n.Notify("Upload failed", file+": "+res.DescribeUpload())
	default:
		return
	}
	if err != nil {
		s.Log.Warn("notification failed", "file", file, "err", err)
	}
}
