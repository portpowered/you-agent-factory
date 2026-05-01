package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/agent-factory/pkg/interfaces"
)

// portos:func-length-exception owner=agent-factory reason=legacy-config-roundtrip-fixture review=2026-07-18 removal=split-roundtrip-assertions-before-next-config-schema-change
func TestFactoryConfigMapper_FlattenAndExpandPreservesConfigContent(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	original := &interfaces.FactoryConfig{
		Project: "sample-service",
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

	if expanded.Project != original.Project {
		t.Fatalf("expected project %q, got %q", original.Project, expanded.Project)
	}

	if len(expanded.WorkTypes) != len(original.WorkTypes) {
		t.Fatalf("expected %d work types, got %d", len(original.WorkTypes), len(expanded.WorkTypes))
	}

	if expanded.Workstations[0].Kind != original.Workstations[0].Kind {
		t.Fatalf("expected workstation kind %q, got %q", original.Workstations[0].Kind, expanded.Workstations[0].Kind)
	}

	if expanded.Workstations[0].ID != original.Workstations[0].ID {
		t.Fatalf("expected workstation id %q, got %q", original.Workstations[0].ID, expanded.Workstations[0].ID)
	}

	if expanded.Workstations[0].Resources[0].Capacity != original.Workstations[0].Resources[0].Capacity {
		t.Fatalf("expected resource capacity %d, got %d", original.Workstations[0].Resources[0].Capacity, expanded.Workstations[0].Resources[0].Capacity)
	}
	if len(expanded.Workstations[0].Guards) != 1 {
		t.Fatalf("expected one workstation guard, got %#v", expanded.Workstations[0].Guards)
	}
	if expanded.Workstations[0].Guards[0].Type != interfaces.GuardTypeVisitCount {
		t.Fatalf("expected visit_count guard, got %#v", expanded.Workstations[0].Guards[0])
	}
	if expanded.Workstations[0].Guards[0].Workstation != "review-story" || expanded.Workstations[0].Guards[0].MaxVisits != 3 {
		t.Fatalf("expected visit_count guard details to roundtrip, got %#v", expanded.Workstations[0].Guards[0])
	}
}

func TestFactoryConfigMapper_ExpandSupportsCanonicalBoundaryKeysAndCapacity(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"id": "analytics-platform",
		"inputTypes": [{"name":"default","type":"DEFAULT"}],
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"resources": [{"name":"agent-slot","capacity":2}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"id":"execute-story-id",
			"name":"execute-story",
			"kind":"CRON",
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

	if len(cfg.WorkTypes) != 1 || cfg.WorkTypes[0].Name != "story" {
		t.Fatalf("expected one parsed work type named story, got %#v", cfg.WorkTypes)
	}
	if cfg.Project != "analytics-platform" {
		t.Fatalf("expected id analytics-platform to map to project, got %q", cfg.Project)
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

	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}

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
			"kind":"repeater",
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
		"inputTypes": [{"name":"default","type":"default"}],
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
			"kind":"CRON",
			"type":"MODEL_WORKSTATION",
			"worker":"executor",
			"promptFile":"prompt.md",
			"promptTemplate":"Implement {{ .WorkID }}.",
			"outputSchema":"schema.json",
			"onRejection":{"workType":"story","state":"init"},
			"onFailure":{"workType":"story","state":"failed"},
			"resources":[{"name":"agent-slot","capacity":2}],
			"stopWords":["DONE"],
			"workingDirectory":"/repo/{{ .WorkID }}",
			"cron":{"schedule":"*/10 * * * *","triggerAtStart":true,"expiryWindow":"20s"},
			"inputs":[
				{"workType":"story","state":"init"},
				{"workType":"story","state":"complete","guards":[{"type":"all_children_complete","parentInput":"story","spawnedBy":"fanout"}]}
			],
			"outputs":[{"workType":"story","state":"complete"}],
			"guards":[{"type":"visit_count","workstation":"execute-story","maxVisits":3}]
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
			"kind":"cron",
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

func mustDecodeFactoryPayload(t *testing.T, flattened []byte) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(flattened, &payload); err != nil {
		t.Fatalf("unmarshal flattened payload: %v", err)
	}
	return payload
}

func assertNoRetiredExhaustionRulesPayload(t *testing.T, payload map[string]any) {
	t.Helper()

	if _, ok := payload["exhaustionRules"]; ok {
		t.Fatalf("expected canonical payload not to advertise exhaustionRules, got %#v", payload["exhaustionRules"])
	}
	if _, ok := payload["exhaustion_rules"]; ok {
		t.Fatalf("expected canonical payload not to advertise exhaustion_rules, got %#v", payload["exhaustion_rules"])
	}
}

func assertLoopBreakerPayload(t *testing.T, payload map[string]any, name string, watchedWorkstation string, maxVisits int) {
	t.Helper()

	workstations, ok := payload["workstations"].([]any)
	if !ok || len(workstations) != 1 {
		t.Fatalf("expected one guarded loop breaker workstation, got %#v", payload["workstations"])
	}

	loopBreaker := findPayloadWorkstationByName(workstations, name)
	if loopBreaker == nil {
		t.Fatalf("expected guarded loop breaker workstation %q in %#v", name, workstations)
	}
	if got := loopBreaker["type"]; got != interfaces.WorkstationTypeLogical {
		t.Fatalf("loop breaker type = %#v, want %q", got, interfaces.WorkstationTypeLogical)
	}
	guards, ok := loopBreaker["guards"].([]any)
	if !ok || len(guards) != 1 {
		t.Fatalf("expected one loop breaker guard, got %#v", loopBreaker["guards"])
	}
	guard := guards[0].(map[string]any)
	if got := guard["type"]; got != "VISIT_COUNT" {
		t.Fatalf("guard type = %#v, want %q", got, "VISIT_COUNT")
	}
	if got := guard["workstation"]; got != watchedWorkstation {
		t.Fatalf("guard workstation = %#v, want %s", got, watchedWorkstation)
	}
	if got := guard["maxVisits"]; got != float64(maxVisits) {
		t.Fatalf("guard maxVisits = %#v, want %d", got, maxVisits)
	}
}

func findPayloadWorkstationByName(workstations []any, name string) map[string]any {
	for _, item := range workstations {
		workstation, ok := item.(map[string]any)
		if ok && workstation["name"] == name {
			return workstation
		}
	}
	return nil
}

func assertExpandedLoopBreaker(t *testing.T, cfg *interfaces.FactoryConfig, name string, watchedWorkstation string, maxVisits int) {
	t.Helper()

	if len(cfg.Workstations) != 1 {
		t.Fatalf("expected 1 workstation after expand, got %#v", cfg.Workstations)
	}

	var loopBreaker *interfaces.FactoryWorkstationConfig
	for i := range cfg.Workstations {
		if cfg.Workstations[i].Name == name {
			loopBreaker = &cfg.Workstations[i]
			break
		}
	}
	if loopBreaker == nil {
		t.Fatalf("expected expanded loop breaker workstation %q in %#v", name, cfg.Workstations)
	}
	if loopBreaker.Type != interfaces.WorkstationTypeLogical {
		t.Fatalf("expanded loop breaker type = %q, want %q", loopBreaker.Type, interfaces.WorkstationTypeLogical)
	}
	if len(loopBreaker.Guards) != 1 {
		t.Fatalf("expected expanded loop breaker to retain one guard, got %#v", loopBreaker.Guards)
	}
	if loopBreaker.Guards[0].Workstation != watchedWorkstation || loopBreaker.Guards[0].MaxVisits != maxVisits {
		t.Fatalf("expanded loop breaker guard = %#v, want visit_count on %s max %d", loopBreaker.Guards[0], watchedWorkstation, maxVisits)
	}
}

func TestFactoryConfigMapper_ExpandRejectsRetiredCronIntervalField(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"workTypes": [{"name":"task","states":[{"name":"ready","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"daily-refresh",
			"kind":"cron",
			"worker":"executor",
			"outputs":[{"workType":"task","state":"complete"}],
			"cron":{"interval":"5m"}
		}]
	}`)

	_, err := mapper.Expand(raw)
	if err == nil {
		t.Fatal("expected retired cron interval to be rejected")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workstations[0].cron.interval is not supported; use cron.schedule") {
		t.Fatalf("expected retired cron interval error, got %v", err)
	}
}

func TestFactoryConfigMapper_ExpandRejectsUnsupportedGeneratedBoundaryField(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"unsupported_field": true
		}]
	}`)

	_, err := mapper.Expand(raw)
	if err == nil {
		t.Fatal("expected unsupported workstation field to be rejected")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), `json: unknown field "unsupported_field"`) {
		t.Fatalf("expected generated boundary unknown-field error, got %v", err)
	}
}

func TestFactoryConfigMapper_ExpandPreservesPerInputGuard(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"workTypes": [
			{"name":"request","states":[{"name":"waiting","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"}]},
			{"name":"page","states":[{"name":"complete","type":"TERMINAL"}]}
		],
		"workers": [{"name":"collect-worker"}],
		"workstations": [{
			"name":"collector",
			"worker":"collect-worker",
			"inputs":[
				{"workType":"request","state":"waiting"},
				{"workType":"page","state":"complete","guards":[{"type":"all_children_complete","parentInput":"request","spawnedBy":"splitter"}]}
			],
			"outputs":[{"workType":"request","state":"complete"}]
		}]
	}`)

	cfg, err := mapper.Expand(raw)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}

	guard := cfg.Workstations[0].Inputs[1].Guard
	if guard == nil {
		t.Fatal("expected per-input guard to be preserved")
	}
	if guard.Type != interfaces.GuardTypeAllChildrenComplete || guard.ParentInput != "request" || guard.SpawnedBy != "splitter" {
		t.Fatalf("unexpected per-input guard: %#v", guard)
	}
}

func TestFactoryConfigMapper_ExpandAndFlattenPreservesSameNamePerInputGuard(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
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
				{"workType":"taskItem","state":"ready","guards":[{"type":"same_name","matchInput":"planItem"}]}
			],
			"outputs":[{"workType":"taskItem","state":"matched"}]
		}]
	}`)

	cfg, err := mapper.Expand(raw)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}

	guard := cfg.Workstations[0].Inputs[1].Guard
	if guard == nil {
		t.Fatal("expected same-name guard to be preserved")
	}
	if guard.Type != interfaces.GuardTypeSameName || guard.MatchInput != "planItem" {
		t.Fatalf("unexpected same-name guard: %#v", guard)
	}
	if guard.ParentInput != "" || guard.SpawnedBy != "" {
		t.Fatalf("expected same-name guard to keep parent-aware fields empty, got %#v", guard)
	}

	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}

	payload := mustDecodeFactoryPayload(t, flattened)
	workstations := payload["workstations"].([]any)
	inputs := workstations[0].(map[string]any)["inputs"].([]any)
	guardPayload := inputs[1].(map[string]any)["guards"].([]any)[0].(map[string]any)
	if got := guardPayload["type"]; got != "SAME_NAME" {
		t.Fatalf("expected same-name guard type, got %#v", got)
	}
	if got := guardPayload["matchInput"]; got != "planItem" {
		t.Fatalf("expected same-name guard matchInput=planItem, got %#v", got)
	}
	assertMissingKey(t, guardPayload, "parentInput")
	assertMissingKey(t, guardPayload, "spawnedBy")
}

