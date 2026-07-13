package responseevents

import shared "github.com/portpowered/infinite-you/pkg/interfaces/responseevents"

type Phase = shared.Phase

const (
	PhaseStarted   = shared.PhaseStarted
	PhaseDelta     = shared.PhaseDelta
	PhaseUpdated   = shared.PhaseUpdated
	PhaseCompleted = shared.PhaseCompleted
	PhaseFailed    = shared.PhaseFailed
	PhaseCanceled  = shared.PhaseCanceled
)
