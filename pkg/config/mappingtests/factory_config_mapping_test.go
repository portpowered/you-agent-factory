package mappingtests

import (
	"encoding/json"
	. "github.com/portpowered/infinite-you/pkg/config"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// portos:func-length-exception owner=agent-factory reason=legacy-config-roundtrip-fixture review=2026-07-18 removal=split-roundtrip-assertions-before-next-config-schema-change
func TestFactoryConfigMapper_FlattenAndExpandPreservesConfigContent(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	original := &interfaces.FactoryConfig{
		Name:    "customer-facing-name",
		Project: "sample-service",
		Version: &interfaces.FactoryVersion{
			Logical:  7,
			Physical: time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC),
		},
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "story",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Resources: []interfaces.ResourceConfig{{Name: "agent-slot", Capacity: 2}},
		Workers:   []interfaces.WorkerConfig{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				ID:             "execute-story-id",
				Name:           "execute-story",
				Kind:           interfaces.WorkstationKindStandard,
				Type:           interfaces.WorkstationTypeLogical,
				WorkerTypeName: "executor",
				Inputs: []interfaces.IOConfig{
					{WorkTypeName: "story", StateName: "init"},
				},
				Outputs: []interfaces.IOConfig{
					{WorkTypeName: "story", StateName: "complete"},
				},
				Guards: []interfaces.GuardConfig{{
					Type:        interfaces.GuardTypeVisitCount,
					Workstation: "review-story",
					MaxVisits:   3,
				}},
				Resources: []interfaces.ResourceConfig{
					{Name: "agent-slot", Capacity: 2},
				},
				StopWords: []string{"DONE", "RETRY"},
				Cron:      nil,
			},
		},
	}

	flattened, err := mapper.Flatten(original)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}

	expanded, err := mapper.Expand(flattened)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}

	assertExpandedConfigRoundTrip(t, expanded, original)
}

func assertExpandedConfigRoundTrip(t *testing.T, expanded, original *interfaces.FactoryConfig) {
	t.Helper()

	if expanded.Project != original.Project {
		t.Fatalf("expected project %q, got %q", original.Project, expanded.Project)
	}
	if expanded.Name != original.Name {
		t.Fatalf("expected name %q, got %q", original.Name, expanded.Name)
	}
	if expanded.Version == nil || expanded.Version.Logical != original.Version.Logical || !expanded.Version.Physical.Equal(original.Version.Physical) {
		t.Fatalf("expected version %#v, got %#v", original.Version, expanded.Version)
	}
	if len(expanded.WorkTypes) != len(original.WorkTypes) {
		t.Fatalf("expected %d work types, got %d", len(original.WorkTypes), len(expanded.WorkTypes))
	}
	assertExpandedWorkstationRoundTrip(t, expanded.Workstations[0], original.Workstations[0])
}

func assertExpandedWorkstationRoundTrip(t *testing.T, got, want interfaces.FactoryWorkstationConfig) {
	t.Helper()

	if got.Kind != want.Kind {
		t.Fatalf("expected workstation kind %q, got %q", want.Kind, got.Kind)
	}
	if got.ID != want.ID {
		t.Fatalf("expected workstation id %q, got %q", want.ID, got.ID)
	}
	if got.Resources[0].Capacity != want.Resources[0].Capacity {
		t.Fatalf("expected resource capacity %d, got %d", want.Resources[0].Capacity, got.Resources[0].Capacity)
	}
	if len(got.Guards) != 1 {
		t.Fatalf("expected one workstation guard, got %#v", got.Guards)
	}
	if got.Guards[0].Type != interfaces.GuardTypeVisitCount {
		t.Fatalf("expected visit_count guard, got %#v", got.Guards[0])
	}
	if got.Guards[0].Workstation != "review-story" || got.Guards[0].MaxVisits != 3 {
		t.Fatalf("expected visit_count guard details to roundtrip, got %#v", got.Guards[0])
	}
}

