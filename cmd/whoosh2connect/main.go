// Command whoosh2connect syncs MyWhoosh cycling sessions to Garmin Connect.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"whoosh2connect/internal/app"
	"whoosh2connect/internal/config"
	"whoosh2connect/internal/device"
	"whoosh2connect/internal/garmin"
	"whoosh2connect/internal/mywhoosh"
	"whoosh2connect/internal/state"
	"whoosh2connect/internal/tui"
	"whoosh2connect/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// write renders to w, ignoring write errors (best-effort terminal output).
func write(w io.Writer, format string, a ...interface{}) {
	_, _ = fmt.Fprintf(w, format, a...)
}

func run(args []string, stdin *os.File, stdout, stderr *os.File) int {
	conf := flag.NewFlagSet("whoosh2connect", flag.ContinueOnError)
	conf.SetOutput(stderr)
	configPath := conf.String("config", "", "path to config file (default: XDG config dir)")
	mwEmail := conf.String("mywhoosh-email", "", "MyWhoosh account email")
	garminEmail := conf.String("garmin-email", "", "Garmin Connect email")
	devicePreset := conf.String("device-preset", "", "device preset ID")
	deviceName := conf.String("device-name", "", "override device name")
	deviceSerial := conf.Uint("device-serial", 0, "override device serial (non-zero uint32)")
	noTUI := conf.Bool("no-tui", false, "sync headless and exit")
	showVersion := conf.Bool("version", false, "print version and exit")
	conf.Usage = func() {
		write(stderr, "whoosh2connect — sync MyWhoosh sessions to Garmin Connect\n\n")
		write(stderr, "Usage: whoosh2connect [flags]")
		write(stderr, "\nFlags:")
		conf.PrintDefaults()
		write(stderr, "\nConfig precedence: flags > env > file > defaults.")
		write(stderr, "Env: WHOOSH2CONNECT_MYWHOOSH_EMAIL, WHOOSH2CONNECT_GARMIN_EMAIL,")
		write(stderr, "     WHOOSH2CONNECT_MYWHOOSH_PASSWORD, WHOOSH2CONNECT_GARMIN_PASSWORD,")
		write(stderr, "     WHOOSH2CONNECT_DEVICE_PRESET, WHOOSH2CONNECT_DEVICE_NAME,")
		write(stderr, "     WHOOSH2CONNECT_DEVICE_SERIAL, WHOOSH2CONNECT_CONFIG")
	}
	if err := conf.Parse(args); err != nil {
		return 2
	}

	if *showVersion {
		write(stdout, "%s\n", version.String())
		return 0
	}

	o := config.Overrides{}
	if *configPath != "" {
		o.ConfigPath = configPath
	}
	if *mwEmail != "" {
		o.MyWhooshEmail = mwEmail
	}
	if *garminEmail != "" {
		o.GarminEmail = garminEmail
	}
	if *devicePreset != "" {
		o.DevicePreset = devicePreset
	}
	if *deviceName != "" {
		o.DeviceName = deviceName
	}
	if *deviceSerial != 0 {
		s := uint32(*deviceSerial)
		o.DeviceSerial = &s
	}

	cfg, paths, err := config.Load(o)
	if err != nil {
		write(stderr, "config: %v\n", err)
		return 1
	}

	dev, err := resolveDevice(cfg, paths)
	if err != nil {
		write(stderr, "device: %v\n", err)
		return 1
	}

	svc, err := buildService(paths, dev)
	if err != nil {
		write(stderr, "state: %v\n", err)
		return 1
	}

	if *noTUI {
		return headless(cfg, svc, stdin, stdout, stderr)
	}

	p := tea.NewProgram(
		tui.New(tui.Options{
			Service: svc,
			Settings: tui.Settings{
				MywhooshEmail:    cfg.MyWhoosh.Email,
				MywhooshPassword: os.Getenv("WHOOSH2CONNECT_MYWHOOSH_PASSWORD"),
				GarminEmail:      cfg.Garmin.Email,
				GarminPassword:   os.Getenv("WHOOSH2CONNECT_GARMIN_PASSWORD"),
				DevicePreset:     cfg.Device.Preset,
			},
			SaveSettings: func(s tui.Settings) error {
				cfg.MyWhoosh.Email = s.MywhooshEmail
				cfg.Garmin.Email = s.GarminEmail
				cfg.Device.Preset = s.DevicePreset
				return config.Save(paths.ConfigFile, cfg)
			},
		}),
		tea.WithAltScreen(),
	)
	if _, err := p.Run(); err != nil {
		write(stderr, "tui: %v\n", err)
		return 1
	}
	return 0
}

