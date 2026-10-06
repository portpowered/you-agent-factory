package contractvalidator_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestMCPToolCatalogInputSchemaDiagnostics_AuthoredCatalogPasses(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	resolved, diagnostics := contractvalidator.LoadAndResolve(root, "contracts/mcp/tools.json", []string{"contracts/mcp/tools.json"})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve authored catalog diagnostics = %+v", diagnostics)
	}
	got := contractvalidator.MCPToolCatalogInputSchemaDiagnostics("contracts/mcp/tools.json", resolved)
	if len(got) != 0 {
		t.Fatalf("MCPToolCatalogInputSchemaDiagnostics() = %+v, want none", got)
	}
}

// Contract proof: composed schemas retain the legacy node constraints and
// reject open alternatives before a catalog can be published.
func TestMCPComposedCatalogSchemaConformance(t *testing.T) {
	t.Parallel()
	meta := readMCPContractObject(t, "../../contracts/mcp/tool-catalog.schema.json")
	defs := meta["$defs"].(map[string]any)
	resource := map[string]any{"$schema": meta["$schema"], "$ref": "#/$defs/closedDraftObjectAlternatives"}
	selected := map[string]any{}
	for _, name := range []string{"closedSchemaNode", "closedDraftSchema", "closedComposedDraftSchema", "closedDraftObjectAlternatives"} {
		selected[name] = defs[name]
	}
	resource["$defs"] = selected
	policy := compileMCPContractSchema(t, resource)
	catalog := readMCPContractObject(t, "../../contracts/mcp/tools.json")
	subagent := catalog["tools"].(map[string]any)["mcp.tool.you.subagent"].(map[string]any)
	input := subagent["input"].(map[string]any)["schema"].(map[string]any)
	result := subagent["result"].(map[string]any)["domain"].(map[string]any)["success"].(map[string]any)["schema"].(map[string]any)
	for name, schema := range map[string]map[string]any{"input": input, "result": result} {
		t.Run(name, func(t *testing.T) {
			if err := policy.Validate(schema); err != nil {
				t.Fatalf("closed composition rejected: %v", err)
			}
			for _, mutation := range []string{"open branch", "missing closure", "empty alternatives", "missing draft", "wrong type", "open root", "implicit nested openness"} {
				t.Run(mutation, func(t *testing.T) {
					data, _ := json.Marshal(schema)
					var changed map[string]any
					if err := json.Unmarshal(data, &changed); err != nil {
						t.Fatal(err)
					}
					branch := changed["oneOf"].([]any)[0].(map[string]any)
					switch mutation {
					case "open branch":
						branch["additionalProperties"] = true
					case "missing closure":
						delete(branch, "additionalProperties")
					case "empty alternatives":
						changed["oneOf"] = []any{}
					case "missing draft":
						delete(changed, "$schema")
					case "wrong type":
						changed["type"] = "array"
					case "open root":
						changed["additionalProperties"] = true
					case "implicit nested openness":
						branch["properties"].(map[string]any)["nested"] = map[string]any{"type": "object"}
					}
					if err := policy.Validate(changed); err == nil {
						t.Fatal("unsafe composition accepted")
					}
				})
			}
		})
	}
}

func TestMCPSubagentActionSchemaConformance(t *testing.T) {
	t.Parallel()
	catalog := readMCPContractObject(t, "../../contracts/mcp/tools.json")
	subagent := catalog["tools"].(map[string]any)["mcp.tool.you.subagent"].(map[string]any)
	schema := compileMCPContractSchema(t, subagent["input"].(map[string]any)["schema"].(map[string]any))
	for _, raw := range []string{`{"prompt":"controlled"}`, `{"action":"RUN","prompt":"controlled"}`, `{"action":"LIST"}`, `{"action":"READ","workerSessionId":"w"}`, `{"action":"CONTROL","workerSessionId":"w","operation":"CANCEL"}`} {
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(value); err != nil {
			t.Fatalf("valid action %s: %v", raw, err)
		}
	}
	for _, raw := range []string{`{"action":"UNKNOWN"}`, `{"action":"LIST","prompt":"cross-action"}`, `{"action":"READ","workerSessionId":"w","operation":"CANCEL"}`, `{"action":"RUN","prompt":"p","unknown":true}`, `{"action":"CONTROL","workerSessionId":"w","operation":"CANCEL","view":"logs"}`} {
		var value any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(value); err == nil {
			t.Fatalf("invalid action accepted: %s", raw)
		}
	}
}

func readMCPContractObject(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestMCPSubagentResultSchemaConformance(t *testing.T) {
	t.Parallel()
	catalog := readMCPContractObject(t, "../../contracts/mcp/tools.json")
	subagent := catalog["tools"].(map[string]any)["mcp.tool.you.subagent"].(map[string]any)
	result := subagent["result"].(map[string]any)["domain"].(map[string]any)["success"].(map[string]any)
	schema := compileMCPContractSchema(t, result["schema"].(map[string]any))
	for _, raw := range []string{
		`{"sessionId":"factory","status":"COMPLETED","text":"controlled"}`,
		`{"workerSessions":[],"nextToken":null}`,
		`{"session":{"workerSessionId":"w"},"logs":{"events":[]}}`,
		`{"workerSessionId":"w","action":"TERMINATE","outcome":"APPLIED","state":"TERMINATED","forced":true,"dispatchId":"d"}`,
		`{"accepted":true,"phase":"SUCCESSOR_ADMISSION","requestId":"r","sourceWorkerSessionId":"w","successorWorkerSessionId":"s","source":{},"successor":{}}`,
	} {
		var value map[string]any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(value); err != nil {
			t.Fatalf("valid result %s: %v", raw, err)
		}
		value["unknown"] = true
		if err := schema.Validate(value); err == nil {
			t.Fatalf("open result accepted: %s", raw)
		}
	}
}

func compileMCPContractSchema(t *testing.T, value map[string]any) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	const id = "https://example.test/mcp-contract.schema.json"
	if err := compiler.AddResource(id, value); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(id)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestMCPToolCatalogInputSchemaDiagnostics_SkipsNonAuthoredCatalog(t *testing.T) {
	got := contractvalidator.MCPToolCatalogInputSchemaDiagnostics(
		"contracts/testdata/mcp/valid-minimal.json",
		map[string]any{"tools": map[string]any{}},
	)
	if len(got) != 0 {
		t.Fatalf("MCPToolCatalogInputSchemaDiagnostics() = %+v, want skip", got)
	}
}

func TestWorkerSessionInputSchemaDriftIsRejected(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	resolved, diagnostics := contractvalidator.LoadAndResolve(root, "contracts/mcp/tools.json", []string{"contracts/mcp/tools.json"})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve catalog: %v", diagnostics)
	}
	tools := resolved.(map[string]any)["tools"].(map[string]any)
	read := tools["mcp.tool.you.subagent"].(map[string]any)
	schema := read["input"].(map[string]any)["schema"].(map[string]any)
	schema["oneOf"].([]any)[2].(map[string]any)["additionalProperties"] = true
	got := contractvalidator.MCPToolCatalogInputSchemaDiagnostics("contracts/mcp/tools.json", resolved)
	if len(got) != 1 || got[0].Code != "catalog.input_schema.parity" {
		t.Fatalf("Worker Session schema drift was accepted: %v", got)
	}
}
