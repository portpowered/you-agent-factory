package validation

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"

// Profile selects which validation rules run for one OpenAPI factory payload.
type Profile = factorydefinitions.ValidationProfile

const (
	// ProfileTopology runs structural validation on a mapped FactoryConfig without
	// canonical JSON normalization or LoadFromCanonicalJSON blocking checks. It
	// matches POST /factory-validations: duplicate identifiers, dangling references,
	// conflicting outputs, and outcome-route / work-type completion invariants.
	ProfileTopology = factorydefinitions.ValidationProfileTopology

	// ProfilePrePersist runs the same pre-write checks as editable factory save and
	// CLI save-from-file: canonical JSON normalization via LoadFromCanonicalJSON
	// (including bundled-file and blocking-load rules), then full structural
	// validation when load succeeds. Blocking-load failures return a subset of
	// structural targets that omit deferred outcome-route-only codes.
	ProfilePrePersist = factorydefinitions.ValidationProfilePrePersist
)

type WorkflowSourceReader = factorydefinitions.WorkflowSourceReader
type WorkstationLoader = factorydefinitions.WorkstationLoader
