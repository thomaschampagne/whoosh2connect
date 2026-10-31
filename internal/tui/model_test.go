package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"whoosh2connect/internal/app"
	"whoosh2connect/internal/garmin"
	"whoosh2connect/internal/mywhoosh"
)

type fakeService struct {
	sessions []app.Session
	loginErr error
	mfaErr   error
	uploaded []string
	uploads  int
}

func (f *fakeService) LoginMyWhoosh(ctx context.Context, e, p string) error {
	if f.loginErr != nil {
		return f.loginErr
	}
	return nil
}
func (f *fakeService) LoginGarmin(ctx context.Context, e, p string) error {
	return f.mfaErr
}
func (f *fakeService) SubmitMFA(ctx context.Context, code string) error { return nil }
func (f *fakeService) Sessions(ctx context.Context) ([]app.Session, error) {
	return f.sessions, nil
}
func (f *fakeService) Upload(ctx context.Context, act mywhoosh.Activity) app.UploadResult {
	f.uploads++
	f.uploaded = append(f.uploaded, act.ID)
	return app.UploadResult{Status: app.StatusUploaded}
}

func activity(id string) mywhoosh.Activity {
	return mywhoosh.Activity{ID: id, Name: "Ride " + id}
}

func session(id string) app.Session {
	return app.Session{Activity: activity(id)}
}

// boot returns the bootstrap command's message (skipping the spinner tick).
func boot(t *testing.T, m Model) tea.Msg {
	t.Helper()
	cmd := m.Init()
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("init cmd = %T, want BatchMsg", cmd())
	}
	return batch[1]()
}

// upd applies msg and returns the updated Model.
func upd(t *testing.T, m *Model, msg tea.Msg) tea.Cmd {
	t.Helper()
	mm, cmd := m.Update(msg)
	*m = mm.(Model)
	return cmd
}

func TestBootstrapSuccessShowsList(t *testing.T) {
	f := &fakeService{sessions: []app.Session{session("1"), session("2")}}
	m := New(Options{Service: f, Settings: Settings{MywhooshEmail: "a", GarminEmail: "b"}})
	upd(t, &m, boot(t, m))
	if m.screen != screenList {
		t.Fatalf("screen = %v", m.screen)
	}
	if len(m.sessions) != 2 {
		t.Fatalf("sessions = %d", len(m.sessions))
	}
}

func TestBootstrapErrorGoesToSettings(t *testing.T) {
	f := &fakeService{loginErr: context.Canceled}
	m := New(Options{Service: f})
	upd(t, &m, boot(t, m))
	if m.screen != screenSettings {
		t.Fatalf("screen = %v", m.screen)
	}
	if m.status == "" {
		t.Fatal("expected error status")
	}
}

func TestBootstrapMFA(t *testing.T) {
	f := &fakeService{mfaErr: garmin.ErrMFARequired}
	m := New(Options{Service: f})
	cmd := upd(t, &m, boot(t, m))
	if cmd == nil {
		t.Fatal("expected mfa input focus")
	}
	if m.screen != screenMFA {
		t.Fatalf("screen = %v", m.screen)
	}
}

func TestMFAEnterSubmits(t *testing.T) {
	f := &fakeService{}
	m := New(Options{Service: f})
	m.screen = screenMFA
	m.mfaInput.SetValue("123456")

	cmd := upd(t, &m, tea.KeyMsg{Type: tea.KeyEnter})
	mMsg := cmd().(mfaMsg)
	if mMsg.err != nil {
		t.Fatal(mMsg.err)
	}
	cmd = upd(t, &m, mMsg)
	if cmd == nil {
		t.Fatal("expected refresh cmd after MFA success")
	}
}

