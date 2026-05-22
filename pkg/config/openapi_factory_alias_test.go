package config

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type generatedFactoryRetiredAliasCase struct {
	name        string
	field       string
	replacement string
	payload     string
}

var generatedFactoryRetiredAliasCases = []generatedFactoryRetiredAliasCase{
	{
		name:        "worker snake case provider alias",
		field:       "workers[0].model_provider",
		replacement: "use modelProvider",
		payload: `{
				"name":"worker-snake-case-provider-alias-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","model_provider":"CODEX"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
	},
	{
		name:        "nested worker definition provider alias",
		field:       "workers[0].definition.provider",
		replacement: "use executorProvider",
		payload: `{
				"name":"nested-worker-definition-provider-alias-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{
					"name":"executor",
					"definition":{"type":"MODEL_WORKER","provider":"script_wrap"}
				}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
	},
	{
		name:        "workstation resource usage alias",
		field:       "workstations[0].resource_usage",
		replacement: "use resources",
		payload: `{
				"name":"workstation-resource-usage-alias-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}],
					"resource_usage":[{"name":"agent-slot","capacity":2}]
				}]
			}`,
	},
	{
		name:        "workstation stop token alias",
		field:       "workstations[0].stop_token",
		replacement: "use stopWords",
		payload: `{
				"name":"workstation-stop-token-alias-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"stop_token":"DONE",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
	},
	{
		name:        "cron trigger alias",
		field:       "workstations[0].cron.trigger_at_start",
		replacement: "use triggerAtStart",
		payload: `{
				"name":"cron-trigger-alias-factory",
				"workTypes": [{"name":"story","states":[{"name":"ready","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor"}],
				"workstations": [{
					"name":"scheduled-story",
					"behavior":"CRON",
					"worker":"executor",
					"outputs":[{"workType":"story","state":"complete"}],
					"cron":{"schedule":"*/5 * * * *","trigger_at_start":true}
				}]
			}`,
	},
	{
		name:        "nested workstation definition alias",
		field:       "workstations[0].definition.runtime_type",
		replacement: "use type",
		payload: `{
				"name":"definition-alias-factory",
				"workTypes": [{"name":"story","states":[{"name":"ready","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor"}],
				"workstations": [{
					"name":"scheduled-story",
					"behavior":"STANDARD",
					"worker":"executor",
					"inputs":[{"workType":"story","state":"ready"}],
					"outputs":[{"workType":"story","state":"complete"}],
					"definition":{"runtime_type":"MODEL_WORKSTATION"}
				}]
			}`,
	},
	{
		name:        "nested workstation definition cron alias",
		field:       "workstations[0].definition.cron.trigger_at_start",
		replacement: "use triggerAtStart",
		payload: `{
				"name":"nested-definition-cron-alias-factory",
				"workTypes": [{"name":"story","states":[{"name":"ready","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor"}],
				"workstations": [{
					"name":"scheduled-story",
					"behavior":"CRON",
					"worker":"executor",
					"outputs":[{"workType":"story","state":"complete"}],
					"definition":{
						"cron":{"schedule":"*/5 * * * *","trigger_at_start":true}
					}
				}]
			}`,
	},
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsRetiredRenamedFieldAliasesAtBoundary(t *testing.T) {
	for _, tc := range generatedFactoryRetiredAliasCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedFactoryRejectsRetiredRenamedFieldAlias(t, tc)
		})
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_AllowsDeeperNestedDefinitionAliasesOutsideBoundaryScope(t *testing.T) {
	payload := []byte(`{
		"name":"deep-definition-alias-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{
			"name":"executor",
			"definition":{
				"type":"MODEL_WORKER",
				"definition":{"provider":"script_wrap"}
			}
		}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	_, err := GeneratedFactoryFromOpenAPIJSON(payload)
	if err != nil && strings.Contains(err.Error(), "workers[0].definition.definition.provider") {
		t.Fatalf("expected deeper nested definition alias to stay outside generated boundary rejection scope, got %v", err)
	}
}

func assertGeneratedFactoryRejectsRetiredRenamedFieldAlias(t *testing.T, tc generatedFactoryRetiredAliasCase) {
	t.Helper()

	_, err := GeneratedFactoryFromOpenAPIJSON([]byte(tc.payload))
	if err == nil {
		t.Fatal("expected retired renamed alias to fail at generated boundary")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), tc.field) {
		t.Fatalf("expected retired field path %q, got %v", tc.field, err)
	}
	if !strings.Contains(err.Error(), tc.replacement) {
		t.Fatalf("expected replacement hint %q, got %v", tc.replacement, err)
	}
}

func TestFactoryConfigFromOpenAPIJSON_MapsCopyReferencedScriptsField(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"copy-referenced-scripts-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"copyReferencedScripts": true
		}, {
			"name":"review-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"complete"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if len(cfg.Workstations) != 2 {
		t.Fatalf("expected two workstations, got %d", len(cfg.Workstations))
	}
	if !cfg.Workstations[0].CopyReferencedScripts {
		t.Fatalf("expected execute-story copyReferencedScripts=true, got %#v", cfg.Workstations[0])
	}
	if cfg.Workstations[1].CopyReferencedScripts {
		t.Fatalf("expected omitted copyReferencedScripts to default false, got %#v", cfg.Workstations[1])
	}
}

func TestFactoryConfigFromOpenAPIJSON_PreservesMapKeysAndCurrentInputGuards(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"preserve-map-keys-and-input-guards-factory",
		"metadata":{"factory_hash":"sha256:test"},
		"workTypes": [
			{"name":"chapter","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]},
			{"name":"page","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}
		],
		"resources": [],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"finish-chapter",
			"worker":"executor",
			"inputs":[
				{"workType":"chapter","state":"init"},
				{"workType":"page","state":"complete","guards":[{"type":"ALL_CHILDREN_COMPLETE","parentInput":"chapter","spawnedBy":"chapter-parser"}]}
			],
			"outputs":[{"workType":"chapter","state":"complete"}],
			"env":{"TEAM":"{{ index .Tags \"team\" }}"}
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	ws := cfg.Workstations[0]
	if got := ws.Env["TEAM"]; got != `{{ index .Tags "team" }}` {
		t.Fatalf("expected env TEAM to be preserved, got %q in %#v", got, ws.Env)
	}
	if ws.Inputs[1].Guard == nil {
		t.Fatal("expected current input guards array to preserve guard")
	}
	if ws.Inputs[1].Guard.ParentInput != "chapter" {
		t.Fatalf("expected guard parent input chapter, got %q", ws.Inputs[1].Guard.ParentInput)
	}
	if ws.Inputs[1].Guard.SpawnedBy != "chapter-parser" {
		t.Fatalf("expected guard spawned_by chapter-parser, got %q", ws.Inputs[1].Guard.SpawnedBy)
	}

	data, err := MarshalCanonicalFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal canonical output: %v", err)
	}
	workstations := out["workstations"].([]any)
	outWs := workstations[0].(map[string]any)
	env := outWs["env"].(map[string]any)
	if _, ok := env["TEAM"]; !ok {
		t.Fatalf("expected canonical output to preserve env key TEAM, got %#v", env)
	}
	if _, ok := env["team"]; ok {
		t.Fatalf("expected canonical output not to normalize env key TEAM to team")
	}
	inputs := outWs["inputs"].([]any)
	guards := inputs[1].(map[string]any)["guards"].([]any)
	guard := guards[0].(map[string]any)
	if _, ok := guard["parentInput"]; !ok {
		t.Fatalf("expected canonical guard parentInput key, got %#v", guard)
	}
}

func TestMarshalCanonicalFactoryConfig_EmitsCamelCaseConfigKeys(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Resources: []interfaces.ResourceConfig{{Name: "agent-slot", Capacity: 2}},
		Workers:   []interfaces.WorkerConfig{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "execute-story",
			WorkerTypeName: "executor",
			Inputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "init",
			}},
			Outputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "complete",
			}},
			Resources: []interfaces.ResourceConfig{{Name: "agent-slot", Capacity: 2}},
			StopWords: []string{"legacy", "retry"},
			Cron:      nil,
		}},
	}

	data, err := MarshalCanonicalFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal canonical output: %v", err)
	}

	if _, ok := out["workTypes"]; !ok {
		t.Fatalf("expected canonical workTypes key")
	}
	if _, ok := out["work_types"]; ok {
		t.Fatalf("expected snake_case work_types key to be normalized out")
	}

	wsValue, ok := out["workstations"].([]any)
	if !ok || len(wsValue) == 0 {
		t.Fatalf("expected workstations array in canonical output")
	}
	ws := wsValue[0].(map[string]any)
	if _, ok := ws["resources"]; !ok {
		t.Fatalf("expected workstation resources key")
	}
	if _, ok := ws["resourceUsage"]; ok {
		t.Fatalf("expected canonical resources key")
	}
}

