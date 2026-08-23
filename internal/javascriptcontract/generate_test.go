package javascriptcontract

import (
	"encoding/json"
	"reflect"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const minimalCatalog = `{"sharedSchemas":{"javascript.schema.agent_run_spec":{"schema":{"type":"object","additionalProperties":false,"properties":{"prompt":{"type":"string"}},"required":["prompt"]}}}}`

func TestGenerateRuntimeCatalogProjectsEveryRuntimeField(t *testing.T) {
	payload, err := GenerateRuntimeCatalog([]byte(minimalCatalog))
	if err != nil {
		t.Fatalf("GenerateRuntimeCatalog() error = %v", err)
	}
	fields := projectedFields(t, payload)
	want := factoryruntime.JavaScriptChildFieldDescriptors()
	if len(fields) != len(want) {
		t.Fatalf("projected field count = %d, want %d", len(fields), len(want))
	}
	for _, descriptor := range want {
		projected, ok := fields[descriptor.Name].(map[string]any)
		if !ok {
			t.Fatalf("projected field %q = %#v, want object", descriptor.Name, fields[descriptor.Name])
		}
		if projected["type"] != descriptor.JSONType {
			t.Errorf("projected field %q type = %#v, want %q", descriptor.Name, projected["type"], descriptor.JSONType)
		}
	}
	required := projectedRequired(t, payload)
	if !reflect.DeepEqual(required, []any{"prompt"}) {
		t.Fatalf("required = %#v, want [prompt]", required)
	}
}

func TestProjectRuntimeCatalogSupportsForwardFieldEvolution(t *testing.T) {
	fields := factoryruntime.JavaScriptChildFieldDescriptors()
	fields = append(fields, factoryruntime.JavaScriptChildFieldDescriptor{
		Name: "futureField", JSONType: "string",
	})
	payload, err := ProjectRuntimeCatalog([]byte(minimalCatalog), fields)
	if err != nil {
		t.Fatalf("ProjectRuntimeCatalog() error = %v", err)
	}
	projected := projectedFields(t, payload)
	if _, ok := projected["futureField"]; !ok {
		t.Fatalf("projected fields = %#v, want futureField", projected)
	}
}

func TestProjectRuntimeCatalogIsDeterministic(t *testing.T) {
	first, err := GenerateRuntimeCatalog([]byte(minimalCatalog))
	if err != nil {
		t.Fatalf("first projection error = %v", err)
	}
	second, err := GenerateRuntimeCatalog([]byte(minimalCatalog))
	if err != nil {
		t.Fatalf("second projection error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("repeated projection changed bytes")
	}
}

func TestProjectRuntimeCatalogRejectsMissingSchema(t *testing.T) {
	_, err := GenerateRuntimeCatalog([]byte(`{"sharedSchemas":{}}`))
	if err == nil || err.Error() != "contracts/javascript/runtime-api.json is missing object /sharedSchemas/javascript.schema.agent_run_spec" {
		t.Fatalf("error = %v, want actionable missing schema error", err)
	}
}

func projectedFields(t *testing.T, payload []byte) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode projected catalog: %v", err)
	}
	shared := document["sharedSchemas"].(map[string]any)
	agentRun := shared["javascript.schema.agent_run_spec"].(map[string]any)
	schema := agentRun["schema"].(map[string]any)
	return schema["properties"].(map[string]any)
}

func projectedRequired(t *testing.T, payload []byte) []any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode projected catalog: %v", err)
	}
	shared := document["sharedSchemas"].(map[string]any)
	agentRun := shared["javascript.schema.agent_run_spec"].(map[string]any)
	schema := agentRun["schema"].(map[string]any)
	return schema["required"].([]any)
}
