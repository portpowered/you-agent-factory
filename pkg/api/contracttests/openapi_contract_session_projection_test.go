package apicontract_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestOpenAPIContract_FactorySessionExposesRuntimeProjectionSchema(t *testing.T) {
	schemas := loadBundledOpenAPIComponentSchemas(t)
	assertSchemaPropertyRef(t, schemas, "FactorySession", "runtime", "#/components/schemas/FactorySessionRuntime")
	assertSchemaPropertyRef(t, schemas, "FactorySessionSummary", "runtime", "#/components/schemas/FactorySessionRuntime")
	assertSchemaPropertyRef(t, schemas, "FactorySessionRuntime", "orchestratorKind", "#/components/schemas/FactoryOrchestratorKind")
	assertSchemaPropertyRef(t, schemas, "FactorySessionRuntime", "status", "#/components/schemas/FactorySessionStatus")
	assertSchemaPropertyRef(t, schemas, "FactorySessionRuntime", "petri", "#/components/schemas/FactorySessionPetriProjection")
	assertSchemaPropertyRef(t, schemas, "FactorySessionRuntime", "javascript", "#/components/schemas/FactorySessionJavaScriptProjection")
	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionStatus"), "FactorySessionStatus", []string{"ACTIVE", "IDLE", "FINISHED"})
}

func TestGeneratedFactorySessionContracts_RuntimeTypesAgreeWithOpenAPI(t *testing.T) {
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySession{}), "Runtime", reflect.TypeOf(factoryapi.FactorySessionRuntime{}))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionSummary{}), "Runtime", reflect.TypeOf((*factoryapi.FactorySessionRuntime)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionRuntime{}), "OrchestratorKind", reflect.TypeOf(factoryapi.FactoryOrchestratorKind("")))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionRuntime{}), "Petri", reflect.TypeOf((*factoryapi.FactorySessionPetriProjection)(nil)))
	assertGeneratedFieldType(t, reflect.TypeOf(factoryapi.FactorySessionRuntime{}), "Javascript", reflect.TypeOf((*factoryapi.FactorySessionJavaScriptProjection)(nil)))
}

func TestGeneratedFactorySessionContracts_JavaScriptRuntimeRoundTrip(t *testing.T) {
	phase := "review"
	argsDigest := "sha256:args"
	checkpoints := []factoryapi.FactorySessionJavaScriptCheckpointRef{{Id: "ckpt-1"}}
	session := factoryapi.FactorySession{
		Id: "session-js",
		Target: factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindDefault,
		},
		FactoryDir: "/factories/js",
		FolderPath: "/workspace",
		Project:    "dynamic-workflow",
		Runtime: factoryapi.FactorySessionRuntime{
			OrchestratorKind: factoryapi.JAVASCRIPT,
			Status:           factoryapi.FactorySessionStatusIDLE,
			Progress: factoryapi.FactorySessionProgress{
				FactoryState:  "UNKNOWN",
				Categories:    factoryapi.StatusCategories{},
				InFlightCount: 0,
				TotalTokens:   0,
			},
			Usage: factoryapi.FactorySessionUsage{Resources: []factoryapi.ResourceUsage{}},
			Lifecycle: factoryapi.FactorySessionLifecycle{
				StartedAt: time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC),
			},
			Javascript: &factoryapi.FactorySessionJavaScriptProjection{
				Phase:      &phase,
				Phases:     []string{"plan", "review"},
				ArgsDigest: &argsDigest,
				Checkpoints: &checkpoints,
				ScriptStatus: factoryapi.FactorySessionJavaScriptScriptStatusRUNNING,
				ChildDispatchCounts: factoryapi.FactorySessionJavaScriptChildDispatchCounts{
					Queued: 1, Running: 0, Completed: 2,
				},
			},
		},
	}
	encoded, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("marshal generated JavaScript session: %v", err)
	}
	var decoded factoryapi.FactorySession
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal generated JavaScript session: %v", err)
	}
	if decoded.Runtime.Javascript == nil || decoded.Runtime.OrchestratorKind != factoryapi.JAVASCRIPT {
		t.Fatalf("decoded session runtime = %#v", decoded.Runtime)
	}
}
