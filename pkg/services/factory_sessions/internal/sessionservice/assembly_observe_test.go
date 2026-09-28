package service

import (
	"context"
	"errors"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
)

type observeStubRuntime struct {
	factoryruntime.Service
	result factoryruntime.ObserveResult
	err    error
}

func (r *observeStubRuntime) Observe(context.Context, factoryruntime.ObserveRequest) (factoryruntime.ObserveResult, error) {
	return r.result, r.err
}

func TestAssemblyObserveForSessionUsesCanonicalRegistry(t *testing.T) {
	t.Parallel()
	state := newWorkResolverSessionState()
	want := factoryruntime.ObserveResult{}
	state.Register(sessionruntime.Registration{
		SessionID: "sess-observe",
		Handle:    struct{}{},
		Select:    true,
		Runtime: &factorysessions.LiveRuntime{
			Factory: &observeStubRuntime{result: want},
		},
	})
	assembly := &Assembly{state: state}

	got, err := assembly.ObserveForSession(context.Background(), "sess-observe", factoryruntime.ObserveRequest{})
	if err != nil {
		t.Fatalf("ObserveForSession: %v", err)
	}
	_ = got

	if _, err := assembly.ObserveForSession(context.Background(), "missing", factoryruntime.ObserveRequest{}); !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("missing error = %v, want ErrSessionNotFound", err)
	}
}
