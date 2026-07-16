package classifier

import (
	"encoding/json"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestValidateCustomization_AllowsReroutingAndModelSelection(t *testing.T) {
	data := customizedClassifierJSON(t, func(document map[string]any) {
		workstations := document["workstations"].([]any)
		classifier := workstations[0].(map[string]any)
		routes := classifier["classificationRoutes"].([]any)
		routes[0].(map[string]any)["outputs"] = []any{map[string]any{"workType": "task", "state": "medium"}}

		workers := document["workers"].([]any)
		workers[2].(map[string]any)["model"] = "gpt-5.4"
	})
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(data)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	if err := packages.ValidateCustomization(cfg); err != nil {
		t.Fatalf("ValidateCustomization: %v", err)
	}
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, data)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	route := workstation(t, loaded.FactoryConfig(), ClassifierWorkstation).ClassificationRoutes[0]
	if len(route.Outputs) != 1 || route.Outputs[0].StateName != "medium" {
		t.Fatalf("custom small route = %#v, want task/medium", route)
	}
	if got := workerByName(t, loaded.FactoryConfig(), "run-medium").Model; got != "gpt-5.4" {
		t.Fatalf("custom run-medium model = %q, want gpt-5.4", got)
	}
}

func TestValidateCustomization_RejectsInvalidRoutesAndSelections(t *testing.T) {
	tests := []struct {
		name string
		edit func(map[string]any)
		want []string
	}{
		{
			name: "unknown label",
			edit: func(document map[string]any) {
				document["workstations"].([]any)[0].(map[string]any)["classificationRoutes"].([]any)[0].(map[string]any)["label"] = "tiny"
			},
			want: []string{"classificationRoutes[0].label", "tiny", "small, medium, or large"},
		},
		{
			name: "duplicate label",
			edit: func(document map[string]any) {
				routes := document["workstations"].([]any)[0].(map[string]any)["classificationRoutes"].([]any)
				routes[1].(map[string]any)["label"] = "small"
			},
			want: []string{"small", "duplicated"},
		},
		{
			name: "absent target",
			edit: func(document map[string]any) {
				routes := document["workstations"].([]any)[0].(map[string]any)["classificationRoutes"].([]any)
				routes[0].(map[string]any)["outputs"] = []any{map[string]any{"workType": "task", "state": "missing"}}
			},
			want: []string{"small", "missing", "exactly one target workstation"},
		},
		{
			name: "missing model selection",
			edit: func(document map[string]any) {
				document["workers"].([]any)[1].(map[string]any)["model"] = ""
				delete(document["workers"].([]any)[1].(map[string]any), "preset")
			},
			want: []string{"small", "run-small", "model selection"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := customClassifierConfig(t, test.edit)
			err := packages.ValidateCustomization(cfg)
			if err == nil {
				t.Fatal("ValidateCustomization succeeded")
			}
			for _, fragment := range test.want {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("error = %q, want %q", err, fragment)
				}
			}
		})
	}
}

func TestPersistNamedFactory_RejectsInvalidClassifierCustomizationBeforeSessionBuild(t *testing.T) {
	data := customizedClassifierJSON(t, func(document map[string]any) {
		document["workstations"].([]any)[0].(map[string]any)["classificationRoutes"].([]any)[0].(map[string]any)["label"] = "tiny"
	})
	_, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, data)
	if err == nil {
		t.Fatal("PersistNamedFactory succeeded")
	}
	for _, fragment := range []string{"validate packaged factory customization", "tiny", "small, medium, or large"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error = %q, want %q", err, fragment)
		}
	}
}

func customClassifierConfig(t *testing.T, edit func(map[string]any)) *interfaces.FactoryConfig {
	t.Helper()
	data := customizedClassifierJSON(t, edit)
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(data)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	return cfg
}

func customizedClassifierJSON(t *testing.T, edit func(map[string]any)) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(BuiltInFactoryJSON, &document); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	edit(document)
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return data
}
