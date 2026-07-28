package factorydefinitions

import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"

// Deletion-only aliases retain temporary Factory Definitions root symbols for
// worker and provider execution vocabulary rehomed to pkg/services/workers and
// pkg/services/providers in CLN-DEF-CONTRACTS story 005. Peers must import
// Workers or Providers root execution contracts instead; remove this file when
// downstream consumers finish cutover.

type (
	WorkstationInput           = contracts.WorkstationInput
	WorkstationOutput          = contracts.WorkstationOutput
	WorkstationRequestPayload  = contracts.WorkstationRequestPayload
	WorkstationResponsePayload = contracts.WorkstationResponsePayload
	WorkstationResult          = contracts.WorkstationResult
)