func TestFactoryConfigMapper_FlattenOmitsEmptyOptionalCollectionsAndExpandsPopulatedOptionals(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	flattened, err := mapper.Flatten(&interfaces.FactoryConfig{
		Name: "optional-boundary-fields",
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			ID:               "execute-story-id",
			Name:             "execute-story",
			WorkerTypeName:   "executor",
			Inputs:           []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs:          []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
			WorkingDirectory: "/tmp/story",
			StopWords:        []string{},
			Env:              map[string]string{},
		}},
	})
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}

	payload := mustDecodeFactoryPayload(t, flattened)
	workstationPayload := payload["workstations"].([]any)[0].(map[string]any)
	assertMissingKey(t, workstationPayload, "stopWords")
	assertMissingKey(t, workstationPayload, "env")
	if got := workstationPayload["id"]; got != "execute-story-id" {
		t.Fatalf("flattened workstation id = %#v, want %q", got, "execute-story-id")
	}
	if got := workstationPayload["workingDirectory"]; got != "/tmp/story" {
		t.Fatalf("flattened workingDirectory = %#v, want %q", got, "/tmp/story")
	}

	expanded, err := mapper.Expand([]byte(`{
		"name": "optional-boundary-fields",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"id":"execute-story-id",
			"name":"execute-story",
			"worker":"executor",
			"workingDirectory":"/tmp/story",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"stopWords":["DONE"],
			"env":{"MODE":"strict"}
		}]
	}`))
	if err != nil {
		t.Fatalf("mapper.Expand populated optionals: %v", err)
	}

	workstation := expanded.Workstations[0]
	if workstation.WorkingDirectory != "/tmp/story" {
		t.Fatalf("expanded workingDirectory = %q, want /tmp/story", workstation.WorkingDirectory)
	}
	if len(workstation.StopWords) != 1 || workstation.StopWords[0] != "DONE" {
		t.Fatalf("expanded stopWords = %#v, want [DONE]", workstation.StopWords)
	}
	if workstation.Env["MODE"] != "strict" {
		t.Fatalf("expanded env = %#v, want MODE=strict", workstation.Env)
	}
}

func TestFactoryConfigToOpenAPI_CopiesOptionalCollections(t *testing.T) {
	stopWords := []string{"DONE"}
	env := map[string]string{"MODE": "strict"}
	cfg := &interfaces.FactoryConfig{
		Name: "copy-safe-optionals",
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			ID:             "execute-story-id",
			Name:           "execute-story",
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
			StopWords:      stopWords,
			Env:            env,
		}},
	}

	generated := FactoryConfigToOpenAPI(cfg)
	stopWords[0] = "MUTATED"
	stopWords = append(stopWords, "LATE")
	env["MODE"] = "mutated"
	env["LATE"] = "value"

	workstation := (*generated.Workstations)[0]
	if workstation.StopWords == nil || len(*workstation.StopWords) != 1 || (*workstation.StopWords)[0] != "DONE" {
		t.Fatalf("generated stopWords = %#v, want copied pre-mutation values", workstation.StopWords)
	}
	if workstation.Env == nil || (*workstation.Env)["MODE"] != "strict" {
		t.Fatalf("generated env = %#v, want copied pre-mutation values", workstation.Env)
	}
	if _, ok := (*workstation.Env)["LATE"]; ok {
		t.Fatalf("generated env = %#v, want copied map to omit post-mapping additions", workstation.Env)
	}
}

