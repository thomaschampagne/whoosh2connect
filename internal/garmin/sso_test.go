package garmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testConsumer = `{"consumer_key":"ckey","consumer_secret":"csecret"}`

type mockGarmin struct {
	t           *testing.T
	loginResp   string
	verifyResp  string
	gotVerify   map[string]interface{}
	gotLogin    map[string]interface{}
	sawOAuth1   bool
	sawExchange bool
}

func newMock(t *testing.T) (*mockGarmin, *httptest.Server) {
	m := &mockGarmin{t: t}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth_consumer.json":
			_, _ = w.Write([]byte(testConsumer))
		case "/mobile/sso/en/sign-in":
			w.Header().Set("Set-Cookie", "SESS=xyz; Path=/")
			_, _ = w.Write([]byte("<html></html>"))
		case "/mobile/api/login":
			if err := json.NewDecoder(r.Body).Decode(&m.gotLogin); err != nil {
				t.Errorf("decode login body: %v", err)
			}
			_, _ = w.Write([]byte(m.loginResp))
		case "/mobile/api/mfa/verifyCode":
			if err := json.NewDecoder(r.Body).Decode(&m.gotVerify); err != nil {
				t.Errorf("decode verify body: %v", err)
			}
			_, _ = w.Write([]byte(m.verifyResp))
		case "/portal/sso/embed":
			_, _ = w.Write([]byte("ok"))
		case "/oauth-service/oauth/preauthorized":
			m.sawOAuth1 = true
			if auth := r.Header.Get("Authorization"); !strings.Contains(auth, "oauth_consumer_key") || !strings.Contains(auth, "oauth_signature") {
				t.Errorf("preauthorized missing OAuth1 auth: %q", auth)
			}
			_, _ = w.Write([]byte("oauth_token=ot1&oauth_token_secret=os1"))
		case "/oauth-service/oauth/exchange/user/2.0":
			m.sawExchange = true
			if auth := r.Header.Get("Authorization"); !strings.Contains(auth, "oauth_consumer_key") {
				t.Errorf("exchange missing OAuth1 auth: %q", auth)
			}
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse exchange form: %v", err)
			}
			if r.PostForm.Get("audience") != "GARMIN_CONNECT_MOBILE_ANDROID_DI" {
				t.Errorf("audience = %q", r.PostForm.Get("audience"))
			}
			_, _ = w.Write([]byte(`{"scope":"s","jti":"j","token_type":"Bearer","access_token":"at","refresh_token":"rt","expires_in":9000,"refresh_token_expires_in":90000000}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return m, srv
}

func testHosts(srv *httptest.Server) hosts {
	h := defaultHosts("garmin.com")
	h.sso = srv.URL
	h.connectAPI = srv.URL
	h.consumerURL = srv.URL + "/oauth_consumer.json"
	return h
}

func TestLoginSuccess(t *testing.T) {
	m, srv := newMock(t)
	m.loginResp = `{"responseStatus":{"type":"SUCCESSFUL","message":"ok"},"serviceTicketId":"t1"}`
	h := testHosts(srv)
	hc := &http.Client{}

	flow, err := newSSOFlow(context.Background(), hc, h, "me@x", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if flow.Pending() {
		t.Fatal("unexpected pending")
	}
	if m.gotLogin["username"] != "me@x" || m.gotLogin["password"] != "pw" {
		t.Errorf("login body = %+v", m.gotLogin)
	}

	o1, o2, err := flow.Complete(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !m.sawOAuth1 || !m.sawExchange {
		t.Errorf("sawOAuth1=%v sawExchange=%v", m.sawOAuth1, m.sawExchange)
	}
	if o1.OAuthToken != "ot1" || o1.OAuthTokenSecret != "os1" {
		t.Errorf("oauth1 = %+v", o1)
	}
	if o2.AccessToken != "at" || o2.RefreshToken != "rt" {
		t.Errorf("oauth2 = %+v", o2)
	}
	if o2.Expired() {
		t.Error("fresh token reported expired")
	}
}

func TestLoginMFAFlow(t *testing.T) {
	m, srv := newMock(t)
	m.loginResp = `{"responseStatus":{"type":"MFA_REQUIRED","message":"code sent"},"customerMfaInfo":{"mfaLastMethodUsed":"EMAIL"}}`
	m.verifyResp = `{"responseStatus":{"type":"SUCCESSFUL","message":"ok"},"serviceTicketId":"t2"}`
	h := testHosts(srv)
	hc := &http.Client{}

	flow, err := newSSOFlow(context.Background(), hc, h, "me@x", "pw")
	if err != ErrMFARequired {
		t.Fatalf("err = %v, want ErrMFARequired", err)
	}
	if !flow.Pending() || flow.MFAMethod() != "EMAIL" {
		t.Errorf("pending=%v method=%s", flow.Pending(), flow.MFAMethod())
	}

	o1, o2, err := flow.SubmitMFA(context.Background(), "123456")
	if err != nil {
		t.Fatal(err)
	}
	if m.gotVerify["mfaVerificationCode"] != "123456" {
		t.Errorf("verify body = %+v", m.gotVerify)
	}
	if m.gotVerify["mfaMethod"] != "EMAIL" {
		t.Errorf("mfa method = %+v", m.gotVerify)
	}
	if o1.OAuthToken != "ot1" || o2.AccessToken != "at" {
		t.Errorf("tokens = %+v / %+v", o1, o2)
	}
	if flow.Pending() {
		t.Error("flow still pending after submit")
	}
}

func TestLoginMFAWrongCode(t *testing.T) {
	m, srv := newMock(t)
	m.loginResp = `{"responseStatus":{"type":"MFA_REQUIRED","message":"x"},"customerMfaInfo":{"mfaLastMethodUsed":"EMAIL"}}`
	m.verifyResp = `{"responseStatus":{"type":"FAILED","message":"Invalid code"}}`
	h := testHosts(srv)

	flow, err := newSSOFlow(context.Background(), &http.Client{}, h, "me@x", "pw")
	if err != ErrMFARequired {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := flow.SubmitMFA(context.Background(), "000000"); err == nil {
		t.Fatal("expected verify failure")
	}
}

func TestLoginBadCredentials(t *testing.T) {
	m, srv := newMock(t)
	m.loginResp = `{"responseStatus":{"type":"BAD_CREDENTIALS","message":"nope"}}`
	h := testHosts(srv)

	_, err := newSSOFlow(context.Background(), &http.Client{}, h, "me@x", "nope")
	if err == nil || !strings.Contains(err.Error(), "check credentials") {
		t.Fatalf("err = %v", err)
	}
}

func TestSubmitMFAWithoutPending(t *testing.T) {
	m, srv := newMock(t)
	m.loginResp = `{"responseStatus":{"type":"SUCCESSFUL","message":"ok"},"serviceTicketId":"t1"}`
	h := testHosts(srv)
	flow, err := newSSOFlow(context.Background(), &http.Client{}, h, "me@x", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := flow.SubmitMFA(context.Background(), "123456"); err == nil {
		t.Fatal("expected error when no MFA pending")
	}
}

func TestExchangeNoAccessToken(t *testing.T) {
	m, srv := newMock(t)
	m.loginResp = `{"responseStatus":{"type":"SUCCESSFUL","message":"ok"},"serviceTicketId":"t1"}`
	h := testHosts(srv)
	// Override exchange to return no access token.
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth_consumer.json":
			_, _ = w.Write([]byte(testConsumer))
		case "/mobile/sso/en/sign-in":
			_, _ = w.Write([]byte("x"))
		case "/mobile/api/login":
			_, _ = w.Write([]byte(m.loginResp))
		case "/portal/sso/embed":
			_, _ = w.Write([]byte("x"))
		case "/oauth-service/oauth/preauthorized":
			_, _ = w.Write([]byte("oauth_token=ot1&oauth_token_secret=os1"))
		case "/oauth-service/oauth/exchange/user/2.0":
			_, _ = w.Write([]byte(`{"scope":"s"}`))
		default:
			http.NotFound(w, r)
		}
	})
	hc := &http.Client{}
	flow, err := newSSOFlow(context.Background(), hc, h, "me@x", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := flow.Complete(context.Background()); err == nil {
		t.Fatal("expected error for missing access token")
	}
}

func TestDefaultHosts(t *testing.T) {
	h := defaultHosts("garmin.com")
	if h.sso != "https://sso.garmin.com" || h.connectAPI != "https://connectapi.garmin.com" {
		t.Errorf("hosts = %+v", h)
	}
	if h.mobileService != "https://mobile.integration.garmin.com/gcm/android" {
		t.Errorf("service = %s", h.mobileService)
	}
}
