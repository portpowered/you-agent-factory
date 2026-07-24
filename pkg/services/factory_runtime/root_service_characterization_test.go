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
