package work

import "errors"

// Invocation/return-policy typed failures peers can distinguish on the root
// Service slice (PrepareInvocationInput / ResolvePrimaryResult).
// Implementations may wrap these sentinels with additional context; peers
// should branch with errors.Is. Structured *InputError and *PrimaryResultError
// values remain valid typed failures for more specific codes.
var (
	// ErrInvalidInvocationInput reports that invocation-input preparation
	// rejected raw edge values that failed Work-owned validation.
	ErrInvalidInvocationInput = errors.New("invalid invocation input")

	// ErrUnsupportedReturnPolicy reports that primary-result selection rejected
	// an unsupported or invalid invocation return-policy configuration.
	ErrUnsupportedReturnPolicy = errors.New("unsupported invocation return policy")
)
