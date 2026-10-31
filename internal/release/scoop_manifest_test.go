// Package release tests the Scoop manifest produced by the justfile recipes.
package release

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// repoRoot is the directory holding the justfile, two levels up from here.
const repoRoot = "../.."

func requireJust(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("just"); err != nil {
		t.Skip("just not available")
	}
}

func renderManifest(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("just", append([]string{"scoop-manifest"}, args...)...)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("just scoop-manifest %v failed: %v", args, err)
	}
	return out
}

func TestScoopManifestRenders(t *testing.T) {
	requireJust(t)
	const ver = "1.2.3"
	const hash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	var m struct {
		Version      string     `json:"version"`
		Homepage     string     `json:"homepage"`
		License      string     `json:"license"`
		Bin          string     `json:"bin"`
		Shortcuts    [][]string `json:"shortcuts"`
		Checkver     string     `json:"checkver"`
		Architecture map[string]struct {
			URL  string `json:"url"`
			Hash string `json:"hash"`
		} `json:"architecture"`
		Autoupdate struct {
			Architecture map[string]struct {
				URL string `json:"url"`
			} `json:"architecture"`
			Hash struct {
				URL string `json:"url"`
			} `json:"hash"`
		} `json:"autoupdate"`
	}
	if err := json.Unmarshal(renderManifest(t, ver, hash), &m); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if m.Version != ver {
		t.Errorf("version = %q, want %q", m.Version, ver)
	}
	if m.Homepage != "https://github.com/thomaschampagne/whoosh2connect" {
		t.Errorf("homepage = %q", m.Homepage)
	}
	if m.Bin != "whoosh2connect.exe" {
		t.Errorf("bin = %q", m.Bin)
	}
	if len(m.Shortcuts) != 1 || m.Shortcuts[0][0] != "whoosh2connect.exe" || m.Shortcuts[0][1] != "whoosh2connect" {
		t.Errorf("shortcuts = %v", m.Shortcuts)
	}
	if m.Checkver != "github" {
		t.Errorf("checkver = %q", m.Checkver)
	}
	arch, ok := m.Architecture["64bit"]
	if !ok {
		t.Fatal("missing 64bit architecture")
	}
	wantURL := "https://github.com/thomaschampagne/whoosh2connect/releases/download/1.2.3/whoosh2connect.exe"
	if arch.URL != wantURL {
		t.Errorf("download url = %q, want %q", arch.URL, wantURL)
	}
	if arch.Hash != hash {
		t.Errorf("hash = %q, want %q", arch.Hash, hash)
	}
	if got := m.Autoupdate.Hash.URL; got != "$baseurl/checksums.txt" {
		t.Errorf("autoupdate hash url = %q", got)
	}
}

func TestScoopManifestValuesAreLiteral(t *testing.T) {
	requireJust(t)
	const ver = "1.2.3-$(echo pwned)"
	const hash = "$(echo pwned)"
	var m struct {
		Version      string `json:"version"`
		Architecture map[string]struct {
			Hash string `json:"hash"`
		} `json:"architecture"`
	}
	if err := json.Unmarshal(renderManifest(t, ver, hash), &m); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if m.Version != ver {
		t.Errorf("version = %q, want the literal %q", m.Version, ver)
	}
	if got := m.Architecture["64bit"].Hash; got != hash {
		t.Errorf("hash = %q, want the literal %q", got, hash)
	}
}

func TestScoopManifestRequiresArgs(t *testing.T) {
	requireJust(t)
	for _, args := range [][]string{{}, {"1.2.3"}} {
		cmd := exec.Command("just", append([]string{"scoop-manifest"}, args...)...)
		cmd.Dir = repoRoot
		if err := cmd.Run(); err == nil {
			t.Errorf("scoop-manifest %v must fail", args)
		}
	}
}

// TestBucketManifestInSync skips until the first release commits bucket/.
func TestBucketManifestInSync(t *testing.T) {
	requireJust(t)
	path := filepath.Join(repoRoot, "bucket", "whoosh2connect.json")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("no bucket manifest yet, written by the first release")
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var m struct {
		Version      string `json:"version"`
		Architecture map[string]struct {
			Hash string `json:"hash"`
		} `json:"architecture"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("bucket manifest is not valid JSON: %v", err)
	}
	out := renderManifest(t, m.Version, m.Architecture["64bit"].Hash)
	if !bytes.Equal(bytes.TrimSpace(out), bytes.TrimSpace(raw)) {
		t.Errorf("bucket/whoosh2connect.json is stale; regenerate with just scoop-manifest")
	}
}