func TestMFAEscCancels(t *testing.T) {
	m := New(Options{Service: &fakeService{}})
	m.screen = screenMFA
	upd(t, &m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.screen != screenList {
		t.Fatalf("screen = %v", m.screen)
	}
}

func TestUploadSelected(t *testing.T) {
	f := &fakeService{sessions: []app.Session{session("7")}}
	m := New(Options{Service: f})
	m.screen = screenList
	m.sessions = f.sessions

	cmd := upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	mMsg := cmd().(uploadMsg)
	upd(t, &m, mMsg)
	if f.uploads != 1 || len(f.uploaded) != 1 || f.uploaded[0] != "7" {
		t.Fatalf("uploads = %v", f.uploaded)
	}
	if m.results["7"] == "" {
		t.Fatal("no result recorded")
	}
}

func TestUploadAllConfirm(t *testing.T) {
	f := &fakeService{sessions: []app.Session{
		session("1"),
		{Activity: activity("2"), Uploaded: true},
		session("3"),
	}}
	m := New(Options{Service: f})
	m.screen = screenList
	m.sessions = f.sessions

	upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.screen != screenConfirm {
		t.Fatalf("screen = %v", m.screen)
	}
	if m.confirm != 2 {
		t.Fatalf("confirm = %d", m.confirm)
	}

	upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if m.screen != screenList {
		t.Fatalf("confirm-n screen = %v", m.screen)
	}
	if f.uploads != 0 {
		t.Fatal("cancel must not upload")
	}

	upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.screen != screenConfirm {
		t.Fatalf("screen = %v", m.screen)
	}
	cmd := upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	mMsg := cmd().(uploadMsg)
	upd(t, &m, mMsg)
	if f.uploads != 2 {
		t.Fatalf("uploads = %d, want 2", f.uploads)
	}
}

func TestSettingsNavigateAndSave(t *testing.T) {
	f := &fakeService{sessions: []app.Session{session("1")}}
	saved := Settings{}
	m := New(Options{
		Service: f,
		SaveSettings: func(s Settings) error {
			saved = s
			return nil
		},
	})
	m.screen = screenList

	upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if m.screen != screenSettings {
		t.Fatalf("screen = %v", m.screen)
	}

	upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("me@x.com")})
	if fieldValue(m.settings, 0) != "me@x.com" {
		t.Fatalf("field0 = %q", fieldValue(m.settings, 0))
	}

	// Invalid save stays on the form, shows an error, and focuses the first
	// offending field (Garmin email).
	upd(t, &m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.screen != screenSettings || m.status == "" {
		t.Fatalf("invalid save: screen=%v status=%q", m.screen, m.status)
	}
	if m.form.focus != fldGarminEmail {
		t.Fatalf("focus = %d, want Garmin email", m.form.focus)
	}

	// shift+tab navigates backwards (form nav fix).
	upd(t, &m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.form.focus != fldMywhooshPassword {
		t.Fatalf("shift+tab focus = %d, want MyWhoosh password", m.form.focus)
	}

	upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("mwpass")})
	for _, v := range []string{"g@x.com", "gpass", "edge_540"} {
		upd(t, &m, tea.KeyMsg{Type: tea.KeyTab})
		upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(v)})
	}

	cmd := upd(t, &m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.screen != screenLoading {
		t.Fatalf("screen = %v", m.screen)
	}
	if cmd == nil {
		t.Fatal("expected bootstrap re-login after save")
	}
	want := Settings{MywhooshEmail: "me@x.com", MywhooshPassword: "mwpass", GarminEmail: "g@x.com", GarminPassword: "gpass", DevicePreset: "edge_540", DeviceName: "", DeviceSerial: ""}
	if saved != want {
		t.Fatalf("saved = %+v", saved)
	}
}

func TestViewShowsKeybindingsAndDevice(t *testing.T) {
	f := &fakeService{sessions: []app.Session{session("1")}}
	m := New(Options{Service: f, Settings: Settings{DevicePreset: "edge_540"}})
	m.screen = screenList
	m.showHelp = true

	v := m.View()
	for _, want := range []string{"upload all", "edge_540", "whoosh2connect"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestDarkOption(t *testing.T) {
	m := New(Options{Service: &fakeService{}, Dark: true})
	if m.th.text != lightTheme.text {
		t.Error("dark=true should select light theme")
	}
}

func TestPendingCount(t *testing.T) {
	f := &fakeService{sessions: []app.Session{
		session("1"),
		{Activity: activity("2"), Uploaded: true},
	}}
	m := New(Options{Service: f})
	m.screen = screenList
	m.sessions = f.sessions
	if n := m.pendingCount(); n != 1 {
		t.Fatalf("pending = %d", n)
	}
}

func (f *fakeService) UploadPending(ctx context.Context, sessions []app.Session, report func(mywhoosh.Activity, app.UploadResult)) {
	for _, s := range sessions {
		if s.Uploaded {
			continue
		}
		report(s.Activity, f.Upload(ctx, s.Activity))
	}
}

func TestMouseToggleKey(t *testing.T) {
	m := New(Options{Service: &fakeService{}})
	if !m.mouseOn {
		t.Fatal("mouse should start on")
	}
	m.screen = screenList
	if cmd := upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}); cmd == nil || m.mouseOn {
		t.Fatalf("first m: mouseOn = %v, want off with a disable cmd", m.mouseOn)
	}
	if cmd := upd(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}); cmd == nil || !m.mouseOn {
		t.Fatalf("second m: mouseOn = %v, want on with an enable cmd", m.mouseOn)
	}
}
