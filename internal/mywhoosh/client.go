// Package mywhoosh provides a MyWhoosh account API client: login, activity
// listing, and FIT download. API requests and the JSON shape are ported from
// the mywoosh2garmin reference client.
package mywhoosh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultLoginURL       = "https://services.mywhoosh.com/http-service/api/login"
	defaultActivitiesBase = "https://service14.mywhoosh.com/v2/"
)

// Activity is a single MyWhoosh ride.
type Activity struct {
	ID             string  `json:"id"`
	Name           string  `json:"title"`
	Date           float64 `json:"date"`
	ActivityFileID string  `json:"activityFileId"`
	Distance       float64 `json:"distance,omitempty"`
	RideDuration   string  `json:"rideDuration,omitempty"`
	AvgPower       float64 `json:"watt,omitempty"`
	AvgHR          float64 `json:"heartrate,omitempty"`
	Elevation      float64 `json:"elevation,omitempty"`
	WattPerKg      float64 `json:"wattPerKg,omitempty"`
	RouteName      string  `json:"routeName,omitempty"`
	SportType      string  `json:"sportType,omitempty"`
	StartDatetime  string  `json:"startDatetime,omitempty"`

	start time.Time
}

// ParsedStart returns the activity start time, derived from startDatetime when
// present, else from the epoch Date field.
func (a *Activity) ParsedStart() time.Time {
	if !a.start.IsZero() {
		return a.start
	}
	if a.StartDatetime != "" {
		if t, err := time.Parse("2006-01-02T15:04:05.000Z", a.StartDatetime); err == nil {
			return t
		}
	}
	return time.Unix(int64(a.Date), 0)
}

// DisplayName returns the activity name, or a sensible default.
func (a *Activity) DisplayName() string {
	if a.Name != "" {
		return a.Name
	}
	return "Activity"
}

// Options configures a Client. Empty values fall back to the production
// endpoints, which lets tests inject httptest servers.
type Options struct {
	LoginURL      string
	ActivitiesURL string // base for driver/profile/... API
	TokenFile     string // "" disables token persistence
	HTTPClient    *http.Client
}

// Client manages MyWhoosh authentication and API calls.
type Client struct {
	token    string
	whooshID string
	opts     Options
	hc       *http.Client
}

// New creates a MyWhoosh client.
func New(opts Options) *Client {
	if opts.LoginURL == "" {
		opts.LoginURL = defaultLoginURL
	}
	if opts.ActivitiesURL == "" {
		opts.ActivitiesURL = defaultActivitiesBase
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{opts: opts, hc: opts.HTTPClient}
}

// IsAuthenticated reports whether a token is held.
func (c *Client) IsAuthenticated() bool { return c.token != "" }

// Resume loads the cached session if present.
func (c *Client) Resume(ctx context.Context) error {
	if c.opts.TokenFile == "" {
		return fmt.Errorf("no token file configured")
	}
	access, whooshID, err := readTokenFile(c.opts.TokenFile)
	if err != nil {
		return fmt.Errorf("no cached session: %w", err)
	}
	if access == "" {
		return fmt.Errorf("cached session is empty")
	}
	c.token = access
	c.whooshID = whooshID
	return nil
}

// Login authenticates with email and password and persists the session.
func (c *Client) Login(ctx context.Context, email, password string) error {
	payload := map[string]interface{}{
		"Username":      email,
		"Password":      password,
		"Platform":      "Android",
		"Action":        1001,
		"CorrelationId": uuid.New().String(),
		"DeviceId":      uuid.New().String(),
		"Authorization": "",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal login payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.opts.LoginURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("login failed (HTTP %d): %s", resp.StatusCode, truncate(respBody))
	}

	var result struct {
		Success      bool   `json:"Success"`
		Message      string `json:"Message"`
		AccessToken  string `json:"AccessToken"`
		RefreshToken string `json:"RefreshToken"`
		WhooshId     string `json:"WhooshId"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parse login response: %w", err)
	}
	if !result.Success {
		if result.Message == "" {
			result.Message = "unknown error"
		}
		return fmt.Errorf("login failed: %s", result.Message)
	}
	if result.AccessToken == "" {
		return fmt.Errorf("login failed: no access token in response")
	}

	c.token = result.AccessToken
	c.whooshID = result.WhooshId
	if c.opts.TokenFile != "" {
		if err := writeTokenFile(c.opts.TokenFile, result.AccessToken, result.WhooshId); err != nil {
			return fmt.Errorf("persist session: %w", err)
		}
	}
	return nil
}

// ListActivities fetches all activities, newest first.
func (c *Client) ListActivities(ctx context.Context) ([]Activity, error) {
	if c.token == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	var all []Activity
	for page := 1; ; page++ {
		payload := fmt.Sprintf(`{"sortDate":"DESC","page":%d}`, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.opts.ActivitiesURL+"rider/profile/activities", strings.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.token)

		resp, err := c.hc.Do(req)
		if err != nil {
			return nil, fmt.Errorf("activities request: %w", err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("fetch activities failed (HTTP %d): %s", resp.StatusCode, truncate(body))
		}

		var result struct {
			Data struct {
				Results    []json.RawMessage `json:"results"`
				TotalPages int               `json:"totalPages"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("parse activities: %w", err)
		}
		for _, raw := range result.Data.Results {
			var act Activity
			if err := json.Unmarshal(raw, &act); err == nil {
				act.start = act.ParsedStart()
				all = append(all, act)
			}
		}
		if page >= result.Data.TotalPages {
			break
		}
	}
	return all, nil
}

// DownloadFit downloads the FIT file for an activity file ID.
func (c *Client) DownloadFit(ctx context.Context, fileID string) ([]byte, error) {
	if c.token == "" {
		return nil, fmt.Errorf("not authenticated")
	}
	payload, err := json.Marshal(map[string]string{"fileId": fileID})
	if err != nil {
		return nil, fmt.Errorf("marshal download payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.opts.ActivitiesURL+"rider/profile/download-activity-file", strings.NewReader(string(payload)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("download failed (HTTP %d): %s", resp.StatusCode, truncate(body))
	}

	var result struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse download response: %w", err)
	}
	if result.Data == "" {
		return nil, fmt.Errorf("no download URL in response")
	}

	fitReq, err := http.NewRequestWithContext(ctx, http.MethodGet, result.Data, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}
	fitResp, err := c.hc.Do(fitReq)
	if err != nil {
		return nil, fmt.Errorf("fetch FIT file: %w", err)
	}
	defer func() { _ = fitResp.Body.Close() }()
	fitData, err := io.ReadAll(fitResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read FIT file: %w", err)
	}
	if fitResp.StatusCode >= 400 {
		return nil, fmt.Errorf("FIT download failed (HTTP %d)", fitResp.StatusCode)
	}
	return fitData, nil
}

func truncate(b []byte) string {
	if len(b) > 300 {
		return string(b[:300])
	}
	return string(b)
}
