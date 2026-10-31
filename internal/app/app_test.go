package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"whoosh2connect/internal/device"
	"whoosh2connect/internal/garmin"
	"whoosh2connect/internal/mywhoosh"
	"whoosh2connect/internal/state"
)

type fakeMyWhoosh struct {
	acts []mywhoosh.Activity
	fits map[string][]byte
	auth bool
}

func (f *fakeMyWhoosh) IsAuthenticated() bool            { return f.auth }
func (f *fakeMyWhoosh) Resume(ctx context.Context) error { return errors.New("no session") }
func (f *fakeMyWhoosh) Login(ctx context.Context, e, p string) error {
	f.auth = true
	return nil
}
func (f *fakeMyWhoosh) ListActivities(ctx context.Context) ([]mywhoosh.Activity, error) {
	return f.acts, nil
}
func (f *fakeMyWhoosh) DownloadFit(ctx context.Context, fileID string) ([]byte, error) {
	data, ok := f.fits[fileID]
	if !ok {
		return nil, errors.New("fit not found")
	}
	return data, nil
}

type fakeGarmin struct {
	err     error
	upload  func(path string) error
	uploads int
	path    string
}

func (f *fakeGarmin) Resume(ctx context.Context) error { return errors.New("no session") }
func (f *fakeGarmin) Login(ctx context.Context, e, p string) error {
	return f.err
}
func (f *fakeGarmin) SubmitMFA(ctx context.Context, code string) error {
	return nil
}
func (f *fakeGarmin) UploadFIT(ctx context.Context, filePath string) error {
	f.uploads++
	f.path = filePath
	if f.upload != nil {
		return f.upload(filePath)
	}
	return nil
}

func fixture() []byte {
	return fixtureAt(time.Now().Truncate(time.Second))
}

// fixtureAt builds an activity FIT starting at now. Different starts give
// different bytes, so distinct rides do not collide on the FIT hash.
func fixtureAt(now time.Time) []byte {
	activity := filedef.NewActivity()
	activity.FileId.
		SetType(typedef.FileActivity).
		SetTimeCreated(now).
		SetManufacturer(typedef.ManufacturerDevelopment).
		SetProduct(0)
	for i := 0; i < 3; i++ {
		activity.Records = append(activity.Records,
			mesgdef.NewRecord(nil).
				SetTimestamp(now.Add(time.Duration(i)*time.Second)).
				SetPower(200).
				SetHeartRate(150).
				SetCadence(90).
				SetTemperature(25))
	}
	activity.Sessions = append(activity.Sessions,
		mesgdef.NewSession(nil).
			SetTimestamp(now.Add(3*time.Second)).
			SetStartTime(now).
			SetSport(typedef.SportCycling))
	activity.Activity = mesgdef.NewActivity(nil).
		SetTimestamp(now.Add(3 * time.Second)).
		SetType(typedef.ActivityManual).
		SetNumSessions(1)

	f := activity.ToFIT(nil)
	var buf bytes.Buffer
	if err := encoder.New(&buf).Encode(&f); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func newService(t *testing.T, mw *fakeMyWhoosh, g *fakeGarmin) *Service {
	t.Helper()
	st, err := state.Open(t.TempDir() + "/state.json")
	if err != nil {
		t.Fatal(err)
	}
	return New(mw, g, st, device.Device{
		Preset: device.Presets()[0],
		Serial: 1234,
	}, nil)
}

func activity() mywhoosh.Activity {
	return mywhoosh.Activity{ID: "act-1", ActivityFileID: "file-1", Name: "Indoor Ride", StartDatetime: "2026-01-08T14:10:00.000Z"}
}

func TestSessionsAnnotated(t *testing.T) {
	mw := &fakeMyWhoosh{acts: []mywhoosh.Activity{activity()}, fits: map[string][]byte{"file-1": fixture()}}
	g := &fakeGarmin{}
	svc := newService(t, mw, g)

	sessions, err := svc.Sessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].Uploaded {
		t.Fatalf("sessions = %+v", sessions)
	}

	res := svc.Upload(context.Background(), sessions[0].Activity)
	if res.Status != StatusUploaded {
		t.Fatalf("status = %s (%v)", res.Status, res.Err)
	}
	sessions, _ = svc.Sessions(context.Background())
	if !sessions[0].Uploaded {
		t.Fatal("session not marked uploaded")
	}
	// Idempotence: second upload skips.
	res = svc.Upload(context.Background(), sessions[0].Activity)
	if res.Status != StatusSkipped {
		t.Fatalf("status = %s", res.Status)
	}
	if g.uploads != 1 {
		t.Fatalf("uploads = %d, want 1", g.uploads)
	}
}

