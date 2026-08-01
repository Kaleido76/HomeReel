package store

import "time"

// timeLayout is a fixed-width RFC3339-like format with nanosecond precision.
// Fixed width keeps lexicographic comparisons (used for scan recency) correct.
const timeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// nowRFC3339 returns the current UTC time in timeLayout.
func nowRFC3339() string {
	return time.Now().UTC().Format(timeLayout)
}