func TestFactoryConfigMapper_ExpandAndFlattenPreservesMatchesFieldsWorkstationGuard(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"workTypes": [
			{"name":"asset","states":[{"name":"ready","type":"PROCESSING"},{"name":"matched","type":"TERMINAL"}]}
		],
		"workers": [{"name":"matcher"}],
		"workstations": [{
			"name":"match-assets",
			"worker":"matcher",
			"inputs":[{"workType":"asset","state":"ready"}],
			"outputs":[{"workType":"asset","state":"matched"}],
			"guards":[{"type":"matches_fields","matchConfig":{"inputKey":".Name"}}]
		}]
	}`)

	cfg, err := mapper.Expand(raw)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}

	if len(cfg.Workstations[0].Guards) != 1 {
		t.Fatalf("expected matches-fields guard to be preserved, got %#v", cfg.Workstations[0].Guards)
	}
	guard := cfg.Workstations[0].Guards[0]
	if guard.Type != interfaces.GuardTypeMatchesFields {
		t.Fatalf("expected matches-fields guard type, got %#v", guard)
	}
	if guard.MatchConfig == nil || guard.MatchConfig.InputKey != ".Name" {
		t.Fatalf("expected matches-fields guard matchConfig.inputKey, got %#v", guard.MatchConfig)
	}

	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}

	payload := mustDecodeFactoryPayload(t, flattened)
	workstations := payload["workstations"].([]any)
	guardPayload := workstations[0].(map[string]any)["guards"].([]any)[0].(map[string]any)
	if got := guardPayload["type"]; got != "MATCHES_FIELDS" {
		t.Fatalf("expected matches-fields guard type, got %#v", got)
	}
	matchConfig, ok := guardPayload["matchConfig"].(map[string]any)
	if !ok {
		t.Fatalf("expected matchConfig object, got %#v", guardPayload["matchConfig"])
	}
	if got := matchConfig["inputKey"]; got != ".Name" {
		t.Fatalf("expected matchConfig.inputKey=.Name, got %#v", got)
	}
}

func TestFactoryConfigMapper_FlattenOmitsUnsetWorkerTypeFields(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.WorkerConfig{{
			Name: "executor",
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "process-task",
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
		}},
	}

	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}

	payload := mustDecodeFactoryPayload(t, flattened)
	workers := payload["workers"].([]any)
	worker := workers[0].(map[string]any)
	if _, ok := worker["type"]; ok {
		t.Fatalf("expected canonical worker payload to omit unset type, got %#v", worker)
	}
}

func TestFactoryConfigMapper_FlattenAndExpandPreservesInlineRuntimeDefinitions(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{
			"name":"executor",
			"type":"MODEL_WORKER",
			"model":"claude-sonnet-4-20250514",
			"modelProvider":"claude",
			"stopToken":"COMPLETE",
			"body":"You are the executor."
		}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"type":"MODEL_WORKSTATION",
			"promptTemplate":"Implement {{ .WorkID }}.",
			"stopWords":["DONE"]
		}]
	}`)

	cfg, err := mapper.Expand(raw)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}
	if cfg.Workers[0].ModelProvider != "claude" {
		t.Fatalf("expected model provider claude, got %q", cfg.Workers[0].ModelProvider)
	}
	if cfg.Workers[0].StopToken != "COMPLETE" {
		t.Fatalf("expected stop token COMPLETE, got %q", cfg.Workers[0].StopToken)
	}
	if cfg.Workstations[0].Type == "" {
		t.Fatal("expected workstation runtime config to be preserved")
	}
	if cfg.Workstations[0].PromptTemplate != "Implement {{ .WorkID }}." {
		t.Fatalf("expected prompt template to round-trip, got %q", cfg.Workstations[0].PromptTemplate)
	}

	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(flattened, &payload); err != nil {
		t.Fatalf("unmarshal flattened payload: %v", err)
	}
	workersPayload := payload["workers"].([]any)
	workerPayload := workersPayload[0].(map[string]any)
	if _, ok := workerPayload["modelProvider"]; !ok {
		t.Fatalf("expected canonical inline worker definition to use modelProvider key")
	}
	if _, ok := workerPayload["model_provider"]; ok {
		t.Fatalf("expected canonical inline worker definition not to include model_provider key")
	}
	workstationsPayload := payload["workstations"].([]any)
	workstationPayload := workstationsPayload[0].(map[string]any)
	if _, ok := workstationPayload["promptTemplate"]; !ok {
		t.Fatalf("expected canonical inline workstation runtime config to use promptTemplate key")
	}
	if _, ok := workstationPayload["prompt_template"]; ok {
		t.Fatalf("expected canonical inline workstation runtime config not to include prompt_template key")
	}
	if _, ok := workstationPayload["definition"]; ok {
		t.Fatalf("expected canonical inline workstation runtime config to be flattened")
	}
	if _, ok := workstationPayload["runtimeType"]; ok {
		t.Fatalf("expected canonical inline workstation runtime config not to use runtimeType")
	}
	if got, ok := workstationPayload["type"].(string); !ok || got != "MODEL_WORKSTATION" {
		t.Fatalf("expected canonical inline workstation runtime type, got %#v", workstationPayload["type"])
	}
}

