package stringutil

// ShortenString returns the first N slice of a string.
func ShortenString(str string, n int) string {
	if len(str) <= n {
		return str
	}
	return str[:n]
}
