package builtinreview_test

import (
	"encoding/json"
	"os"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	builtinreview "github.com/portpowered/infinite-you/pkg/factory/packages/definitions/review"
)

func TestBuiltInReviewFactoryJSON_AssemblesDeclaredPromptAssets(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(builtinreview.BuiltInReviewFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	for _, tc := range []struct{ name, path string }{{"review-work-executor", "prompts/executor.md"}, {"review-work-reviewer", "prompts/reviewer.md"}} {
		want, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", tc.path, err)
		}
		for _, worker := range cfg.Workers {
			if worker.Name == tc.name && worker.Body == string(want) {
				goto found
			}
		}
		t.Fatalf("worker %q did not receive prompt asset", tc.name)
	found:
	}
}

func TestBuiltInReviewFactoryJSON_DeclaresPromptFilesWithoutInlineBodies(t *testing.T) {
	var authored map[string]any
	if err := json.Unmarshal(builtinreview.FactoryJSON(), &authored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, collection := range []string{"workers", "workstations"} {
		for _, raw := range authored[collection].([]any) {
			entry := raw.(map[string]any)
			if _, ok := entry["body"]; ok {
				t.Fatalf("%s %q has inline body", collection, entry["name"])
			}
			if entry["promptFile"] == nil {
				t.Fatalf("%s %q missing promptFile", collection, entry["name"])
			}
		}
	}
}
