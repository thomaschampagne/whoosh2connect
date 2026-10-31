package mywhoosh

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"whoosh2connect/internal/fsutil"
)

// cachedToken is the on-disk session format.
type cachedToken struct {
	AccessToken string `json:"access_token"`
	WhooshID    string `json:"whoosh_id"`
}

func readTokenFile(path string) (access, whooshID string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var ct cachedToken
	if err := json.Unmarshal(data, &ct); err != nil {
		return "", "", fmt.Errorf("parse token file: %w", err)
	}
	return ct.AccessToken, ct.WhooshID, nil
}

func writeTokenFile(path, access, whooshID string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cachedToken{AccessToken: access, WhooshID: whooshID}, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, data, 0o600)
}
