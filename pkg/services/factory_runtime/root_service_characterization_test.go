package factory_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// peerRootService is a fake peer implementer of the singular Factory Runtime
// root Service. It depends only on the published root package plus approved
// peer contracts (Definitions, Work) and never imports factory_runtime/internal.
type peerRootService struct {
	pauseErr error
}

var _ factoryruntime.Service = (*peerRootService)(nil)

func (s *peerRootService) Pause(context.Context) error  { return s.pauseErr }
func (s *peerRootService) Resume(context.Context) error { return nil }
func (s *peerRootService) Terminate(context.Context, factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	return factoryruntime.TerminateResult{Outcome: factoryruntime.ControlOutcomeAccepted}, nil
}
func (s *peerRootService) GetFactoryEvents(context.Context) ([]factorydefinitions.FactoryEvent, error) {
	return nil, nil
}
func (s *peerRootService) WaitToComplete() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}
func (s *peerRootService) SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return work.WorkRequestSubmitResult{}, nil
}
func (s *peerRootService) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	return nil, nil
}
func (s *peerRootService) GetEngineStateSnapshot(context.Context) (*factoryruntime.StateSnapshot, error) {
	return nil, nil
}
func (s *peerRootService) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{}, nil
}
func (s *peerRootService) PlanDispatch(context.Context, factoryruntime.PlanDispatchRequest) (factoryruntime.PlanDispatchResult, error) {
	return factoryruntime.PlanDispatchResult{Outcome: factoryruntime.DispatchPlanOutcomeAccepted}, nil
}
func (s *peerRootService) AcceptDispatchResult(context.Context, factoryruntime.AcceptDispatchResultRequest) (factoryruntime.AcceptDispatchResultResult, error) {
	return factoryruntime.AcceptDispatchResultResult{Outcome: factoryruntime.DispatchPlanOutcomeRetired}, nil
}
func (s *peerRootService) CaptureCheckpoint(context.Context, factoryruntime.CaptureCheckpointRequest) (factoryruntime.CaptureCheckpointResult, error) {
	return factoryruntime.CaptureCheckpointResult{Outcome: factoryruntime.CheckpointOutcomeCaptured}, nil
}
func (s *peerRootService) LoadCheckpoint(context.Context, factoryruntime.LoadCheckpointRequest) (factoryruntime.LoadCheckpointResult, error) {
	return factoryruntime.LoadCheckpointResult{Outcome: factoryruntime.CheckpointOutcomeLoaded}, nil
}
func (s *peerRootService) RestoreCheckpoint(context.Context, factoryruntime.RestoreCheckpointRequest) (factoryruntime.RestoreCheckpointResult, error) {
	return factoryruntime.RestoreCheckpointResult{Outcome: factoryruntime.CheckpointOutcomeRestored}, nil
}
func (s *peerRootService) MoveWork(
	context.Context,
	string,
	string,
	work.WorkStateChangeSource,
	string,
) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, nil
}

func TestRootService_FakePeerPauseReturnsTypedNotRunning(t *testing.T) {
	t.Parallel()

	var runtime factoryruntime.Service = &peerRootService{pauseErr: factoryruntime.ErrNotRunning}

	err := runtime.Pause(context.Background())
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("Pause error = %v, want typed ErrNotRunning", err)
	}
}
