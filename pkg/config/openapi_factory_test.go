package config

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestFactoryConfigFromOpenAPIJSON_MapsCanonicalCamelCaseWorkstationSchema(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"finish-chapter-factory",
		"workTypes": [
			{"name":"chapter","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]},
			{"name":"page","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}
		],
		"resources": [{"name":"agent-slot","capacity":2}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","modelProvider":"CLAUDE","stopToken":"COMPLETE"}],
		"workstations": [{
			"id":"finish-chapter-id",
			"name":"finish-chapter",
			"behavior":"STANDARD",
			"worker":"executor",
			"type":"LOGICAL_MOVE",
			"promptTemplate":"Finish {{ .WorkID }}.",
			"inputs":[
				{"workType":"chapter","state":"init"},
				{"workType":"page","state":"complete","guards":[{"type":"ALL_CHILDREN_COMPLETE","parentInput":"chapter","spawnedBy":"chapter-parser"}]}
			],
			"outputs":[{"workType":"chapter","state":"complete"}],
			"resources":[{"name":"agent-slot","capacity":2}],
			"guards":[{"type":"VISIT_COUNT","workstation":"review-story","maxVisits":3}],
			"env":{"TEAM":"{{ index .Tags \"team\" }}"}
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if len(cfg.Workstations) != 1 {
		t.Fatalf("expected one workstation, got %d", len(cfg.Workstations))
	}
	ws := cfg.Workstations[0]
	if ws.ID != "finish-chapter-id" || ws.Kind != interfaces.WorkstationKindStandard {
		t.Fatalf("expected current topology fields to map, got %#v", ws)
	}
	if ws.Type != interfaces.WorkstationTypeLogical || ws.PromptTemplate != "Finish {{ .WorkID }}." {
		t.Fatalf("expected current runtime fields to map, got %#v", ws)
	}
	if ws.Resources[0].Capacity != 2 {
		t.Fatalf("expected resource usage capacity 2, got %d", ws.Resources[0].Capacity)
	}
	if len(ws.Guards) != 1 || ws.Guards[0].Type != interfaces.GuardTypeVisitCount {
		t.Fatalf("expected visit_count workstation guard to map, got %#v", ws.Guards)
	}
	if ws.Guards[0].Workstation != "review-story" || ws.Guards[0].MaxVisits != 3 {
		t.Fatalf("expected visit_count workstation guard details, got %#v", ws.Guards[0])
	}
	if ws.Inputs[1].Guard == nil {
		t.Fatal("expected input guards array to map to internal input guard")
	}
	if ws.Inputs[1].Guard.ParentInput != "chapter" || ws.Inputs[1].Guard.SpawnedBy != "chapter-parser" {
		t.Fatalf("expected current guard fields to map, got %#v", ws.Inputs[1].Guard)
	}
	if got := ws.Env["TEAM"]; got != `{{ index .Tags "team" }}` {
		t.Fatalf("expected env TEAM to be preserved, got %q in %#v", got, ws.Env)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_DecodesCanonicalCamelCaseNestedFields(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"customer-facing-name",
		"id": "customer-project",
		"workTypes": [
			{"name":"chapter","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]},
			{"name":"page","states":[{"name":"complete","type":"TERMINAL"}]}
		],
		"resources": [{"name":"agent-slot","capacity":2}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","modelProvider":"CLAUDE","stopToken":"COMPLETE"}],
		"workstations": [{
			"id":"finish-chapter-id",
			"name":"finish-chapter",
			"behavior":"REPEATER",
			"worker":"executor",
			"type":"MODEL_WORKSTATION",
			"promptTemplate":"Finish {{ .WorkID }}.",
			"inputs":[
				{"workType":"chapter","state":"init"},
				{"workType":"page","state":"complete","guards":[{"type":"ALL_CHILDREN_COMPLETE","parentInput":"chapter","spawnedBy":"chapter-parser"}]}
			],
			"outputs":[{"workType":"chapter","state":"complete"}],
			"resources":[{"name":"agent-slot","capacity":2}]
		}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	if generated.Id == nil || *generated.Id != "customer-project" {
		t.Fatalf("expected generated id customer-project, got %#v", generated.Id)
	}
	if generated.Workers == nil || len(*generated.Workers) != 1 {
		t.Fatalf("expected one generated worker, got %#v", generated.Workers)
	}
	worker := (*generated.Workers)[0]
	if worker.ModelProvider == nil || *worker.ModelProvider != factoryapi.WorkerModelProviderClaude {
		t.Fatalf("expected generated worker modelProvider CLAUDE, got %#v", worker.ModelProvider)
	}
	if worker.StopToken == nil || *worker.StopToken != "COMPLETE" {
		t.Fatalf("expected generated worker stopToken COMPLETE, got %#v", worker.StopToken)
	}
	if generated.Workstations == nil || len(*generated.Workstations) != 1 {
		t.Fatalf("expected one generated workstation, got %#v", generated.Workstations)
	}
	workstation := (*generated.Workstations)[0]
	if workstation.PromptTemplate == nil || *workstation.PromptTemplate != "Finish {{ .WorkID }}." {
		t.Fatalf("expected generated promptTemplate to survive boundary decode, got %#v", workstation.PromptTemplate)
	}
	if workstation.Resources == nil || len(*workstation.Resources) != 1 || (*workstation.Resources)[0].Capacity != 2 {
		t.Fatalf("expected generated resources capacity 2, got %#v", workstation.Resources)
	}
	if len(workstation.Inputs) != 2 || workstation.Inputs[1].Guards == nil || len(*workstation.Inputs[1].Guards) != 1 {
		t.Fatalf("expected generated nested guards to survive boundary decode, got %#v", workstation.Inputs)
	}
	guard := (*workstation.Inputs[1].Guards)[0]
	if guard.ParentInput == nil || *guard.ParentInput != "chapter" || guard.SpawnedBy == nil || *guard.SpawnedBy != "chapter-parser" {
		t.Fatalf("expected generated guard camelCase fields to survive boundary decode, got %#v", guard)
	}

	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	if cfg.Workstations[0].Type != interfaces.WorkstationTypeModel {
		t.Fatalf("expected runtime workstation type MODEL_WORKSTATION, got %#v", cfg.Workstations[0])
	}
	if cfg.Workstations[0].Resources[0].Capacity != 2 {
		t.Fatalf("expected runtime resources capacity 2, got %#v", cfg.Workstations[0].Resources)
	}
	if cfg.Workstations[0].Inputs[1].Guard == nil {
		t.Fatal("expected runtime guard to survive generated boundary mapping")
	}
	if cfg.Workstations[0].Inputs[1].Guard.ParentInput != "chapter" || cfg.Workstations[0].Inputs[1].Guard.SpawnedBy != "chapter-parser" {
		t.Fatalf("expected runtime guard fields to match generated boundary, got %#v", cfg.Workstations[0].Inputs[1].Guard)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_DecodesSameNameInputGuard(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"same-name-input-guard-factory",
		"workTypes": [
			{"name":"planItem","states":[{"name":"ready","type":"PROCESSING"}]},
			{"name":"taskItem","states":[{"name":"ready","type":"PROCESSING"},{"name":"matched","type":"TERMINAL"}]}
		],
		"workers": [{"name":"matcher"}],
		"workstations": [{
			"name":"match-items",
			"worker":"matcher",
			"inputs":[
				{"workType":"planItem","state":"ready"},
				{"workType":"taskItem","state":"ready","guards":[{"type":"SAME_NAME","matchInput":"planItem"}]}
			],
			"outputs":[{"workType":"taskItem","state":"matched"}]
		}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	if generated.Workstations == nil || len(*generated.Workstations) != 1 {
		t.Fatalf("expected one generated workstation, got %#v", generated.Workstations)
	}
	workstation := (*generated.Workstations)[0]
	if len(workstation.Inputs) != 2 || workstation.Inputs[1].Guards == nil || len(*workstation.Inputs[1].Guards) != 1 {
		t.Fatalf("expected generated same-name guard to survive boundary decode, got %#v", workstation.Inputs)
	}
	guard := (*workstation.Inputs[1].Guards)[0]
	if guard.Type != factoryapi.GuardTypeSameName {
		t.Fatalf("expected generated guard type SAME_NAME, got %#v", guard.Type)
	}
	if guard.MatchInput == nil || *guard.MatchInput != "planItem" {
		t.Fatalf("expected generated guard matchInput planItem, got %#v", guard.MatchInput)
	}
	if guard.ParentInput != nil || guard.SpawnedBy != nil {
		t.Fatalf("expected same-name guard to keep parent-aware fields unset, got %#v", guard)
	}

	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	runtimeGuard := cfg.Workstations[0].Inputs[1].Guard
	if runtimeGuard == nil {
		t.Fatal("expected runtime same-name guard to survive generated mapping")
	}
	if runtimeGuard.Type != interfaces.GuardTypeSameName || runtimeGuard.MatchInput != "planItem" {
		t.Fatalf("expected runtime same-name guard fields to match generated boundary, got %#v", runtimeGuard)
	}
	if runtimeGuard.ParentInput != "" || runtimeGuard.SpawnedBy != "" {
		t.Fatalf("expected runtime same-name guard to keep parent-aware fields empty, got %#v", runtimeGuard)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_DecodesMatchesFieldsWorkstationGuard(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"matches-fields-guard-factory",
		"workTypes": [
			{"name":"asset","states":[{"name":"ready","type":"PROCESSING"},{"name":"matched","type":"TERMINAL"}]}
		],
		"workers": [{"name":"matcher"}],
		"workstations": [{
			"name":"match-assets",
			"worker":"matcher",
			"inputs":[{"workType":"asset","state":"ready"}],
			"outputs":[{"workType":"asset","state":"matched"}],
			"guards":[{"type":"MATCHES_FIELDS","matchConfig":{"inputKey":".Tags[\"_last_output\"]"}}]
		}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	workstation := (*generated.Workstations)[0]
	if workstation.Guards == nil || len(*workstation.Guards) != 1 {
		t.Fatalf("expected generated matches-fields guard to survive boundary decode, got %#v", workstation.Guards)
	}
	guard := (*workstation.Guards)[0]
	if guard.Type != factoryapi.GuardTypeMatchesFields {
		t.Fatalf("expected generated guard type MATCHES_FIELDS, got %#v", guard.Type)
	}
	if guard.MatchConfig == nil || guard.MatchConfig.InputKey != `.Tags["_last_output"]` {
		t.Fatalf("expected generated guard matchConfig.inputKey, got %#v", guard.MatchConfig)
	}

	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	runtimeGuard := cfg.Workstations[0].Guards[0]
	if runtimeGuard.Type != interfaces.GuardTypeMatchesFields {
		t.Fatalf("expected runtime guard type matches_fields, got %#v", runtimeGuard)
	}
	if runtimeGuard.MatchConfig == nil || runtimeGuard.MatchConfig.InputKey != `.Tags["_last_output"]` {
		t.Fatalf("expected runtime matches-fields guard matchConfig.inputKey, got %#v", runtimeGuard.MatchConfig)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_DecodesFactoryInferenceThrottleGuard(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"factory-throttle-guard-factory",
		"guards":[{"type":"INFERENCE_THROTTLE_GUARD","modelProvider":"CLAUDE","model":"claude-sonnet-4-20250514","refreshWindow":"15m"}],
		"workTypes": [
			{"name":"asset","states":[{"name":"ready","type":"PROCESSING"},{"name":"matched","type":"TERMINAL"}]}
		],
		"workers": [{"name":"matcher"}],
		"workstations": [{
			"name":"match-assets",
			"worker":"matcher",
			"inputs":[{"workType":"asset","state":"ready"}],
			"outputs":[{"workType":"asset","state":"matched"}]
		}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	if generated.Guards == nil || len(*generated.Guards) != 1 {
		t.Fatalf("expected generated factory guard to survive boundary decode, got %#v", generated.Guards)
	}
	guard := (*generated.Guards)[0]
	if guard.Type != factoryapi.GuardTypeInferenceThrottle {
		t.Fatalf("expected generated guard type INFERENCE_THROTTLE_GUARD, got %#v", guard.Type)
	}
	if guard.ModelProvider != factoryapi.WorkerModelProviderClaude {
		t.Fatalf("expected generated guard modelProvider CLAUDE, got %#v", guard.ModelProvider)
	}
	if guard.Model == nil || *guard.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("expected generated guard model, got %#v", guard.Model)
	}
	if guard.RefreshWindow != "15m" {
		t.Fatalf("expected generated guard refreshWindow, got %#v", guard.RefreshWindow)
	}

	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	if len(cfg.Guards) != 1 {
		t.Fatalf("expected runtime factory guard to survive generated mapping, got %#v", cfg.Guards)
	}
	runtimeGuard := cfg.Guards[0]
	if runtimeGuard.Type != interfaces.GuardTypeInferenceThrottle {
		t.Fatalf("expected runtime guard type inference_throttle_guard, got %#v", runtimeGuard)
	}
	if runtimeGuard.ModelProvider != "claude" || runtimeGuard.Model != "claude-sonnet-4-20250514" || runtimeGuard.RefreshWindow != "15m" {
		t.Fatalf("expected runtime factory guard fields to match generated boundary, got %#v", runtimeGuard)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsFactoryInferenceThrottleGuardWithWorkstationGuardFields(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"factory-throttle-guard-invalid-fields-factory",
		"guards":[{
			"type":"INFERENCE_THROTTLE_GUARD",
			"modelProvider":"CLAUDE",
			"refreshWindow":"15m",
			"workstation":"processor"
		}],
		"workTypes": [
			{"name":"asset","states":[{"name":"ready","type":"PROCESSING"},{"name":"matched","type":"TERMINAL"}]}
		],
		"workers": [{"name":"matcher"}],
		"workstations": [{
			"name":"match-assets",
			"worker":"matcher",
			"inputs":[{"workType":"asset","state":"ready"}],
			"outputs":[{"workType":"asset","state":"matched"}]
		}]
	}`)

	_, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err == nil {
		t.Fatal("expected workstation-only guard fields on factory guard to fail at generated boundary")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "guards[0].workstation is not supported") {
		t.Fatalf("expected factory guard field path in error, got %v", err)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsInferenceThrottleGuardOnWorkstation(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"workstation-throttle-guard-factory",
		"workTypes": [{"name":"story","states":[{"name":"ready","type":"PROCESSING"},{"name":"done","type":"TERMINAL"}]}],
		"workers": [{"name":"writer"}],
		"workstations": [{
			"name":"draft-story",
			"worker":"writer",
			"guards":[{"type":"INFERENCE_THROTTLE_GUARD"}],
			"inputs":[{"workType":"story","state":"ready"}],
			"outputs":[{"workType":"story","state":"done"}]
		}]
	}`)

	_, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err == nil {
		t.Fatal("expected root-only inference throttle guard to fail on workstation guards")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workstations[0].guards[0].type") {
		t.Fatalf("expected workstation guard field path in error, got %v", err)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsInferenceThrottleGuardOnInput(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"input-throttle-guard-factory",
		"workTypes": [{"name":"story","states":[{"name":"ready","type":"PROCESSING"},{"name":"done","type":"TERMINAL"}]}],
		"workers": [{"name":"writer"}],
		"workstations": [{
			"name":"draft-story",
			"worker":"writer",
			"inputs":[{
				"workType":"story",
				"state":"ready",
				"guards":[{"type":"INFERENCE_THROTTLE_GUARD"}]
			}],
			"outputs":[{"workType":"story","state":"done"}]
		}]
	}`)

	_, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err == nil {
		t.Fatal("expected root-only inference throttle guard to fail on input guards")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workstations[0].inputs[0].guards[0].type") {
		t.Fatalf("expected input guard field path in error, got %v", err)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsRetiredFanInFieldAtBoundary(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"retired-fan-in-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"join":{"waitFor":"story","waitState":"complete","require":"all"}
		}]
	}`)

	_, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err == nil {
		t.Fatal("expected retired join field to fail at generated boundary")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workstations[0].join is not supported") {
		t.Fatalf("expected retired join message, got %v", err)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsRetiredExhaustionRulesFieldAtBoundary(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"retired-exhaustion-rules-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"failed","type":"FAILED"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"failed"}]
		}],
		"exhaustionRules": [{
			"name":"execute-story-loop-breaker",
			"watchWorkstation":"execute-story",
			"maxVisits":3,
			"source":{"workType":"story","state":"init"},
			"target":{"workType":"story","state":"failed"}
		}]
	}`)

	_, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err == nil {
		t.Fatal("expected retired exhaustionRules field to fail at generated boundary")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "exhaustion_rules is retired") {
		t.Fatalf("expected retired exhaustion_rules message, got %v", err)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsRetiredCronIntervalFieldAtBoundary(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"retired-cron-interval-factory",
		"workTypes": [{"name":"task","states":[{"name":"ready","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"daily-refresh",
			"behavior":"CRON",
			"worker":"executor",
			"outputs":[{"workType":"task","state":"complete"}],
			"cron":{"interval":"5m"}
		}]
	}`)

	_, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err == nil {
		t.Fatal("expected retired cron interval field to fail at generated boundary")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workstations[0].cron.interval is not supported; use cron.schedule") {
		t.Fatalf("expected retired cron interval message, got %v", err)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsMisCasedEnumValuesAtBoundary(t *testing.T) {
	testCases := []struct {
		name      string
		fieldPath string
		value     string
		payload   string
	}{
		{
			name:      "worker type",
			fieldPath: "workers[0].type",
			value:     "model_worker",
			payload: `{
				"name":"worker-type-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"model_worker"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "worker model provider",
			fieldPath: "workers[0].modelProvider",
			value:     "Claude",
			payload: `{
				"name":"worker-model-provider-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER","modelProvider":"Claude"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"type":"MODEL_WORKSTATION",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "workstation behavior",
			fieldPath: "workstations[0].behavior",
			value:     "cron",
			payload: `{
				"name":"workstation-behavior-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"behavior":"cron",
					"type":"MODEL_WORKSTATION",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "workstation type",
			fieldPath: "workstations[0].type",
			value:     "logical_move",
			payload: `{
				"name":"workstation-type-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"type":"logical_move",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := GeneratedFactoryFromOpenAPIJSON([]byte(tc.payload))
			if err == nil {
				t.Fatal("expected mis-cased enum value to fail at generated boundary")
			}
			if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
				t.Fatalf("expected generated boundary context, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.fieldPath) {
				t.Fatalf("expected field path %q in error, got %v", tc.fieldPath, err)
			}
			if !strings.Contains(err.Error(), `unsupported value "`+tc.value+`"`) {
				t.Fatalf("expected unsupported value %q in error, got %v", tc.value, err)
			}
		})
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_ParsesCanonicalUppercaseSharedEnumsAtBoundary(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"uppercase-enums-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{
			"name":"executor",
			"type":"MODEL_WORKER",
			"modelProvider":"CODEX",
			"executorProvider":"SCRIPT_WRAP"
		}],
		"workstations": [{
			"name":"execute-story",
			"behavior":"STANDARD",
			"worker":"executor",
			"type":"MODEL_WORKSTATION",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	if generated.Workers == nil || len(*generated.Workers) != 1 {
		t.Fatalf("expected one generated worker, got %#v", generated.Workers)
	}
	worker := (*generated.Workers)[0]
	if worker.ModelProvider == nil || *worker.ModelProvider != factoryapi.WorkerModelProviderCodex {
		t.Fatalf("expected generated worker modelProvider CODEX, got %#v", worker.ModelProvider)
	}
	if worker.ExecutorProvider == nil || *worker.ExecutorProvider != factoryapi.WorkerProviderScriptWrap {
		t.Fatalf("expected generated worker executorProvider SCRIPT_WRAP, got %#v", worker.ExecutorProvider)
	}

	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	if got := cfg.Workers[0].ModelProvider; got != "codex" {
		t.Fatalf("expected runtime worker modelProvider codex, got %q", got)
	}
	if got := cfg.Workers[0].ExecutorProvider; got != "script_wrap" {
		t.Fatalf("expected runtime worker executorProvider script_wrap, got %q", got)
	}
	if got := cfg.Workstations[0].Type; got != interfaces.WorkstationTypeModel {
		t.Fatalf("expected runtime workstation type MODEL_WORKSTATION, got %q", got)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsUnsupportedExecutorProviderAtBoundary(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"unsupported-executor-provider-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{
			"name":"executor",
			"type":"MODEL_WORKER",
			"executorProvider":"custom-executor"
		}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"type":"MODEL_WORKSTATION",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	_, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err == nil {
		t.Fatal("expected unsupported executorProvider to fail at generated boundary")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workers[0].executorProvider") {
		t.Fatalf("expected executorProvider field path in error, got %v", err)
	}
	if !strings.Contains(err.Error(), `unsupported value "custom-executor"`) {
		t.Fatalf("expected unsupported executorProvider value in error, got %v", err)
	}
}

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
		Resources: []interfaces.ResourceConfig{
			{Name: "agent-slot", Capacity: 2},
		},
		Workers: []interfaces.WorkerConfig{
			{Name: "executor"},
		},
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
			Name:            "executor",
			Type:            interfaces.WorkerTypeModel,
			ModelProvider:   "openai",
			Model:           "gpt-5.4",
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