func TestUploadDuplicateFromGarmin(t *testing.T) {
	mw := &fakeMyWhoosh{fits: map[string][]byte{"file-1": fixture()}}
	g := &fakeGarmin{upload: func(path string) error { return garmin.ErrDuplicate }}
	svc := newService(t, mw, g)

	res := svc.Upload(context.Background(), activity())
	if res.Status != StatusDuplicate {
		t.Fatalf("status = %s (%v)", res.Status, res.Err)
	}
	if !isUploaded(svc.Store, "act-1") {
		t.Fatal("duplicate should be marked uploaded")
	}
}

func TestUploadDownloadFailure(t *testing.T) {
	mw := &fakeMyWhoosh{fits: map[string][]byte{}}
	svc := newService(t, mw, &fakeGarmin{})
	res := svc.Upload(context.Background(), activity())
	if res.Status != StatusFailed {
		t.Fatalf("status = %s", res.Status)
	}
}

func TestUploadGarminFailure(t *testing.T) {
	mw := &fakeMyWhoosh{fits: map[string][]byte{"file-1": fixture()}}
	g := &fakeGarmin{upload: func(path string) error { return errors.New("boom") }}
	svc := newService(t, mw, g)
	res := svc.Upload(context.Background(), activity())
	if res.Status != StatusFailed {
		t.Fatalf("status = %s", res.Status)
	}
	if isUploaded(svc.Store, "act-1") {
		t.Fatal("failed upload must not be marked")
	}
}

