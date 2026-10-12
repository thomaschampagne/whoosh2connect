package tui

import (
	"fmt"
	"strconv"
	"strings"

	"whoosh2connect/internal/device"
)

// Settings holds the user-editable credentials and device choice. DeviceSerial
// is raw text; empty means "leave the persisted serial unchanged".
type Settings struct {
	MywhooshEmail    string
	MywhooshPassword string
	GarminEmail      string
	GarminPassword   string
	DevicePreset     string
	DeviceName       string
	DeviceSerial     string
}

// Validate returns an error if any field is missing or malformed.
func (s Settings) Validate() error {
	switch {
	case s.MywhooshEmail == "":
		return fmt.Errorf("mywhoosh email is required")
	case s.GarminEmail == "":
		return fmt.Errorf("garmin email is required")
	case s.DevicePreset == "":
		return fmt.Errorf("device preset is required")
	}
	if _, err := device.Lookup(s.DevicePreset); err != nil {
		return fmt.Errorf("device preset: %w", err)
	}
	if s.DeviceSerial != "" {
		if _, err := s.serial(); err != nil {
			return err
		}
	}
	return nil
}

// serial parses DeviceSerial as a non-zero uint32.
func (s Settings) serial() (uint32, error) {
	n, err := strconv.ParseUint(s.DeviceSerial, 10, 32)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("device serial must be a non-zero uint32")
	}
	return uint32(n), nil
}

// HasSerial reports whether the user supplied a serial override.
func (s Settings) HasSerial() bool { return s.DeviceSerial != "" }

// SerialValue returns the parsed override, or 0 when none is set.
func (s Settings) SerialValue() uint32 {
	if !s.HasSerial() {
		return 0
	}
	n, _ := s.serial()
	return n
}

// setField updates field i with a value during settings editing.
func (s *Settings) setField(i int, v string) {
	switch i {
	case 0:
		s.MywhooshEmail = v
	case 1:
		s.MywhooshPassword = v
	case 2:
		s.GarminEmail = v
	case 3:
		s.GarminPassword = v
	case 4:
		s.DevicePreset = v
	case 5:
		s.DeviceName = v
	case 6:
		s.DeviceSerial = v
	}
}

// settingsFields mirror the input order in the settings screen.
var settingsFields = []struct {
	label       string
	masked      bool
	placeholder string
}{
	{label: "MyWhoosh email", placeholder: "you@example.com"},
	{label: "MyWhoosh password", masked: true, placeholder: "hunter2"},
	{label: "Garmin email", placeholder: "you@example.com"},
	{label: "Garmin password", masked: true, placeholder: "hunter2"},
	{label: "Device preset", placeholder: "edge_540"},
	{label: "Device name", placeholder: "(preset default)"},
	{label: "Device serial", placeholder: "(keep current)"},
}

func fieldValue(s Settings, i int) string {
	switch i {
	case 0:
		return s.MywhooshEmail
	case 1:
		return s.MywhooshPassword
	case 2:
		return s.GarminEmail
	case 3:
		return s.GarminPassword
	case 4:
		return s.DevicePreset
	case 5:
		return s.DeviceName
	case 6:
		return s.DeviceSerial
	}
	return ""
}

// fieldError reports the inline validation error for one field, if any.
func fieldError(s Settings, i int) string {
	switch i {
	case 0:
		if s.MywhooshEmail == "" {
			return "required"
		}
	case 2:
		if s.GarminEmail == "" {
			return "required"
		}
	case 4:
		if s.DevicePreset == "" {
			return "required"
		}
		if _, err := device.Lookup(s.DevicePreset); err != nil {
			return "unknown preset (e.g. edge_540, edge_1030)"
		}
	case 6:
		if s.DeviceSerial != "" {
			if _, err := s.serial(); err != nil {
				return "must be a non-zero uint32"
			}
		}
	}
	return ""
}

func renderSettings(th theme, s Settings, cursor int) string {
	var b strings.Builder
	for i, f := range settingsFields {
		label := th.paint(f.label, th.text)
		if i == cursor {
			label = th.bold(f.label, th.accent)
		}
		value := fieldValue(s, i)
		if f.masked && value != "" {
			value = strings.Repeat("•", len(value))
		}
		if value == "" {
			value = th.paint(f.placeholder, th.muted)
		}
		fmt.Fprintf(&b, "  %s: %s", label, value)
		if err := fieldError(s, i); err != "" {
			fmt.Fprintf(&b, "  %s", th.paint("← "+err, th.danger))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(th.paint("  save: ctrl+s  next: tab/↓  cancel: esc", th.muted))
	return b.String()
}
