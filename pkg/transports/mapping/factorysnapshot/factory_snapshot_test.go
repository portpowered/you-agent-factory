package factorysnapshot

import (
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestToAPIPreservesPublicFactoryShape(t *testing.T) {
	t.Parallel()

	workers := []api.Worker{{Name: "reviewer"}}
	workstations := []api.Workstation{{Name: "review", Worker: "reviewer"}}
	want := api.Factory{
		Name:         "factory-a",
		Workers:      &workers,
		Workstations: &workstations,
	}
	snapshot, err := interfaces.NewFactorySnapshot(want)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}

	got, err := ToAPI(snapshot)
	if err != nil {
		t.Fatalf("ToAPI: %v", err)
	}
	if got.Name != want.Name || got.Workers == nil || len(*got.Workers) != 1 || (*got.Workers)[0].Name != "reviewer" {
		t.Fatalf("ToAPI = %#v, want public factory shape preserved", got)
	}
	if got.Workstations == nil || len(*got.Workstations) != 1 || (*got.Workstations)[0].Name != "review" {
		t.Fatalf("ToAPI workstations = %#v, want review", got.Workstations)
	}
}

func TestToAPIHandlesAbsentAndMalformedSnapshots(t *testing.T) {
	t.Parallel()

	got, err := ToAPI(nil)
	if err != nil || got != nil {
		t.Fatalf("ToAPI(nil) = (%#v, %v), want (nil, nil)", got, err)
	}

	malformed := interfaces.FactorySnapshot(`{"workers":`)
	got, err = ToAPI(&malformed)
	if err == nil || !strings.Contains(err.Error(), "map factory snapshot to public contract") {
		t.Fatalf("ToAPI(malformed) = (%#v, %v), want contextual decode error", got, err)
	}
}

func TestObjectFromFactoryConfigRejectsUnrepresentableExampleArguments(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		Name: "invalid-example",
		Examples: []interfaces.InvocationExampleConfig{{
			Name:        "invalid",
			Description: interfaces.NameValueConfig{Type: interfaces.NameValueTypeLocalizableAsset, Value: "Invalid"},
			Args:        interfaces.InvocationExampleArguments{"count": 3},
		}},
	}
	got, err := ObjectFromFactoryConfig(cfg)
	if err == nil || got != nil || !strings.Contains(err.Error(), "map Factory snapshot boundary: factory.examples[0].args.count must be a string or array of strings") {
		t.Fatalf("ObjectFromFactoryConfig() = (%#v, %v), want field-specific mapping error", got, err)
	}
}
