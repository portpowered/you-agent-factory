package work

import "errors"

// Admission typed failures peers can distinguish on the root Service admission
// slice (SubmitWorkRequestForSession). Implementations may wrap these sentinels
// with additional context; peers should branch with errors.Is.
var (
	// ErrInvalidWorkRequest reports that admission rejected a Work Request
	// because required identity or payload fields failed Work-owned validation.
	ErrInvalidWorkRequest = errors.New("invalid Work Request")

	// ErrWorkRequestConflict reports that admission conflicted with an already
	// applied request identity or incompatible admission state.
	ErrWorkRequestConflict = errors.New("Work Request admission conflict")

	// ErrWorkRequestRejected reports that admission policy rejected a Work
	// Request without accepting it into a Factory Session.
	ErrWorkRequestRejected = errors.New("Work Request rejected")
)
