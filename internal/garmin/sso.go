package garmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/dghubble/oauth1"

	"whoosh2connect/internal/httpx"
)

// ErrMFARequired signals that login hit a two-factor challenge and the caller
// must collect the code and call SubmitMFA.
var ErrMFARequired = errors.New("garmin requires a verification code (MFA)")

const (
	clientID          = "GCM_ANDROID_DARK"
	ssoUA             = "com.garmin.android.apps.connectmobile"
	defaultConsumer   = "https://thegarth.s3.amazonaws.com/oauth_consumer.json"
	browserUA         = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148"
	mobileServicePath = "/gcm/android"
	accept            = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	acceptLang        = "en-US,en;q=0.9"
)

// hosts are the endpoint bases for one Garmin account region.
type hosts struct {
	sso           string
	connectAPI    string
	mobileService string
	consumerURL   string
}

func defaultHosts(domain string) hosts {
	if domain == "" {
		domain = "garmin.com"
	}
	return hosts{
		sso:           "https://sso." + domain,
		connectAPI:    "https://connectapi." + domain,
		mobileService: "https://mobile.integration." + domain + mobileServicePath,
		consumerURL:   defaultConsumer,
	}
}

type oauthConsumer struct {
	ConsumerKey    string `json:"consumer_key"`
	ConsumerSecret string `json:"consumer_secret"`
}

func fetchConsumer(ctx context.Context, hc *http.Client, url string) (*oauthConsumer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch consumer: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("consumer fetch HTTP %d", resp.StatusCode)
	}
	var c oauthConsumer
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		return nil, fmt.Errorf("parse consumer: %w", err)
	}
	if c.ConsumerKey == "" || c.ConsumerSecret == "" {
		return nil, fmt.Errorf("consumer JSON missing key/secret")
	}
	return &c, nil
}

// ssoFlow carries one interactive login attempt across the two-phase MFA flow.
type ssoFlow struct {
	hc          *http.Client
	hosts       hosts
	consumer    *oauthConsumer
	loginParams url.Values
	mfaMethod   string
	ticket      string
	lastURL     string
	email       string
}

