package store

import "strings"

// nullFloat maps a zero float to SQL NULL (used for optional numeric columns).
func nullFloat(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}

// nullString maps an empty string to SQL NULL (used for optional text columns).
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return strings.TrimSpace(s)
}