func TestFactoryConfigMapper_FlattenAndExpandPreservesPortableResourceManifest(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	cfg := portableResourceManifestMapperFixture()

	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}

	payload := mustDecodeFactoryPayload(t, flattened)
	assertFlattenedPortableResourceManifestPayload(t, payload)
	assertMissingKey(t, payload, "resource_manifest")

	expanded, err := mapper.Expand(flattened)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}
	assertExpandedPortableResourceManifest(t, expanded)
}

func portableResourceManifestMapperFixture() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "story",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "executor"}},
		ResourceManifest: &interfaces.PortableResourceManifestConfig{
			RequiredTools: []interfaces.RequiredToolConfig{{
				Name:        "python",
				Command:     "python",
				Purpose:     "Runs portable helper scripts",
				VersionArgs: []string{"--version"},
			}},
			BundledFiles: []interfaces.BundledFileConfig{{
				Type:       "SCRIPT",
				TargetPath: "factory/scripts/setup-workspace.py",
				Content: interfaces.BundledFileContentConfig{
					Encoding: "utf-8",
					Inline:   "print('portable')\n",
				},
			}, {
				Type:       "ROOT_HELPER",
				TargetPath: "Makefile",
				Content: interfaces.BundledFileContentConfig{
					Encoding: "utf-8",
					Inline:   "test:\n\tgo test ./...\n",
				},
			}, {
				Type:       "DOC",
				TargetPath: "factory/docs/usage.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: "utf-8",
					Inline:   "# Usage\n",
				},
			}},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "execute-story",
			WorkerTypeName: "executor",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "init"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "complete"}},
		}},
	}
}

