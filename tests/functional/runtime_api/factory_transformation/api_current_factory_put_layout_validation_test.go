package factory_transformation

import (
	"errors"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

func TestCurrentFactoryPUT_PrePersistLayoutFailureRetainsStructuredPath(t *testing.T) {
	t.Parallel()

	var snapshot interfaces.FactorySnapshot
	if err := snapshot.UnmarshalJSON([]byte(`{
		"name":"layout-invalid",
		"layout":{"schemaVersion":1,"annotations":[{
			"id":"note-1","kind":"NOTE","position":{"x":0,"y":0},"size":{"width":0,"height":80},
			"note":{"body":"literal","tone":"NEUTRAL"}
		}]}
	}`)); err != nil {
		t.Fatalf("decode submitted Factory snapshot: %v", err)
	}

	err := validationentry.ValidateEditableFactorySnapshot(&snapshot, nil)
	var topologyErr *apisurface.TopologyValidationError
	if !errors.As(err, &topologyErr) || len(topologyErr.Targets) != 1 {
		t.Fatalf("pre-persist error = %#v, want one structured API validation target", err)
	}
	target := topologyErr.Targets[0]
	if target.Code != factoryvalidation.CodeLayoutInvalidGeometry || target.Path == nil || *target.Path != "factory.layout.annotations[0].size.width" {
		t.Fatalf("pre-persist target = %#v, want field-specific invalid geometry", target)
	}
}
