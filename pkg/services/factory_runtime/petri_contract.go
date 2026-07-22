package factory

import petri "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"

type (
	PetriWorkIDGenerator        = petri.WorkIDGenerator
	PetriTransition             = petri.Transition
	PetriTransitionType         = petri.TransitionType
	PetriRuntimeGuardContext    = petri.RuntimeGuardContext
	PetriRuntimeGuard           = petri.RuntimeGuard
	PetriActivePauseProvider    = petri.ActivePauseProvider
	PetriInferenceThrottleGuard = petri.InferenceThrottleGuard
	PetriCronTimeWindowGuard    = petri.CronTimeWindowGuard
	PetriExpiredTimeWorkGuard   = petri.ExpiredTimeWorkGuard
	PetriGuard                  = petri.Guard
	PetriClockedGuard           = petri.ClockedGuard
	PetriMatchColorGuard        = petri.MatchColorGuard
	PetriSameNameGuard          = petri.SameNameGuard
	PetriSameTraceIDGuard       = petri.SameTraceIDGuard
	PetriAllGuard               = petri.AllGuard
	PetriMatchesFieldsGuard     = petri.MatchesFieldsGuard
	PetriVisitCountGuard        = petri.VisitCountGuard
	PetriAllWithParentGuard     = petri.AllWithParentGuard
	PetriAnyWithParentGuard     = petri.AnyWithParentGuard
	PetriDependencyGuard        = petri.DependencyGuard
	PetriFanoutCountGuard       = petri.FanoutCountGuard
	PetriPlace                  = petri.Place
	PetriArc                    = petri.Arc
	PetriArcDirection           = petri.ArcDirection
	PetriArcCardinality         = petri.ArcCardinality
	PetriCardinalityMode        = petri.CardinalityMode
	PetriMarking                = petri.Marking
	PetriMarkingSnapshot        = petri.MarkingSnapshot
)

const (
	PetriTransitionNormal       = petri.TransitionNormal
	PetriTransitionExhaustion   = petri.TransitionExhaustion
	PetriArcInput               = petri.ArcInput
	PetriArcOutput              = petri.ArcOutput
	PetriCardinalityOne         = petri.CardinalityOne
	PetriCardinalityAll         = petri.CardinalityAll
	PetriCardinalityAllTerminal = petri.CardinalityAllTerminal
	PetriCardinalityN           = petri.CardinalityN
	PetriCardinalityZeroOrMore  = petri.CardinalityZeroOrMore
)
