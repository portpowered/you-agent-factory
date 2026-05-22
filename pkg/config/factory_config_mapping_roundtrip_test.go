package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestFactoryConfigMapper_ExpandRejectsRetiredCronIntervalField(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
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

func TestFactoryConfigMapper_ExpandDoesNotRejectDeeperNestedDefinitionAliasesAtGeneratedBoundary(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
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

	_, err := mapper.Expand(raw)
	if err != nil && strings.Contains(err.Error(), "workers[0].definition.definition.provider") {
		t.Fatalf("expected deeper nested definition alias to stay outside generated boundary rejection scope, got %v", err)
	}
}

func TestFactoryConfigMapper_ExpandPreservesPerInputGuard(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"name":"per-input-guard-test",
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
				{"workType":"page","state":"complete","guards":[{"type":"ALL_CHILDREN_COMPLETE","parentInput":"request","spawnedBy":"splitter"}]}
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
		"name":"same-name-guard-test",
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

func TestFactoryConfigMapper_ExpandAndFlattenPreservesSameTraceIDPerInputGuard(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"name":"same-trace-guard-test",
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
				{"workType":"taskItem","state":"ready","guards":[{"type":"SAME_TRACE_ID","matchInput":"planItem"}]}
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
		t.Fatal("expected same-trace guard to be preserved")
	}
	if guard.Type != interfaces.GuardTypeSameTraceID || guard.MatchInput != "planItem" {
		t.Fatalf("unexpected same-trace guard: %#v", guard)
	}
	if guard.ParentInput != "" || guard.SpawnedBy != "" {
		t.Fatalf("expected same-trace guard to keep parent-aware fields empty, got %#v", guard)
	}

	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}

	payload := mustDecodeFactoryPayload(t, flattened)
	workstations := payload["workstations"].([]any)
	inputs := workstations[0].(map[string]any)["inputs"].([]any)
	guardPayload := inputs[1].(map[string]any)["guards"].([]any)[0].(map[string]any)
	if got := guardPayload["type"]; got != "SAME_TRACE_ID" {
		t.Fatalf("expected same-trace guard type, got %#v", got)
	}
	if got := guardPayload["matchInput"]; got != "planItem" {
		t.Fatalf("expected same-trace guard matchInput=planItem, got %#v", got)
	}
	assertMissingKey(t, guardPayload, "parentInput")
	assertMissingKey(t, guardPayload, "spawnedBy")
}

func TestFactoryConfigMapper_ExpandAndFlattenPreservesMatchesFieldsWorkstationGuard(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"name":"matches-fields-guard-test",
		"workTypes": [
			{"name":"asset","states":[{"name":"ready","type":"PROCESSING"},{"name":"matched","type":"TERMINAL"}]}
		],
		"workers": [{"name":"matcher"}],
		"workstations": [{
			"name":"match-assets",
			"worker":"matcher",
			"inputs":[{"workType":"asset","state":"ready"}],
			"outputs":[{"workType":"asset","state":"matched"}],
			"guards":[{"type":"MATCHES_FIELDS","matchConfig":{"inputKey":".Name"}}]
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
		Workers: []interfaces.WorkerConfig{{Name: "executor"}},
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
		"name":"inline-runtime-definitions-test",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{
			"name":"executor",
			"type":"MODEL_WORKER",
			"model":"claude-sonnet-4-20250514",
			"modelProvider":"CLAUDE",
			"stopToken":"COMPLETE",
			"body":"You are the executor."
		}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"type":"MODEL_WORKSTATION",
			"body":"Implement {{ .WorkID }}.",
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
	if got, ok := workstationPayload["body"].(string); !ok || got != "Implement {{ .WorkID }}." {
		t.Fatalf("expected canonical inline workstation export body to preserve prompt template, got %#v", workstationPayload["body"])
	}
	if _, ok := workstationPayload["promptTemplate"]; ok {
		t.Fatalf("expected canonical inline workstation export not to include promptTemplate")
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

func TestFactoryConfigMapper_ExpandParsesCanonicalWorkstationKindAndRuntimeType(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"name":"canonical-workstation-kind-test",
		"workTypes": [{"name":"task","states":[{"name":"ready","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER"}],
		"workstations": [{
			"name":"daily-refresh",
			"behavior":"CRON",
			"type":"MODEL_WORKSTATION",
			"worker":"executor",
			"inputs":[{"workType":"task","state":"ready"}],
			"outputs":[{"workType":"task","state":"complete"}],
			"cron":{"schedule":"*/5 * * * *","triggerAtStart":true},
			"body":"Refresh {{ .WorkID }}."
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

func TestWorkstationConfigToOpenAPI_UsesBodyAsCanonicalExportPromptField(t *testing.T) {
	workstation := interfaces.FactoryWorkstationConfig{
		Name:           "execute-story",
		WorkerTypeName: "executor",
		Type:           interfaces.WorkstationTypeInvoke,
		Operation:      "TTS",
		OperationBindings: []interfaces.ModelOperationBinding{{
			Slot: "text",
			Selector: &interfaces.ModelOperationBindingSelector{
				Label: "utterance",
				Type:  interfaces.ModelOperationContentTypeText,
			},
			Config: []interfaces.WorkContentPart{{
				Type: interfaces.WorkContentPartTypeText,
				Text: "fallback",
				Slot: "text",
			}},
		}},
		Inputs:         []interfaces.IOConfig{{WorkTypeName: "story", StateName: "ready"}},
		Outputs:        []interfaces.IOConfig{{WorkTypeName: "story", StateName: "done"}},
		Body:           "fallback body that should stay private to authored runtime config",
		PromptTemplate: "Implement {{ .WorkID }}.",
	}

	got := WorkstationConfigToOpenAPI(workstation)
	if got.Body == nil || *got.Body != "Implement {{ .WorkID }}." {
		t.Fatalf("expected exported workstation body to carry prompt template, got %#v", got.Body)
	}
	if got.Operation == nil || *got.Operation != "TTS" {
		t.Fatalf("expected exported workstation operation TTS, got %#v", got.Operation)
	}
	if got.OperationBindings == nil || len(*got.OperationBindings) != 1 {
		t.Fatalf("expected one exported operation binding, got %#v", got.OperationBindings)
	}
	binding := (*got.OperationBindings)[0]
	if binding.Slot != "text" {
		t.Fatalf("expected exported binding slot text, got %#v", binding)
	}
	if binding.Selector == nil || binding.Selector.Label == nil || *binding.Selector.Label != "utterance" {
		t.Fatalf("expected exported binding selector label utterance, got %#v", binding.Selector)
	}
	if binding.Config == nil || len(*binding.Config) != 1 {
		t.Fatalf("expected exported config content, got %#v", binding.Config)
	}
}

func TestFactoryConfigMapper_FlattenRoundTripsCopyReferencedScriptsAsCanonicalCamelCase(t *testing.T) {
	mapper := NewFactoryConfigMapper()

	raw := []byte(`{
		"name":"copy-referenced-scripts-test",
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
