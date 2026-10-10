package testcomposition

import (
	"errors"
	"testing"

	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestPersistenceMappingPreservesRequestAndFailure(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"name":"alpha"}`)
	config := &definitions.FactoryConfig{Name: "alpha"}
	cause := errors.New("invalid representation")
	for _, failure := range []error{nil, cause} {
		composition := New(Representation{MapPersistence: func(got []byte) (definitions.DefinitionValidationRequest, error) {
			if &got[0] != &payload[0] {
				t.Fatal("mapper did not receive the submitted payload")
			}
			return definitions.DefinitionValidationRequest{Config: config, CanonicalPayload: got}, failure
		}}, nil, nil, Effects{})
		got, err := composition.MapFactoryJSONForPersistence(payload)
		if got.Config != config || &got.CanonicalPayload[0] != &payload[0] || !errors.Is(err, failure) {
			t.Fatalf("mapping = %#v, %v; want original request and %v", got, err, failure)
		}
	}
}

func TestPersistenceMappingRequiresRepresentationMapper(t *testing.T) {
	t.Parallel()
	got, err := (Composition{}).MapFactoryJSONForPersistence([]byte(`{}`))
	if err == nil || err.Error() != "Factory Definitions persistence representation mapper is required" || got.Config != nil || got.CanonicalPayload != nil {
		t.Fatalf("missing mapper = %#v, %v", got, err)
	}
}
