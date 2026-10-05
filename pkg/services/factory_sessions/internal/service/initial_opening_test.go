package service

import (
	"context"
	"errors"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"testing"
)

func TestRuntimeActivationInitialFailureRetainsSoleCloserForRetry(t *testing.T) {
	t.Parallel()
	snapshot := activationSnapshot()
	openingErr, closeErr := errors.New("initial opening failed"), errors.New("partial release failed")
	calls := 0
	owner := &Root{initialActivation: func(context.Context, factoryruntime.RuntimeActivationRequest, factoryruntime.SessionObservations) (*factoryruntime.RuntimeInitialOpening, error) {
		return &factoryruntime.RuntimeInitialOpening{Activation: &factoryruntime.RuntimeActivation{
			Close: func(ctx context.Context) error {
				if ctx.Err() != nil {
					t.Fatal("cleanup inherited opening cancellation")
				}
				calls++
				if calls == 1 {
					return closeErr
				}
				return nil
			},
		}}, openingErr
	}}
	opening := &sessionRuntimeOpening{sessionID: "candidate", configured: preparedRuntime{DefinitionSnapshot: &snapshot},
		load: RuntimeLoad{LoadedFactoryCfg: initialOpeningLoadedStub{}}}
	cleanup := &runtimeOpeningCleanup{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := owner.openSessionEngine(ctx, opening, cleanup)
	if !errors.Is(err, openingErr) || calls != 0 {
		t.Fatalf("opening = %v, calls=%d", err, calls)
	}
	joined := errors.Join(err, cleanup.Close())
	if !errors.Is(joined, openingErr) || !errors.Is(joined, closeErr) {
		t.Fatalf("lost primary/cleanup causes: %v", joined)
	}
	if err := cleanup.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cleanup.Close(); err != nil || calls != 2 {
		t.Fatalf("successful release repeated: %v calls=%d", err, calls)
	}
}

type initialOpeningLoadedStub struct {
	factorydefinitions.MutableLoadedFactorySource
}

func (initialOpeningLoadedStub) FactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{Name: "snapshot"}
}