func TestFactoryConfigFromOpenAPI_ExplicitMapperMatchesJSONBoundary(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Project: "customer-project",
		InputTypes: []interfaces.InputTypeConfig{{
			Name: "default",
			Type: interfaces.InputKindDefault,
		}},
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Resources: []interfaces.ResourceConfig{{Name: "agent-slot", Capacity: 2}},
		Workers: []interfaces.WorkerConfig{{
			Name:          "executor",
			Type:          interfaces.WorkerTypeModel,
			ModelProvider: "openai",
			ModelLocality: interfaces.ModelLocalityCloud,
			Model:         "gpt-5.4",
			Operations: []interfaces.ModelOperation{{
				Name: "TTS",
				Inputs: []interfaces.ModelOperationSlot{{
					Name:         "text",
					ContentTypes: []string{interfaces.ModelOperationContentTypeText},
					Required:     true,
				}},
				Outputs: []interfaces.ModelOperationSlot{{
					Name:         "audio",
					ContentTypes: []string{interfaces.ModelOperationContentTypeAudio},
				}},
			}},
			Timeout:         "30m",
			StopToken:       "DONE",
			SkipPermissions: true,
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			ID:             "execute-story-id",
			Name:           "execute-story",
			Kind:           interfaces.WorkstationKindStandard,
			Type:           interfaces.WorkstationTypeModel,
			WorkerTypeName: "executor",
			PromptTemplate: "Implement {{ .WorkID }}.",
			Inputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "init",
			}},
			Outputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "complete",
			}},
			Resources: []interfaces.ResourceConfig{{Name: "agent-slot", Capacity: 2}},
			Env:       map[string]string{"TEAM": `{{ index .Tags "team" }}`},
		}},
	}

	generated := FactoryConfigToOpenAPI(cfg)
	got, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}

	canonicalJSON, err := MarshalCanonicalFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	want, err := FactoryConfigFromOpenAPIJSON(canonicalJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	if !reflect.DeepEqual(got, *want) {
		t.Fatalf("direct generated mapper mismatch\n got: %#v\nwant: %#v", got, *want)
	}
}

