package factory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// peerSealedRootService is the singular peer-shaped consumer used to seal
// CTR-RUN root-contract invariants. It implements only the published root
// Service surface plus approved Work peer contracts and never imports
// factory_runtime/internal, Petri orchestrator packages, or JavaScript
// strategy packages.
type peerSealedRootService struct {
	peerRootService

	pauseErr     error
	observeErr   error
	observation  factoryruntime.Observation
	planErr      error
	acceptErr    error
	captureErr   error
	restoreErr   error
	checkpoint   factoryruntime.Checkpoint
	terminateErr error
}

var _ factoryruntime.Service = (*peerSealedRootService)(nil)

func (s *peerSealedRootService) Pause(context.Context) error {
	if s.pauseErr != nil {
		return s.pauseErr
	}
	return nil
}

func (s *peerSealedRootService) Terminate(context.Context, factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	if s.terminateErr != nil {
		return factoryruntime.TerminateResult{}, s.terminateErr
	}
	return factoryruntime.TerminateResult{Outcome: factoryruntime.ControlOutcomeAccepted}, nil
}

func (s *peerSealedRootService) Observe(_ context.Context, _ factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	if s.observeErr != nil {
		return factoryruntime.ObserveResult{}, s.observeErr
	}
	return factoryruntime.ObserveResult{Observation: s.observation}, nil
}

func (s *peerSealedRootService) PlanDispatch(
	_ context.Context,
	req factoryruntime.PlanDispatchRequest,
) (factoryruntime.PlanDispatchResult, error) {
	if s.planErr != nil {
		return factoryruntime.PlanDispatchResult{}, s.planErr
	}
	return factoryruntime.PlanDispatchResult{
		Outcome:       factoryruntime.DispatchPlanOutcomeAccepted,
		DispatchID:    req.DispatchID,
		CorrelationID: req.CorrelationID,
	}, nil
}

func (s *peerSealedRootService) AcceptDispatchResult(
	_ context.Context,
	req factoryruntime.AcceptDispatchResultRequest,
) (factoryruntime.AcceptDispatchResultResult, error) {
	if s.acceptErr != nil {
		return factoryruntime.AcceptDispatchResultResult{}, s.acceptErr
	}
	return factoryruntime.AcceptDispatchResultResult{
		Outcome:       factoryruntime.DispatchPlanOutcomeRetired,
		DispatchID:    req.DispatchID,
		CorrelationID: req.CorrelationID,
	}, nil
}

func (s *peerSealedRootService) CaptureCheckpoint(
	_ context.Context,
	req factoryruntime.CaptureCheckpointRequest,
) (factoryruntime.CaptureCheckpointResult, error) {
	if s.captureErr != nil {
		return factoryruntime.CaptureCheckpointResult{}, s.captureErr
	}
	checkpoint := s.checkpoint
	if checkpoint.CheckpointID == "" {
		checkpoint = factoryruntime.Checkpoint{
			CheckpointID:  req.CheckpointID,
			SchemaVersion: 1,
			StrategyKind:  "runtime",
			Payload:       []byte(`{"opaque":true}`),
		}
	}
	return factoryruntime.CaptureCheckpointResult{
		Outcome:    factoryruntime.CheckpointOutcomeCaptured,
		Checkpoint: checkpoint,
	}, nil
}

func (s *peerSealedRootService) RestoreCheckpoint(
	_ context.Context,
	req factoryruntime.RestoreCheckpointRequest,
) (factoryruntime.RestoreCheckpointResult, error) {
	if s.restoreErr != nil {
		return factoryruntime.RestoreCheckpointResult{}, s.restoreErr
	}
	return factoryruntime.RestoreCheckpointResult{
		Outcome:      factoryruntime.CheckpointOutcomeRestored,
		CheckpointID: req.Checkpoint.CheckpointID,
	}, nil
}

func (s *peerSealedRootService) MoveWork(
	_ context.Context,
	workID string,
	stateName string,
	_ work.WorkStateChangeSource,
	_ string,
) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{WorkID: workID, FromState: "inbox", ToState: stateName}, nil
}

