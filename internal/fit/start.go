package fit

import (
	"bytes"
	"fmt"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/filedef"
)

// StartTime returns the start time of the first session in an activity FIT
// file. It is used to match a local ride against Garmin Connect.
func StartTime(data []byte) (time.Time, error) {
	lis := filedef.NewListener()
	defer lis.Close()
	if _, err := decoder.New(bytes.NewReader(data),
		decoder.WithMesgListener(lis),
		decoder.WithBroadcastOnly(),
	).Decode(); err != nil {
		return time.Time{}, fmt.Errorf("decode: %w", err)
	}
	act, ok := lis.File().(*filedef.Activity)
	if !ok {
		return time.Time{}, fmt.Errorf("not an activity file (got %T)", lis.File())
	}
	if len(act.Sessions) == 0 || act.Sessions[0].StartTime.IsZero() {
		return time.Time{}, fmt.Errorf("no session start time")
	}
	return act.Sessions[0].StartTime, nil
}
