package fit

import (
	"testing"
)

func TestStartTime(t *testing.T) {
	data := createTestFitFile(t)
	got, err := StartTime(data)
	if err != nil {
		t.Fatal(err)
	}
	want := decodeActivity(t, data).Sessions[0].StartTime
	if !got.Equal(want) {
		t.Fatalf("StartTime = %v, want %v", got, want)
	}
}

func TestStartTimeRejectsGarbage(t *testing.T) {
	if _, err := StartTime([]byte("not a fit file")); err == nil {
		t.Fatal("expected error")
	}
}
