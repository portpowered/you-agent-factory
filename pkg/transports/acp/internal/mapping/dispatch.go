package mapping

import (
	"fmt"
	"slices"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// legalPhasesByKind mirrors
// pkg/services/workers/internal/draftvalidation's
// allowedPhasesByKind -- the one legal Kind/Phase pair policy owner -- so
// this outbound projector rejects exactly the pairs response-draft
// validation already rejects before a record can be published. It is
// duplicated rather than imported: draftvalidation lives under
// pkg/services/workers/internal, which this transport-owned package cannot
// import. TestProjectDispatch_MatchesDeclaredOutcomeTable in dispatch_test.go
// keeps this copy honest against draftvalidation's own
// TestACPL1OutcomeParity_MatchesLegalPairsExactly.
var legalPhasesByKind = map[workers.Kind][]workers.Phase{
	workers.KindSession:    {workers.PhaseStarted, workers.PhaseUpdated, workers.PhaseCompleted, workers.PhaseFailed, workers.PhaseCanceled},
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
// outcome -- MESSAGE_CHUNK, THOUGHT_CHUNK, USAGE_UPDATE, SESSION_INFO_UPDATE,
// or GAP_NOTICE project onto the matching Project* function; NO_OUTPUT kinds
// (RUN, TURN, TOOL, FILE_CHANGE, PLAN, PROGRESS), SESSION's lifecycle phases
// (STARTED/COMPLETED/FAILED/CANCELED), and ERROR (declared ERROR_OUT_OF_BAND
// -- surfaced through a different channel than session/update notifications,
// which this pure projector does not own) all resolve to (nil, nil) -- and
// Project rejects every unknown Kind or Kind/Phase pair that response-draft
// validation would also have rejected, returning (nil, ErrMalformedRecord)
// rather than silently classifying it.
//
// SESSION/UPDATED is the one KindSession phase that is not lifecycle-only:
// it is the source representation for final-proposal.md §6.2's "Title/time
// change -> session_info_update" -- see workers.SessionPayload.Title and
// ProjectSessionInfoUpdate. It routes through the same case as SESSION's
// lifecycle phases because ProjectSessionInfoUpdate itself resolves
// STARTED/COMPLETED/FAILED/CANCELED to (nil, nil), keeping this switch's
// dispatch key purely Kind-based like every other case.
func Project(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	if !isLegalKindPhase(draft.Kind, draft.Phase) {
		return nil, fmt.Errorf(
			"%w: kind %q phase %q is not a declared projectable combination",
			ErrMalformedRecord, draft.Kind, draft.Phase,
		)
	}

	switch draft.Kind {
	case workers.KindSession:
		return ProjectSessionInfoUpdate(draft)
	case workers.KindMessage:
		return ProjectMessage(draft)
	case workers.KindReasoning:
		return ProjectReasoning(draft)
	case workers.KindUsage:
		return ProjectUsage(draft)
	case workers.KindStreamGap:
		return ProjectStreamGap(draft)
	case workers.KindPlan:
		return ProjectPlan(draft)
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
