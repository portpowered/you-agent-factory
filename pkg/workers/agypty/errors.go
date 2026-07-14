package agypty

import "errors"

// ErrPTYAllocationFailed reports that ConPTY or POSIX PTY allocation failed.
// It is distinct from ErrUnsupportedPlatform.
var ErrPTYAllocationFailed = errors.New("agypty: PTY allocation failed")

// errSessionRunPending reports that PTYSession.Run is not implemented yet.
// Story 002 implements bounded capture, timeout, and cleanup in Run.
var errSessionRunPending = errors.New("agypty: PTY session run is not implemented")
