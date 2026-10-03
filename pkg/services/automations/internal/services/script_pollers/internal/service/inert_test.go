package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	scriptpollerswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/wire"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewServiceIsInert(t *testing.T) {
	t.Parallel()

	clock := &observedScheduler{Clock: clockwork.NewFakeClock()}
	runner := &sequenceCommandRunner{}
	cursors := &observedCursorRecorder{}
	logCore, logs := observer.New(zap.InfoLevel)
	policyCalls := 0
	policy := factorydefinitionfixtures.WorkstationExecutionPolicy{
		Resolve: func(*factorydefinitions.FactoryWorkstationConfig) (time.Duration, error) {
			policyCalls++
			return 0, nil
		},
	}
	service := scriptpollerswire.NewService(zap.New(logCore), clock, runner, nil, policy, cursors)
	if service == nil {
		t.Fatal("expected inert script pollers service")
	}
	if clock.calls != 0 || runner.callCount() != 0 || cursors.calls != 0 || policyCalls != 0 || logs.Len() != 0 {
		t.Fatalf("construction applied effects: clock=%d runner=%d cursors=%d policy=%d logs=%d",
			clock.calls, runner.callCount(), cursors.calls, policyCalls, logs.Len())
	}
}

type observedScheduler struct {
	clockwork.Clock
	calls int
}

func (s *observedScheduler) Now() time.Time {
	s.calls++
	return s.Clock.Now()
}

func (s *observedScheduler) After(delay time.Duration) <-chan time.Time {
	s.calls++
	return s.Clock.After(delay)
}

type observedCursorRecorder struct {
	calls int
}

func (r *observedCursorRecorder) GetCursor(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error) {
	r.calls++
	return automations.GetCursorResult{}, nil
}

func (r *observedCursorRecorder) CommitCursor(context.Context, scriptpollers.CommitCursorRequest) error {
	r.calls++
	return nil
}
