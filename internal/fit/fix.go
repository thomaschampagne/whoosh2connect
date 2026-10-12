// Package fit fixes MyWhoosh FIT files for upload to Garmin Connect: it fills
// missing session averages, strips bogus record temperature, and applies the
// configured device spoof. Ported faithfully from the mywoosh2garmin fitfix.go
// reference; invalid sentinels and averaging logic match it exactly.
package fit

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/proto"

	"whoosh2connect/internal/device"
)

// FIT protocol invalid sentinel values per base type.
const (
	uint8Invalid  = uint8(0xFF)
	uint16Invalid = uint16(0xFFFF)
	sint8Invalid  = int8(0x7F)
)

// Stats reports what Fix changed.
type Stats struct {
	Records int
	Samples Samples
	Fixes   Fixes
	Device  device.Device
}

// Samples counts valid sample points gathered from records.
type Samples struct {
	Power, HeartRate, Cadence int
}

// Fixes reports which session averages were corrected.
type Fixes struct {
	Power, HeartRate, Cadence bool
}

// TotalSamples is the number of records Fix read.
func (s Stats) TotalSamples() int { return s.Records }

// Fix reads a MyWhoosh FIT activity from in, fixes it, and writes a fixed
// activity file to out using the given device spoof.
func Fix(in io.Reader, out io.Writer, dev device.Device) (Stats, error) {
	var stats Stats
	stats.Device = dev

	lis := filedef.NewListener()
	defer lis.Close()

	dec := decoder.New(in,
		decoder.WithMesgListener(lis),
		decoder.WithBroadcastOnly(),
	)
	if _, err := dec.Decode(); err != nil {
		return stats, fmt.Errorf("decode: %w", err)
	}

	activity, ok := lis.File().(*filedef.Activity)
	if !ok {
		return stats, fmt.Errorf("not an activity file (got %T)", lis.File())
	}

	var powers []uint16
	var heartRates, cadences []uint8

	for _, rec := range activity.Records {
		if rec.Power != uint16Invalid {
			powers = append(powers, rec.Power)
		}
		if rec.HeartRate != uint8Invalid {
			heartRates = append(heartRates, rec.HeartRate)
		}
		if rec.Cadence != uint8Invalid {
			cadences = append(cadences, rec.Cadence)
		}
		rec.Temperature = sint8Invalid
	}
	stats.Records = len(activity.Records)
	stats.Samples = Samples{
		Power:     len(powers),
		HeartRate: len(heartRates),
		Cadence:   len(cadences),
	}

	for _, sess := range activity.Sessions {
		if shouldFixU16(sess.AvgPower) && len(powers) > 0 {
			sess.AvgPower = avgU16(powers)
			stats.Fixes.Power = true
		}
		if shouldFixU8(sess.AvgHeartRate) && len(heartRates) > 0 {
			sess.AvgHeartRate = avgU8(heartRates)
			stats.Fixes.HeartRate = true
		}
		if shouldFixU8(sess.AvgCadence) && len(cadences) > 0 {
			sess.AvgCadence = avgU8(cadences)
			stats.Fixes.Cadence = true
		}
	}

	if err := device.Apply(activity, dev); err != nil {
		return stats, err
	}

	f := activity.ToFIT(nil)
	if err := encoder.New(out, encoder.WithProtocolVersion(proto.V2)).Encode(&f); err != nil {
		return stats, fmt.Errorf("encode: %w", err)
	}
	return stats, nil
}

func shouldFixU16(v uint16) bool { return v == uint16Invalid || v == 0 }
func shouldFixU8(v uint8) bool   { return v == uint8Invalid || v == 0 }

func avgU16(vals []uint16) uint16 {
	var sum uint64
	for _, v := range vals {
		sum += uint64(v)
	}
	return uint16(sum / uint64(len(vals)))
}

func avgU8(vals []uint8) uint8 {
	var sum uint64
	for _, v := range vals {
		sum += uint64(v)
	}
	return uint8(sum / uint64(len(vals)))
}

var versionRe = regexp.MustCompile(`\d+`)

// cmpVersionParts implements the reference numeric version ordering used when
// picking the newest MyNewActivity file.
func cmpVersionParts(a, b []string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		ai, _ := strconv.Atoi(a[i])
		bi, _ := strconv.Atoi(b[i])
		if ai != bi {
			return ai - bi
		}
	}
	return len(a) - len(b)
}

// MostRecentFitFile returns the MyNewActivity-<version>.fit with the highest
// version number in dir, mirroring the reference's file discovery.
func MostRecentFitFile(dir string, names []string) (string, error) {
	if len(names) == 0 {
		return "", fmt.Errorf("no MyNewActivity files in %s", dir)
	}
	out := make([]string, len(names))
	copy(out, names)
	re := versionRe
	sort.Slice(out, func(i, j int) bool {
		vi := re.FindAllString(out[i], -1)
		vj := re.FindAllString(out[j], -1)
		return cmpVersionParts(vi, vj) > 0
	})
	return out[0], nil
}
