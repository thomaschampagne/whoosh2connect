package garmin

// truncate limits an error response body to a readable prefix.
func truncate(b []byte) string {
	if len(b) > 300 {
		return string(b[:300])
	}
	return string(b)
}
