package config

import (
	"os"
	"path/filepath"
	"testing"
)

func ptrStr(s string) *string { return &s }
func ptrU32(v uint32) *uint32 { return &v }

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Device.Preset != "edge_540" {
		t.Errorf("default preset = %q, want edge_540", cfg.Device.Preset)
	}
	if cfg.Device.Serial != 0 {
		t.Errorf("default serial = %d, want 0", cfg.Device.Serial)
	}
}

func TestLoadFileBeatsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("mywhoosh:\n  email: file@x\ndevice:\n  preset: edge_1030\n  serial: 4242\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(Overrides{ConfigPath: &path})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MyWhoosh.Email != "file@x" {
		t.Errorf("email = %q, want file@x", cfg.MyWhoosh.Email)
	}
	if cfg.Device.Preset != "edge_1030" {
		t.Errorf("preset = %q, want edge_1030", cfg.Device.Preset)
	}
	if cfg.Device.Serial != 4242 {
		t.Errorf("serial = %d, want 4242", cfg.Device.Serial)
	}
}

func TestEnvBeatsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("device:\n  preset: edge_1030\nmywhoosh:\n  email: file@x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WHOOSH2CONNECT_DEVICE_PRESET", "edge_530")
	t.Setenv("WHOOSH2CONNECT_MYWHOOSH_EMAIL", "env@x")
	t.Setenv("WHOOSH2CONNECT_DEVICE_SERIAL", "999")

	cfg, _, err := Load(Overrides{ConfigPath: &path})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Device.Preset != "edge_530" {
		t.Errorf("preset = %q, want edge_530 (env)", cfg.Device.Preset)
	}
	if cfg.MyWhoosh.Email != "env@x" {
		t.Errorf("email = %q, want env@x", cfg.MyWhoosh.Email)
	}
	if cfg.Device.Serial != 999 {
		t.Errorf("serial = %d, want 999", cfg.Device.Serial)
	}
}

func TestFlagsBeatEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(path, []byte("device:\n  preset: edge_1030\n"), 0o600)
	t.Setenv("WHOOSH2CONNECT_DEVICE_PRESET", "edge_530")
	t.Setenv("WHOOSH2CONNECT_GARMIN_EMAIL", "env@x")

	cfg, _, err := Load(Overrides{
		ConfigPath:   &path,
		DevicePreset: ptrStr("edge_1040"),
		GarminEmail:  ptrStr("flag@x"),
		DeviceSerial: ptrU32(7),
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Device.Preset != "edge_1040" {
		t.Errorf("preset = %q, want edge_1040 (flag)", cfg.Device.Preset)
	}
	if cfg.Garmin.Email != "flag@x" {
		t.Errorf("garmin email = %q, want flag@x (flag)", cfg.Garmin.Email)
	}
	if cfg.Device.Serial != 7 {
		t.Errorf("serial = %d, want 7", cfg.Device.Serial)
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	cfg, _, err := Load(Overrides{ConfigPath: &path})
	if err != nil {
		t.Fatalf("missing config should not error: %v", err)
	}
	if cfg.Device.Preset != "edge_540" {
		t.Errorf("preset = %q, want default edge_540", cfg.Device.Preset)
	}
}

func TestSaveRoundTripAndPerms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")
	cfg := Default()
	cfg.Device.Preset = "edge_840"
	cfg.Device.Serial = 12345
	cfg.Device.Name = "My Edge"
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 600", info.Mode().Perm())
	}
	got, _, err := Load(Overrides{ConfigPath: &path})
	if err != nil {
		t.Fatal(err)
	}
	if got.Device != (Device{Preset: "edge_840", Name: "My Edge", Serial: 12345}) {
		t.Errorf("round trip mismatch: %+v", got.Device)
	}
}

func TestEnvConfigPathChoosesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.yaml")
	if err := os.WriteFile(path, []byte("mywhoosh:\n  email: custom@x\ndevice:\n  preset: edge_1050\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WHOOSH2CONNECT_CONFIG", path)

	cfg, _, err := Load(Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MyWhoosh.Email != "custom@x" || cfg.Device.Preset != "edge_1050" {
		t.Errorf("loaded %+v from env config path, want custom.yaml contents", cfg)
	}
}

func TestInvalidEnvSerialIgnored(t *testing.T) {
	isolateHome(t)
	t.Setenv("WHOOSH2CONNECT_DEVICE_SERIAL", "not-a-number")
	cfg, _, err := Load(Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Device.Serial != 0 {
		t.Errorf("serial = %d, want 0 (invalid env ignored)", cfg.Device.Serial)
	}
}

// isolateHome points config and data lookups at a temp dir so tests never read
// the developer's real config file.
func isolateHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
}

func TestDataHome(t *testing.T) {
	home := filepath.Join("h", "user")
	tests := []struct {
		name, goos, xdg, localAppData, want string
	}{
		{"linux default", "linux", "", "", filepath.Join(home, ".local", "share")},
		{"linux xdg wins", "linux", "/x", "", "/x"},
		{"windows localappdata", "windows", "", filepath.Join("l", "app"), filepath.Join("l", "app")},
		{"windows fallback", "windows", "", "", filepath.Join(home, "AppData", "Local")},
		{"darwin", "darwin", "", "", filepath.Join(home, "Library", "Application Support")},
		{"xdg wins on windows too", "windows", "/x", "/l", "/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dataHome(tt.goos, tt.xdg, tt.localAppData, home); got != tt.want {
				t.Fatalf("dataHome = %q, want %q", got, tt.want)
			}
		})
	}
}
