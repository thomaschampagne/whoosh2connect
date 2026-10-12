package tui

import (
	"fmt"
	"strings"
)

// Settings holds the user-editable credentials and device choice.
type Settings struct {
	MywhooshEmail    string
	MywhooshPassword string
	GarminEmail      string
	GarminPassword   string
	DevicePreset     string
}

// Validate returns an error if required fields are missing.
func (s Settings) Validate() error {
	switch {
	case s.MywhooshEmail == "":
		return fmt.Errorf("mywhoosh email is required")
	case s.GarminEmail == "":
		return fmt.Errorf("garmin email is required")
	}
	return nil
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
		fmt.Fprintf(&b, "  %s: %s\n", label, value)
	}
	b.WriteString("\n")
	b.WriteString(th.paint("  save: ctrl+s  next: tab/↓  cancel: esc", th.muted))
	return b.String()
}
