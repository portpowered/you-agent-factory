package factory_test

import (
	"context"
	"errors"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// peerDispatchPlanService extends the singular root Service fake with
// dispatch-plan slice outcomes. It depends only on published root types plus
// approved peer contracts and never imports factory_runtime/internal.
type peerDispatchPlanService struct {
	peerRootService

	planErr    error
	planResult factoryruntime.PlanDispatchResult
	acceptErr  error
	acceptOut  factoryruntime.AcceptDispatchResultResult
}

var _ factoryruntime.Service = (*peerDispatchPlanService)(nil)

func (s *peerDispatchPlanService) PlanDispatch(
	_ context.Context,
	req factoryruntime.PlanDispatchRequest,
) (factoryruntime.PlanDispatchResult, error) {
	if s.planErr != nil {
		return factoryruntime.PlanDispatchResult{}, s.planErr
	}
	if s.planResult.DispatchID != "" || s.planResult.Outcome != "" {
		return s.planResult, nil
	}
	return factoryruntime.PlanDispatchResult{
		Outcome:       factoryruntime.DispatchPlanOutcomeAccepted,
		DispatchID:    req.DispatchID,
		CorrelationID: req.CorrelationID,
	}, nil
}

func (s *peerDispatchPlanService) AcceptDispatchResult(
	_ context.Context,
	req factoryruntime.AcceptDispatchResultRequest,
) (factoryruntime.AcceptDispatchResultResult, error) {
	if s.acceptErr != nil {
		return factoryruntime.AcceptDispatchResultResult{}, s.acceptErr
	}
	if s.acceptOut.Outcome != "" {
		return s.acceptOut, nil
	}
	return factoryruntime.AcceptDispatchResultResult{
		Outcome:       factoryruntime.DispatchPlanOutcomeRetired,
		DispatchID:    req.DispatchID,
		CorrelationID: req.CorrelationID,
	}, nil
}

func TestRootDispatchPlan_FakePeerPlanAndAcceptSuccessShapes(t *testing.T) {
	t.Parallel()

	var runtime factoryruntime.Service = &peerDispatchPlanService{}

	planned, err := factoryruntime.ApplyPlanDispatch(context.Background(), runtime, factoryruntime.PlanDispatchRequest{
		DispatchID:      "dispatch-1",
		CorrelationID:   "corr-1",
		WorkIDs:         []string{"work-1"},
		WorkstationName: "review",
		WorkerType:      "agent",
		ReplayKey:       "replay-1",
	})
	if err != nil {
		t.Fatalf("ApplyPlanDispatch error = %v, want nil", err)
	}
	if planned != (factoryruntime.PlanDispatchResult{
		Outcome:       factoryruntime.DispatchPlanOutcomeAccepted,
		DispatchID:    "dispatch-1",
		CorrelationID: "corr-1",
	}) {
		t.Fatalf("PlanDispatchResult = %#v, want plain root success shape", planned)
	}

	accepted, err := factoryruntime.ApplyAcceptDispatchResult(context.Background(), runtime, factoryruntime.AcceptDispatchResultRequest{
		DispatchID:    "dispatch-1",
		CorrelationID: "corr-1",
		WorkID:        "work-1",
		ResultOutcome: factoryruntime.DispatchResultOutcomeSuccess,
	})
	if err != nil {
		t.Fatalf("ApplyAcceptDispatchResult error = %v, want nil", err)
	}
	if accepted != (factoryruntime.AcceptDispatchResultResult{
		Outcome:       factoryruntime.DispatchPlanOutcomeRetired,
		DispatchID:    "dispatch-1",
		CorrelationID: "corr-1",
	}) {
		t.Fatalf("AcceptDispatchResultResult = %#v, want plain root success shape", accepted)
	}
}

func TestRootDispatchPlan_FakePeerIdempotentDuplicateVocabulary(t *testing.T) {
	t.Parallel()

	var runtime factoryruntime.Service = &peerDispatchPlanService{
		planResult: factoryruntime.PlanDispatchResult{
			Outcome:       factoryruntime.DispatchPlanOutcomeDuplicateIdempotent,
			DispatchID:    "dispatch-1",
			CorrelationID: "corr-1",
		},
		acceptOut: factoryruntime.AcceptDispatchResultResult{
			Outcome:       factoryruntime.DispatchPlanOutcomeDuplicateIdempotent,
			DispatchID:    "dispatch-1",
			CorrelationID: "corr-1",
		},
	}

	planned, err := factoryruntime.ApplyPlanDispatch(context.Background(), runtime, factoryruntime.PlanDispatchRequest{
		DispatchID:    "dispatch-1",
		CorrelationID: "corr-1",
	})
	if err != nil {
		t.Fatalf("ApplyPlanDispatch error = %v, want nil", err)
	}
	if planned.Outcome != factoryruntime.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf("PlanDispatchResult.Outcome = %q, want DUPLICATE_IDEMPOTENT vocabulary", planned.Outcome)
	}

	accepted, err := factoryruntime.ApplyAcceptDispatchResult(context.Background(), runtime, factoryruntime.AcceptDispatchResultRequest{
		DispatchID:    "dispatch-1",
		CorrelationID: "corr-1",
		ResultOutcome: factoryruntime.DispatchResultOutcomeSuccess,
	})
	if err != nil {
		t.Fatalf("ApplyAcceptDispatchResult error = %v, want nil", err)
	}
	if accepted.Outcome != factoryruntime.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf("AcceptDispatchResultResult.Outcome = %q, want DUPLICATE_IDEMPOTENT vocabulary", accepted.Outcome)
	}
}

