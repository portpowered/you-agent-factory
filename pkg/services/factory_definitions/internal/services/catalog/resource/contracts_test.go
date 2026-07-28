package resource_test

import (
	"encoding/json"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	catalogresource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/resource"
)

func TestResourceConfigRoundTripsThroughDefinitionsRootAlias(t *testing.T) {
	t.Parallel()

	cfg := catalogresource.Config{
		ID:       "resource-gpu",
		Name:     "gpu",
		Type:     catalogresource.TypeModel,
		Capacity: 4,
		Model:    "local-model",
	}

	var rootCfg factorydefinitions.ResourceConfig = cfg
	if rootCfg.Name != "gpu" || rootCfg.Capacity != 4 {
		t.Fatalf("root alias mismatch: %#v", rootCfg)
	}

	payload, err := json.Marshal(rootCfg)
	if err != nil {
		t.Fatalf("marshal root alias: %v", err)
	}

	var decoded factorydefinitions.ResourceConfig
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal root alias: %v", err)
	}
	if decoded.Type != catalogresource.TypeModel || decoded.Model != "local-model" {
		t.Fatalf("decoded resource config mismatch: %#v", decoded)
	}
}

func TestResourceTypeConstantsRemainStable(t *testing.T) {
	t.Parallel()

	if factorydefinitions.ResourceTypeModel != catalogresource.TypeModel {
		t.Fatalf("model type constant drift: %q != %q", factorydefinitions.ResourceTypeModel, catalogresource.TypeModel)
	}
	if factorydefinitions.ResourceTypeProviderQuota != catalogresource.TypeProviderQuota {
		t.Fatalf("provider quota type constant drift")
	}
	if factorydefinitions.ResourceTypeInvocationSlot != catalogresource.TypeInvocationSlot {
		t.Fatalf("invocation slot type constant drift")
	}
}
