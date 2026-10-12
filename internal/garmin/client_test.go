package garmin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUploadFITMultipart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/upload-service/upload" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer at" {
			t.Errorf("auth = %q", auth)
		}
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "multipart/form-data") {
			t.Errorf("content-type = %q", ct)
		}
		if ua := r.Header.Get("User-Agent"); ua != apiUserAgent {
			t.Errorf("ua = %q", ua)
		}
		if r.Header.Get("DI-Backend") == "" || r.Header.Get("NK") == "" {
			t.Error("missing DI-Backend/NK headers")
		}
		_, _ = w.Write([]byte(`{"detailedImportResult":{"successes":[{"internalId":1}]}}`))
	}))
	defer srv.Close()

	c := testClient(srv)
	c.OAuth2 = &OAuth2Token{AccessToken: "at", TokenType: "Bearer",
		ExpiresAt: time.Now().Add(time.Hour).Unix()}
	f := fitFile(t)
	if err := c.UploadFIT(context.Background(), f); err != nil {
		t.Fatal(err)
	}
}

func TestUploadFITDuplicate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`duplicate`))
	}))
	defer srv.Close()

	c := testClient(srv)
	c.OAuth2 = &OAuth2Token{AccessToken: "at", TokenType: "Bearer",
		ExpiresAt: time.Now().Add(time.Hour).Unix()}
	err := c.UploadFIT(context.Background(), fitFile(t))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err = %v, want duplicate", err)
	}
}

func TestUploadFITReportedFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"detailedImportResult":{"failures":[{"type":"fitParseException"}]}}`))
	}))
	defer srv.Close()

	c := testClient(srv)
	c.OAuth2 = &OAuth2Token{AccessToken: "at", TokenType: "Bearer",
		ExpiresAt: time.Now().Add(time.Hour).Unix()}
	err := c.UploadFIT(context.Background(), fitFile(t))
	if err == nil || !strings.Contains(err.Error(), "failures") {
		t.Fatalf("err = %v, want failures", err)
	}
}

func TestUploadFITNotAuthenticated(t *testing.T) {
	c := testClient(nil)
	if err := c.UploadFIT(context.Background(), fitFile(t)); err == nil {
		t.Fatal("expected not-authenticated error")
	}
}

func TestUploadFITRefreshesExpiredAndRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth_consumer.json":
			_, _ = w.Write([]byte(testConsumer))
		case "/oauth-service/oauth/exchange/user/2.0":
			_, _ = w.Write([]byte(`{"scope":"s","token_type":"Bearer","access_token":"at2","refresh_token":"rt2","expires_in":9000,"refresh_token_expires_in":90000000}`))
		case "/upload-service/upload":
			if auth := r.Header.Get("Authorization"); auth != "Bearer at2" {
				t.Errorf("upload auth = %q, want refreshed at2", auth)
			}
			_, _ = w.Write([]byte(`{"detailedImportResult":{"successes":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := testClient(srv)
	c.OAuth1 = &OAuth1Token{OAuthToken: "o", OAuthTokenSecret: "s", Domain: "garmin.com"}
	// Force refresh path: expired OAuth2.
	c.OAuth2 = &OAuth2Token{AccessToken: "at", TokenType: "Bearer",
		ExpiresAt: time.Now().Add(-time.Hour).Unix()}
	if err := c.UploadFIT(context.Background(), fitFile(t)); err != nil {
		t.Fatal(err)
	}
	if c.OAuth2.AccessToken != "at2" {
		t.Errorf("token not refreshed: %s", c.OAuth2.AccessToken)
	}
}

func TestResumeRefreshesExpired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth_consumer.json":
			_, _ = w.Write([]byte(testConsumer))
		case "/oauth-service/oauth/exchange/user/2.0":
			_, _ = w.Write([]byte(`{"scope":"s","token_type":"Bearer","access_token":"fresh","refresh_token":"rt","expires_in":9000,"refresh_token_expires_in":90000000}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	o1 := &OAuth1Token{OAuthToken: "o", OAuthTokenSecret: "s", Domain: "garmin.com"}
	o2 := &OAuth2Token{AccessToken: "old", TokenType: "Bearer",
		ExpiresAt: time.Now().Add(-time.Hour).Unix()}
	if err := SaveTokens(dir, o1, o2); err != nil {
		t.Fatal(err)
	}

	c := testClient(srv)
	c.TokenDir = dir
	if err := c.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.OAuth2.AccessToken != "fresh" {
		t.Errorf("oauth2 = %s", c.OAuth2.AccessToken)
	}
}

func TestLoginClientLevel(t *testing.T) {
	m, srv := newMock(t)
	m.loginResp = `{"responseStatus":{"type":"SUCCESSFUL","message":"ok"},"serviceTicketId":"t1"}`

	c := testClient(srv)
	if err := c.Login(context.Background(), "me@x", "pw"); err != nil {
		t.Fatal(err)
	}
	if c.NeedsMFA() {
		t.Fatal("unexpected MFA")
	}
	if c.OAuth1.OAuthToken != "ot1" || c.OAuth2.AccessToken != "at" {
		t.Fatalf("tokens not set: %+v / %+v", c.OAuth1, c.OAuth2)
	}
	if c.TokenDir != "" {
		if _, _, err := LoadTokens(c.TokenDir); err != nil {
			t.Errorf("tokens not persisted: %v", err)
		}
	}
}

func TestLoginClientLevelMFA(t *testing.T) {
	m, srv := newMock(t)
	m.loginResp = `{"responseStatus":{"type":"MFA_REQUIRED","message":"x"},"customerMfaInfo":{"mfaLastMethodUsed":"EMAIL"}}`
	m.verifyResp = `{"responseStatus":{"type":"SUCCESSFUL","message":"ok"},"serviceTicketId":"t2"}`

	c := testClient(srv)
	err := c.Login(context.Background(), "me@x", "pw")
	if err != ErrMFARequired {
		t.Fatalf("err = %v, want ErrMFARequired", err)
	}
	if !c.NeedsMFA() || c.MFAMethod() != "EMAIL" {
		t.Fatalf("needsMFA=%v method=%s", c.NeedsMFA(), c.MFAMethod())
	}
	if err := c.SubmitMFA(context.Background(), "654321"); err != nil {
		t.Fatal(err)
	}
	if c.NeedsMFA() {
		t.Fatal("still needs MFA after submit")
	}
	if m.gotVerify["mfaVerificationCode"] != "654321" {
		t.Errorf("verify = %+v", m.gotVerify)
	}
	if c.OAuth2.AccessToken != "at" {
		t.Errorf("token = %s", c.OAuth2.AccessToken)
	}
}

func TestSubmitMFANoLogin(t *testing.T) {
	c := testClient(nil)
	if err := c.SubmitMFA(context.Background(), "123"); err == nil {
		t.Fatal("expected error")
	}
}

// testClient returns a Client pointed at srv (nil Hosts to keep real hosts for
// non-HTTP tests) with no token dir.
func testClient(srv *httptest.Server) *Client {
	if srv == nil {
		return New(Options{Domain: "garmin.com"})
	}
	h := defaultHosts("garmin.com")
	h.sso = srv.URL
	h.connectAPI = srv.URL
	h.consumerURL = srv.URL + "/oauth_consumer.json"
	return New(Options{Domain: "garmin.com", Hosts: &h})
}

func fitFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "activity.fit")
	if err := os.WriteFile(p, []byte{0x0E, 0x10}, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseUploadResult(t *testing.T) {
	if err := parseUploadResult(200, []byte(`{}`)); err != nil {
		t.Errorf("200 {} -> %v", err)
	}
	if err := parseUploadResult(500, []byte(`boom`)); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("500 err = %v", err)
	}
}

func TestResumeNoTokens(t *testing.T) {
	c := testClient(nil)
	c.TokenDir = filepath.Join(t.TempDir(), "missing")
	if err := c.Resume(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
