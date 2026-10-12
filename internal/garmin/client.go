// Package garmin implements Garmin Connect authentication (mobile SSO with MFA)
// and FIT upload using the reference endpoints and headers.
package garmin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const apiUserAgent = "GCM-iOS-5.19.1.2"

// Options configures a Client. Hosts is only used by tests.
type Options struct {
	TokenDir   string
	Domain     string
	HTTPClient *http.Client
	Hosts      *hosts
}

// Client manages Garmin authentication, tokens, and uploads.
type Client struct {
	OAuth1 *OAuth1Token
	OAuth2 *OAuth2Token

	Domain   string
	TokenDir string
	hc       *http.Client
	hosts    *hosts
	flow     *ssoFlow
}

// New creates a Client.
func New(opts Options) *Client {
	if opts.Domain == "" {
		opts.Domain = "garmin.com"
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &Client{
		Domain:   opts.Domain,
		TokenDir: opts.TokenDir,
		hc:       opts.HTTPClient,
		hosts:    opts.Hosts,
	}
}

// resolveHosts returns the endpoints to use, honoring test overrides.
func (c *Client) resolveHosts() hosts {
	if c.hosts != nil {
		return *c.hosts
	}
	domain := c.Domain
	if c.OAuth1 != nil && c.OAuth1.Domain != "" {
		domain = c.OAuth1.Domain
	}
	return defaultHosts(domain)
}

// NeedsMFA reports whether the last Login hit a two-factor challenge.
func (c *Client) NeedsMFA() bool { return c.flow != nil && c.flow.Pending() }

// MFAMethod returns the pending MFA delivery method.
func (c *Client) MFAMethod() string {
	if !c.NeedsMFA() {
		return ""
	}
	return c.flow.MFAMethod()
}

// Resume loads cached tokens and refreshes the OAuth2 token if needed.
func (c *Client) Resume(ctx context.Context) error {
	if c.TokenDir == "" {
		return fmt.Errorf("no token directory configured")
	}
	oauth1, oauth2, err := LoadTokens(c.TokenDir)
	if err != nil {
		return fmt.Errorf("no cached session: %w", err)
	}
	c.OAuth1 = oauth1
	c.OAuth2 = oauth2

	if !oauth2.Expired() {
		return nil
	}
	if err := c.refreshOAuth2(ctx); err != nil {
		return fmt.Errorf("refresh failed: %w", err)
	}
	return nil
}

// Login starts a Garmin login. On success tokens are saved. If Garmin requires
// a verification code, it returns ErrMFARequired and the caller must call
// SubmitMFA with the code.
func (c *Client) Login(ctx context.Context, email, password string) error {
	host := c.resolveHosts()
	flow, err := newSSOFlow(ctx, c.hc, host, email, password)
	if err == ErrMFARequired {
		c.flow = flow
		return ErrMFARequired
	}
	if err != nil {
		return err
	}
	return c.completeLogin(ctx, flow)
}

// SubmitMFA submits the verification code and completes a pending login.
func (c *Client) SubmitMFA(ctx context.Context, code string) error {
	if c.flow == nil {
		return fmt.Errorf("no MFA login in progress")
	}
	o1, o2, err := c.flow.SubmitMFA(ctx, code)
	if err != nil {
		return err
	}
	c.flow = nil
	c.OAuth1 = o1
	c.OAuth2 = o2
	c.persistTokens()
	return nil
}

func (c *Client) completeLogin(ctx context.Context, flow *ssoFlow) error {
	o1, o2, err := flow.Complete(ctx)
	if err != nil {
		return err
	}
	c.flow = nil
	c.OAuth1 = o1
	c.OAuth2 = o2
	c.persistTokens()
	return nil
}

// persistTokens saves tokens to disk, ignoring persistence errors so a login
// still succeeds when the token dir cannot be written.
func (c *Client) persistTokens() {
	if c.TokenDir == "" {
		return
	}
	if err := SaveTokens(c.TokenDir, c.OAuth1, c.OAuth2); err != nil {
		_ = err
	}
}

// refreshOAuth2 exchanges the OAuth1 token for a fresh OAuth2 token.
func (c *Client) refreshOAuth2(ctx context.Context) error {
	host := c.resolveHosts()
	consumer, err := fetchConsumer(ctx, c.hc, host.consumerURL)
	if err != nil {
		return err
	}
	flow := &ssoFlow{hc: c.hc, hosts: host, consumer: consumer}
	oauth2, err := flow.exchangeOAuth2(ctx, c.OAuth1)
	if err != nil {
		return err
	}
	c.OAuth2 = oauth2
	if c.TokenDir != "" {
		if err := SaveTokens(c.TokenDir, nil, oauth2); err != nil {
			return err
		}
	}
	return nil
}

var (
	// ErrDuplicate indicates the activity already exists on Garmin Connect.
	ErrDuplicate = errors.New("duplicate activity already on Garmin")
)

// UploadFIT uploads the given FIT file, refreshing the OAuth2 token if it has
// expired and retrying once on a rejected token.
func (c *Client) UploadFIT(ctx context.Context, filePath string) error {
	if c.OAuth2 == nil {
		return fmt.Errorf("not authenticated")
	}
	if c.OAuth2.Expired() {
		if err := c.refreshOAuth2(ctx); err != nil {
			return fmt.Errorf("token refresh: %w", err)
		}
	}

	status, body, err := c.doUpload(ctx, filePath)
	if err != nil {
		return err
	}
	if status == 401 {
		if err := c.refreshOAuth2(ctx); err != nil {
			return fmt.Errorf("token refresh: %w", err)
		}
		status, body, err = c.doUpload(ctx, filePath)
		if err != nil {
			return err
		}
	}
	return parseUploadResult(status, body)
}

func (c *Client) doUpload(ctx context.Context, filePath string) (int, []byte, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, nil, err
	}
	defer f.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return 0, nil, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return 0, nil, err
	}
	if err := writer.Close(); err != nil {
		return 0, nil, err
	}

	host := c.resolveHosts()
	uploadURL := host.connectAPI + "/upload-service/upload"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, &buf)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", c.OAuth2.Bearer())
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("User-Agent", apiUserAgent)
	req.Header.Set("DI-Backend", host.connectAPI)
	req.Header.Set("NK", "NT")

	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, body, nil
}

func parseUploadResult(status int, body []byte) error {
	if status == 409 {
		return ErrDuplicate
	}
	if status >= 400 {
		return fmt.Errorf("upload failed (HTTP %d): %s", status, truncate(body))
	}
	var result struct {
		DetailedImportResult struct {
			Failures  []interface{} `json:"failures"`
			Successes []interface{} `json:"successes"`
		} `json:"detailedImportResult"`
	}
	if err := json.Unmarshal(body, &result); err == nil {
		if len(result.DetailedImportResult.Failures) > 0 {
			return fmt.Errorf("upload reported failures: %v", result.DetailedImportResult.Failures)
		}
	}
	return nil
}
