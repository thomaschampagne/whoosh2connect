package garmin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"whoosh2connect/internal/httpx"
)

const (
	activityPageSize = 20
	maxActivityPages = 5
	matchTolerance   = 2 * time.Minute
	garminTimeLayout = "2006-01-02 15:04:05"
)

// RecentActivity is the subset of a Garmin Connect activity used for matching.
type RecentActivity struct {
	ID    int64
	Start time.Time
}

type activityItem struct {
	ActivityID   int64  `json:"activityId"`
	StartTimeGMT string `json:"startTimeGMT"`
}

// FindMatch reports the Garmin activity ID of a ride starting within
// matchTolerance of start. It scans at most maxActivityPages pages of the
// most recent activities, so rides older than that are not seen here.
func (c *Client) FindMatch(ctx context.Context, start time.Time) (string, bool, error) {
	for page := 0; page < maxActivityPages; page++ {
		acts, err := c.listActivities(ctx, page*activityPageSize, activityPageSize)
		if err != nil {
			return "", false, err
		}
		for _, a := range acts {
			if within(a.Start, start, matchTolerance) {
				return strconv.FormatInt(a.ID, 10), true, nil
			}
		}
		if len(acts) < activityPageSize {
			break
		}
	}
	return "", false, nil
}

func within(a, b time.Time, tol time.Duration) bool {
	d := a.Sub(b)
	if d < 0 {
		d = -d
	}
	return d <= tol
}

func (c *Client) listActivities(ctx context.Context, start, limit int) ([]RecentActivity, error) {
	if c.OAuth2 == nil {
		return nil, fmt.Errorf("not authenticated")
	}
	if c.OAuth2.Expired() {
		if err := c.refreshOAuth2(ctx); err != nil {
			return nil, fmt.Errorf("token refresh: %w", err)
		}
	}
	host := c.resolveHosts()
	u := fmt.Sprintf("%s/activitylist-service/activities/search/activities?start=%d&limit=%d",
		host.connectAPI, start, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create activities request: %w", err)
	}
	req.Header.Set("Authorization", c.OAuth2.Bearer())
	req.Header.Set("User-Agent", apiUserAgent)
	req.Header.Set("DI-Backend", host.connectAPI)
	req.Header.Set("NK", "NT")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("activities request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := httpx.ReadBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read activities: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("list activities failed (HTTP %d): %s", resp.StatusCode, httpx.Snippet(body))
	}
	return parseActivities(body)
}

func parseActivities(body []byte) ([]RecentActivity, error) {
	var items []activityItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("parse activities: %w", err)
	}
	out := make([]RecentActivity, 0, len(items))
	for _, it := range items {
		start, err := time.ParseInLocation(garminTimeLayout, it.StartTimeGMT, time.UTC)
		if err != nil {
			continue
		}
		out = append(out, RecentActivity{ID: it.ActivityID, Start: start})
	}
	return out, nil
}
