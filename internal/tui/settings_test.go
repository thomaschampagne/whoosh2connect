package tui

import (
	"strings"
	"testing"
)

func TestSettingsValidate(t *testing.T) {
	tests := []struct {
		name    string
		s       Settings
		wantErr string
	}{
		{name: "ok",
			s: Settings{MywhooshEmail: "m@x.com", GarminEmail: "g@x.com", DevicePreset: "edge_540"}},
		{name: "missing mywhoosh", s: Settings{GarminEmail: "g@x.com", DevicePreset: "edge_540"},
			wantErr: "mywhoosh email"},
		{name: "missing garmin", s: Settings{MywhooshEmail: "m@x.com", DevicePreset: "edge_540"},
			wantErr: "garmin email"},
		{name: "missing preset", s: Settings{MywhooshEmail: "m@x.com", GarminEmail: "g@x.com"},
			wantErr: "preset"},
		{name: "unknown preset", s: Settings{MywhooshEmail: "m@x.com", GarminEmail: "g@x.com", DevicePreset: "edge_999"},
			wantErr: "preset"},
		{name: "bad serial", s: Settings{MywhooshEmail: "m@x.com", GarminEmail: "g@x.com", DevicePreset: "edge_540", DeviceSerial: "xyz"},
			wantErr: "device serial must be a whole number"},
		{name: "zero serial", s: Settings{MywhooshEmail: "m@x.com", GarminEmail: "g@x.com", DevicePreset: "edge_540", DeviceSerial: "0"},
			wantErr: "device serial must be a whole number"},
		{name: "ok with serial", s: Settings{MywhooshEmail: "m@x.com", GarminEmail: "g@x.com", DevicePreset: "edge_540", DeviceSerial: "123456789", DeviceName: "My Edge"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.s.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestSerialValue(t *testing.T) {
	if got := (Settings{DeviceSerial: "4242"}).SerialValue(); got != 4242 {
		t.Fatalf("SerialValue() = %d", got)
	}
	if got := (Settings{}).SerialValue(); got != 0 {
		t.Fatalf("SerialValue() no serial = %d", got)
	}
}

func TestSettingsSetField(t *testing.T) {
	var s Settings
	vals := []string{"m@x.com", "p1", "g@x.com", "p2", "edge_1030", "My Edge", "777"}
	if len(vals) != len(settingsFields) {
		t.Fatalf("test values (%d) != settingsFields (%d)", len(vals), len(settingsFields))
	}
	for i, v := range vals {
		s.setField(i, v)
		if got := fieldValue(s, i); got != v {
			t.Fatalf("field %d = %q, want %q", i, got, v)
		}
	}
}