func TestLoginGarminMFAPropagates(t *testing.T) {
	g := &fakeGarmin{err: garmin.ErrMFARequired}
	svc := newService(t, &fakeMyWhoosh{}, g)
	err := svc.LoginGarmin(context.Background(), "e", "p")
	if !errors.Is(err, garmin.ErrMFARequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestLoginMyWhooshResumeOrLogin(t *testing.T) {
	mw := &fakeMyWhoosh{}
	svc := newService(t, mw, &fakeGarmin{})
	if err := svc.LoginMyWhoosh(context.Background(), "e", "p"); err != nil {
		t.Fatal(err)
	}
	if !mw.auth {
		t.Fatal("mywhoosh not logged in")
	}
}

func TestProcessLocalFitUploadsAndDedupes(t *testing.T) {
	data := fixture()
	path := filepath.Join(t.TempDir(), "MyNewActivity-1.0.fit")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	id := "local:hash"

	mw := &fakeMyWhoosh{}
	g := &fakeGarmin{}
	svc := newService(t, mw, g)

	res := svc.ProcessLocalFit(context.Background(), id, path)
	if res.Status != StatusUploaded {
		t.Fatalf("status = %s (%v)", res.Status, res.Err)
	}
	if g.uploads != 1 {
		t.Fatalf("uploads = %d, want 1", g.uploads)
	}
	if !isUploaded(svc.Store, id) {
		t.Fatal("local fit not marked uploaded")
	}

	res = svc.ProcessLocalFit(context.Background(), id, path)
	if res.Status != StatusSkipped {
		t.Fatalf("second call status = %s", res.Status)
	}
}

func TestProcessLocalFitDuplicate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "MyNewActivity-1.0.fit")
	if err := os.WriteFile(path, fixture(), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &fakeGarmin{upload: func(p string) error { return garmin.ErrDuplicate }}
	svc := newService(t, &fakeMyWhoosh{}, g)

	res := svc.ProcessLocalFit(context.Background(), "local:dup", path)
	if res.Status != StatusDuplicate {
		t.Fatalf("status = %s (%v)", res.Status, res.Err)
	}
	if !isUploaded(svc.Store, "local:dup") {
		t.Fatal("duplicate should be marked uploaded")
	}
}

func TestProcessLocalFitReadFailure(t *testing.T) {
	svc := newService(t, &fakeMyWhoosh{}, &fakeGarmin{})
	res := svc.ProcessLocalFit(context.Background(), "local:x", filepath.Join(t.TempDir(), "missing.fit"))
	if res.Status != StatusFailed {
		t.Fatalf("status = %s", res.Status)
	}
}

func TestDescribeUpload(t *testing.T) {
	cases := []struct {
		r    UploadResult
		want string
	}{
		{UploadResult{Status: StatusUploaded}, "uploaded"},
		{UploadResult{Status: StatusDuplicate}, "already on Garmin (duplicate)"},
		{UploadResult{Status: StatusSkipped}, "skipped (already synced)"},
		{UploadResult{Status: StatusFailed, Err: errors.New("nope")}, "nope"},
	}
	for _, c := range cases {
		if got := c.r.DescribeUpload(); got != c.want {
			t.Errorf("DescribeUpload(%q) = %q", c.r.Status, got)
		}
	}
}

func isUploaded(st *state.Store, id string) bool {
	e, ok := st.ByID(id)
	return ok && e.Status == state.StatusUploaded
}

func TestUploadLedgerPaths(t *testing.T) {
	fit := fixture()
	act := func(id string) mywhoosh.Activity {
		return mywhoosh.Activity{ID: id, ActivityFileID: "f-" + id}
	}
	tests := []struct {
		name        string
		seed        func(*testing.T, *Service)
		garminErr   error
		wantStatus  string
		wantReason  string
		wantUploads int
		wantLedger  state.Status
	}{
		{
			name:        "fresh ride uploads once",
			wantStatus:  StatusUploaded,
			wantUploads: 1,
			wantLedger:  state.StatusUploaded,
		},
		{
			name: "same fit under new id is a hash hit",
			seed: func(t *testing.T, s *Service) {
				if r := s.Upload(context.Background(), act("first")); r.Status != StatusUploaded {
					t.Fatalf("seed: %+v", r)
				}
			},
			wantStatus:  StatusSkipped,
			wantReason:  ReasonLedgerHash,
			wantUploads: 0,
			wantLedger:  state.StatusUploaded,
		},
		{
			name: "already uploaded id is skipped",
			seed: func(t *testing.T, s *Service) {
				if err := s.Store.Put(state.Entry{ID: "a1", Status: state.StatusUploaded}); err != nil {
					t.Fatal(err)
				}
			},
			wantStatus:  StatusSkipped,
			wantReason:  ReasonLedgerID,
			wantUploads: 0,
			wantLedger:  state.StatusUploaded,
		},
		{
			name:        "lost response leaves failed entry for retry",
			garminErr:   errors.New("connection reset"),
			wantStatus:  StatusFailed,
			wantUploads: 1,
			wantLedger:  state.StatusFailed,
		},
		{
			name:        "garmin 409 is recorded as uploaded",
			garminErr:   garmin.ErrDuplicate,
			wantStatus:  StatusDuplicate,
			wantUploads: 1,
			wantLedger:  state.StatusUploaded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := &fakeMyWhoosh{fits: map[string][]byte{"f-a1": fit, "f-first": fit, "f-second": fit}}
			g := &fakeGarmin{err: nil}
			g.upload = func(string) error { return tt.garminErr }
			svc := newService(t, mw, g)
			if tt.seed != nil {
				tt.seed(t, svc)
				g.uploads = 0
			}
			id := "a1"
			if tt.name == "same fit under new id is a hash hit" {
				id = "second"
			}
			res := svc.Upload(context.Background(), act(id))
			if res.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q (err %v)", res.Status, tt.wantStatus, res.Err)
			}
			if res.Reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", res.Reason, tt.wantReason)
			}
			if g.uploads != tt.wantUploads {
				t.Errorf("uploads = %d, want %d", g.uploads, tt.wantUploads)
			}
			e, ok := svc.Store.ByID(id)
			if !ok || e.Status != tt.wantLedger {
				t.Errorf("ledger = %+v ok=%v, want %q", e, ok, tt.wantLedger)
			}
		})
	}
}

