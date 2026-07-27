package linear

import "errors"

var (
	// ErrWorkAdmission reports that Work admission failed for a generated hosted-source
	// request. The wrapped error preserves Work-owned rejection semantics without
	// reinterpreting them inside hosted_sources.
	ErrWorkAdmission = errors.New("hosted sources: work admission failed")
)