func TestRootContractSeal_SingularServiceReachesAllPublishedSlices(t *testing.T) {
	t.Parallel()

	var runtime factoryruntime.Service = &peerSealedRootService{
		observation: factoryruntime.Observation{
			Status: factoryruntime.ObservationStatusActive,
			Progress: factoryruntime.ObservationProgress{
				InFlightDispatchCount: 1,
				TickCount:             3,
			},
			InFlightDispatches: []factoryruntime.ObservationDispatchSummary{{
				DispatchID: "dispatch-1",
				WorkIDs:    []string{"work-1"},
				Status:     "IN_FLIGHT",
			}},
			Health: factoryruntime.ObservationHealth{
				FactoryState: "RUNNING",
				Uptime:       time.Second,
			},
		},
	}

	assertSealedControlSuccess(t, runtime)
	assertSealedObservationSuccess(t, runtime)
	assertSealedDispatchPlanSuccess(t, runtime)
	assertSealedCheckpointSuccess(t, runtime)
}

func assertSealedControlSuccess(t *testing.T, runtime factoryruntime.Service) {
	t.Helper()
	pauseResult, err := factoryruntime.ApplyPause(context.Background(), runtime, factoryruntime.PauseRequest{})
	if err != nil {
		t.Fatalf("control ApplyPause error = %v, want nil", err)
	}
	if pauseResult.Outcome != factoryruntime.ControlOutcomeAccepted {
		t.Fatalf("control PauseResult.Outcome = %q, want ACCEPTED", pauseResult.Outcome)
	}
}

func assertSealedObservationSuccess(t *testing.T, runtime factoryruntime.Service) {
	t.Helper()
	observed, err := factoryruntime.ApplyObserve(context.Background(), runtime, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeFull,
	})
	if err != nil {
		t.Fatalf("observation ApplyObserve error = %v, want nil", err)
	}
	if observed.Observation.Status != factoryruntime.ObservationStatusActive {
		t.Fatalf("observation Status = %q, want ACTIVE", observed.Observation.Status)
	}
}

func assertSealedDispatchPlanSuccess(t *testing.T, runtime factoryruntime.Service) {
	t.Helper()
	planned, err := factoryruntime.ApplyPlanDispatch(context.Background(), runtime, factoryruntime.PlanDispatchRequest{
		DispatchID:      "dispatch-1",
		CorrelationID:   "corr-1",
		WorkIDs:         []string{"work-1"},
		WorkstationName: "review",
	})
	if err != nil {
		t.Fatalf("dispatch-plan ApplyPlanDispatch error = %v, want nil", err)
	}
	if planned.Outcome != factoryruntime.DispatchPlanOutcomeAccepted {
		t.Fatalf("dispatch-plan Outcome = %q, want ACCEPTED", planned.Outcome)
	}

	accepted, err := factoryruntime.ApplyAcceptDispatchResult(context.Background(), runtime, factoryruntime.AcceptDispatchResultRequest{
		DispatchID:    "dispatch-1",
		CorrelationID: "corr-1",
		ResultOutcome: factoryruntime.DispatchResultOutcomeSuccess,
	})
	if err != nil {
		t.Fatalf("dispatch-plan ApplyAcceptDispatchResult error = %v, want nil", err)
	}
	if accepted.Outcome != factoryruntime.DispatchPlanOutcomeRetired {
		t.Fatalf("dispatch-plan Accept Outcome = %q, want RETIRED", accepted.Outcome)
	}
}

func assertSealedCheckpointSuccess(t *testing.T, runtime factoryruntime.Service) {
	t.Helper()
	captured, err := factoryruntime.ApplyCaptureCheckpoint(context.Background(), runtime, factoryruntime.CaptureCheckpointRequest{
		CheckpointID: "checkpoint-1",
	})
	if err != nil {
		t.Fatalf("checkpoint ApplyCaptureCheckpoint error = %v, want nil", err)
	}
	if captured.Outcome != factoryruntime.CheckpointOutcomeCaptured || len(captured.Checkpoint.Payload) == 0 {
		t.Fatalf("checkpoint Capture result = %#v, want CAPTURED opaque payload", captured)
	}

	restored, err := factoryruntime.ApplyRestoreCheckpoint(context.Background(), runtime, factoryruntime.RestoreCheckpointRequest{
		Checkpoint: captured.Checkpoint,
	})
	if err != nil {
		t.Fatalf("checkpoint ApplyRestoreCheckpoint error = %v, want nil", err)
	}
	if restored.Outcome != factoryruntime.CheckpointOutcomeRestored {
		t.Fatalf("checkpoint Restore Outcome = %q, want RESTORED", restored.Outcome)
	}
}

func TestRootContractSeal_SingularServiceTypedFailuresAcrossSlices(t *testing.T) {
	t.Parallel()

	for _, tc := range sealedTypedFailureCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.call(t, sealedTypedFailurePeer())
		})
	}
}

type sealedTypedFailureCase struct {
	name string
	call func(t *testing.T, runtime factoryruntime.Service)
}

