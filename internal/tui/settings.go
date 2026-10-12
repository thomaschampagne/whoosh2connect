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

// Validate returns an error if any field is missing or malformed, with a
// message a person can act on.
func (s Settings) Validate() error {
	switch {
	case strings.TrimSpace(s.MywhooshEmail) == "":
		return fmt.Errorf("mywhoosh email is required")
	case strings.TrimSpace(s.GarminEmail) == "":
		return fmt.Errorf("garmin email is required")
	case strings.TrimSpace(s.DevicePreset) == "":
		return fmt.Errorf("a device preset is required")
	}
	if _, err := device.Lookup(s.DevicePreset); err != nil {
		return fmt.Errorf("unknown device preset %q — start typing to pick one from the list", s.DevicePreset)
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
	n, err := strconv.ParseUint(strings.TrimSpace(s.DeviceSerial), 10, 32)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("device serial must be a whole number from 1 to 4294967295")
	}
	return uint32(n), nil
}

// HasSerial reports whether the user supplied a serial override.
func (s Settings) HasSerial() bool { return strings.TrimSpace(s.DeviceSerial) != "" }

// SerialValue returns the parsed override, or 0 when none is set.
func (s Settings) SerialValue() uint32 {
	if !s.HasSerial() {
		return 0
	}
	n, _ := s.serial()
	return n
}

// errorField returns the index of the first field that fails validation, or -1
// when the settings are valid.
func (s Settings) errorField() int {
	for i := range settingsFields {
		if fieldError(s, i) != "" {
			return i
		}
	}
	return -1
}

// fieldKind drives input behaviour and the autocomplete for each form field.
type fieldKind int

const (
	fieldText fieldKind = iota
	fieldSecret
	fieldPreset
	fieldSerial
)

// field indices, matching settingsFields order.
const (
	fldMywhooshEmail = iota
	fldMywhooshPassword
	fldGarminEmail
	fldGarminPassword
	fldDevicePreset
	fldDeviceName
	fldDeviceSerial
)

// fieldSpec describes one settings form field.
type fieldSpec struct {
	label       string
	placeholder string
	kind        fieldKind
	maxLen      int
}

// settingsFields mirror the input order in the settings screen.
var settingsFields = []fieldSpec{
	{label: "MyWhoosh email", placeholder: "you@example.com", kind: fieldText, maxLen: 254},
	{label: "MyWhoosh password", placeholder: "hunter2", kind: fieldSecret, maxLen: 256},
	{label: "Garmin email", placeholder: "you@example.com", kind: fieldText, maxLen: 254},
	{label: "Garmin password", placeholder: "hunter2", kind: fieldSecret, maxLen: 256},
	{label: "Device preset", placeholder: "edge_540", kind: fieldPreset, maxLen: 40},
	{label: "Device name", placeholder: "(preset default)", kind: fieldText, maxLen: 64},
	{label: "Device serial", placeholder: "(auto-generate)", kind: fieldSerial, maxLen: 10},
}

// setField updates field i with a value.
func (s *Settings) setField(i int, v string) {
	switch i {
	case fldMywhooshEmail:
		s.MywhooshEmail = v
	case fldMywhooshPassword:
		s.MywhooshPassword = v
	case fldGarminEmail:
		s.GarminEmail = v
	case fldGarminPassword:
		s.GarminPassword = v
	case fldDevicePreset:
		s.DevicePreset = v
	case fldDeviceName:
		s.DeviceName = v
	case fldDeviceSerial:
		s.DeviceSerial = v
	}
}

func fieldValue(s Settings, i int) string {
	switch i {
	case fldMywhooshEmail:
		return s.MywhooshEmail
	case fldMywhooshPassword:
		return s.MywhooshPassword
	case fldGarminEmail:
		return s.GarminEmail
	case fldGarminPassword:
		return s.GarminPassword
	case fldDevicePreset:
		return s.DevicePreset
	case fldDeviceName:
		return s.DeviceName
	case fldDeviceSerial:
		return s.DeviceSerial
	}
	return ""
}

// fieldError reports the inline validation hint for one field, if any. Messages
// are written for people, not parsers.
func fieldError(s Settings, i int) string {
	switch i {
	case fldMywhooshEmail:
		if strings.TrimSpace(s.MywhooshEmail) == "" {
			return "required"
		}
	case fldGarminEmail:
		if strings.TrimSpace(s.GarminEmail) == "" {
			return "required"
		}
	case fldDevicePreset:
		if strings.TrimSpace(s.DevicePreset) == "" {
			return "required"
		}
		if _, err := device.Lookup(s.DevicePreset); err != nil {
			return "unknown preset — start typing to pick from the list"
		}
	case fldDeviceSerial:
		if strings.TrimSpace(s.DeviceSerial) != "" {
			if _, err := s.serial(); err != nil {
				return "must be a whole number from 1 to 4294967295"
			}
		}
	}
	return ""
}