// resolveDevice looks up the preset, generating and persisting a stable serial
// on first use so Garmin sees one consistent device.
func resolveDevice(cfg config.Config, paths config.Paths) (device.Device, error) {
	preset, err := device.Lookup(cfg.Device.Preset)
	if err != nil {
		return device.Device{}, fmt.Errorf("%v (choose from edge_540, edge_1030, edge_530, ...)", err)
	}
	dev := device.Device{Preset: preset, Name: cfg.Device.Name, Serial: cfg.Device.Serial}
	if dev.Serial == 0 {
		s, err := device.GenerateSerial()
		if err != nil {
			return device.Device{}, fmt.Errorf("generate serial: %w", err)
		}
		dev.Serial = s
		cfg.Device.Serial = s
		if err := config.Save(paths.ConfigFile, cfg); err != nil {
			return device.Device{}, fmt.Errorf("persist serial: %w", err)
		}
	}
	if err := dev.Validate(); err != nil {
		return device.Device{}, err
	}
	return dev, nil
}

func buildService(paths config.Paths, dev device.Device) (*app.Service, error) {
	httpClient := &http.Client{Timeout: 60 * time.Second}
	mwc := mywhoosh.New(mywhoosh.Options{
		TokenFile:  paths.TokenDir + "/mywhoosh_token.json",
		HTTPClient: httpClient,
	})
	gc := garmin.New(garmin.Options{
		TokenDir:   paths.TokenDir,
		Domain:     "garmin.com",
		HTTPClient: httpClient,
	})
	store, err := state.Open(paths.StateFile)
	if err != nil {
		return nil, err
	}
	return app.New(mwc, gc, store, dev), nil
}

// headless performs one full sync without the TUI.
func headless(cfg config.Config, svc *app.Service, stdin *os.File, stdout, stderr *os.File) int {
	if cfg.MyWhoosh.Email == "" || cfg.Garmin.Email == "" {
		write(stderr, "headless mode needs --mywhoosh-email and --garmin-email")
		return 1
	}
	mwPass := os.Getenv("WHOOSH2CONNECT_MYWHOOSH_PASSWORD")
	garminPass := os.Getenv("WHOOSH2CONNECT_GARMIN_PASSWORD")

	ctx := context.Background()
	if err := svc.LoginMyWhoosh(ctx, cfg.MyWhoosh.Email, mwPass); err != nil {
		write(stderr, "mywhoosh: %v\n", err)
		return 1
	}
	err := svc.LoginGarmin(ctx, cfg.Garmin.Email, garminPass)
	if err == garmin.ErrMFARequired {
		write(stderr, "Garmin requires a verification code: ")
		code, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil {
			write(stderr, "read code: %v\n", err)
			return 1
		}
		if err := svc.SubmitMFA(ctx, strings.TrimSpace(code)); err != nil {
			write(stderr, "mfa: %v\n", err)
			return 1
		}
	} else if err != nil {
		write(stderr, "garmin: %v\n", err)
		return 1
	}

	sessions, err := svc.Sessions(ctx)
	if err != nil {
		write(stderr, "sessions: %v\n", err)
		return 1
	}
	if len(sessions) == 0 {
		write(stdout, "no sessions found")
		return 0
	}

	for _, s := range sessions {
		if s.Uploaded {
			write(stdout, "skip   %s (already synced)\n", s.ID)
			continue
		}
		res := svc.Upload(ctx, s.Activity)
		write(stdout, "%s %s\n", statusWord(res), s.ID)
		if res.Status == app.StatusFailed {
			write(stdout, "  %v\n", res.Err)
			return 1
		}
	}
	return 0
}

func statusWord(res app.UploadResult) string {
	switch res.Status {
	case app.StatusUploaded:
		return "uploaded"
	case app.StatusDuplicate:
		return "duplicate"
	case app.StatusSkipped:
		return "skip"
	default:
		return "failed  "
	}
}