// newSSOFlow starts a login: seeds cookies, submits credentials, and returns a
// flow. If the flow needs MFA, Pending() is true and the caller must call
// SubmitMFA with the verification code.
func newSSOFlow(ctx context.Context, hc *http.Client, host hosts, email, password string) (*ssoFlow, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("cookie jar: %w", err)
	}
	hc.Jar = jar
	flow := &ssoFlow{hc: hc, hosts: host, email: email}

	consumer, err := fetchConsumer(ctx, hc, host.consumerURL)
	if err != nil {
		return nil, err
	}
	flow.consumer = consumer

	flow.loginParams = url.Values{
		"clientId": {clientID},
		"locale":   {"en-US"},
		"service":  {host.mobileService},
	}

	if _, err := flow.get(ctx, host.sso+"/mobile/sso/en/sign-in?clientId="+clientID,
		browserHeaders(), false); err != nil {
		return nil, fmt.Errorf("sso sign-in page: %w", err)
	}

	body := fmt.Sprintf(`{"username":%q,"password":%q,"rememberMe":false,"captchaToken":""}`,
		email, password)
	respBytes, err := flow.post(ctx, host.sso+"/mobile/api/login?"+flow.loginParams.Encode(),
		"application/json", strings.NewReader(body), browserHeaders(), false)
	if err != nil {
		return nil, fmt.Errorf("sso login: %w", err)
	}

	var respJson struct {
		ResponseStatus struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"responseStatus"`
		ServiceTicketID string `json:"serviceTicketId"`
		CustomerMfaInfo struct {
			MfaLastMethodUsed string `json:"mfaLastMethodUsed"`
		} `json:"customerMfaInfo"`
	}
	if err := json.Unmarshal(respBytes, &respJson); err != nil {
		return nil, fmt.Errorf("parse sso login response: %w", err)
	}

	switch respJson.ResponseStatus.Type {
	case "SUCCESSFUL":
		flow.mfaMethod = ""
		flow.ticket = respJson.ServiceTicketID
		return flow, nil
	case "MFA_REQUIRED":
		flow.mfaMethod = respJson.CustomerMfaInfo.MfaLastMethodUsed
		if flow.mfaMethod == "" {
			flow.mfaMethod = "email"
		}
		return flow, ErrMFARequired
	default:
		msg := respJson.ResponseStatus.Message
		if msg == "" {
			msg = respJson.ResponseStatus.Type
		}
		return nil, fmt.Errorf("sso login failed: %s (check credentials)", msg)
	}
}

// Pending reports whether this flow is waiting on an MFA code.
func (f *ssoFlow) Pending() bool { return f.mfaMethod != "" }

// Complete finishes the flow with the ticket obtained at login.
func (f *ssoFlow) Complete(ctx context.Context) (*OAuth1Token, *OAuth2Token, error) {
	if f.ticket == "" {
		return nil, nil, fmt.Errorf("no service ticket")
	}
	return f.complete(ctx, f.ticket)
}

// MFAMethod returns the MFA delivery method (typically "email").
func (f *ssoFlow) MFAMethod() string { return f.mfaMethod }

// SubmitMFA submits the verification code and completes the login.
func (f *ssoFlow) SubmitMFA(ctx context.Context, code string) (*OAuth1Token, *OAuth2Token, error) {
	if !f.Pending() {
		return nil, nil, fmt.Errorf("no MFA in progress")
	}
	body := fmt.Sprintf(`{"mfaMethod":%q,"mfaVerificationCode":%q,"rememberMyBrowser":false,"reconsentList":[],"mfaSetup":false}`,
		f.mfaMethod, code)
	respBytes, err := f.post(ctx, f.hosts.sso+"/mobile/api/mfa/verifyCode?"+f.loginParams.Encode(),
		"application/json", strings.NewReader(body), browserHeaders(), false)
	if err != nil {
		return nil, nil, fmt.Errorf("mfa verify: %w", err)
	}

	var respJson struct {
		ResponseStatus struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"responseStatus"`
		ServiceTicketID string `json:"serviceTicketId"`
	}
	if err := json.Unmarshal(respBytes, &respJson); err != nil {
		return nil, nil, fmt.Errorf("parse mfa response: %w", err)
	}
	if respJson.ResponseStatus.Type != "SUCCESSFUL" {
		return nil, nil, fmt.Errorf("mfa verification failed: %s", respJson.ResponseStatus.Message)
	}
	f.mfaMethod = ""
	return f.complete(ctx, respJson.ServiceTicketID)
}

// complete finishes the flow: best-effort LB cookie, then OAuth1 + OAuth2.
func (f *ssoFlow) complete(ctx context.Context, ticket string) (*OAuth1Token, *OAuth2Token, error) {
	// Cloudflare LB cookie for backend pinning — best-effort, non-fatal.
	_, _ = f.get(ctx, f.hosts.sso+"/portal/sso/embed", browserHeaders(), true)

	oauth1Token, err := f.getOAuth1Token(ctx, ticket)
	if err != nil {
		return nil, nil, err
	}
	oauth2Token, err := f.exchangeOAuth2(ctx, oauth1Token)
	if err != nil {
		return nil, nil, err
	}
	return oauth1Token, oauth2Token, nil
}