func sealedTypedFailurePeer() *peerSealedRootService {
	return &peerSealedRootService{
		pauseErr:     factoryruntime.ErrNotRunning,
		terminateErr: factoryruntime.ErrAlreadyStopped,
		observeErr:   factoryruntime.ErrNotFound,
		planErr:      factoryruntime.ErrDuplicateDispatchIntent,
		acceptErr:    factoryruntime.ErrUnknownDispatchCorrelation,
		captureErr:   factoryruntime.ErrCheckpointNotFound,
		restoreErr:   factoryruntime.ErrCorruptCheckpoint,
	}
}

func sealedTypedFailureCases() []sealedTypedFailureCase {
	return []sealedTypedFailureCase{
		{
			name: "control not running",
			call: func(t *testing.T, runtime factoryruntime.Service) {
				t.Helper()
				_, err := factoryruntime.ApplyPause(context.Background(), runtime, factoryruntime.PauseRequest{})
				if !errors.Is(err, factoryruntime.ErrNotRunning) {
					t.Fatalf("ApplyPause error = %v, want ErrNotRunning", err)
				}
			},
		},
		{
			name: "control already stopped",
			call: func(t *testing.T, runtime factoryruntime.Service) {
				t.Helper()
				_, err := factoryruntime.ApplyTerminate(context.Background(), runtime, factoryruntime.TerminateRequest{Reason: "stop"})
				if !errors.Is(err, factoryruntime.ErrAlreadyStopped) {
					t.Fatalf("ApplyTerminate error = %v, want ErrAlreadyStopped", err)
				}
			},
		},
		{
			name: "observation not found",
			call: func(t *testing.T, runtime factoryruntime.Service) {
				t.Helper()
				_, err := factoryruntime.ApplyObserve(context.Background(), runtime, factoryruntime.ObserveRequest{})
				if !errors.Is(err, factoryruntime.ErrNotFound) {
					t.Fatalf("ApplyObserve error = %v, want ErrNotFound", err)
				}
			},
		},
		{
			name: "dispatch-plan duplicate intent",
			call: func(t *testing.T, runtime factoryruntime.Service) {
				t.Helper()
				_, err := factoryruntime.ApplyPlanDispatch(context.Background(), runtime, factoryruntime.PlanDispatchRequest{
					DispatchID:    "dispatch-1",
					CorrelationID: "corr-1",
					WorkIDs:       []string{"work-1"},
				})
				if !errors.Is(err, factoryruntime.ErrDuplicateDispatchIntent) {
					t.Fatalf("ApplyPlanDispatch error = %v, want ErrDuplicateDispatchIntent", err)
				}
			},
		},
		{
			name: "dispatch-plan unknown correlation",
			call: func(t *testing.T, runtime factoryruntime.Service) {
				t.Helper()
				_, err := factoryruntime.ApplyAcceptDispatchResult(context.Background(), runtime, factoryruntime.AcceptDispatchResultRequest{
					DispatchID:    "missing",
					CorrelationID: "missing",
					ResultOutcome: factoryruntime.DispatchResultOutcomeSuccess,
				})
				if !errors.Is(err, factoryruntime.ErrUnknownDispatchCorrelation) {
					t.Fatalf("ApplyAcceptDispatchResult error = %v, want ErrUnknownDispatchCorrelation", err)
				}
			},
		},
		{
			name: "checkpoint corrupt",
			call: func(t *testing.T, runtime factoryruntime.Service) {
				t.Helper()
				_, err := factoryruntime.ApplyRestoreCheckpoint(context.Background(), runtime, factoryruntime.RestoreCheckpointRequest{
					Checkpoint: factoryruntime.Checkpoint{
						CheckpointID:  "checkpoint-1",
						SchemaVersion: 1,
						Payload:       []byte(`{"opaque":true}`),
					},
				})
				if !errors.Is(err, factoryruntime.ErrCorruptCheckpoint) {
					t.Fatalf("ApplyRestoreCheckpoint error = %v, want ErrCorruptCheckpoint", err)
				}
			},
		},
		{
			name: "checkpoint missing",
			call: func(t *testing.T, runtime factoryruntime.Service) {
				t.Helper()
				_, err := factoryruntime.ApplyCaptureCheckpoint(context.Background(), runtime, factoryruntime.CaptureCheckpointRequest{
					CheckpointID: "missing",
				})
				if !errors.Is(err, factoryruntime.ErrCheckpointNotFound) {
					t.Fatalf("ApplyCaptureCheckpoint error = %v, want ErrCheckpointNotFound", err)
				}
			},
		},
	}
}