func TestFactoryConfigFromOpenAPI_CopiesOptionalCollections(t *testing.T) {
	stopWords := []string{"DONE"}
	env := factoryapi.StringMap{"MODE": "strict"}
	apiCfg := factoryapi.Factory{
		Name: "copy-safe-openapi-optionals",
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "story",
			States: []factoryapi.WorkState{
				{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
			},
		}},
		Workers: &[]factoryapi.Worker{{Name: "executor"}},
		Workstations: &[]factoryapi.Workstation{{
			Id:        stringPtr("execute-story-id"),
			Name:      "execute-story",
			Worker:    "executor",
			Inputs:    []factoryapi.WorkstationIO{{WorkType: "story", State: "init"}},
			Outputs:   &[]factoryapi.WorkstationIO{{WorkType: "story", State: "complete"}},
			StopWords: &stopWords,
			Env:       &env,
		}},
	}

	cfg, err := FactoryConfigFromOpenAPI(apiCfg)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}

	stopWords[0] = "MUTATED"
	stopWords = append(stopWords, "LATE")
	env["MODE"] = "mutated"
	env["LATE"] = "value"

	workstation := cfg.Workstations[0]
	if len(workstation.StopWords) != 1 || workstation.StopWords[0] != "DONE" {
		t.Fatalf("expanded stopWords = %#v, want copied pre-mutation values", workstation.StopWords)
	}
	if workstation.Env["MODE"] != "strict" {
		t.Fatalf("expanded env = %#v, want copied pre-mutation values", workstation.Env)
	}
	if _, ok := workstation.Env["LATE"]; ok {
		t.Fatalf("expanded env = %#v, want copied map to omit post-mapping additions", workstation.Env)
	}
}

