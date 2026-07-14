package responseevents

import shared "github.com/portpowered/infinite-you/pkg/interfaces/responseevents"

type Kind = shared.Kind

const (
	KindSession    = shared.KindSession
	KindRun        = shared.KindRun
	KindTurn       = shared.KindTurn
	KindMessage    = shared.KindMessage
	KindReasoning  = shared.KindReasoning
	KindTool       = shared.KindTool
	KindFileChange = shared.KindFileChange
	KindPlan       = shared.KindPlan
	KindProgress   = shared.KindProgress
	KindUsage      = shared.KindUsage
	KindError      = shared.KindError
	KindStreamGap  = shared.KindStreamGap
)
