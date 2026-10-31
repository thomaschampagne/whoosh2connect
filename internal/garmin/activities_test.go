package garmin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFindMatch(t *testing.T) {
	start := time.Date(2026, 1, 8, 14, 10, 21, 0, time.UTC)
	page1 := `[{"activityId": 111, "startTimeGMT": "2026-01-08 14:00:00"},
	           {"activityId": 222, "startTimeGMT": "2026-01-08 14:10:23"}]`
	empty := `[]`
	tests := []struct {
		name    string
		start   time.Time
		pages   []string
		wantID  string
		wantHit bool
		wantErr bool
	}{
		{"exact start matches", start, []string{page1}, "222", true, false},
		{"exactly 2 minutes before matches", time.Date(2026, 1, 8, 14, 8, 23, 0, time.UTC), []string{page1}, "222", true, false},
		{"2 minutes plus 1s before misses", time.Date(2026, 1, 8, 14, 8, 22, 0, time.UTC), []string{page1}, "", false, false},
		{"no activities misses", start, []string{empty}, "", false, false},
		{"match on second full page", start, []string{fullPage(), page1}, "222", true, false},
		{"server error propagates", start, nil, "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newActivityServer(t, tt.pages)
			defer srv.Close()
			c := newTestClient(srv.URL)
			id, found, err := c.FindMatch(context.Background(), tt.start)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if found != tt.wantHit || id != tt.wantID {
				t.Fatalf("got id=%q found=%v, want id=%q found=%v", id, found, tt.wantID, tt.wantHit)
			}
		})
	}
}

func TestFindMatchStopsAtPageCap(t *testing.T) {
	full := make([]string, maxActivityPages)
	for i := range full {
		full[i] = `[{"activityId": 1, "startTimeGMT": "2000-01-01 00:00:00"}]`
	}
	srv := newActivityServer(t, full)
	defer srv.Close()
	c := newTestClient(srv.URL)
	_, found, err := c.FindMatch(context.Background(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || found {
		t.Fatalf("found=%v err=%v, want miss without error", found, err)
	}
}

func newActivityServer(t *testing.T, pages []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/activitylist-service/activities/search/activities" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if pages == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		idx := 0
		if s := r.URL.Query().Get("start"); s != "" {
			idx = atoiOr(s, 0) / activityPageSize
		}
		if idx >= len(pages) {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(strings.TrimSpace(pages[idx])))
	}))
}

func newTestClient(base string) *Client {
	c := New(Options{HTTPClient: &http.Client{Timeout: 5 * time.Second}})
	c.hosts = &hosts{connectAPI: base}
	c.OAuth2 = &OAuth2Token{AccessToken: "tok", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	return c
}

func atoiOr(s string, def int) int {
	n := def
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func fullPage() string {
	items := make([]string, activityPageSize)
	for i := range items {
		items[i] = `{"activityId": 1, "startTimeGMT": "2000-01-01 00:00:00"}`
	}
	return "[" + strings.Join(items, ",") + "]"
}

func TestListActivitiesUnauthorizedTriesRefresh(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	_, _, err := c.FindMatch(context.Background(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "token refresh") {
		t.Fatalf("err = %v, want token refresh attempt on 401", err)
	}
}
