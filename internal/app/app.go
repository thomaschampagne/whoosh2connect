// Package app orchestrates the sync workflow: MyWhoosh session listing and FIT
// download, FIT fixing, device spoofing, Garmin upload, and idempotent state.
package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"whoosh2connect/internal/device"
	"whoosh2connect/internal/fit"
	"whoosh2connect/internal/garmin"
	"whoosh2connect/internal/mywhoosh"
	"whoosh2connect/internal/state"
)

// Upload statuses returned by Service.Upload.
const (
	StatusUploaded  = "uploaded"
	StatusDuplicate = "duplicate"
	StatusFailed    = "failed"
	StatusSkipped   = "skipped"
)

// MyWhooshAPI is the slice of the MyWhoosh client the service needs.
type MyWhooshAPI interface {
	IsAuthenticated() bool
	Resume(ctx context.Context) error
	Login(ctx context.Context, email, password string) error
	ListActivities(ctx context.Context) ([]mywhoosh.Activity, error)
	DownloadFit(ctx context.Context, fileID string) ([]byte, error)
}

// GarminAPI is the slice of the Garmin client the service needs.
type GarminAPI interface {
	Resume(ctx context.Context) error
	Login(ctx context.Context, email, password string) error
	NeedsMFA() bool
	SubmitMFA(ctx context.Context, code string) error
	UploadFIT(ctx context.Context, filePath string) error
}

// Service ties the pieces together.
type Service struct {
	MyWhoosh MyWhooshAPI
	Garmin   GarminAPI
	Store    *state.Store
	Device   device.Device
}

// New creates a Service.
func New(mw MyWhooshAPI, g GarminAPI, st *state.Store, dev device.Device) *Service {
	return &Service{MyWhoosh: mw, Garmin: g, Store: st, Device: dev}
}

// Session is a MyWhoosh activity annotated with upload status.
type Session struct {
	mywhoosh.Activity
	Uploaded   bool
	UploadedAt time.Time
}

// UploadResult reports the outcome of an upload attempt.
type UploadResult struct {
	Status string
	Stats  fit.Stats
	Err    error
}

// LoginMyWhoosh resumes a cached session or logs in fresh.
func (s *Service) LoginMyWhoosh(ctx context.Context, email, password string) error {
	if s.MyWhoosh.IsAuthenticated() {
		return nil
	}
	if err := s.MyWhoosh.Resume(ctx); err == nil {
		return nil
	}
	return s.MyWhoosh.Login(ctx, email, password)
}

// LoginGarmin resumes a cached session or logs in fresh, returning
// garmin.ErrMFARequired when a verification code is needed.
func (s *Service) LoginGarmin(ctx context.Context, email, password string) error {
	if s.Garmin.NeedsMFA() {
		return garmin.ErrMFARequired
	}
	if err := s.Garmin.Resume(ctx); err == nil {
		return nil
	}
	return s.Garmin.Login(ctx, email, password)
}

// Sessions lists activities with upload status.
func (s *Service) Sessions(ctx context.Context) ([]Session, error) {
	acts, err := s.MyWhoosh.ListActivities(ctx)
	if err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}
	out := make([]Session, 0, len(acts))
	for _, a := range acts {
		sess := Session{Activity: a}
		if at, ok := s.Store.UploadedAt(a.ID); ok {
			sess.Uploaded = true
			sess.UploadedAt = at
		}
		out = append(out, sess)
	}
	return out, nil
}

// Upload downloads, fixes, and uploads one activity. Duplicates are detected
// both locally (already marked) and remotely (Garmin 409) and marked uploaded.
func (s *Service) Upload(ctx context.Context, act mywhoosh.Activity) UploadResult {
	if s.Store.IsUploaded(act.ID) {
		return UploadResult{Status: StatusSkipped}
	}

	fileID := act.ActivityFileID
	if fileID == "" {
		fileID = act.ID
	}
	fitData, err := s.MyWhoosh.DownloadFit(ctx, fileID)
	if err != nil {
		return UploadResult{Status: StatusFailed, Err: fmt.Errorf("download: %w", err)}
	}

	var fixed bytes.Buffer
	stats, err := fit.Fix(bytes.NewReader(fitData), &fixed, s.Device)
	if err != nil {
		return UploadResult{Status: StatusFailed, Err: fmt.Errorf("fix fit: %w", err)}
	}

	tmp, err := os.CreateTemp("", "whoosh2connect-*.fit")
	if err != nil {
		return UploadResult{Status: StatusFailed, Err: fmt.Errorf("temp file: %w", err)}
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := tmp.Write(fixed.Bytes()); err != nil {
		return UploadResult{Status: StatusFailed, Err: fmt.Errorf("write temp: %w", err)}
	}
	if err := tmp.Sync(); err != nil {
		return UploadResult{Status: StatusFailed, Err: fmt.Errorf("sync temp: %w", err)}
	}

	err = s.Garmin.UploadFIT(ctx, tmp.Name())
	if err != nil {
		if errors.Is(err, garmin.ErrDuplicate) {
			if merr := s.Store.MarkUploaded(act.ID); merr != nil {
				return UploadResult{Status: StatusFailed, Err: merr}
			}
			return UploadResult{Status: StatusDuplicate, Stats: stats, Err: err}
		}
		return UploadResult{Status: StatusFailed, Err: fmt.Errorf("upload: %w", err)}
	}

	if err := s.Store.MarkUploaded(act.ID); err != nil {
		return UploadResult{Status: StatusFailed, Err: err}
	}
	return UploadResult{Status: StatusUploaded, Stats: stats}
}

// DescribeUpload summarises a result for TUI display.
func (r UploadResult) DescribeUpload() string {
	switch r.Status {
	case StatusUploaded:
		return "uploaded"
	case StatusDuplicate:
		return "already on Garmin (duplicate)"
	case StatusSkipped:
		return "skipped (already synced)"
	default:
		if r.Err != nil {
			return r.Err.Error()
		}
		return "failed"
	}
}
