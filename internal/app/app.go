// Package app orchestrates the sync workflow: MyWhoosh session listing and FIT
// download, FIT fixing, device spoofing, Garmin upload, and idempotent state.
// Both the MyWhoosh API path and the local watch path share one pipeline (run),
// so dedupe rules apply identically to every ride.
package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"whoosh2connect/internal/device"
	"whoosh2connect/internal/fit"
	"whoosh2connect/internal/fsutil"
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

// Reasons explain a skipped upload.
const (
	ReasonLedgerID    = "ledger hit: activity id"
	ReasonLedgerHash  = "ledger hit: fit hash"
	ReasonRemoteMatch = "garmin match: start time"
	ReasonInFlight    = "already uploading"
	ReasonNoStart     = "no start time, cannot check Garmin"
)

// Dry-run actions.
const (
	ActionUpload = "upload"
	ActionSkip   = "skip"
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

// Service ties the pieces together. Its zero value is not usable; use New.
type Service struct {
	MyWhoosh MyWhooshAPI
	Garmin   GarminAPI
	Store    *state.Store
	Device   device.Device
	Remote   RemoteChecker
	Log      *slog.Logger

	mu       sync.Mutex
	inflight map[string]struct{}
}

// New creates a Service. A nil logger discards log output.
func New(mw MyWhooshAPI, g GarminAPI, st *state.Store, dev device.Device, log *slog.Logger) *Service {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Service{
		MyWhoosh: mw,
		Garmin:   g,
		Store:    st,
		Device:   dev,
		Log:      log,
		inflight: map[string]struct{}{},
	}
}

// Session is a MyWhoosh activity annotated with upload status.
type Session struct {
	mywhoosh.Activity
	Uploaded   bool
	UploadedAt time.Time
}

// UploadResult reports the outcome of an upload attempt. Status uploaded with
// a non-nil Err means the ride reached Garmin but the ledger write failed; the
// next remote check confirms it.
type UploadResult struct {
	Status string
	Reason string
	Stats  fit.Stats
	Err    error
}

// Decision is what a sync would do for one activity. The FIT hash check runs
// only after download, so a planned upload may still be skipped at run time.
type Decision struct {
	Action string
	Reason string
}

// Finding is one ledger entry that failed verification against Garmin.
type Finding struct {
	ID     string
	Start  time.Time
	Detail string
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

// Upload downloads, fixes, and uploads one MyWhoosh activity.
func (s *Service) Upload(ctx context.Context, act mywhoosh.Activity) UploadResult {
	fileID := fileIDOf(act)
	return s.run(ctx, act.ID, act.ParsedStart(), func(ctx context.Context) ([]byte, error) {
		data, err := s.MyWhoosh.DownloadFit(ctx, fileID)
		if err != nil {
			return nil, fmt.Errorf("download: %w", err)
		}
		return data, nil
	})
}

// UploadPending uploads every activity in sessions that is not yet synced,
// in order, stopping early when ctx is cancelled. report is called once per
// attempted activity.
func (s *Service) UploadPending(ctx context.Context, sessions []Session, report func(mywhoosh.Activity, UploadResult)) {
	for _, sess := range sessions {
		if ctx.Err() != nil {
			return
		}
		if sess.Uploaded {
			continue
		}
		report(sess.Activity, s.Upload(ctx, sess.Activity))
	}
}

// ProcessLocalFit fixes and uploads a local MyWhoosh FIT export (watch mode).
// The ledger key is id; dedupe by spoofed-FIT hash still applies.
func (s *Service) ProcessLocalFit(ctx context.Context, id, path string) UploadResult {
	raw, err := os.ReadFile(path)
	if err != nil {
		return UploadResult{Status: StatusFailed, Err: fmt.Errorf("read fit: %w", err)}
	}
	start, err := fit.StartTime(raw)
	if err != nil {
		s.Log.Warn("no start time in fit, remote check skipped", "id", id, "err", err)
	}
	return s.run(ctx, id, start, func(context.Context) ([]byte, error) { return raw, nil })
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
	start := act.ParsedStart()
	if start.IsZero() {
		return Decision{Action: ActionSkip, Reason: ReasonNoStart}, nil
	}
	_, found, err := s.Remote.FindMatch(ctx, start)
	if err != nil {
		return Decision{}, fmt.Errorf("remote check: %w", err)
	}
	if found {
		return Decision{Action: ActionSkip, Reason: ReasonRemoteMatch}, nil
	}
	return Decision{Action: ActionUpload}, nil
}

// Verify checks every uploaded ledger entry against Garmin Connect. It uploads
// nothing and reports entries that Garmin does not show.
func (s *Service) Verify(ctx context.Context) ([]Finding, error) {
	if s.Remote == nil {
		return nil, fmt.Errorf("remote check not configured")
	}
	var out []Finding
	if err := s.Store.Refresh(); err != nil {
		return nil, fmt.Errorf("reload ledger: %w", err)
	}
	for _, e := range s.Store.Entries() {
		if e.Status != state.StatusUploaded {
			continue
		}
		if e.Start.IsZero() {
			out = append(out, Finding{ID: e.ID, Detail: "no start time recorded, cannot verify"})
			continue
		}
		_, found, err := s.Remote.FindMatch(ctx, e.Start)
		if err != nil {
			return out, fmt.Errorf("verify %s: %w", e.ID, err)
		}
		if !found {
			out = append(out, Finding{ID: e.ID, Start: e.Start, Detail: "uploaded but not found on Garmin"})
		}
	}
	return out, nil
}

// run is the single upload pipeline for every ride source. fetch supplies the
// raw FIT bytes and is called only after the ledger and remote guards pass.
func (s *Service) run(ctx context.Context, id string, start time.Time, fetch func(context.Context) ([]byte, error)) UploadResult {
	if e, ok := s.Store.ByID(id); ok && e.Status == state.StatusUploaded {
		return UploadResult{Status: StatusSkipped, Reason: ReasonLedgerID}
	}
	if !s.begin(id) {
		return UploadResult{Status: StatusSkipped, Reason: ReasonInFlight}
	}
	defer s.end(id)

	if res, skip := s.checkRemote(ctx, id, start); skip {
		s.Log.Info("ride skipped", "id", id, "reason", res.Reason, "status", res.Status)
		return res
	}
	raw, err := fetch(ctx)
	if err != nil {
		return s.fail(id, "", start, err)
	}
	res := s.uploadFit(ctx, id, start, raw)
	s.Log.Info("ride processed", "id", id, "status", res.Status)
	return res
}

func (s *Service) begin(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.inflight[id]; busy {
		return false
	}
	s.inflight[id] = struct{}{}
	return true
}

func (s *Service) end(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.inflight, id)
}

// uploadFit fixes and spoofs raw FIT bytes, checks the ledger by hash, then
// uploads. The ledger records "uploading" before the POST so a lost response
// is visible on the next run.
func (s *Service) uploadFit(ctx context.Context, id string, start time.Time, raw []byte) UploadResult {
	var fixed bytes.Buffer
	stats, err := fit.Fix(bytes.NewReader(raw), &fixed, s.Device)
	if err != nil {
		return s.fail(id, "", start, fmt.Errorf("fix fit: %w", err))
	}
	sum := sha256.Sum256(fixed.Bytes())
	hash := hex.EncodeToString(sum[:])

	if prev, ok := s.Store.ByHash(hash); ok && prev.Status == state.StatusUploaded {
		if err := s.Store.Put(state.Entry{ID: id, Hash: hash, Start: start, Status: state.StatusUploaded}); err != nil {
			return s.fail(id, hash, start, err)
		}
		return UploadResult{Status: StatusSkipped, Reason: ReasonLedgerHash, Stats: stats}
	}

	if err := s.Store.Put(state.Entry{ID: id, Hash: hash, Start: start, Status: state.StatusUploading}); err != nil {
		return s.fail(id, hash, start, fmt.Errorf("ledger: %w", err))
	}

	err = s.uploadBytes(ctx, fixed.Bytes())
	switch {
	case errors.Is(err, garmin.ErrDuplicate):
		return s.markUploaded(id, hash, start, stats, err, StatusDuplicate)
	case err != nil:
		return UploadResult{Status: StatusFailed, Stats: stats, Err: s.recordFailure(id, hash, start, fmt.Errorf("upload: %w", err))}
	}
	return s.markUploaded(id, hash, start, stats, nil, StatusUploaded)
}

// markUploaded records a confirmed upload. If the ledger write fails the ride
// is still on Garmin, so the status stays uploaded and the ledger error is
// returned as a warning.
func (s *Service) markUploaded(id, hash string, start time.Time, stats fit.Stats, cause error, status string) UploadResult {
	err := s.Store.Put(state.Entry{ID: id, Hash: hash, Start: start, Status: state.StatusUploaded})
	if err != nil {
		s.Log.Error("ledger write failed after upload", "id", id, "err", err)
		return UploadResult{Status: status, Stats: stats, Err: fmt.Errorf("ledger write failed: %w", err)}
	}
	return UploadResult{Status: status, Stats: stats, Err: cause}
}

// checkRemote asks Garmin whether the ride already exists. It returns skip=true
// when the ride must not be uploaded: either a match was found (recorded as
// uploaded) or the check failed (the ride is never uploaded unverified). A zero
// start time disables the check.
func (s *Service) checkRemote(ctx context.Context, id string, start time.Time) (UploadResult, bool) {
	if s.Remote == nil {
		return UploadResult{}, false
	}
	if start.IsZero() {
		return UploadResult{Status: StatusFailed, Err: errors.New("no start time, cannot check Garmin for duplicates")}, true
	}
	gid, found, err := s.Remote.FindMatch(ctx, start)
	if err != nil {
		return UploadResult{Status: StatusFailed, Err: fmt.Errorf("remote check: %w", err)}, true
	}
	if !found {
		return UploadResult{}, false
	}
	if err := s.Store.Put(state.Entry{ID: id, GarminID: gid, Start: start, Status: state.StatusUploaded}); err != nil {
		return UploadResult{Status: StatusFailed, Err: fmt.Errorf("ledger: %w", err)}, true
	}
	return UploadResult{Status: StatusSkipped, Reason: ReasonRemoteMatch}, true
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

func (s *Service) fail(id, hash string, start time.Time, err error) UploadResult {
	return UploadResult{Status: StatusFailed, Err: s.recordFailure(id, hash, start, err)}
}

// recordFailure stores the failure in the ledger and returns err, or the
// ledger error if that also fails.
func (s *Service) recordFailure(id, hash string, start time.Time, err error) error {
	if hash == "" {
		if prev, ok := s.Store.ByID(id); ok {
			hash = prev.Hash
		}
	}
	if merr := s.Store.Put(state.Entry{ID: id, Hash: hash, Start: start, Status: state.StatusFailed, LastError: err.Error()}); merr != nil {
		return fmt.Errorf("%w (ledger: %v)", err, merr)
	}
	return err
}

// DescribeUpload summarises a result for TUI display.
func (r UploadResult) DescribeUpload() string {
	switch r.Status {
	case StatusUploaded:
		if r.Err != nil {
			return "uploaded, " + r.Err.Error()
		}
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

// SaveSpoofed downloads act, fixes and spoofs it like an upload would, and
// writes the result to dir as <id>.fit with mode 0600. It touches neither the
// ledger nor Garmin. Only call it on explicit user request.
func (s *Service) SaveSpoofed(ctx context.Context, act mywhoosh.Activity, dir string) (string, error) {
	name, err := safeFileStem(act.ID)
	if err != nil {
		return "", err
	}
	raw, err := s.MyWhoosh.DownloadFit(ctx, fileIDOf(act))
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	var fixed bytes.Buffer
	if _, err := fit.Fix(bytes.NewReader(raw), &fixed, s.Device); err != nil {
		return "", fmt.Errorf("fix fit: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	path := filepath.Join(dir, name+".fit")
	if err := fsutil.WriteFileAtomic(path, fixed.Bytes(), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func fileIDOf(act mywhoosh.Activity) string {
	if act.ActivityFileID != "" {
		return act.ActivityFileID
	}
	return act.ID
}

// safeFileStem accepts only letters, digits, '-' and '_', so an ID from the
// network can never name a path outside the target directory.
func safeFileStem(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("empty activity id")
	}
	for _, r := range id {
		ok := r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !ok {
			return "", fmt.Errorf("unsafe activity id %q", id)
		}
	}
	return id, nil
}
