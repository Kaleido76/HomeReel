package domain

import "time"

// TimeLayout is the fixed-width nanosecond UTC timestamp used across the data
// layer. Fixed width keeps lexicographic string ordering equal to time ordering
// (used for scan recency and history comparisons).
const TimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// Now returns the current UTC time formatted with TimeLayout.
func Now() string {
	return time.Now().UTC().Format(TimeLayout)
}
