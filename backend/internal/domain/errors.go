package domain

import "errors"

var (
	// ErrNotFound is returned when a record does not exist.
	ErrNotFound = errors.New("not found")
	// ErrInvalid is returned when input fails validation.
	ErrInvalid = errors.New("invalid input")
)