type fakeRemote struct {
	found bool
	err   error
	calls int
}

func (f *fakeRemote) FindMatch(ctx context.Context, start time.Time) (string, bool, error) {
	f.calls++
	return "g-9", f.found, f.err
}

func TestUploadRemoteGuard(t *testing.T) {
	tests := []struct {
		name        string
		remote      *fakeRemote
		wantStatus  string
		wantReason  string
		wantUploads int
		wantLedger  state.Status
	}{
		{"no checker uploads", nil, StatusUploaded, "", 1, state.StatusUploaded},
		{"no remote match uploads", &fakeRemote{}, StatusUploaded, "", 1, state.StatusUploaded},
		{"remote match skips and records", &fakeRemote{found: true}, StatusSkipped, ReasonRemoteMatch, 0, state.StatusUploaded},
		{"remote error never uploads", &fakeRemote{err: errors.New("http 503")}, StatusFailed, "", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := &fakeMyWhoosh{fits: map[string][]byte{"f-a1": fixture()}}
			g := &fakeGarmin{}
			svc := newService(t, mw, g)
			if tt.remote != nil {
				svc.Remote = tt.remote
			}
			res := svc.Upload(context.Background(), mywhoosh.Activity{ID: "a1", ActivityFileID: "f-a1", StartDatetime: "2026-01-08T14:10:00.000Z"})
			if res.Status != tt.wantStatus || res.Reason != tt.wantReason {
				t.Errorf("got %q/%q, want %q/%q (err %v)", res.Status, res.Reason, tt.wantStatus, tt.wantReason, res.Err)
			}
			if g.uploads != tt.wantUploads {
				t.Errorf("uploads = %d, want %d", g.uploads, tt.wantUploads)
			}
			if tt.wantLedger != "" {
				e, ok := svc.Store.ByID("a1")
				if !ok || e.Status != tt.wantLedger {
					t.Errorf("ledger = %+v ok=%v, want %q", e, ok, tt.wantLedger)
				}
			}
		})
	}
}

func TestPlan(t *testing.T) {
	tests := []struct {
		name       string
		seed       *state.Entry
		remote     *fakeRemote
		wantAction string
		wantReason string
		wantErr    bool
	}{
		{"new ride would upload", nil, nil, ActionUpload, "", false},
		{"ledger id hit skips", &state.Entry{ID: "a1", Status: state.StatusUploaded}, nil, ActionSkip, ReasonLedgerID, false},
		{"remote match skips", nil, &fakeRemote{found: true}, ActionSkip, ReasonRemoteMatch, false},
		{"remote error surfaces", nil, &fakeRemote{err: errors.New("http 503")}, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &fakeGarmin{}
			svc := newService(t, &fakeMyWhoosh{}, g)
			if tt.seed != nil {
				if err := svc.Store.Put(*tt.seed); err != nil {
					t.Fatal(err)
				}
			}
			if tt.remote != nil {
				svc.Remote = tt.remote
			}
			d, err := svc.Plan(context.Background(), mywhoosh.Activity{ID: "a1"})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if d.Action != tt.wantAction || d.Reason != tt.wantReason {
				t.Errorf("got %+v, want action %q reason %q", d, tt.wantAction, tt.wantReason)
			}
			if g.uploads != 0 {
				t.Errorf("plan uploaded %d times, want 0", g.uploads)
			}
			if _, ok := svc.Store.ByID("a1"); ok && tt.seed == nil {
				t.Error("plan wrote to ledger")
			}
		})
	}
}
