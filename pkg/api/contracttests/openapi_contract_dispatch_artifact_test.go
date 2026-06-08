package apicontract_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestOpenAPIContract_FactoryDispatchAndArtifactSchemasExposeSharedProjectionFields(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	runtimeSchema := schemaObject(t, schemas, "FactorySessionRuntime")
	runtimeProperties, _ := runtimeSchema["properties"].(map[string]any)
	assertArrayItemRef(t, runtimeProperties, "dispatches", "#/components/schemas/FactoryDispatch")
	assertArrayItemRef(t, runtimeProperties, "artifacts", "#/components/schemas/FactoryArtifact")
	assertSchemaPropertyRef(t, schemas, "FactoryDispatch", "orchestratorKind", "#/components/schemas/FactoryOrchestratorKind")
	assertSchemaPropertyRef(t, schemas, "FactoryDispatch", "dispatchKind", "#/components/schemas/FactoryDispatchKind")
	assertSchemaPropertyRef(t, schemas, "FactoryDispatch", "status", "#/components/schemas/FactoryDispatchStatus")
	assertSchemaPropertyRef(t, schemas, "FactoryDispatch", "petri", "#/components/schemas/FactoryDispatchPetriProjection")
	assertSchemaPropertyRef(t, schemas, "FactoryDispatch", "javascript", "#/components/schemas/FactoryDispatchJavaScriptProjection")
	assertSchemaPropertyRef(t, schemas, "FactoryArtifact", "kind", "#/components/schemas/FactoryArtifactKind")
	assertSchemaPropertyRef(t, schemas, "FactoryArtifact", "visibility", "#/components/schemas/FactoryArtifactVisibility")
	assertSchemaPropertyRef(t, schemas, "FactoryArtifact", "auditMode", "#/components/schemas/FactoryArtifactAuditMode")
	assertEnumValues(t, schemaObject(t, schemas, "FactoryDispatchKind"), "FactoryDispatchKind", []string{
		"PETRI_TRANSITION",
		"JAVASCRIPT_AGENT",
		"JAVASCRIPT_VERIFY",
		"JAVASCRIPT_SYNTHESIZE",
		"JAVASCRIPT_TOOL",
		"JAVASCRIPT_SCRIPT",
		"JAVASCRIPT_SYSTEM",
	})
	assertEnumValues(t, schemaObject(t, schemas, "FactoryDispatchStatus"), "FactoryDispatchStatus", []string{
		"QUEUED", "RUNNING", "COMPLETED", "FAILED",
	})
	assertEnumValues(t, schemaObject(t, schemas, "FactoryArtifactKind"), "FactoryArtifactKind", []string{
		"FINAL_RESULT",
		"CHILD_RESULT",
		"FINDING",
		"PATCH",
		"LOG",
		"DATASET",
		"CHECKPOINT",
		"WORKTREE_SUMMARY",
	})
}

func TestGeneratedDispatchArtifactContracts_RuntimeTypesAgreeWithOpenAPI(t *testing.T) {
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionRuntime{}), "Dispatches", reflect.TypeOf((*[]factoryapi.FactoryDispatch)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionRuntime{}), "Artifacts", reflect.TypeOf((*[]factoryapi.FactoryArtifact)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactoryDispatch{}), "Petri", reflect.TypeOf((*factoryapi.FactoryDispatchPetriProjection)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactoryDispatch{}), "Javascript", reflect.TypeOf((*factoryapi.FactoryDispatchJavaScriptProjection)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactoryArtifact{}), "AuditMode", reflect.TypeOf((*factoryapi.FactoryArtifactAuditMode)(nil)))
}

func TestGeneratedDispatchArtifactContracts_PetriAndJavaScriptRoundTrip(t *testing.T) {
	label := "process"
	phase := "review"
	dispatches := []factoryapi.FactoryDispatch{{
		Id:               "dispatch-petri-1",
		SessionId:        "~default",
		OrchestratorKind: factoryapi.PETRI,
		DispatchKind:     factoryapi.FactoryDispatchKindPETRITRANSITION,
		Status:           factoryapi.FactoryDispatchStatusRUNNING,
		Label:            &label,
		Petri: &factoryapi.FactoryDispatchPetriProjection{
			TransitionId: "tr-process",
		},
	}, {
		Id:               "dispatch-agent-1",
		SessionId:        "session-js",
		OrchestratorKind: factoryapi.JAVASCRIPT,
		DispatchKind:     factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
		Status:           factoryapi.FactoryDispatchStatusCOMPLETED,
		Phase:            &phase,
		Javascript: &factoryapi.FactoryDispatchJavaScriptProjection{
			TaskKind: factoryapi.FactoryDispatchJavaScriptTaskKindAGENT,
		},
	}}
	auditMode := factoryapi.FactoryArtifactAuditModeREDACTED
	artifacts := []factoryapi.FactoryArtifact{{
		Id:         "artifact-child-1",
		Kind:       factoryapi.FactoryArtifactKindCHILDRESULT,
		Visibility: factoryapi.FactoryArtifactVisibilityPUBLIC,
		AuditMode:  &auditMode,
	}}
	runtime := factoryapi.FactorySessionRuntime{
		OrchestratorKind: factoryapi.JAVASCRIPT,
		Status:           factoryapi.FactorySessionStatusIDLE,
		Progress: factoryapi.FactorySessionProgress{
			FactoryState:  "UNKNOWN",
			Categories:    factoryapi.StatusCategories{},
			InFlightCount: 0,
			TotalTokens:   0,
		},
		Usage:     factoryapi.FactorySessionUsage{Resources: []factoryapi.ResourceUsage{}},
		Lifecycle: factoryapi.FactorySessionLifecycle{StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()},
		Dispatches: &dispatches,
		Artifacts:  &artifacts,
	}
	encoded, err := json.Marshal(runtime)
	if err != nil {
		t.Fatalf("marshal generated runtime: %v", err)
	}
	var decoded factoryapi.FactorySessionRuntime
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal generated runtime: %v", err)
	}
	if decoded.Dispatches == nil || len(*decoded.Dispatches) != 2 {
		t.Fatalf("decoded dispatches = %#v, want two entries", decoded.Dispatches)
	}
	if decoded.Artifacts == nil || len(*decoded.Artifacts) != 1 {
		t.Fatalf("decoded artifacts = %#v, want one entry", decoded.Artifacts)
	}
}
