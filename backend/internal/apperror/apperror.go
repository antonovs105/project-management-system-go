// Package apperror defines stable error categories shared across domain boundaries.
package apperror

import "errors"

var (
	// ErrNotFound classifies resources that are absent or deliberately hidden.
	ErrNotFound = errors.New("resource not found")
	// ErrForbidden classifies authenticated operations lacking permission.
	ErrForbidden = errors.New("operation forbidden")
	// ErrConflict classifies operations rejected by current resource state.
	ErrConflict = errors.New("resource conflict")
)

// classified preserves a safe message while exposing a stable error kind.
type classified struct {
	kind    error
	message string
}

// Error returns the client-safe domain message.
func (e classified) Error() string { return e.message }

// Unwrap exposes the stable error category to errors.Is.
func (e classified) Unwrap() error { return e.kind }

// New returns an error with a stable category and a client-safe message.
func New(kind error, message string) error {
	return classified{kind: kind, message: message}
}
