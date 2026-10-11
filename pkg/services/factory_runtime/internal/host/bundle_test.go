package host_test

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"testing"

	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
)

type modelScopeBindingFactory struct {
	factoryhost.Engine
	scope modelprovider.RuntimeScopeRef
	calls int
}

func (f *modelScopeBindingFactory) BindModelsRuntimeScope(scope modelprovider.RuntimeScopeRef) error {
	f.scope = scope
	f.calls++
	return nil
}

func TestBundleBindModelsRuntimeScopeForwardsToConcreteFactory(t *testing.T) {
	t.Parallel()

	scope, err := (modelprovider.RuntimeScopeRef{}).Parse("factory-session:models")
	if err != nil {
		t.Fatalf("parse Models scope: %v", err)
	}
	factory := &modelScopeBindingFactory{}
	bundle := &factoryhost.Bundle{Factory: factory}

	if err := bundle.BindModelsRuntimeScope(scope); err != nil {
		t.Fatalf("BindModelsRuntimeScope: %v", err)
	}
	if factory.calls != 1 || factory.scope != scope {
		t.Fatalf("forwarded scope = %#v, calls = %d; want %#v and one call", factory.scope, factory.calls, scope)
	}
}

func TestRuntimeSessionLoggerPreservesSelectedDiagnostics(t *testing.T) {
	t.Parallel()
	for _, session := range []string{"first", "peer"} {
		t.Run(session, func(t *testing.T) {
			t.Parallel()
			core, observed := observer.New(zap.DebugLevel)
			logger := zap.New(core).With(zap.String("session_id", session))
			bundle := &factoryhost.Bundle{Logger: logger}
			bundle.RuntimeLogger().Warn("selected runtime failure", zap.String("cause", "flush"))
			entries := observed.All()
			if len(entries) != 1 || entries[0].Message != "selected runtime failure" || entries[0].ContextMap()["session_id"] != session || entries[0].ContextMap()["cause"] != "flush" {
				t.Fatalf("diagnostics = %#v", entries)
			}
		})
	}
}
