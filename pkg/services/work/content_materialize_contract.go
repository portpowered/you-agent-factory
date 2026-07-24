package work

import "errors"

// Materialization typed failures peers can distinguish on the root Service
// content materialization slice (MaterializeContentURL). Implementations may
// wrap these sentinels with additional context; peers should branch with
// errors.Is.
var (
	// ErrUnsafeContentURL reports that materialization rejected a content URL
	// that would resolve to a disallowed or private target.
	ErrUnsafeContentURL = errors.New("unsafe Work content URL")

	// ErrContentURLInaccessible reports that a remote or otherwise resolvable
	// content URL could not be retrieved.
	ErrContentURLInaccessible = errors.New("Work content URL inaccessible")
)