func assertFlattenedPortableResourceManifestPayload(t *testing.T, payload map[string]any) {
	t.Helper()

	resourceManifest, ok := payload["supportingFiles"].(map[string]any)
	if !ok {
		t.Fatalf("expected canonical payload to include supportingFiles, got %#v", payload["supportingFiles"])
	}
	requiredTools, ok := resourceManifest["requiredTools"].([]any)
	if !ok || len(requiredTools) != 1 {
		t.Fatalf("expected one required tool, got %#v", resourceManifest["requiredTools"])
	}
	requiredTool := requiredTools[0].(map[string]any)
	if got := requiredTool["command"]; got != "python" {
		t.Fatalf("required tool command = %#v, want %q", got, "python")
	}
	if got := requiredTool["purpose"]; got != "Runs portable helper scripts" {
		t.Fatalf("required tool purpose = %#v", got)
	}
	versionArgs, ok := requiredTool["versionArgs"].([]any)
	if !ok || len(versionArgs) != 1 || versionArgs[0] != "--version" {
		t.Fatalf("required tool versionArgs = %#v", requiredTool["versionArgs"])
	}

	bundledFiles, ok := resourceManifest["bundledFiles"].([]any)
	if !ok || len(bundledFiles) != 3 {
		t.Fatalf("expected three bundled files, got %#v", resourceManifest["bundledFiles"])
	}
	assertBundledFilePayload(t, bundledFiles[0].(map[string]any), "ROOT_HELPER", "Makefile", "test:\n\tgo test ./...\n")
	assertBundledFilePayload(t, bundledFiles[1].(map[string]any), "DOC", "factory/docs/usage.md", "# Usage\n")
	assertBundledFilePayload(t, bundledFiles[2].(map[string]any), "SCRIPT", "factory/scripts/setup-workspace.py", "print('portable')\n")
}

