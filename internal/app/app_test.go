package app

import (
	"bytes"
	"context"
	"errors"
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

func (f *fakeGarmin) Resume(ctx context.Context) error { return nil }
func (f *fakeGarmin) Login(ctx context.Context, e, p string) error {
	return f.err
}
func (f *fakeGarmin) NeedsMFA() bool { return f.err == garmin.ErrMFARequired }
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
	now := time.Now().Truncate(time.Second)
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
	})
}

func activity() mywhoosh.Activity {
	return mywhoosh.Activity{ID: "act-1", ActivityFileID: "file-1", Name: "Indoor Ride"}
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
	if !svc.Store.IsUploaded("act-1") {
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
	if svc.Store.IsUploaded("act-1") {
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
