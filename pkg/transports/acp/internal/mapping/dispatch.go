package mapping

import (
	"fmt"
	"slices"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// legalPhasesByKind mirrors
// pkg/services/workers/internal/services/workstations/draftvalidation's
// allowedPhasesByKind -- the one legal Kind/Phase pair policy owner -- so
// this outbound projector rejects exactly the pairs response-draft
// validation already rejects before a record can be published. It is
// duplicated rather than imported: draftvalidation lives under
// pkg/services/workers/internal, which this transport-owned package cannot
// import. TestProjectDispatch_MatchesDeclaredOutcomeTable in dispatch_test.go
// keeps this copy honest against draftvalidation's own
// TestACPL1OutcomeParity_MatchesLegalPairsExactly.
var legalPhasesByKind = map[workers.Kind][]workers.Phase{
	workers.KindSession:    {workers.PhaseStarted, workers.PhaseCompleted, workers.PhaseFailed, workers.PhaseCanceled},
	workers.KindRun:        {workers.PhaseStarted, workers.PhaseCompleted, workers.PhaseFailed, workers.PhaseCanceled},
	workers.KindTurn:       {workers.PhaseStarted, workers.PhaseCompleted, workers.PhaseFailed, workers.PhaseCanceled},
	workers.KindMessage:    {workers.PhaseStarted, workers.PhaseDelta, workers.PhaseCompleted},
	workers.KindReasoning:  {workers.PhaseStarted, workers.PhaseDelta, workers.PhaseCompleted},
	workers.KindTool:       {workers.PhaseStarted, workers.PhaseDelta, workers.PhaseCompleted, workers.PhaseFailed, workers.PhaseCanceled},
	workers.KindFileChange: {workers.PhaseUpdated},
	workers.KindPlan:       {workers.PhaseUpdated},
	workers.KindProgress:   {workers.PhaseUpdated},
	workers.KindUsage:      {workers.PhaseUpdated},
	workers.KindError:      {workers.PhaseUpdated, workers.PhaseFailed},
	workers.KindStreamGap:  {workers.PhaseUpdated},
}

// Project is the one exhaustive dispatch entry point over every current
// workers.Kind/workers.Phase pair: it routes a legal pair to its declared
// outcome -- MESSAGE_CHUNK, THOUGHT_CHUNK, USAGE_UPDATE, or GAP_NOTICE
// project onto the matching Project* function; NO_OUTPUT kinds (SESSION,
// RUN, TURN, TOOL, FILE_CHANGE, PLAN, PROGRESS) and ERROR (declared
// ERROR_OUT_OF_BAND -- surfaced through a different channel than
// session/update notifications, which this pure projector does not own)
// both resolve to (nil, nil) -- and rejects every unknown Kind or
// Kind/Phase pair that response-draft validation would also have rejected,
// returning (nil, ErrMalformedRecord) rather than silently classifying it.
//
// SESSION deserves the same note final-proposal.md §6.2 makes about PLAN:
// the doc's projection table lists "Title/time change -> session_info_update"
// as an intended L1 capability, but workers.SessionPayload carries no title
// or other ACP-supported customer metadata field, and workers.Kind has no
// separate value for a metadata change distinct from session lifecycle --
// only KindSession's lifecycle phases (STARTED/COMPLETED/FAILED/CANCELED)
// exist today, and draftvalidation already declares all of them NO_OUTPUT.
// Adding a title/time source fact would mean inventing a new taxonomy
// member, which conflicts with this PRD's own top-level "no new event
// taxonomy" acceptance criterion. L1 therefore declares session_info_update
// out of scope here as a deliberate scope cut, not a missing case --
// identical in kind to PLAN's scope cut, not because the dispatch is
// incomplete.
func Project(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	if !isLegalKindPhase(draft.Kind, draft.Phase) {
		return nil, fmt.Errorf(
			"%w: kind %q phase %q is not a declared projectable combination",
			ErrMalformedRecord, draft.Kind, draft.Phase,
		)
	}

	switch draft.Kind {
	case workers.KindMessage:
		return ProjectMessage(draft)
	case workers.KindReasoning:
		return ProjectReasoning(draft)
	case workers.KindUsage:
		return ProjectUsage(draft)
	case workers.KindStreamGap:
		return ProjectStreamGap(draft)
	default:
		return nil, nil
	}
}

func isLegalKindPhase(kind workers.Kind, phase workers.Phase) bool {
	phases, ok := legalPhasesByKind[kind]
	if !ok {
		return false
	}
	return slices.Contains(phases, phase)
}
