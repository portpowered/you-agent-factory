package hostedsources

import hostedlinear "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/internal/linear"

var (
	// ErrWorkAdmission reports that Work admission failed for a generated hosted-source
	// request. The wrapped error preserves Work-owned rejection semantics without
	// reinterpreting them inside hosted_sources.
	ErrWorkAdmission = hostedlinear.ErrWorkAdmission
	// ErrSecretResolution reports that hosted-source credential resolution failed.
	ErrSecretResolution = hostedlinear.ErrSecretResolution
)