func TestFactoryConfigFromOpenAPI_ReportsNestedGeneratedFieldPathOnMappingError(t *testing.T) {
	guards := []factoryapi.Guard{
		{Type: factoryapi.GuardTypeAllChildrenComplete},
		{Type: factoryapi.GuardTypeAnyChildFailed},
	}
	workstations := []factoryapi.Workstation{{
		Name:   "finish-story",
		Worker: "executor",
		Inputs: []factoryapi.WorkstationIO{{
			WorkType: "story",
			State:    "init",
			Guards:   &guards,
		}},
		Outputs: []factoryapi.WorkstationIO{{
			WorkType: "story",
			State:    "complete",
		}},
	}}
	workTypes := []factoryapi.WorkType{{
		Name: "story",
		States: []factoryapi.WorkState{
			{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
			{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
		},
	}}
	workers := []factoryapi.Worker{{Name: "executor"}}

	_, err := FactoryConfigFromOpenAPI(factoryapi.Factory{
		WorkTypes:    &workTypes,
		Workers:      &workers,
		Workstations: &workstations,
	})
	if err == nil {
		t.Fatal("expected mapping error")
	}
	if !strings.Contains(err.Error(), "factory.workstations[0].inputs[0].guards") {
		t.Fatalf("expected nested field path in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "expected at most 1 guard") {
		t.Fatalf("expected guard cardinality context in error, got %v", err)
	}
}
