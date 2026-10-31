package garmin

// maxResponseBytes caps how much of a Garmin response body is read.
const maxResponseBytes = 10 << 20

// truncate limits an error response body to a readable prefix.
func truncate(b []byte) string {
	if len(b) > 300 {
		return string(b[:300])
	}
	return string(b)
}
