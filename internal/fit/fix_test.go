package fit

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"whoosh2connect/internal/device"
)

// createTestFitFile builds a synthetic MyWhoosh-like FIT:
// records with power/HR/cadence/temperature, and a session with NO averages.
func createTestFitFile(t *testing.T) []byte {
	t.Helper()
	now := time.Now().Truncate(time.Second)
	activity := filedef.NewActivity()
	activity.FileId.
		SetType(typedef.FileActivity).
		SetTimeCreated(now).
		SetManufacturer(typedef.ManufacturerDevelopment).
		SetProduct(0)

	for i := 0; i < 10; i++ {
		activity.Records = append(activity.Records,
			mesgdef.NewRecord(nil).
				SetTimestamp(now.Add(time.Duration(i)*time.Second)).
				SetPower(200).
				SetHeartRate(150).
				SetCadence(90).
				SetTemperature(25))
	}
	activity.Laps = append(activity.Laps,
		mesgdef.NewLap(nil).SetTimestamp(now.Add(10*time.Second)).SetStartTime(now))
	activity.Sessions = append(activity.Sessions,
		mesgdef.NewSession(nil).
			SetTimestamp(now.Add(10*time.Second)).
			SetStartTime(now).
			SetSport(typedef.SportCycling))
	activity.Activity = mesgdef.NewActivity(nil).
		SetTimestamp(now.Add(10 * time.Second)).
		SetType(typedef.ActivityManual).
		SetNumSessions(1)

	f := activity.ToFIT(nil)
	var buf bytes.Buffer
	if err := encoder.New(&buf).Encode(&f); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodeActivity(t *testing.T, data []byte) *filedef.Activity {
	t.Helper()
	lis := filedef.NewListener()
	defer lis.Close()
	if _, err := decoder.New(bytes.NewReader(data),
		decoder.WithMesgListener(lis),
		decoder.WithBroadcastOnly(),
	).Decode(); err != nil {
		t.Fatal(err)
	}
	a, ok := lis.File().(*filedef.Activity)
	if !ok {
		t.Fatalf("expected activity, got %T", lis.File())
	}
	return a
}

func testDevice(t *testing.T) device.Device {
	t.Helper()
	p, err := device.Lookup("edge_540")
	if err != nil {
		t.Fatal(err)
	}
	return device.Device{Preset: p, Serial: 3420897194}
}

func TestFixFillsAveragesStripsTempSpoofsDevice(t *testing.T) {
	in := createTestFitFile(t)
	var out bytes.Buffer

	stats, err := Fix(bytes.NewReader(in), &out, testDevice(t))
	if err != nil {
		t.Fatal(err)
	}

	if stats.Records != 10 {
		t.Errorf("records = %d, want 10", stats.Records)
	}
	if stats.Samples.Power != 10 || stats.Samples.HeartRate != 10 || stats.Samples.Cadence != 10 {
		t.Errorf("samples = %+v", stats.Samples)
	}
	if !stats.Fixes.Power || !stats.Fixes.HeartRate || !stats.Fixes.Cadence {
		t.Errorf("fixes = %+v", stats.Fixes)
	}

	result := decodeActivity(t, out.Bytes())

	for i, rec := range result.Records {
		if rec.Temperature != sint8Invalid {
			t.Errorf("record %d: temperature not stripped (got %d)", i, rec.Temperature)
		}
	}
	if len(result.Sessions) != 1 {
		t.Fatalf("sessions = %d", len(result.Sessions))
	}
	sess := result.Sessions[0]
	if sess.AvgPower != 200 {
		t.Errorf("avg power = %d, want 200", sess.AvgPower)
	}
	if sess.AvgHeartRate != 150 {
		t.Errorf("avg HR = %d, want 150", sess.AvgHeartRate)
	}
	if sess.AvgCadence != 90 {
		t.Errorf("avg cadence = %d, want 90", sess.AvgCadence)
	}
	if result.FileId.Manufacturer != device.Manufacturer {
		t.Errorf("manufacturer = %d", result.FileId.Manufacturer)
	}
	if result.FileId.Product != 4061 {
		t.Errorf("product = %d, want 4061", result.FileId.Product)
	}
	if result.FileId.SerialNumber != 3420897194 {
		t.Errorf("serial = %d", result.FileId.SerialNumber)
	}
}

func TestFixPreservesExistingAverages(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	activity := filedef.NewActivity()
	activity.FileId.SetType(typedef.FileActivity).SetTimeCreated(now).SetManufacturer(1)
	for i := 0; i < 3; i++ {
		activity.Records = append(activity.Records,
			mesgdef.NewRecord(nil).SetTimestamp(now.Add(time.Duration(i)*time.Second)).SetPower(500))
	}
	activity.Sessions = append(activity.Sessions,
		mesgdef.NewSession(nil).
			SetTimestamp(now.Add(3*time.Second)).
			SetStartTime(now).
			SetAvgPower(120).
			SetAvgHeartRate(140).
			SetAvgCadence(80))

	var buf bytes.Buffer
	f := activity.ToFIT(nil)
	if err := encoder.New(&buf).Encode(&f); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if _, err := Fix(bytes.NewReader(buf.Bytes()), &out, testDevice(t)); err != nil {
		t.Fatal(err)
	}
	result := decodeActivity(t, out.Bytes())
	sess := result.Sessions[0]
	if sess.AvgPower != 120 || sess.AvgHeartRate != 140 || sess.AvgCadence != 80 {
		t.Errorf("existing averages clobbered: %+v", sess)
	}
}

func TestFixNoSamplesLeavesAveragesUnset(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	activity := filedef.NewActivity()
	activity.FileId.SetType(typedef.FileActivity).SetTimeCreated(now).SetManufacturer(1)
	for i := 0; i < 3; i++ {
		activity.Records = append(activity.Records,
			mesgdef.NewRecord(nil).SetTimestamp(now.Add(time.Duration(i)*time.Second)))
	}
	activity.Sessions = append(activity.Sessions,
		mesgdef.NewSession(nil).SetTimestamp(now.Add(3*time.Second)).SetStartTime(now))

	var buf bytes.Buffer
	f := activity.ToFIT(nil)
	if err := encoder.New(&buf).Encode(&f); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	stats, err := Fix(bytes.NewReader(buf.Bytes()), &out, testDevice(t))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Fixes.Power || stats.Fixes.HeartRate || stats.Fixes.Cadence {
		t.Errorf("fixes applied with no samples: %+v", stats.Fixes)
	}
}

func TestFixRejectsNonActivity(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("this is not a fit file")
	if _, err := Fix(bytes.NewReader(buf.Bytes()), &bytes.Buffer{}, testDevice(t)); err == nil {
		t.Fatal("expected error for non-FIT input")
	}
}

func TestMostRecentFitFile(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		filepath.Join(dir, "MyNewActivity-3.7.0.fit"),
		filepath.Join(dir, "MyNewActivity-3.8.5.fit"),
		filepath.Join(dir, "MyNewActivity-3.8.1.fit"),
	}
	for _, n := range names {
		if err := os.WriteFile(n, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := MostRecentFitFile(dir, names)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "MyNewActivity-3.8.5.fit") {
		t.Errorf("most recent = %s", got)
	}
}

func TestMostRecentFitFileEmpty(t *testing.T) {
	if _, err := MostRecentFitFile(t.TempDir(), nil); err == nil {
		t.Fatal("expected error for empty list")
	}
}

func TestStatsTotalSamples(t *testing.T) {
	stats := Stats{Records: 7}
	if stats.TotalSamples() != 7 {
		t.Errorf("TotalSamples = %d", stats.TotalSamples())
	}
}
