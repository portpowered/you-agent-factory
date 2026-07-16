package validation

import interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"

// Profile selects which validation rules run for one OpenAPI factory payload.
type Profile string

const (
	// ProfileTopology runs structural validation on a mapped FactoryConfig without
	// canonical JSON normalization or LoadFromCanonicalJSON blocking checks. It
	// matches POST /factory-validations: duplicate identifiers, dangling references,
	// conflicting outputs, and outcome-route / work-type completion invariants.
	ProfileTopology Profile = "topology"

	// ProfilePrePersist runs the same pre-write checks as editable factory save and
	// CLI save-from-file: canonical JSON normalization via LoadFromCanonicalJSON
	// (including bundled-file and blocking-load rules), then full structural
	// validation when load succeeds. Blocking-load failures return a subset of
	// structural targets that omit deferred outcome-route-only codes.
	ProfilePrePersist Profile = "pre_persist"
)

// WorkflowSourceReader loads file-backed JavaScript workflow source for validation.
type WorkflowSourceReader interface {
	ReadWorkflowSource(sourceRef string) (string, error)
}

// Options configures ValidateFactoryAPI for a single validation invocation.
type Options struct {
	// Profile selects topology-only vs pre-persist canonical checks. When empty,
	// ProfileTopology is used.
	Profile Profile

	// WorkstationLoader is required for ProfilePrePersist when split worker or
	// workstation definitions must be resolved during LoadFromCanonicalJSON.
	// Values implementing pkg/config.WorkstationLoader are accepted.
	WorkstationLoader WorkstationLoader

	// WorkflowSourceReader resolves file-backed orchestrator.javascript.sourceRef
	// values for workflow source validation. Inline source is always validated.
	WorkflowSourceReader WorkflowSourceReader
}

// WorkstationLoader loads workstation definitions by name for pre-persist validation.
type WorkstationLoader interface {
	Load(name string) (*interfaces.FactoryWorkstationConfig, error)
}

// ResolvedProfile returns the profile to apply, defaulting to ProfileTopology.
func (o Options) ResolvedProfile() Profile {
	if o.Profile == "" {
		return ProfileTopology
	}
	return o.Profile
}
