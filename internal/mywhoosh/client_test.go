package mywhoosh

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestClient starts a fake MyWhoosh API and returns a client pointed at it.
func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New(Options{
		LoginURL:      srv.URL + "/login",
		ActivitiesURL: srv.URL + "/v2/",
		TokenFile:     filepath.Join(t.TempDir(), "tokens", "mywhoosh_token.json"),
	})
	return c, srv
}

func TestLoginSuccessPersistsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("bad body: %v", err)
		}
		if body["Username"] != "me@x" || body["Platform"] != "Android" {
			t.Errorf("payload = %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Success":true,"Message":"","AccessToken":"abc","RefreshToken":"r","WhooshId":"w1"}`))
	}))
	defer srv.Close()

	tokenFile := filepath.Join(t.TempDir(), "mywhoosh_token.json")
	c := New(Options{LoginURL: srv.URL + "/login", TokenFile: tokenFile})
	if err := c.Login(context.Background(), "me@x", "pw"); err != nil {
		t.Fatal(err)
	}
	if !c.IsAuthenticated() {
		t.Fatal("not authenticated after login")
	}
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	var ct cachedToken
	if err := json.Unmarshal(data, &ct); err != nil {
		t.Fatal(err)
	}
	if ct.AccessToken != "abc" || ct.WhooshID != "w1" {
		t.Errorf("cached = %+v", ct)
	}
	info, _ := os.Stat(tokenFile)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 600", info.Mode().Perm())
	}
}

func TestLoginFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Success":false,"Message":"bad credentials"}`))
	}))
	defer srv.Close()

	c := New(Options{LoginURL: srv.URL + "/login"})
	err := c.Login(context.Background(), "me@x", "wrong")
	if err == nil || !strings.Contains(err.Error(), "bad credentials") {
		t.Fatalf("err = %v", err)
	}
}

func TestResumeCachedSession(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "mywhoosh_token.json")
	if err := os.MkdirAll(filepath.Dir(tokenFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenFile, []byte(`{"access_token":"cached","whoosh_id":"w9"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	c.opts.TokenFile = tokenFile
	if err := c.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.token != "cached" {
		t.Errorf("token = %q", c.token)
	}
}

func TestResumeMissing(t *testing.T) {
	c := New(Options{TokenFile: filepath.Join(t.TempDir(), "nope.json")})
	if err := c.Resume(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestListActivitiesPaginates(t *testing.T) {
	var pages int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/rider/profile/activities" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer abc" {
			t.Errorf("auth = %q", got)
		}
		pages++
		if pages == 1 {
			_, _ = w.Write([]byte(`{"data":{"totalPages":2,"results":[{"id":"a1","title":"Morning","watt":210,"distance":40.1}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"totalPages":2,"results":[{"id":"a2","title":"Evening"}]}}`))
	}))
	defer srv.Close()

	c := New(Options{ActivitiesURL: srv.URL + "/v2/"})
	c.token = "abc"
	acts, err := c.ListActivities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 2 {
		t.Fatalf("len = %d", len(acts))
	}
	if acts[0].ID != "a1" || acts[0].AvgPower != 210 {
		t.Errorf("first = %+v", acts[0])
	}
	if acts[1].DisplayName() != "Evening" {
		t.Errorf("second name = %q", acts[1].DisplayName())
	}
}

func TestDownloadFit(t *testing.T) {
	fitData := []byte{0x0E, 0x10, 0x01, 0x02}
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fitData)
	}))
	defer fileServer.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/rider/profile/download-activity-file" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":"` + fileServer.URL + `/file.fit"}`))
	}))
	defer srv.Close()

	c := New(Options{ActivitiesURL: srv.URL + "/v2/"})
	c.token = "abc"
	got, err := c.DownloadFit(context.Background(), "f1")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(fitData) {
		t.Errorf("data = %x", got)
	}
}

func TestDownloadFitNoURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":""}`))
	}))
	defer srv.Close()
	c := New(Options{ActivitiesURL: srv.URL + "/v2/"})
	c.token = "abc"
	if _, err := c.DownloadFit(context.Background(), "f1"); err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestNotAuthenticated(t *testing.T) {
	c := New(Options{})
	if _, err := c.ListActivities(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if _, err := c.DownloadFit(context.Background(), "f1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParsedStart(t *testing.T) {
	a := &Activity{StartDatetime: "2026-01-02T03:04:05.000Z"}
	if got := a.ParsedStart(); got.Format(time.RFC3339) != "2026-01-02T03:04:05Z" {
		t.Errorf("start = %s", got)
	}
	// fallback: epoch seconds
	a = &Activity{Date: 1700000000}
	if got := a.ParsedStart(); got.Unix() != 1700000000 {
		t.Errorf("fallback start = %d", got.Unix())
	}
}

func TestDownloadFitEscapesFileID(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rider/profile/download-activity-file" {
			gotBody, _ = io.ReadAll(r.Body)
			_, _ = w.Write([]byte(`{"data":"` + "http://" + r.Host + `/f.fit"}`))
			return
		}
		_, _ = w.Write([]byte("fit"))
	}))
	defer srv.Close()
	c := New(Options{ActivitiesURL: srv.URL + "/", HTTPClient: srv.Client()})
	c.token = "tok"
	fileID := `a","x":"b`
	if _, err := c.DownloadFit(context.Background(), fileID); err != nil {
		t.Fatal(err)
	}
	var body map[string]string
	if err := json.Unmarshal(gotBody, &body); err != nil {
		t.Fatalf("payload not valid JSON: %v (%s)", err, gotBody)
	}
	if body["fileId"] != fileID {
		t.Errorf("fileId = %q, want %q", body["fileId"], fileID)
	}
}

func TestListActivitiesStopsAtPageCap(t *testing.T) {
	var calls int
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"data":{"results":[{"id":"x"}],"totalPages":1000000}}`))
	}))
	c.token = "abc"
	acts, err := c.ListActivities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != maxActivityPages {
		t.Errorf("requests = %d, want cap %d", calls, maxActivityPages)
	}
	if len(acts) != maxActivityPages {
		t.Errorf("activities = %d, want %d", len(acts), maxActivityPages)
	}
}
