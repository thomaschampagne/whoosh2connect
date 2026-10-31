// Command whoosh2connect syncs MyWhoosh cycling sessions to Garmin Connect.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"whoosh2connect/internal/app"
	"whoosh2connect/internal/config"
	"whoosh2connect/internal/device"
	"whoosh2connect/internal/garmin"
	"whoosh2connect/internal/logx"
	"whoosh2connect/internal/mywhoosh"
	"whoosh2connect/internal/state"
	"whoosh2connect/internal/tui"
	"whoosh2connect/internal/version"
	"whoosh2connect/internal/watch"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// write renders to w, ignoring write errors (best-effort terminal output).
func write(w io.Writer, format string, a ...interface{}) {
	_, _ = fmt.Fprintf(w, format, a...)
}

// serialText renders device serial text for the settings screen, leaving zero
// blank so the screen shows the "keep current" placeholder.
func serialText(n uint32) string {
	if n == 0 {
		return ""
	}
	return strconv.FormatUint(uint64(n), 10)
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
	dryRun := conf.Bool("dry-run", false, "headless: print what would upload or skip, change nothing")
	verify := conf.Bool("verify", false, "check ledger entries against Garmin Connect, upload nothing")
	showVersion := conf.Bool("version", false, "print version and exit")
	watchDir := conf.String("watch", "", `watch local MyWhoosh dir for new rides and auto-upload; "auto" discovers it`)
	watchInterval := conf.Int("watch-interval", 10, "seconds between scans in watch mode")
	watchStable := conf.Int("watch-stable", 2, "consecutive stable scans before a ride counts as done")
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

	logger, logCloser, err := logx.Open(filepath.Join(paths.DataDir, "whoosh2connect.log"), slog.LevelInfo)
	if err != nil {
		write(stderr, "log: %v\n", err)
		return 1
	}
	defer func() { _ = logCloser.Close() }()

	svc, err := buildService(paths, dev, logger)
	if err != nil {
		write(stderr, "state: %v\n", err)
		return 1
	}

	if *watchDir != "" {
		dir := *watchDir
		if dir == "auto" {
			d, err := watch.DefaultDir()
			if err != nil {
				write(stderr, "watch: %v\n", err)
				return 1
			}
			dir = d
		}
		st, err := os.Stat(dir)
		if err != nil || !st.IsDir() {
			write(stderr, "watch: %s is not a directory\n", dir)
			return 1
		}
		w, err := watch.New(watch.Options{
			Dir:         dir,
			Interval:    time.Duration(*watchInterval) * time.Second,
			StableScans: *watchStable,
		})
		if err != nil {
			write(stderr, "watch: %v\n", err)
			return 1
		}
		return watchLoop(w, svc, cfg, stdin, stdout, stderr)
	}

	if *verify {
		return verifyLedger(cfg, svc, stdin, stdout, stderr)
	}

	if *noTUI {
		return headless(cfg, svc, *dryRun, stdin, stdout, stderr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := tea.NewProgram(
		tui.New(tui.Options{
			Context: ctx,
			Service: svc,
			Settings: tui.Settings{
				MywhooshEmail:    cfg.MyWhoosh.Email,
				MywhooshPassword: os.Getenv("WHOOSH2CONNECT_MYWHOOSH_PASSWORD"),
				GarminEmail:      cfg.Garmin.Email,
				GarminPassword:   os.Getenv("WHOOSH2CONNECT_GARMIN_PASSWORD"),
				DevicePreset:     cfg.Device.Preset,
				DeviceName:       cfg.Device.Name,
				DeviceSerial:     serialText(cfg.Device.Serial),
			},
			SaveSettings: func(s tui.Settings) error {
				cfg.MyWhoosh.Email = s.MywhooshEmail
				cfg.Garmin.Email = s.GarminEmail
				cfg.Device.Preset = s.DevicePreset
				cfg.Device.Name = s.DeviceName
				if s.HasSerial() {
					cfg.Device.Serial = s.SerialValue()
				}
				return config.Save(paths.ConfigFile, cfg)
			},
		}),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
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

func buildService(paths config.Paths, dev device.Device, log *slog.Logger) (*app.Service, error) {
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
	s := app.New(mwc, gc, store, dev, log)
	s.Remote = gc
	return s, nil
}

// headless performs one full sync without the TUI.
func headless(cfg config.Config, svc *app.Service, dryRun bool, stdin *os.File, stdout, stderr *os.File) int {
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
	err := loginGarmin(ctx, svc, cfg.Garmin.Email, garminPass, stdin, stderr)
	if err != nil {
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

	if dryRun {
		return planSessions(ctx, svc, sessions, stdout, stderr)
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

// planSessions prints the dry-run decision for each session and uploads nothing.
func planSessions(ctx context.Context, svc *app.Service, sessions []app.Session, stdout, stderr *os.File) int {
	for _, s := range sessions {
		d, err := svc.Plan(ctx, s.Activity)
		if err != nil {
			write(stderr, "plan %s: %v\n", s.ID, err)
			return 1
		}
		if d.Action == app.ActionUpload {
			write(stdout, "would upload %s\n", s.ID)
			continue
		}
		write(stdout, "would skip   %s (%s)\n", s.ID, d.Reason)
	}
	return 0
}

// loginGarmin logs into Garmin, prompting for an MFA code on stdin when the
// account requires a verification step.
func loginGarmin(ctx context.Context, svc *app.Service, email, password string, stdin *os.File, stderr *os.File) error {
	err := svc.LoginGarmin(ctx, email, password)
	if err != garmin.ErrMFARequired {
		return err
	}
	write(stderr, "Garmin requires a verification code: ")
	code, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil {
		return fmt.Errorf("read code: %w", err)
	}
	if err := svc.SubmitMFA(ctx, strings.TrimSpace(code)); err != nil {
		return fmt.Errorf("mfa: %w", err)
	}
	return nil
}

// watchLoop scans a local MyWhoosh FIT directory for finished rides, fixing,
// spoofing, and uploading each one. Rides whose upload failed are retried on a
// later scan; Ctrl-C exits.
func watchLoop(w *watch.Watcher, svc *app.Service, cfg config.Config, stdin *os.File, stdout, stderr *os.File) int {
	if cfg.Garmin.Email == "" {
		write(stderr, "watch mode needs --garmin-email\n")
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := loginGarmin(ctx, svc, cfg.Garmin.Email, os.Getenv("WHOOSH2CONNECT_GARMIN_PASSWORD"), stdin, stderr); err != nil {
		write(stderr, "garmin: %v\n", err)
		return 1
	}
	write(stdout, "watching %s (every %s)\n", w.Dir(), w.Interval())

	for {
		rides, err := w.Scan()
		if err != nil {
			write(stderr, "watch: %v\n", err)
			return 1
		}
		for _, r := range rides {
			res := svc.ProcessLocalFit(ctx, "local:"+r.Hash, r.Path)
			write(stdout, "%s %s — %s\n", statusWord(res), r.File, res.DescribeUpload())
			if res.Status == app.StatusFailed {
				w.Forget(r.Hash)
			}
		}
		select {
		case <-ctx.Done():
			return 0
		case <-time.After(w.Interval()):
		}
	}
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

// verifyLedger cross-checks uploaded ledger entries against Garmin Connect and
// prints each mismatch. It returns 1 when any mismatch is found.
func verifyLedger(cfg config.Config, svc *app.Service, stdin *os.File, stdout, stderr *os.File) int {
	if cfg.Garmin.Email == "" {
		write(stderr, "verify needs --garmin-email\n")
		return 1
	}
	ctx := context.Background()
	if err := loginGarmin(ctx, svc, cfg.Garmin.Email, os.Getenv("WHOOSH2CONNECT_GARMIN_PASSWORD"), stdin, stderr); err != nil {
		write(stderr, "garmin: %v\n", err)
		return 1
	}
	findings, err := svc.Verify(ctx)
	for _, f := range findings {
		write(stdout, "mismatch %s: %s\n", f.ID, f.Detail)
	}
	if err != nil {
		write(stderr, "verify: %v\n", err)
		return 1
	}
	if len(findings) > 0 {
		return 1
	}
	write(stdout, "ledger matches Garmin Connect\n")
	return 0
}
