package agypty

import "errors"

// ErrPTYAllocationFailed reports that ConPTY or POSIX PTY allocation failed.
// It is distinct from ErrUnsupportedPlatform.
var ErrPTYAllocationFailed = errors.New("agypty: PTY allocation failed")

// ErrSessionTimedOut reports that idle or hard timeout canceled the PTY session.
var ErrSessionTimedOut = errors.New("agypty: session timed out")

// ErrNonzeroExit reports that the supervised Agy child exited with a nonzero status.
var ErrNonzeroExit = errors.New("agypty: process exited with nonzero status")