func assertBundledFilePayload(t *testing.T, payload map[string]any, wantType, wantTargetPath string, wantInline string) {
	t.Helper()

	if got := payload["type"]; got != wantType {
		t.Fatalf("bundled file type = %#v, want %q", got, wantType)
	}
	if got := payload["targetPath"]; got != wantTargetPath {
		t.Fatalf("bundled file targetPath = %#v, want %q", got, wantTargetPath)
	}
	content, ok := payload["content"].(map[string]any)
	if !ok {
		t.Fatalf("expected bundled file content object, got %#v", payload["content"])
	}
	if got := content["encoding"]; got != "utf-8" {
		t.Fatalf("bundled file encoding = %#v", got)
	}
	if got := content["inline"]; got != wantInline {
		t.Fatalf("bundled file inline = %#v, want %q", got, wantInline)
	}
}

func assertExpandedPortableResourceManifest(t *testing.T, expanded *interfaces.FactoryConfig) {
	t.Helper()

	if expanded.ResourceManifest == nil {
		t.Fatal("expected resourceManifest to round-trip")
	}
	if len(expanded.ResourceManifest.RequiredTools) != 1 {
		t.Fatalf("expected one required tool after expand, got %#v", expanded.ResourceManifest.RequiredTools)
	}
	if expanded.ResourceManifest.RequiredTools[0].Purpose != "Runs portable helper scripts" {
		t.Fatalf("required tool purpose after expand = %#v", expanded.ResourceManifest.RequiredTools[0])
	}
	if len(expanded.ResourceManifest.BundledFiles) != 3 {
		t.Fatalf("expected three bundled files after expand, got %#v", expanded.ResourceManifest.BundledFiles)
	}
	if expanded.ResourceManifest.BundledFiles[0].TargetPath != "Makefile" || expanded.ResourceManifest.BundledFiles[0].Content.Inline != "test:\n\tgo test ./...\n" {
		t.Fatalf("bundled root helper after expand = %#v", expanded.ResourceManifest.BundledFiles[0])
	}
	if expanded.ResourceManifest.BundledFiles[1].Content.Inline != "# Usage\n" {
		t.Fatalf("bundled doc inline after expand = %#v", expanded.ResourceManifest.BundledFiles[1])
	}
	if expanded.ResourceManifest.BundledFiles[2].Content.Inline != "print('portable')\n" {
		t.Fatalf("bundled script inline after expand = %#v", expanded.ResourceManifest.BundledFiles[2])
	}
}

