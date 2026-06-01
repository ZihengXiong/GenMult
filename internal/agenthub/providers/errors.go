package providers

import "errors"

var (
	// ErrCLINotFound is returned when the required CLI binary is not in PATH.
	ErrCLINotFound = errors.New("CLI binary not found in PATH")

	// ErrAuthFailure is returned when the CLI reports an authentication error.
	ErrAuthFailure = errors.New("CLI authentication failure")

	// ErrRateLimit is returned when the CLI reports a rate-limiting error.
	ErrRateLimit = errors.New("CLI rate limit exceeded")
)
