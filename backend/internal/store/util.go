package store

import "strings"

// nullInt maps a zero int to SQL NULL (used for optional numeric columns).
func nullInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

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
