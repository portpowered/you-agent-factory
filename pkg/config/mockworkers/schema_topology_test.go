package mockworkers_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config/mockworkers"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestMockWorkersSchema_DeclaresDraft202012AndStableID(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	if got, _ := schema["$schema"].(string); got != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("$schema = %q, want Draft 2020-12", got)
	}
	if got, _ := schema["$id"].(string); got != mockworkers.SchemaID {
		t.Fatalf("$id = %q, want %q", got, mockworkers.SchemaID)
	}
}

func TestMockWorkersSchema_ClosesAuthoredObjects(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	assertObjectClosed(t, schema, "/")

	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatal("$defs missing or not an object")
	}
	for name, def := range defs {
		defObject, ok := def.(map[string]any)
		if !ok {
			t.Fatalf("$defs[%q] is not an object", name)
		}
		assertObjectClosed(t, defObject, "/$defs/"+name)
	}
}

func TestMockWorkersSchema_ExpressesB03TopologyFields(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	rootProps := objectProperties(t, schema, "/")
	assertPropertyNames(t, rootProps, []string{"mockWorkers", "unmatchedDispatchPolicy"}, "/")

	mockWorker := defSchema(t, schema, "mockWorker")
	mockWorkerProps := objectProperties(t, mockWorker, "/$defs/mockWorker")
	assertPropertyNames(t, mockWorkerProps, []string{
		"id",
		"workerName",
		"workstationName",
		"workInputs",
		"runType",
		"scriptConfig",
		"rejectConfig",
	}, "/$defs/mockWorker")

	workInput := defSchema(t, schema, "workInput")
	assertPropertyNames(t, objectProperties(t, workInput, "/$defs/workInput"), []string{
		"workId",
		"workType",
		"state",
		"inputName",
		"traceId",
		"channel",
		"payloadHash",
	}, "/$defs/workInput")

	scriptConfig := defSchema(t, schema, "scriptConfig")
	assertPropertyNames(t, objectProperties(t, scriptConfig, "/$defs/scriptConfig"), []string{
		"command",
		"args",
		"env",
		"workingDirectory",
		"stdin",
		"timeout",
	}, "/$defs/scriptConfig")

	rejectConfig := defSchema(t, schema, "rejectConfig")
	assertPropertyNames(t, objectProperties(t, rejectConfig, "/$defs/rejectConfig"), []string{
		"stdout",
		"stderr",
		"exitCode",
	}, "/$defs/rejectConfig")

	inventory := mockworkers.ProjectTopologyInventory()
	for _, field := range inventory.Fields {
		if field.JSONName == "" {
			t.Fatalf("topology field %q missing jsonName", field.ID)
		}
	}
}

func TestMockWorkersSchema_ExpressesRunTypeAndPolicyEnums(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	mockWorker := defSchema(t, schema, "mockWorker")
	runType := objectProperties(t, mockWorker, "/$defs/mockWorker")["runType"].(map[string]any)
	assertEnumValues(t, runType, []string{"accept", "script", "reject"}, "/$defs/mockWorker.runType")

	rootProps := objectProperties(t, schema, "/")
	policy := rootProps["unmatchedDispatchPolicy"].(map[string]any)
	assertEnumValues(t, policy, []string{"accept", "passthrough"}, "/unmatchedDispatchPolicy")

	required, _ := schema["required"].([]any)
	if !containsString(required, "mockWorkers") {
		t.Fatalf("required = %#v, want mockWorkers", required)
	}
}

func TestMockWorkersSchema_EnforcesScriptConditionalUnion(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	mockWorker := defSchema(t, schema, "mockWorker")
	allOf, ok := mockWorker["allOf"].([]any)
	if !ok || len(allOf) == 0 {
		t.Fatal("mockWorker.allOf missing script conditional union")
	}

	scriptConfig := defSchema(t, schema, "scriptConfig")
	required, _ := scriptConfig["required"].([]any)
	if !containsString(required, "command") {
		t.Fatalf("scriptConfig.required = %#v, want command", required)
	}
	command := objectProperties(t, scriptConfig, "/$defs/scriptConfig")["command"].(map[string]any)
	if minLength, _ := command["minLength"].(float64); minLength < 1 {
		t.Fatalf("scriptConfig.command.minLength = %v, want >= 1", command["minLength"])
	}
}

