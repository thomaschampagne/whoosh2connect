// Package app orchestrates the sync workflow: MyWhoosh session listing and FIT
// download, FIT fixing, device spoofing, Garmin upload, and idempotent state.
package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	SubmitMFA(ctx context.Context, code string) error
	UploadFIT(ctx context.Context, filePath string) error
}

// RemoteChecker reports whether Garmin Connect already holds a ride that starts
// at the given time. A nil checker disables the remote guard.
type RemoteChecker interface {
	FindMatch(ctx context.Context, start time.Time) (garminID string, found bool, err error)
}

// Service ties the pieces together.
type Service struct {
	MyWhoosh MyWhooshAPI
	Garmin   GarminAPI
	Store    *state.Store
	Device   device.Device
	Remote   RemoteChecker
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

// Reasons explain a skipped upload.
const (
	ReasonLedgerID    = "ledger hit: activity id"
	ReasonLedgerHash  = "ledger hit: fit hash"
	ReasonRemoteMatch = "garmin match: start time"
)

// UploadResult reports the outcome of an upload attempt.
type UploadResult struct {
	Status string
	Reason string
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
	if err := s.Garmin.Resume(ctx); err == nil {
		return nil
	}
	return s.Garmin.Login(ctx, email, password)
}

// SubmitMFA submits a verification code for a pending Garmin login.
func (s *Service) SubmitMFA(ctx context.Context, code string) error {
	return s.Garmin.SubmitMFA(ctx, code)
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
		if e, ok := s.Store.ByID(a.ID); ok && e.Status == state.StatusUploaded {
			sess.Uploaded = true
			sess.UploadedAt = e.UpdatedAt
		}
		out = append(out, sess)
	}
	return out, nil
}

// Upload downloads, fixes, and uploads one activity. Duplicates are detected
// both locally (already marked) and remotely (Garmin 409) and marked uploaded.
func (s *Service) Upload(ctx context.Context, act mywhoosh.Activity) UploadResult {
	if e, ok := s.Store.ByID(act.ID); ok && e.Status == state.StatusUploaded {
		return UploadResult{Status: StatusSkipped, Reason: ReasonLedgerID}
	}
	if res, skip := s.checkRemote(ctx, act); skip {
		return res
	}

	fileID := act.ActivityFileID
	if fileID == "" {
		fileID = act.ID
	}
	fitData, err := s.MyWhoosh.DownloadFit(ctx, fileID)
	if err != nil {
		return UploadResult{Status: StatusFailed, Err: fmt.Errorf("download: %w", err)}
	}
	return s.uploadFit(ctx, act.ID, fitData)
}

// ProcessLocalFit fixes and uploads a local MyWhoosh FIT export (watch mode).
// The ledger key is the ride's ID; dedup by spoofed-FIT hash still applies.
func (s *Service) ProcessLocalFit(ctx context.Context, id, path string) UploadResult {
	if e, ok := s.Store.ByID(id); ok && e.Status == state.StatusUploaded {
		return UploadResult{Status: StatusSkipped, Reason: ReasonLedgerID}
	}
	fitData, err := os.ReadFile(path)
	if err != nil {
		return UploadResult{Status: StatusFailed, Err: fmt.Errorf("read fit: %w", err)}
	}
	return s.uploadFit(ctx, id, fitData)
}

