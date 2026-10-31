package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runForTest(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close() }()
	out, err := os.CreateTemp(t.TempDir(), "out*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()
	errOut, err := os.CreateTemp(t.TempDir(), "err*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = errOut.Close() }()
	code := run(args, stdin, out, errOut)
	o, _ := os.ReadFile(out.Name())
	e, _ := os.ReadFile(errOut.Name())
	return code, string(o), string(e)
}

func TestVersionFlag(t *testing.T) {
	code, out, _ := runForTest(t, "--version")
	if code != 0 || !strings.Contains(out, "whoosh2connect") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestUnknownPreset(t *testing.T) {
	code, _, errOut := runForTest(t, "--device-preset", "edge_99xx")
	if code != 1 || !strings.Contains(errOut, "unknown device preset") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
}

func TestHeadlessNeedsEmails(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	code, _, errOut := runForTest(t, "--no-tui", "--config", cfg)
	if code != 1 || !strings.Contains(errOut, "--mywhoosh-email") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
}

func TestWatchNeedsGarminEmail(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	code, _, errOut := runForTest(t, "--watch", t.TempDir(), "--config", cfg, "--watch-interval", "1", "--watch-stable", "1")
	if code != 1 || !strings.Contains(errOut, "--garmin-email") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
}

func TestWatchRejectsMissingDir(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	code, _, errOut := runForTest(t, "--watch", filepath.Join(t.TempDir(), "nope"), "--garmin-email", "g@x.com", "--config", cfg)
	if code != 1 || !strings.Contains(errOut, "not a directory") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
}

func TestWatchAutoOnUnsupportedOSLogsHint(t *testing.T) {
	if !strings.HasPrefix(strings.ToUpper(runtime.GOOS), "LINUX") {
		t.Skip("linux-specific")
	}
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	code, _, errOut := runForTest(t, "--watch", "auto", "--garmin-email", "g@x.com", "--config", cfg)
	if code != 1 || !strings.Contains(errOut, "--watch") {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
}

func TestUnknownFlag(t *testing.T) {
	code, _, _ := runForTest(t, "--nope")
	if code != 2 {
		t.Fatalf("code=%d", code)
	}
}

func TestResolveWatchDir(t *testing.T) {
	existing := t.TempDir()
	tests := []struct {
		name    string
		flag    string
		cfgDir  string
		want    string
		wantErr bool
	}{
		{name: "flag wins over config", flag: existing, cfgDir: filepath.Join(t.TempDir(), "gone"), want: existing},
		{name: "config dir used without flag", cfgDir: existing, want: existing},
		{name: "missing flag dir errors", flag: filepath.Join(t.TempDir(), "gone"), wantErr: true},
		{name: "missing config dir errors", cfgDir: filepath.Join(t.TempDir(), "gone"), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveWatchDir(tc.flag, tc.cfgDir)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("dir = %q, want %q", got, tc.want)
			}
		})
	}
}
