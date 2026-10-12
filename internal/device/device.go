package device

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/muktihari/fit/profile/filedef"
)

// ErrInvalidSerial signals that a serial number is not usable (zero).
var ErrInvalidSerial = errors.New("serial number must be a non-zero uint32")

// Device is a fully-resolved spoofing target: a preset plus optional name and
// serial overrides.
type Device struct {
	Preset Preset
	// Name overrides the device ProductName. Empty means use the preset name.
	Name string
	// Serial is persisted per-install. Must be non-zero.
	Serial uint32
}

// DisplayName returns the name Garmin Connect will show for this device.
func (d Device) DisplayName() string {
	if d.Name != "" {
		return d.Name
	}
	return d.Preset.DefaultName
}

// Validate returns an error if the device is unusable.
func (d Device) Validate() error {
	if d.Preset.ID == "" {
		return errors.New("device preset is empty")
	}
	if err := ValidateSerial(d.Serial); err != nil {
		return err
	}
	return nil
}

// ValidateSerial checks that a serial is a non-zero uint32.
func ValidateSerial(serial uint32) error {
	if serial == 0 {
		return ErrInvalidSerial
	}
	return nil
}

// GenerateSerial returns a cryptographically random non-zero uint32.
func GenerateSerial() (uint32, error) {
	var buf [4]byte
	for {
		if _, err := rand.Read(buf[:]); err != nil {
			return 0, fmt.Errorf("read entropy: %w", err)
		}
		n := binary.BigEndian.Uint32(buf[:])
		if n != 0 {
			return n, nil
		}
	}
}

// Apply spoofs the given device onto the activity's file_id and every
// device_info message, and optionally sets the product name.
func Apply(activity *filedef.Activity, d Device) error {
	if err := d.Validate(); err != nil {
		return err
	}

	activity.FileId.Manufacturer = Manufacturer
	activity.FileId.Product = d.Preset.Product.Uint16()
	activity.FileId.SerialNumber = d.Serial
	if name := d.DisplayName(); name != "" {
		activity.FileId.ProductName = name
	}

	for _, di := range activity.DeviceInfos {
		di.Manufacturer = Manufacturer
		di.Product = d.Preset.Product.Uint16()
		di.SerialNumber = d.Serial
		if name := d.DisplayName(); name != "" {
			di.ProductName = name
		}
	}
	return nil
}