func (f *ssoFlow) getOAuth1Token(ctx context.Context, ticket string) (*OAuth1Token, error) {
	cfg := oauth1.NewConfig(f.consumer.ConsumerKey, f.consumer.ConsumerSecret)
	client := cfg.Client(ctx, oauth1.NewToken("", ""))
	client.Jar = f.hc.Jar
	client.Timeout = 30 * time.Second

	reqURL := fmt.Sprintf("%s/oauth-service/oauth/preauthorized?ticket=%s&login-url=%s&accepts-mfa-tokens=true",
		f.hosts.connectAPI, url.QueryEscape(ticket), url.QueryEscape(f.hosts.mobileService))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ssoUA)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("preauthorized request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := httpx.ReadBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("preauthorized HTTP %d: %s", resp.StatusCode, httpx.Snippet(body))
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, fmt.Errorf("parse oauth1 response: %w", err)
	}
	return &OAuth1Token{
		OAuthToken:       values.Get("oauth_token"),
		OAuthTokenSecret: values.Get("oauth_token_secret"),
		MFAToken:         values.Get("mfa_token"),
		MFAExpiration:    values.Get("mfa_expiration_timestamp"),
		Domain:           strings.TrimPrefix(f.hosts.connectAPI, "https://connectapi."),
	}, nil
}

func (f *ssoFlow) exchangeOAuth2(ctx context.Context, oauth1Token *OAuth1Token) (*OAuth2Token, error) {
	cfg := oauth1.NewConfig(f.consumer.ConsumerKey, f.consumer.ConsumerSecret)
	client := cfg.Client(ctx, oauth1.NewToken(oauth1Token.OAuthToken, oauth1Token.OAuthTokenSecret))
	client.Jar = f.hc.Jar
	client.Timeout = 30 * time.Second

	form := url.Values{
		"audience": {"GARMIN_CONNECT_MOBILE_ANDROID_DI"},
	}
	if oauth1Token.MFAToken != "" {
		form.Set("mfa_token", oauth1Token.MFAToken)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		f.hosts.connectAPI+"/oauth-service/oauth/exchange/user/2.0",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ssoUA)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := httpx.ReadBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("oauth2 exchange HTTP %d: %s", resp.StatusCode, httpx.Snippet(respBody))
	}

	var raw struct {
		Scope                 string `json:"scope"`
		Jti                   string `json:"jti"`
		TokenType             string `json:"token_type"`
		AccessToken           string `json:"access_token"`
		RefreshToken          string `json:"refresh_token"`
		ExpiresIn             int64  `json:"expires_in"`
		RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
	}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("parse oauth2: %w", err)
	}
	if raw.AccessToken == "" {
		return nil, fmt.Errorf("oauth2 exchange returned no access token")
	}
	now := time.Now().Unix()
	return &OAuth2Token{
		Scope:                 raw.Scope,
		Jti:                   raw.Jti,
		TokenType:             raw.TokenType,
		AccessToken:           raw.AccessToken,
		RefreshToken:          raw.RefreshToken,
		ExpiresIn:             raw.ExpiresIn,
		ExpiresAt:             now + raw.ExpiresIn,
		RefreshTokenExpiresIn: raw.RefreshTokenExpiresIn,
		RefreshTokenExpiresAt: now + raw.RefreshTokenExpiresIn,
	}, nil
}

// get performs a GET preserving cookies, optionally sending the Referer header.
func (f *ssoFlow) get(ctx context.Context, url string, headers http.Header, useReferer bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return f.do(ctx, req, headers, useReferer)
}

// post performs a POST preserving cookies, optionally sending the Referer header.
func (f *ssoFlow) post(ctx context.Context, url, contentType string, body io.Reader, headers http.Header, useReferer bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return f.do(ctx, req, headers, useReferer)
}

func (f *ssoFlow) do(ctx context.Context, req *http.Request, headers http.Header, useReferer bool) ([]byte, error) {
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if useReferer && f.lastURL != "" {
		req.Header.Set("Referer", f.lastURL)
	}
	resp, err := f.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	f.lastURL = resp.Request.URL.String()
	body, err := httpx.ReadBody(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, httpx.Snippet(body))
	}
	return body, nil
}

func browserHeaders() http.Header {
	h := http.Header{}
	h.Set("User-Agent", browserUA)
	h.Set("Accept", accept)
	h.Set("Accept-Language", acceptLang)
	h.Set("Sec-Fetch-Mode", "navigate")
	h.Set("Sec-Fetch-Dest", "document")
	return h
}