func TestFactoryConfigMapper_ExpandSupportsCanonicalBoundaryKeysAndCapacity(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"name": "analytics-factory",
		"id": "analytics-platform",
		"inputTypes": [{"name":"default","type":"DEFAULT"}],
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"resources": [{"name":"agent-slot","capacity":2}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"id":"execute-story-id",
			"name":"execute-story",
			"behavior":"CRON",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"cron":{"schedule":"*/10 * * * *","triggerAtStart":true,"jitter":"1s","expiryWindow":"20s"},
			"resources":[{"name":"agent-slot","capacity":2}]
		}]
	}`)

	cfg, err := mapper.Expand(raw)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}
	assertExpandedCanonicalBoundaryConfig(t, cfg)

	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}
	assertFlattenedCanonicalBoundaryPayload(t, flattened)
}

func TestFactoryConfigMapper_FlattenAndExpandPreservesNonSuccessRouteArrays(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	original := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "retry", Type: interfaces.StateTypeProcessing},
				{Name: "rejected", Type: interfaces.StateTypeProcessing},
				{Name: "failed", Type: interfaces.StateTypeFailed},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "executor"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "execute-story",
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
			OnContinue: []interfaces.IOConfig{
				{WorkTypeName: "story", StateName: "retry"},
				{WorkTypeName: "story", StateName: "complete"},
			},
			OnRejection: []interfaces.IOConfig{
				{WorkTypeName: "story", StateName: "init"},
				{WorkTypeName: "story", StateName: "rejected"},
			},
			OnFailure: []interfaces.IOConfig{
				{WorkTypeName: "story", StateName: "failed"},
				{WorkTypeName: "story", StateName: "rejected"},
			},
		}},
	}

	flattened, err := mapper.Flatten(original)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}

	payload := mustDecodeFactoryPayload(t, flattened)
	workstationPayload := payload["workstations"].([]any)[0].(map[string]any)
	for _, key := range []string{"onContinue", "onRejection", "onFailure"} {
		routes, ok := workstationPayload[key].([]any)
		if !ok {
			t.Fatalf("%s should flatten as an array, got %#v", key, workstationPayload[key])
		}
		if len(routes) != 2 {
			t.Fatalf("%s flattened route count = %d, want 2", key, len(routes))
		}
	}

	expanded, err := mapper.Expand(flattened)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}

	got := expanded.Workstations[0]
	want := original.Workstations[0]
	if !reflect.DeepEqual(got.OnContinue, want.OnContinue) {
		t.Fatalf("onContinue roundtrip mismatch\n got: %#v\nwant: %#v", got.OnContinue, want.OnContinue)
	}
	if !reflect.DeepEqual(got.OnRejection, want.OnRejection) {
		t.Fatalf("onRejection roundtrip mismatch\n got: %#v\nwant: %#v", got.OnRejection, want.OnRejection)
	}
	if !reflect.DeepEqual(got.OnFailure, want.OnFailure) {
		t.Fatalf("onFailure roundtrip mismatch\n got: %#v\nwant: %#v", got.OnFailure, want.OnFailure)
	}
}

func TestFactoryConfigMapper_FlattenAndExpandPreservesClassificationRoutes(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	original := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "classifier"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "classify-task",
			Type:           interfaces.WorkstationTypeClassify,
			WorkerTypeName: "classifier",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			ClassificationRoutes: []interfaces.ClassificationRouteConfig{
				{Label: "approved", Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}}},
				{Label: "spam", Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}}},
			},
			OnFailure: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
		}},
	}

	flattened, err := mapper.Flatten(original)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}

	payload := mustDecodeFactoryPayload(t, flattened)
	workstationPayload := payload["workstations"].([]any)[0].(map[string]any)
	routes, ok := workstationPayload["classificationRoutes"].([]any)
	if !ok || len(routes) != 2 {
		t.Fatalf("classificationRoutes should flatten as two routes, got %#v", workstationPayload["classificationRoutes"])
	}

	expanded, err := mapper.Expand(flattened)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}

	if got, want := expanded.Workstations[0].ClassificationRoutes, original.Workstations[0].ClassificationRoutes; !reflect.DeepEqual(got, want) {
		t.Fatalf("classificationRoutes roundtrip mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func assertExpandedCanonicalBoundaryConfig(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()

	if len(cfg.WorkTypes) != 1 || cfg.WorkTypes[0].Name != "story" {
		t.Fatalf("expected one parsed work type named story, got %#v", cfg.WorkTypes)
	}
	if cfg.Project != "analytics-platform" {
		t.Fatalf("expected id analytics-platform to map to project, got %q", cfg.Project)
	}
	if cfg.Name != "analytics-factory" {
		t.Fatalf("expected name analytics-factory to map through boundary, got %q", cfg.Name)
	}
	if cfg.Workstations[0].ID != "execute-story-id" {
		t.Fatalf("expected workstation id execute-story-id, got %q", cfg.Workstations[0].ID)
	}
	if cfg.Workstations[0].Kind != "cron" {
		t.Fatalf("expected workstation kind cron, got %q", cfg.Workstations[0].Kind)
	}
	if cfg.Workstations[0].Resources[0].Capacity != 2 {
		t.Fatalf("expected canonical capacity 2, got %d", cfg.Workstations[0].Resources[0].Capacity)
	}
	if cfg.Workstations[0].Cron == nil || cfg.Workstations[0].Cron.Jitter != "1s" || cfg.Workstations[0].Cron.ExpiryWindow != "20s" {
		t.Fatalf("expected cron jitter and expiry_window to be preserved, got %#v", cfg.Workstations[0].Cron)
	}
	if cfg.Workstations[0].Cron.Schedule != "*/10 * * * *" || !cfg.Workstations[0].Cron.TriggerAtStart {
		t.Fatalf("expected cron schedule and trigger_at_start to be preserved, got %#v", cfg.Workstations[0].Cron)
	}
}

func assertFlattenedCanonicalBoundaryPayload(t *testing.T, flattened []byte) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(flattened, &payload); err != nil {
		t.Fatalf("unmarshal flattened payload: %v", err)
	}
	if _, ok := payload["workTypes"]; !ok {
		t.Fatalf("expected flattened payload to use workTypes key")
	}
	if _, ok := payload["work_types"]; ok {
		t.Fatalf("expected flattened payload not to include work_types key")
	}
	workstationsPayload := payload["workstations"].([]any)
	workstationPayload := workstationsPayload[0].(map[string]any)
	cronPayload := workstationPayload["cron"].(map[string]any)
	if _, ok := cronPayload["expiryWindow"]; !ok {
		t.Fatalf("expected canonical cron config to use expiryWindow key")
	}
	if _, ok := cronPayload["expiry_window"]; ok {
		t.Fatalf("expected canonical cron config not to include expiry_window key")
	}
}

func TestFactoryConfigMapper_ExpandRejectsRetiredExhaustionRulesWithMigrationGuidance(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"failed","type":"FAILED"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"behavior":"REPEATER",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"init"}]
		}],
		"exhaustionRules": [{
			"name":"execute-story-loop-breaker",
			"watchWorkstation":"execute-story",
			"maxVisits":3,
			"source":{"workType":"story","state":"init"},
			"target":{"workType":"story","state":"failed"}
		}]
	}`)

	_, err := mapper.Expand(raw)
	if err == nil {
		t.Fatal("expected retired exhaustion_rules field to be rejected")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "exhaustion_rules is retired") {
		t.Fatalf("expected retired exhaustion_rules error, got %v", err)
	}
	if !strings.Contains(err.Error(), "guarded LOGICAL_MOVE workstation") {
		t.Fatalf("expected LOGICAL_MOVE migration guidance, got %v", err)
	}
	if !strings.Contains(err.Error(), "visit_count guard") {
		t.Fatalf("expected visit_count migration guidance, got %v", err)
	}
}

