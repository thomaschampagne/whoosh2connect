package garmin

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tokens")
	o1 := &OAuth1Token{OAuthToken: "ot1", OAuthTokenSecret: "os1", MFAToken: "mt1", Domain: "garmin.com"}
	o2 := &OAuth2Token{
		Scope: "scope", Jti: "jti", TokenType: "Bearer", AccessToken: "at1",
		RefreshToken: "rt1", ExpiresIn: 3600, ExpiresAt: time.Now().Unix() + 3600,
		RefreshTokenExpiresIn: 86400, RefreshTokenExpiresAt: time.Now().Unix() + 86400,
	}
	if err := SaveTokens(dir, o1, o2); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"oauth1_token.json", "oauth2_token.json"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s perm = %o, want 600", name, info.Mode().Perm())
		}
	}

	got1, got2, err := LoadTokens(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got1.OAuthToken != "ot1" || got1.OAuthTokenSecret != "os1" || got1.MFAToken != "mt1" || got1.Domain != "garmin.com" {
		t.Errorf("oauth1 = %+v", got1)
	}
	if got2.AccessToken != "at1" || got2.RefreshToken != "rt1" {
		t.Errorf("oauth2 = %+v", got2)
	}
}

func TestSaveOAuth2Only(t *testing.T) {
	dir := t.TempDir()
	if err := SaveTokens(dir, nil, &OAuth2Token{AccessToken: "x", RefreshToken: "y"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadTokens(dir); err == nil {
		t.Fatal("expected error: oauth1 file missing")
	}
}

func TestLoadMissing(t *testing.T) {
	if _, _, err := LoadTokens(t.TempDir()); err == nil {
		t.Fatal("expected error")
	}
}

func TestExpiryHelpers(t *testing.T) {
	now := time.Now()
	if (&OAuth2Token{ExpiresAt: now.Add(time.Hour).Unix()}).Expired() {
		t.Error("future token reported expired")
	}
	if !(&OAuth2Token{ExpiresAt: now.Add(-time.Hour).Unix()}).Expired() {
		t.Error("past token reported valid")
	}
	if (&OAuth2Token{RefreshTokenExpiresAt: now.Add(time.Hour).Unix()}).RefreshExpired() {
		t.Error("future refresh reported expired")
	}
	if !(&OAuth2Token{RefreshTokenExpiresAt: now.Add(-time.Hour).Unix()}).RefreshExpired() {
		t.Error("past refresh reported valid")
	}
}

func TestBearer(t *testing.T) {
	tok := &OAuth2Token{AccessToken: "secret", TokenType: "Bearer"}
	if got := tok.Bearer(); got != "Bearer secret" {
		t.Errorf("bearer = %q", got)
	}
}
