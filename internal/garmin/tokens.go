package garmin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// OAuth1Token represents the OAuth1 credentials obtained from Garmin SSO.
// It lasts roughly a year and is used to refresh the short-lived OAuth2 token.
type OAuth1Token struct {
	OAuthToken       string `json:"oauth_token"`
	OAuthTokenSecret string `json:"oauth_token_secret"`
	MFAToken         string `json:"mfa_token,omitempty"`
	MFAExpiration    string `json:"mfa_expiration_timestamp,omitempty"`
	Domain           string `json:"domain,omitempty"`
}

// OAuth2Token is the short-lived bearer token used for API access.
type OAuth2Token struct {
	Scope                 string `json:"scope"`
	Jti                   string `json:"jti"`
	TokenType             string `json:"token_type"`
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	ExpiresIn             int64  `json:"expires_in"`
	ExpiresAt             int64  `json:"expires_at"`
	RefreshTokenExpiresIn int64  `json:"refresh_token_expires_in"`
	RefreshTokenExpiresAt int64  `json:"refresh_token_expires_at"`
}

// Expired reports whether the access token has expired.
func (t *OAuth2Token) Expired() bool {
	return t.ExpiresAt < time.Now().Unix()
}

// RefreshExpired reports whether the refresh token has expired.
func (t *OAuth2Token) RefreshExpired() bool {
	return t.RefreshTokenExpiresAt < time.Now().Unix()
}

// Bearer returns the Authorization header value.
func (t *OAuth2Token) Bearer() string {
	return fmt.Sprintf("Bearer %s", t.AccessToken)
}

// SaveTokens writes OAuth1 and/or OAuth2 tokens to dir with mode 0600.
func SaveTokens(dir string, oauth1 *OAuth1Token, oauth2 *OAuth2Token) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create token dir: %w", err)
	}
	if oauth1 != nil {
		if err := writeJSON(filepath.Join(dir, "oauth1_token.json"), oauth1); err != nil {
			return err
		}
	}
	if oauth2 != nil {
		if err := writeJSON(filepath.Join(dir, "oauth2_token.json"), oauth2); err != nil {
			return err
		}
	}
	return nil
}

// LoadTokens reads both token files from dir.
func LoadTokens(dir string) (*OAuth1Token, *OAuth2Token, error) {
	var oauth1 OAuth1Token
	if err := readJSON(filepath.Join(dir, "oauth1_token.json"), &oauth1); err != nil {
		return nil, nil, fmt.Errorf("load oauth1: %w", err)
	}
	var oauth2 OAuth2Token
	if err := readJSON(filepath.Join(dir, "oauth2_token.json"), &oauth2); err != nil {
		return nil, nil, fmt.Errorf("load oauth2: %w", err)
	}
	return &oauth1, &oauth2, nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
