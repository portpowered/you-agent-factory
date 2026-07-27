package linear

import "errors"

var (
	// ErrWorkAdmission reports that Work admission failed for a generated hosted-source
	// request. The wrapped error preserves Work-owned rejection semantics without
	// reinterpreting them inside hosted_sources.
	ErrWorkAdmission = errors.New("hosted sources: work admission failed")
	// ErrSecretResolution reports that hosted-source credential resolution failed.
	// The wrapped error preserves the underlying resolution cause without performing
	// Work admission or reporting successful observation convergence.
	ErrSecretResolution = errors.New("hosted sources: secret resolution failed")
)
