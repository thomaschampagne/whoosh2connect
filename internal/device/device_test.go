package device

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
)

func TestRequiredPresetsExist(t *testing.T) {
	for _, id := range []string{"edge_540", "edge_1030"} {
		p, err := Lookup(id)
		if err != nil {
			t.Fatalf("required preset %s missing: %v", id, err)
		}
		if p.Product == 0 {
			t.Errorf("preset %s has zero product", id)
		}
		if p.DefaultName == "" {
			t.Errorf("preset %s has empty name", id)
		}
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, err := Lookup("edge_9999"); err == nil {
		t.Fatal("expected error for unknown preset")
	}
}

func TestPresetsSorted(t *testing.T) {
	ps := Presets()
	if len(ps) != len(presets) {
		t.Fatalf("len(Presets()) = %d, want %d", len(ps), len(presets))
	}
	for i := 1; i < len(ps); i++ {
		if ps[i-1].DefaultName > ps[i].DefaultName {
			t.Errorf("presets not sorted at %d: %s > %s", i, ps[i-1].DefaultName, ps[i].DefaultName)
		}
	}
}

func TestPresetProductIDsMatchSDK(t *testing.T) {
	cases := map[string]uint16{
		"edge_540":     4061,
		"edge_1030":    2713,
		"edge_530":     3121,
		"edge_520":     2067,
		"edge_1040":    3843,
		"edge_830":     3122,
		"edge_explore": 3011,
		"edge_1050":    4440,
		"edge_840":     4062,
		"edge_1000":    1836,
	}
	for id, want := range cases {
		p, err := Lookup(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.Product.Uint16(); got != want {
			t.Errorf("%s product = %d, want %d", id, got, want)
		}
	}
}

func newTestActivity() *filedef.Activity {
	now := time.Now().Truncate(time.Second)
	a := filedef.NewActivity()
	a.FileId.SetTimeCreated(now).SetManufacturer(1).SetProduct(0).SetSerialNumber(0)
	a.DeviceInfos = append(a.DeviceInfos,
		mesgdef.NewDeviceInfo(nil).SetTimestamp(now),
		mesgdef.NewDeviceInfo(nil).SetTimestamp(now.Add(time.Second)),
	)
	for i := 0; i < 5; i++ {
		a.Records = append(a.Records, mesgdef.NewRecord(nil).SetTimestamp(now.Add(time.Duration(i)*time.Second)).SetPower(200))
	}
	a.Sessions = append(a.Sessions, mesgdef.NewSession(nil).SetTimestamp(now.Add(5*time.Second)).SetStartTime(now))
	return a
}

func TestApplyPresetFields(t *testing.T) {
	p, _ := Lookup("edge_540")
	d := Device{Preset: p, Serial: 3420897194}
	a := newTestActivity()

	if err := Apply(a, d); err != nil {
		t.Fatal(err)
	}
	if a.FileId.Manufacturer != Manufacturer {
		t.Errorf("file_id manufacturer = %d, want %d", a.FileId.Manufacturer, Manufacturer)
	}
	if want := uint16(4061); a.FileId.Product != want {
		t.Errorf("file_id product = %d, want %d", a.FileId.Product, want)
	}
	if a.FileId.SerialNumber != 3420897194 {
		t.Errorf("file_id serial = %d, want 3420897194", a.FileId.SerialNumber)
	}
	if a.FileId.ProductName != "Edge 540" {
		t.Errorf("file_id name = %q, want %q", a.FileId.ProductName, "Edge 540")
	}
	for i, di := range a.DeviceInfos {
		if di.Manufacturer != Manufacturer || di.Product != 4061 || di.SerialNumber != 3420897194 {
			t.Errorf("device_info[%d] not spoofed: %+v", i, di)
		}
	}
}

func TestApplyNameOverride(t *testing.T) {
	p, _ := Lookup("edge_540")
	d := Device{Preset: p, Name: "My Edge 540", Serial: 42}
	a := newTestActivity()
	if err := Apply(a, d); err != nil {
		t.Fatal(err)
	}
	if a.FileId.ProductName != "My Edge 540" {
		t.Errorf("name = %q, want override", a.FileId.ProductName)
	}
	if d.DisplayName() != "My Edge 540" {
		t.Errorf("DisplayName = %q", d.DisplayName())
	}
}

func TestApplyInvalidSerial(t *testing.T) {
	p, _ := Lookup("edge_540")
	if err := Apply(newTestActivity(), Device{Preset: p, Serial: 0}); err == nil {
		t.Fatal("expected error for zero serial")
	}
}

func TestValidateSerial(t *testing.T) {
	if err := ValidateSerial(0); err == nil {
		t.Error("zero serial should be invalid")
	}
	if err := ValidateSerial(1); err != nil {
		t.Errorf("one should be valid: %v", err)
	}
}

func TestGenerateSerialNonZero(t *testing.T) {
	for i := 0; i < 100; i++ {
		s, err := GenerateSerial()
		if err != nil {
			t.Fatal(err)
		}
		if s == 0 {
			t.Fatal("generated zero serial")
		}
	}
}

func TestApplyRoundTripThroughEncoder(t *testing.T) {
	p, _ := Lookup("edge_1030")
	d := Device{Preset: p, Name: "Rider Edge", Serial: 777}
	out := filepath.Join(t.TempDir(), "fixed.fit")

	in := newTestActivity()
	if err := Apply(in, d); err != nil {
		t.Fatal(err)
	}
	f := in.ToFIT(nil)
	fh, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fh.Close() }()
	if err := encoder.New(fh, encoder.WithProtocolVersion(2)).Encode(&f); err != nil {
		t.Fatal(err)
	}

	back, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = back.Close() }()
	lis := filedef.NewListener()
	defer lis.Close()
	if _, err := decoder.New(back, decoder.WithMesgListener(lis), decoder.WithBroadcastOnly()).Decode(); err != nil {
		t.Fatal(err)
	}
	got := lis.File().(*filedef.Activity)
	if got.FileId.Product != 2713 || got.FileId.SerialNumber != 777 || got.FileId.Manufacturer != Manufacturer {
		t.Errorf("file_id after round trip: %+v", got.FileId)
	}
	if got.FileId.ProductName != "Rider Edge" {
		t.Errorf("name after round trip = %q", got.FileId.ProductName)
	}
}