func TestFactoryConfigMapper_FlattenPreservesGuardedLogicalMoveLoopBreakers(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	original := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "failed", Type: interfaces.StateTypeFailed},
			},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:    "process-task-loop-breaker",
			Type:    interfaces.WorkstationTypeLogical,
			Inputs:  []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs: []interfaces.IOConfig{{WorkTypeName: "task", StateName: "failed"}},
			Guards: []interfaces.GuardConfig{{
				Type:        interfaces.GuardTypeVisitCount,
				Workstation: "process-task",
				MaxVisits:   3,
			}},
		}},
	}

	flattened, err := mapper.Flatten(original)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}

	payload := mustDecodeFactoryPayload(t, flattened)
	assertNoRetiredExhaustionRulesPayload(t, payload)
	assertLoopBreakerPayload(t, payload, "process-task-loop-breaker", "process-task", 3)

	expanded, err := mapper.Expand(flattened)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}
	assertExpandedLoopBreaker(t, expanded, "process-task-loop-breaker", "process-task", 3)
}

func TestFactoryConfigMapper_ExpandRejectsRetiredLegacyPayloadAliases(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	legacy := []byte(`{
		"id": "analytics-platform",
		"inputTypes": [{"name":"default","type":"DEFAULT"}],
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"failed","type":"FAILED"},{"name":"complete","type":"TERMINAL"}]}],
		"resources": [{"name":"agent-slot","capacity":2}],
		"workers": [{
			"name":"executor",
			"type":"MODEL_WORKER",
			"provider":"script_wrap",
			"skipPermissions":true
		}],
		"workstations": [{
			"id":"execute-story-id",
			"name":"execute-story",
			"behavior":"CRON",
			"type":"MODEL_WORKSTATION",
			"worker":"executor",
			"promptFile":"prompt.md",
			"body":"Implement {{ .WorkID }}.",
			"outputSchema":"schema.json",
			"onRejection":[{"workType":"story","state":"init"}],
			"onFailure":[{"workType":"story","state":"failed"}],
			"resources":[{"name":"agent-slot","capacity":2}],
			"stopWords":["DONE"],
			"workingDirectory":"/repo/{{ .WorkID }}",
			"cron":{"schedule":"*/10 * * * *","triggerAtStart":true,"expiryWindow":"20s"},
			"inputs":[
				{"workType":"story","state":"init"},
				{"workType":"story","state":"complete","guards":[{"type":"ALL_CHILDREN_COMPLETE","parentInput":"story","spawnedBy":"fanout"}]}
			],
			"outputs":[{"workType":"story","state":"complete"}],
			"guards":[{"type":"VISIT_COUNT","workstation":"execute-story","maxVisits":3}]
		}]
	}`)

	_, err := mapper.Expand(legacy)
	if err == nil {
		t.Fatal("expected retired legacy payload aliases to be rejected")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workers[0].provider is not supported; use executorProvider") {
		t.Fatalf("expected provider retirement guidance, got %v", err)
	}
}