func TestMockWorkersSchema_EnforcesRejectExitCodeBounds(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	rejectConfig := defSchema(t, schema, "rejectConfig")
	exitCode := objectProperties(t, rejectConfig, "/$defs/rejectConfig")["exitCode"].(map[string]any)
	if got, _ := exitCode["minimum"].(float64); got != 1 {
		t.Fatalf("rejectConfig.exitCode.minimum = %v, want 1", exitCode["minimum"])
	}
	if got, _ := exitCode["maximum"].(float64); got != 255 {
		t.Fatalf("rejectConfig.exitCode.maximum = %v, want 255", exitCode["maximum"])
	}
}

func TestMockWorkersSchema_DoesNotAdvertiseUnsupportedCapabilities(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	lower := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		"\"media\"",
		"\"artifact\"",
		"\"responsesequence\"",
		"\"response_sequence\"",
		"\"dispatchdelay\"",
		"\"dispatch_delay\"",
		"\"sleep\"",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("schema advertises unsupported capability property %s", forbidden)
		}
	}

	description, _ := schema["description"].(string)
	for _, phrase := range []string{"response sequence", "artifact payload", "dispatch delay"} {
		if strings.Contains(strings.ToLower(description), phrase) {
			t.Fatalf("root description advertises unsupported capability %q", phrase)
		}
	}
}

func TestMockWorkersSchema_CompilesAsDraft202012(t *testing.T) {
	t.Parallel()

	compileAuthoredMockWorkersSchema(t)
}

func loadAuthoredMockWorkersSchema(t *testing.T) map[string]any {
	t.Helper()

	path := testutil.MustRepoPath(t, mockworkers.SchemaRelativePath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema %s: %v", path, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	return schema
}

func compileAuthoredMockWorkersSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()

	path := testutil.MustRepoPath(t, mockworkers.SchemaRelativePath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema %s: %v", path, err)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(mockworkers.SchemaID, document); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}
	compiled, err := compiler.Compile(mockworkers.SchemaID)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return compiled
}

func defSchema(t *testing.T, schema map[string]any, name string) map[string]any {
	t.Helper()

	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatal("$defs missing or not an object")
	}
	def, ok := defs[name].(map[string]any)
	if !ok {
		t.Fatalf("$defs[%q] missing or not an object", name)
	}
	return def
}

func objectProperties(t *testing.T, object map[string]any, path string) map[string]any {
	t.Helper()

	props, ok := object["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s.properties missing or not an object", path)
	}
	return props
}

func assertObjectClosed(t *testing.T, object map[string]any, path string) {
	t.Helper()

	if object["type"] != "object" {
		return
	}
	if object["additionalProperties"] != false {
		t.Fatalf("%s.additionalProperties = %#v, want false", path, object["additionalProperties"])
	}
}

func assertPropertyNames(t *testing.T, props map[string]any, want []string, path string) {
	t.Helper()

	got := make([]string, 0, len(props))
	for name := range props {
		got = append(got, name)
	}
	if len(got) != len(want) {
		t.Fatalf("%s property count = %d (%#v), want %d (%#v)", path, len(got), got, len(want), want)
	}
	for _, name := range want {
		if _, ok := props[name]; !ok {
			t.Fatalf("%s missing property %q in %#v", path, name, got)
		}
	}
}

func assertEnumValues(t *testing.T, property map[string]any, want []string, path string) {
	t.Helper()

	raw, ok := property["enum"].([]any)
	if !ok {
		t.Fatalf("%s.enum missing", path)
	}
	got := make([]string, 0, len(raw))
	for _, value := range raw {
		text, ok := value.(string)
		if !ok {
			t.Fatalf("%s.enum contains non-string %#v", path, value)
		}
		got = append(got, text)
	}
	if len(got) != len(want) {
		t.Fatalf("%s.enum = %#v, want %#v", path, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s.enum = %#v, want %#v", path, got, want)
		}
	}
}

func containsString(values []any, want string) bool {
	for _, value := range values {
		if text, ok := value.(string); ok && text == want {
			return true
		}
	}
	return false
}
