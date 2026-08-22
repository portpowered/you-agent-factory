package host_test

import (
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