func TestFactoryConfigMapper_FlattenMergesWorkstationStopWordsAndDetachesSlices(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	cfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:             "execute-story",
			StopWords:        []string{"CANONICAL", "SHARED"},
			RuntimeStopWords: []string{"RUNTIME", "SHARED", "TAIL"},
		}},
	}

	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}

	payload := mustDecodeFactoryPayload(t, flattened)
	workstationPayload := payload["workstations"].([]any)[0].(map[string]any)
	gotAny, ok := workstationPayload["stopWords"].([]any)
	if !ok {
		t.Fatalf("expected flattened stopWords array, got %#v", workstationPayload["stopWords"])
	}
	got := make([]string, len(gotAny))
	for i, value := range gotAny {
		got[i], ok = value.(string)
		if !ok {
			t.Fatalf("expected stopWords[%d] to be a string, got %#v", i, value)
		}
	}

	want := []string{"CANONICAL", "SHARED", "RUNTIME", "TAIL"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("flattened stopWords mismatch\n got: %#v\nwant: %#v", got, want)
	}

	cfg.Workstations[0].StopWords[0] = "CHANGED-CANONICAL"
	cfg.Workstations[0].RuntimeStopWords[0] = "CHANGED-RUNTIME"

	gotAny = workstationPayload["stopWords"].([]any)
	got = make([]string, len(gotAny))
	for i, value := range gotAny {
		got[i] = value.(string)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("flattened stopWords should be detached from source slices\n got: %#v\nwant: %#v", got, want)
	}
}

func TestFactoryConfigMapper_ExpandReportsCanonicalRouteArrayFieldPath(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"name":"story-factory",
		"id":"story-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"onContinue":[{
				"workType":"story",
				"state":"complete",
				"guards":[
					{"type":"ALL_CHILDREN_COMPLETE"},
					{"type":"ANY_CHILD_FAILED"}
				]
			}]
		}]
	}`)

	_, err := mapper.Expand(raw)
	if err == nil {
		t.Fatal("expected invalid onContinue route to fail")
	}
	if !strings.Contains(err.Error(), "factory.workstations[0].onContinue[0].guards") {
		t.Fatalf("expected canonical route array path in error, got %v", err)
	}
}

func TestFactoryConfigMapper_ExpandRejectsRetiredNestedWorkerDefinitionAliases(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
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
	}`)

	_, err := mapper.Expand(raw)
	if err == nil {
		t.Fatal("expected retired nested worker definition alias to be rejected")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workers[0].definition.provider is not supported; use executorProvider") {
		t.Fatalf("expected nested provider retirement guidance, got %v", err)
	}
}

func TestFactoryConfigMapper_ExpandRejectsRetiredTopLevelWorkstationAliases(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"runtimeType":"SCRIPT",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	_, err := mapper.Expand(raw)
	if err == nil {
		t.Fatal("expected retired top-level workstation alias to be rejected")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workstations[0].runtimeType is not supported; use type") {
		t.Fatalf("expected top-level workstation retirement guidance, got %v", err)
	}
}

func TestFactoryConfigMapper_ExpandRejectsRetiredNestedWorkstationDefinitionAliases(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"definition":{"runtimeType":"SCRIPT"},
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	_, err := mapper.Expand(raw)
	if err == nil {
		t.Fatal("expected retired nested workstation definition alias to be rejected")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workstations[0].definition.runtimeType is not supported; use type") {
		t.Fatalf("expected nested workstation retirement guidance, got %v", err)
	}
}

func TestFactoryConfigMapper_ExpandRejectsRetiredNestedWorkstationCronAliases(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"workTypes": [{"name":"task","states":[{"name":"ready","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"daily-refresh",
			"behavior":"CRON",
			"worker":"executor",
			"definition":{
				"cron":{"trigger_at_start":true}
			},
			"outputs":[{"workType":"task","state":"complete"}]
		}]
	}`)

	_, err := mapper.Expand(raw)
	if err == nil {
		t.Fatal("expected retired nested cron alias to be rejected")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workstations[0].definition.cron.trigger_at_start is not supported; use triggerAtStart") {
		t.Fatalf("expected nested cron retirement guidance, got %v", err)
	}
}

func TestFactoryConfigMapper_ExpandRejectsRetiredFanInField(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
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

	_, err := mapper.Expand(raw)
	if err == nil {
		t.Fatal("expected workstation join to be rejected")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workstations[0].join is not supported") {
		t.Fatalf("expected retired join error, got %v", err)
	}
}
