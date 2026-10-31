// Package config loads whoosh2connect configuration from defaults, a YAML file
// in the XDG config directory, environment variables, and CLI flags, in that
// order of increasing precedence.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"gopkg.in/yaml.v3"

	"whoosh2connect/internal/fsutil"
)

// AppName is the directory name used under the XDG config/data directories.
const AppName = "whoosh2connect"

// MyWhoosh holds MyWhoosh account settings.
type MyWhoosh struct {
	Email string `yaml:"email"`
}

// Garmin holds Garmin Connect account settings.
type Garmin struct {
	Email string `yaml:"email"`
}

// Device holds the spoofing target. Serial 0 means "not yet assigned".
type Device struct {
	Preset string `yaml:"preset"`
	Name   string `yaml:"name"`
	Serial uint32 `yaml:"serial"`
}

// Config is the full application configuration.
type Config struct {
	MyWhoosh MyWhoosh `yaml:"mywhoosh"`
	Garmin   Garmin   `yaml:"garmin"`
	Device   Device   `yaml:"device"`
}

// Paths locates the on-disk files the app uses.
type Paths struct {
	ConfigDir  string
	ConfigFile string
	DataDir    string
	StateFile  string
	TokenDir   string
}

// Default returns the built-in defaults.
func Default() Config {
	return Config{
		Device: Device{Preset: "edge_540"},
	}
}

// dataHome returns the base directory for application data. XDG_DATA_HOME
// wins on any OS when set; otherwise each OS uses its own convention.
func dataHome(goos, xdg, localAppData, home string) string {
	switch {
	case xdg != "":
		return xdg
	case goos == "windows":
		if localAppData != "" {
			return localAppData
		}
		return filepath.Join(home, "AppData", "Local")
	case goos == "darwin":
		return filepath.Join(home, "Library", "Application Support")
	default:
		return filepath.Join(home, ".local", "share")
	}
}

// DefaultPaths resolves the config and data directories for the current OS.
func DefaultPaths() (Paths, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve config dir: %w", err)
	}
	configDir = filepath.Join(configDir, AppName)

	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home dir: %w", err)
	}
	dataDir := filepath.Join(dataHome(runtime.GOOS, os.Getenv("XDG_DATA_HOME"), os.Getenv("LOCALAPPDATA"), home), AppName)

	p := Paths{
		ConfigDir:  configDir,
		ConfigFile: filepath.Join(configDir, "config.yaml"),
		DataDir:    dataDir,
		StateFile:  filepath.Join(dataDir, "state.json"),
		TokenDir:   dataDir,
	}
	return p, nil
}

// Overrides carries values supplied on the command line. A nil field means the
// flag was not set, so lower-precedence sources win.
type Overrides struct {
	ConfigPath    *string
	MyWhooshEmail *string
	GarminEmail   *string
	DevicePreset  *string
	DeviceName    *string
	DeviceSerial  *uint32
}

// Load resolves configuration using the precedence flags > env > file > defaults
// and returns it together with the resolved paths.
func Load(o Overrides) (Config, Paths, error) {
	paths, err := DefaultPaths()
	if err != nil {
		return Config{}, Paths{}, err
	}
	if o.ConfigPath != nil && *o.ConfigPath != "" {
		paths.ConfigFile = *o.ConfigPath
	} else if v := os.Getenv("WHOOSH2CONNECT_CONFIG"); v != "" {
		paths.ConfigFile = v
	}

	cfg := Default()

	data, err := os.ReadFile(paths.ConfigFile)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return Config{}, Paths{}, fmt.Errorf("parse %s: %w", paths.ConfigFile, err)
		}
	case os.IsNotExist(err):
		// No file — defaults stand.
	default:
		return Config{}, Paths{}, fmt.Errorf("read %s: %w", paths.ConfigFile, err)
	}

	applyEnv(&cfg)
	applyOverrides(&cfg, o)

	return cfg, paths, nil
}

func applyEnv(cfg *Config) {
	if v, ok := os.LookupEnv("WHOOSH2CONNECT_MYWHOOSH_EMAIL"); ok {
		cfg.MyWhoosh.Email = v
	}
	if v, ok := os.LookupEnv("WHOOSH2CONNECT_GARMIN_EMAIL"); ok {
		cfg.Garmin.Email = v
	}
	if v, ok := os.LookupEnv("WHOOSH2CONNECT_DEVICE_PRESET"); ok {
		cfg.Device.Preset = v
	}
	if v, ok := os.LookupEnv("WHOOSH2CONNECT_DEVICE_NAME"); ok {
		cfg.Device.Name = v
	}
	if v, ok := os.LookupEnv("WHOOSH2CONNECT_DEVICE_SERIAL"); ok {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			cfg.Device.Serial = uint32(n)
		}
	}
}

func applyOverrides(cfg *Config, o Overrides) {
	if o.MyWhooshEmail != nil {
		cfg.MyWhoosh.Email = *o.MyWhooshEmail
	}
	if o.GarminEmail != nil {
		cfg.Garmin.Email = *o.GarminEmail
	}
	if o.DevicePreset != nil {
		cfg.Device.Preset = *o.DevicePreset
	}
	if o.DeviceName != nil {
		cfg.Device.Name = *o.DeviceName
	}
	if o.DeviceSerial != nil {
		cfg.Device.Serial = *o.DeviceSerial
	}
}

// Save writes cfg to path, creating parent directories with mode 0700 and the
// file with mode 0600.
func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := fsutil.WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