func TestRootDispatchPlan_FakePeerTypedFailures(t *testing.T) {
	t.Parallel()

	t.Run("duplicate intent", func(t *testing.T) {
		t.Parallel()
		var runtime factoryruntime.Service = &peerDispatchPlanService{
			planErr: factoryruntime.ErrDuplicateDispatchIntent,
		}
		_, err := factoryruntime.ApplyPlanDispatch(context.Background(), runtime, factoryruntime.PlanDispatchRequest{
			DispatchID:    "dispatch-dup",
			CorrelationID: "corr-dup",
		})
		if !errors.Is(err, factoryruntime.ErrDuplicateDispatchIntent) {
			t.Fatalf("ApplyPlanDispatch error = %v, want ErrDuplicateDispatchIntent", err)
		}
	})

	t.Run("unknown correlation", func(t *testing.T) {
		t.Parallel()
		var runtime factoryruntime.Service = &peerDispatchPlanService{
			acceptErr: factoryruntime.ErrUnknownDispatchCorrelation,
		}
		_, err := factoryruntime.ApplyAcceptDispatchResult(context.Background(), runtime, factoryruntime.AcceptDispatchResultRequest{
			DispatchID:    "dispatch-missing",
			CorrelationID: "corr-missing",
			ResultOutcome: factoryruntime.DispatchResultOutcomeSuccess,
		})
		if !errors.Is(err, factoryruntime.ErrUnknownDispatchCorrelation) {
			t.Fatalf("ApplyAcceptDispatchResult error = %v, want ErrUnknownDispatchCorrelation", err)
		}
	})

	t.Run("invalid result boundary", func(t *testing.T) {
		t.Parallel()
		var runtime factoryruntime.Service = &peerDispatchPlanService{
			acceptErr: factoryruntime.ErrInvalidDispatchResultBoundary,
		}
		_, err := factoryruntime.ApplyAcceptDispatchResult(context.Background(), runtime, factoryruntime.AcceptDispatchResultRequest{
			DispatchID:    "dispatch-1",
			CorrelationID: "corr-1",
			ResultOutcome: factoryruntime.DispatchResultOutcome("petri-token-payload"),
		})
		if !errors.Is(err, factoryruntime.ErrInvalidDispatchResultBoundary) {
			t.Fatalf("ApplyAcceptDispatchResult error = %v, want ErrInvalidDispatchResultBoundary", err)
		}
	})

	t.Run("nil runtime not found", func(t *testing.T) {
		t.Parallel()
		_, err := factoryruntime.ApplyPlanDispatch(context.Background(), nil, factoryruntime.PlanDispatchRequest{
			DispatchID: "dispatch-1",
		})
		if !errors.Is(err, factoryruntime.ErrNotFound) {
			t.Fatalf("ApplyPlanDispatch(nil) error = %v, want ErrNotFound", err)
		}
	})
}
