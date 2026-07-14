package agypty

import "errors"

// ErrPTYAllocationFailed reports that ConPTY or POSIX PTY allocation failed.
// It is distinct from ErrUnsupportedPlatform.
var ErrPTYAllocationFailed = errors.New("agypty: PTY allocation failed")
