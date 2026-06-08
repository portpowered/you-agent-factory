package apicontract_test

import (
	"encoding/json"
	"reflect"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestOpenAPIContract_FactoryExposesOrchestratorSchema(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	assertSchemaPropertyRef(t, schemas, "Factory", "orchestrator", "#/components/schemas/FactoryOrchestrator")
	assertSchemaPropertyRef(t, schemas, "FactoryOrchestrator", "kind", "#/components/schemas/FactoryOrchestratorKind")
	assertSchemaPropertyRef(t, schemas, "FactoryOrchestrator", "petri", "#/components/schemas/FactoryOrchestratorPetriConfig")
	assertSchemaPropertyRef(t, schemas, "FactoryOrchestrator", "javascript", "#/components/schemas/FactoryOrchestratorJavaScriptConfig")
	assertEnumValues(t, schemaObject(t, schemas, "FactoryOrchestratorKind"), "FactoryOrchestratorKind", []string{"PETRI", "JAVASCRIPT"})
}

func TestGeneratedFactoryContracts_OrchestratorTypesAgreeWithOpenAPI(t *testing.T) {
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.Factory{}), "Orchestrator", reflect.TypeOf((*factoryapi.FactoryOrchestrator)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactoryOrchestrator{}), "Kind", reflect.TypeOf(factoryapi.FactoryOrchestratorKind("")))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactoryOrchestrator{}), "Petri", reflect.TypeOf((*factoryapi.FactoryOrchestratorPetriConfig)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactoryOrchestrator{}), "Javascript", reflect.TypeOf((*factoryapi.FactoryOrchestratorJavaScriptConfig)(nil)))
}

func TestGeneratedFactoryContracts_JavaScriptOrchestratorRoundTrip(t *testing.T) {
	argsSchema := map[string]any{"type": "object"}
	defaultPolicy := map[string]any{"maxAgents": 2}
	factory := factoryapi.Factory{
		Name: "dynamic-workflow",
		Orchestrator: &factoryapi.FactoryOrchestrator{
			Kind: factoryapi.JAVASCRIPT,
			Javascript: &factoryapi.FactoryOrchestratorJavaScriptConfig{
				SourceRef:     stringPtr("factory/workflows/review.js"),
				Entrypoint:    stringPtr("main"),
				ArgsSchema:    &argsSchema,
				DefaultPolicy: &defaultPolicy,
			},
		},
	}

	encoded, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("marshal generated JavaScript factory: %v", err)
	}
	var decoded factoryapi.Factory
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal generated JavaScript factory: %v", err)
	}
	if decoded.Orchestrator == nil || decoded.Orchestrator.Kind != factoryapi.JAVASCRIPT {
		t.Fatalf("decoded orchestrator = %#v, want JAVASCRIPT", decoded.Orchestrator)
	}
	if decoded.Orchestrator.Javascript == nil || decoded.Orchestrator.Javascript.SourceRef == nil {
		t.Fatalf("decoded javascript config = %#v", decoded.Orchestrator.Javascript)
	}
}
