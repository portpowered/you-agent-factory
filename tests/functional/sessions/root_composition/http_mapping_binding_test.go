package root_composition_test

import (
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/composition"
)

type mappingStatusProjector struct {
	factoryruntime.FactoryStatusProjector
}
type mappingContentPreparation struct{ work.ContentPreparation }

// TestHTTPMappingCompositionRetainsThinBindingGuards exercises the public
// composition constructors and their nil-role guard without opening runtime
// effects. The application edge owns binding; this package only adapts roles.
func TestHTTPMappingCompositionRetainsThinBindingGuards(t *testing.T) {
	t.Parallel()

	if binder, err := composition.NewHTTPBinder(nil, nil); binder != nil || err == nil {
		t.Fatalf("NewHTTPBinder(nil, nil) = (%v, %v), want nil binder and error", binder, err)
	}
	binder, err := composition.NewHTTPBinder(&mappingStatusProjector{}, &mappingContentPreparation{})
	if err != nil {
		t.Fatalf("NewHTTPBinder(valid roles): %v", err)
	}
	if _, err := binder.Bind(nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("Bind(nil roles) returned nil error")
	}

	_ = composition.NewRuntimeAPI(nil, nil)
	_ = composition.NewLiveSessionAPI(nil)
	_ = composition.NewWorkAPI(nil, nil)
	_ = composition.NewFactoryDefinitionAPI(nil)
	_ = composition.NewInvocationAPI(nil)
	_ = composition.NewDurableAPI(nil, nil)
}