// uploadFit fixes and spoofs raw FIT bytes, checks the ledger by hash, then
// uploads. The ledger records "uploading" before the POST so a lost response
// is visible on the next run.
func (s *Service) uploadFit(ctx context.Context, id string, fitData []byte) UploadResult {
	var fixed bytes.Buffer
	stats, err := fit.Fix(bytes.NewReader(fitData), &fixed, s.Device)
	if err != nil {
		return s.fail(id, "", fmt.Errorf("fix fit: %w", err))
	}
	sum := sha256.Sum256(fixed.Bytes())
	hash := hex.EncodeToString(sum[:])

	if prev, ok := s.Store.ByHash(hash); ok && prev.Status == state.StatusUploaded {
		if err := s.Store.Put(state.Entry{ID: id, Hash: hash, Status: state.StatusUploaded}); err != nil {
			return s.fail(id, hash, err)
		}
		return UploadResult{Status: StatusSkipped, Reason: ReasonLedgerHash, Stats: stats}
	}

	if err := s.Store.Put(state.Entry{ID: id, Hash: hash, Status: state.StatusUploading}); err != nil {
		return s.fail(id, hash, fmt.Errorf("ledger: %w", err))
	}

	err = s.uploadBytes(ctx, fixed.Bytes())
	switch {
	case errors.Is(err, garmin.ErrDuplicate):
		if merr := s.Store.Put(state.Entry{ID: id, Hash: hash, Status: state.StatusUploaded}); merr != nil {
			return s.fail(id, hash, merr)
		}
		return UploadResult{Status: StatusDuplicate, Stats: stats, Err: err}
	case err != nil:
		return UploadResult{Status: StatusFailed, Stats: stats, Err: s.recordFailure(id, hash, fmt.Errorf("upload: %w", err))}
	}

	if err := s.Store.Put(state.Entry{ID: id, Hash: hash, Status: state.StatusUploaded}); err != nil {
		return UploadResult{Status: StatusFailed, Stats: stats, Err: err}
	}
	return UploadResult{Status: StatusUploaded, Stats: stats}
}

// checkRemote asks Garmin whether the ride already exists. It returns skip=true
// when the ride must not be uploaded: either a match was found (recorded as
// uploaded) or the check failed (the ride is never uploaded unverified).
func (s *Service) checkRemote(ctx context.Context, act mywhoosh.Activity) (UploadResult, bool) {
	if s.Remote == nil {
		return UploadResult{}, false
	}
	gid, found, err := s.Remote.FindMatch(ctx, act.ParsedStart())
	if err != nil {
		return UploadResult{Status: StatusFailed, Err: fmt.Errorf("remote check: %w", err)}, true
	}
	if !found {
		return UploadResult{}, false
	}
	if err := s.Store.Put(state.Entry{ID: act.ID, GarminID: gid, Status: state.StatusUploaded}); err != nil {
		return UploadResult{Status: StatusFailed, Err: err}, true
	}
	return UploadResult{Status: StatusSkipped, Reason: ReasonRemoteMatch}, true
}

// Dry-run actions.
const (
	ActionUpload = "upload"
	ActionSkip   = "skip"
)

// Decision is what a sync would do for one activity. The FIT hash check runs
// only after download, so a planned upload may still be skipped at run time.
type Decision struct {
	Action string
	Reason string
}

// Plan reports what Upload would do for act without downloading, uploading,
// or writing to the ledger.
func (s *Service) Plan(ctx context.Context, act mywhoosh.Activity) (Decision, error) {
	if e, ok := s.Store.ByID(act.ID); ok && e.Status == state.StatusUploaded {
		return Decision{Action: ActionSkip, Reason: ReasonLedgerID}, nil
	}
	if s.Remote == nil {
		return Decision{Action: ActionUpload}, nil
	}
	_, found, err := s.Remote.FindMatch(ctx, act.ParsedStart())
	if err != nil {
		return Decision{}, fmt.Errorf("remote check: %w", err)
	}
	if found {
		return Decision{Action: ActionSkip, Reason: ReasonRemoteMatch}, nil
	}
	return Decision{Action: ActionUpload}, nil
}

// uploadBytes writes fixed FIT bytes to a private temp file, uploads it, and
// removes the file regardless of outcome.
func (s *Service) uploadBytes(ctx context.Context, data []byte) error {
	tmp, err := os.CreateTemp("", "whoosh2connect-*.fit")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	return s.Garmin.UploadFIT(ctx, tmp.Name())
}

func (s *Service) fail(id, hash string, err error) UploadResult {
	return UploadResult{Status: StatusFailed, Err: s.recordFailure(id, hash, err)}
}

// recordFailure stores the failure in the ledger and returns err, or the
// ledger error if that also fails.
func (s *Service) recordFailure(id, hash string, err error) error {
	if merr := s.Store.Put(state.Entry{ID: id, Hash: hash, Status: state.StatusFailed, LastError: err.Error()}); merr != nil {
		return fmt.Errorf("%w (ledger: %v)", err, merr)
	}
	return err
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