func TestFactoryConfigMapper_ExpandParsesCanonicalWorkstationKindAndRuntimeType(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"workTypes": [{"name":"task","states":[{"name":"ready","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER"}],
		"workstations": [{
			"name":"daily-refresh",
			"kind":"cron",
			"type":"MODEL_WORKSTATION",
			"worker":"executor",
			"inputs":[{"workType":"task","state":"ready"}],
			"outputs":[{"workType":"task","state":"complete"}],
			"cron":{"schedule":"*/5 * * * *","triggerAtStart":true},
			"promptTemplate":"Refresh {{ .WorkID }}."
		}]
	}`)

	cfg, err := mapper.Expand(raw)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}

	ws := cfg.Workstations[0]
	if ws.Kind != interfaces.WorkstationKindCron {
		t.Fatalf("expected workstation kind cron, got %q", ws.Kind)
	}
	if ws.Type != interfaces.WorkstationTypeModel {
		t.Fatalf("expected runtime workstation type MODEL_WORKSTATION, got %q", ws.Type)
	}
	if ws.Cron == nil || ws.Cron.Schedule != "*/5 * * * *" || !ws.Cron.TriggerAtStart {
		t.Fatalf("expected cron schedule and startup trigger to be retained, got %#v", ws.Cron)
	}
	if ws.PromptTemplate != "Refresh {{ .WorkID }}." {
		t.Fatalf("expected prompt template to be retained, got %q", ws.PromptTemplate)
	}
}

func TestFactoryConfigMapper_FlattenRoundTripsCopyReferencedScriptsAsCanonicalCamelCase(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
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

	cfg, err := mapper.Expand(raw)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}
	if !cfg.Workstations[0].CopyReferencedScripts {
		t.Fatalf("expected execute-story copyReferencedScripts=true, got %#v", cfg.Workstations[0])
	}
	if cfg.Workstations[1].CopyReferencedScripts {
		t.Fatalf("expected omitted copyReferencedScripts to default false, got %#v", cfg.Workstations[1])
	}

	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}

	payload := mustDecodeFactoryPayload(t, flattened)
	workstations, ok := payload["workstations"].([]any)
	if !ok || len(workstations) != 2 {
		t.Fatalf("expected two workstations in canonical payload, got %#v", payload["workstations"])
	}

	executeStory := findPayloadWorkstationByName(workstations, "execute-story")
	if executeStory == nil {
		t.Fatalf("expected execute-story workstation in %#v", workstations)
	}
	if got, ok := executeStory["copyReferencedScripts"].(bool); !ok || !got {
		t.Fatalf("expected canonical copyReferencedScripts=true, got %#v", executeStory["copyReferencedScripts"])
	}
	assertMissingKey(t, executeStory, "copy_referenced_scripts")

	reviewStory := findPayloadWorkstationByName(workstations, "review-story")
	if reviewStory == nil {
		t.Fatalf("expected review-story workstation in %#v", workstations)
	}
	assertMissingKey(t, reviewStory, "copyReferencedScripts")
	assertMissingKey(t, reviewStory, "copy_referenced_scripts")
}

func assertMissingKey(t *testing.T, payload map[string]any, key string) {
	t.Helper()
	if _, ok := payload[key]; ok {
		t.Fatalf("did not expect key %q in %#v", key, payload)
	}
}
