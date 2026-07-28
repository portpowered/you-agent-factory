// Package invocation_policy defines the Factory Definitions-owned private
// invocation-time policy capability behind the published Definitions root
// policy contracts.
//
// The public surface exposes only published root policy contract accessors and
// construction dependencies. It does not declare peer service implementations,
// Wire/root construction ownership, or sibling catalog/authoring_layout/
// compilation/validation/snapshots_portability/distribution APIs.
package invocation_policy

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service owns invocation-time policy implementations behind the published
// Definitions root policy contracts consumed by Wire, Workers, Runtime, and
// Sessions.
type Service interface {
	DecisionEnvelope() factorydefinitions.DecisionEnvelopeService
	InvocationInterpolation() factorydefinitions.InvocationInterpolationService
	InvocationOutput() factorydefinitions.InvocationOutputShapingService
	InvocationWorkType() factorydefinitions.InvocationWorkTypeService
	QuorumPolicy() factorydefinitions.QuorumPolicyService
	WorkPropagation() factorydefinitions.WorkPropagationPolicyService
	WorkstationExecution() factorydefinitions.WorkstationExecutionPolicyService
	TTSObservability() factorydefinitions.TTSObservabilityService
}

// Dependencies are the exact collaborator ports required by invocation_policy.
// They are supplied by Factory Definitions composition and never selected here:
// invocation_policy does not choose host filesystem adapters or Wire/root
// constructors.
type Dependencies struct{}
