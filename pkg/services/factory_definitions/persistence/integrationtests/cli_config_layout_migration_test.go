package splitreplacetests

import (
	"io"
	"path/filepath"
	"testing"
)

func TestExpandFactoryConfig_PreservesPortableLayoutThroughFlattenRoundTrip(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	writePortableLayoutExpandFixture(t, factoryPath)

	if err := ExpandFactoryConfig(FactoryConfigExpandConfig{Path: factoryPath, Output: io.Discard}); err != nil {
		t.Fatalf("ExpandFactoryConfig: %v", err)
	}

	loaded, err := factorydefinitioncomposition.LoadedFactoryLoader(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(expanded layout): %v", err)
	}
	if loaded.FactoryConfig().Layout == nil || len(loaded.FactoryConfig().Layout.Nodes) != 1 {
		t.Fatalf("expanded runtime layout = %#v, want one node", loaded.FactoryConfig().Layout)
	}
	if loaded.FactoryConfig().Layout.Nodes[0].ID != "workstation:execute-story" {
		t.Fatalf("expanded layout node = %q, want workstation:execute-story", loaded.FactoryConfig().Layout.Nodes[0].ID)
	}

	flattened, err := factorydefinitioncomposition.FlattenFactoryConfig(dir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig(expanded layout): %v", err)
	}
	payload := decodeConfigPayload(t, flattened, "flattened portable layout")
	layout, ok := payload["layout"].(map[string]any)
	if !ok {
		t.Fatalf("flattened layout = %#v, want object", payload["layout"])
	}
	if layout["schemaVersion"] != float64(1) {
		t.Fatalf("flattened layout schemaVersion = %#v, want 1", layout["schemaVersion"])
	}
	nodes, ok := layout["nodes"].([]any)
	if !ok || len(nodes) != 1 {
		t.Fatalf("flattened layout nodes = %#v, want one node", layout["nodes"])
	}
	node, ok := nodes[0].(map[string]any)
	if !ok || node["id"] != "workstation:execute-story" {
		t.Fatalf("flattened layout node = %#v, want workstation:execute-story", nodes[0])
	}
}

func writePortableLayoutExpandFixture(t *testing.T, factoryPath string) {
	t.Helper()

	writeCLITestFile(t, factoryPath, `{
		"name":"expand-config-portable-layout",
		"layout":{
			"schemaVersion":1,
			"nodes":[{"id":"workstation:execute-story","position":{"x":128,"y":256},"size":{"width":320,"height":180},"locked":true}],
			"edges":[{"id":"output:workstation:execute-story->work-type:story","waypoints":[{"x":180,"y":220}],"labelPosition":{"x":200,"y":210}}],
			"groups":[{"id":"group-1","label":"Main lane","nodeIds":["workstation:execute-story"],"bounds":{"x":100,"y":200,"width":400,"height":240}}],
			"viewport":{"x":40,"y":60,"zoom":0.9},
			"preferences":{"direction":"RIGHT"}
		},
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers":[{"name":"executor","type":"MODEL_WORKER","model":"claude-sonnet-4-20250514","body":"You are the expanded executor."}],
		"workstations":[{"name":"execute-story","worker":"executor","inputs":[{"workType":"story","state":"init"}],"outputs":[{"workType":"story","state":"complete"}],"type":"MODEL_WORKSTATION","body":"Complete {{ .WorkID }} deterministically."}]
	}`)
}
