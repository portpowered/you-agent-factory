package processmemory

import "errors"

// CommitReader reads the current process's committed memory in bytes.
type CommitReader func() (uint64, error)

// ErrUnavailable identifies platforms or process states where committed
// process memory cannot be read without substituting a different signal.
var ErrUnavailable = errors.New("process commit is unavailable")
