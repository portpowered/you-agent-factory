package openapitests

import (
	"encoding/json"
	. "github.com/portpowered/infinite-you/pkg/config"
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
			"body":"Finish {{ .WorkID }}.",
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
	assertCanonicalWorkstationSchema(t, cfg)
}

func TestGeneratedFactoryFromOpenAPIJSON_DecodesCanonicalWorkstationCronFields(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"cron-factory",
		"workTypes": [{"name":"story","states":[{"name":"ready","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor"}],
		"workstations": [{
			"name":"scheduled-story",
			"behavior":"CRON",
			"worker":"executor",
			"outputs":[{"workType":"story","state":"complete"}],
			"cron":{"schedule":"*/5 * * * *","triggerAtStart":true,"jitter":"1s","expiryWindow":"20s"}
		}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	workstation := requireSingleGeneratedWorkstation(t, generated)
	assertGeneratedCronWorkstation(t, workstation)
	assertCanonicalCronJSON(t, generated)
}

func TestFactoryConfigFromOpenAPIJSON_MapsClassifierWorkstationRoutes(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"classifier-factory",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers": [{"name":"classifier","type":"MODEL_WORKER"}],
		"workstations": [{
			"name":"classify-task",
			"type":"CLASSIFIER_WORKSTATION",
			"worker":"classifier",
			"inputs":[{"workType":"task","state":"init"}],
			"classificationRoutes":[
				{"label":"approved","outputs":[{"workType":"task","state":"done"}]},
				{"label":"spam","outputs":[{"workType":"task","state":"failed"}]}
			],
			"onFailure":[{"workType":"task","state":"failed"}]
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	ws := cfg.Workstations[0]
	if ws.Type != interfaces.WorkstationTypeClassify {
		t.Fatalf("expected classifier workstation type, got %#v", ws)
	}
	if len(ws.ClassificationRoutes) != 2 || ws.ClassificationRoutes[1].Label != "spam" {
		t.Fatalf("expected classifier routes to map, got %#v", ws.ClassificationRoutes)
	}

	public := WorkstationConfigToOpenAPI(ws)
	if public.ClassificationRoutes == nil || len(*public.ClassificationRoutes) != 2 {
		t.Fatalf("expected classifier routes to roundtrip to openapi, got %#v", public.ClassificationRoutes)
	}
	if public.Outputs != nil && len(*public.Outputs) != 0 {
		t.Fatalf("expected classifier workstation to keep normal outputs empty, got %#v", public.Outputs)
	}
}

func TestFactoryConfigFromOpenAPIJSON_RejectsNonClassifierWithoutOutputsDuringValidation(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"missing-outputs-factory",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER"}],
		"workstations": [{
			"name":"process-task",
			"type":"MODEL_WORKSTATION",
			"worker":"executor",
			"inputs":[{"workType":"task","state":"init"}],
			"onFailure":[{"workType":"task","state":"failed"}]
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	result := NewConfigValidator().Validate(cfg)
	if !result.HasErrors() {
		t.Fatalf("expected validator to reject missing non-classifier outputs, got %#v", result.Findings)
	}
	if !strings.Contains(result.Error(), "workstation-outputs") {
		t.Fatalf("expected workstation-outputs finding, got %s", result.Error())
	}
}

func TestFactoryConfigFromOpenAPIJSON_AllowsMissingOnFailureWhenSuccessRoutingIsExplicit(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"implicit-failure-factory",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER"}],
		"workstations": [{
			"name":"process-task",
			"type":"MODEL_WORKSTATION",
			"worker":"executor",
			"inputs":[{"workType":"task","state":"init"}],
			"outputs":[{"workType":"task","state":"done"}]
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	result := NewConfigValidator().Validate(cfg)
	if result.HasErrors() {
		t.Fatalf("expected validator to allow omitted onFailure when success routing is explicit, got %#v", result.Findings)
	}
}

func TestFactoryConfigFromOpenAPIJSON_RejectsNonClassifierClassificationRoutesDuringValidation(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"invalid-routes-factory",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER"}],
		"workstations": [{
			"name":"process-task",
			"type":"MODEL_WORKSTATION",
			"worker":"executor",
			"inputs":[{"workType":"task","state":"init"}],
			"outputs":[{"workType":"task","state":"done"}],
			"classificationRoutes":[
				{"label":"approved","outputs":[{"workType":"task","state":"done"}]}
			],
			"onFailure":[{"workType":"task","state":"failed"}]
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	result := NewConfigValidator().Validate(cfg)
	if !result.HasErrors() {
		t.Fatalf("expected validator to reject non-classifier classificationRoutes, got %#v", result.Findings)
	}
	if !strings.Contains(result.Error(), "workstation-classification-routes") {
		t.Fatalf("expected workstation-classification-routes finding, got %s", result.Error())
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
			"body":"Finish {{ .WorkID }}.",
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
	assertGeneratedNestedFactoryBoundary(t, generated)

	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	assertRuntimeNestedFactoryConfig(t, &cfg)
}

func TestGeneratedFactoryFromOpenAPIJSON_DecodesSameNameInputGuard(t *testing.T) {
	assertGeneratedAndRuntimeInputGuardMapping(t, []byte(`{
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
	}`), factoryapi.GuardTypeSameName, interfaces.GuardTypeSameName)
}

func TestGeneratedFactoryFromOpenAPIJSON_DecodesSameTraceIDInputGuard(t *testing.T) {
	assertGeneratedAndRuntimeInputGuardMapping(t, []byte(`{
		"name":"same-trace-input-guard-factory",
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
	}`), factoryapi.GuardTypeSameTraceID, interfaces.GuardTypeSameTraceID)
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

func assertCanonicalWorkstationSchema(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()

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

func requireSingleGeneratedWorkstation(t *testing.T, generated factoryapi.Factory) factoryapi.Workstation {
	t.Helper()

	if generated.Workstations == nil || len(*generated.Workstations) != 1 {
		t.Fatalf("expected one generated workstation, got %#v", generated.Workstations)
	}
	return (*generated.Workstations)[0]
}

func assertGeneratedCronWorkstation(t *testing.T, workstation factoryapi.Workstation) {
	t.Helper()

	if workstation.Cron == nil {
		t.Fatal("expected generated cron to decode")
	}
	if workstation.Cron.Schedule != "*/5 * * * *" {
		t.Fatalf("expected generated cron schedule to decode, got %#v", workstation.Cron)
	}
	if workstation.Cron.TriggerAtStart == nil || !*workstation.Cron.TriggerAtStart {
		t.Fatalf("expected generated cron triggerAtStart=true, got %#v", workstation.Cron.TriggerAtStart)
	}
	if workstation.Cron.Jitter == nil || *workstation.Cron.Jitter != "1s" {
		t.Fatalf("expected generated cron jitter to decode, got %#v", workstation.Cron.Jitter)
	}
	if workstation.Cron.ExpiryWindow == nil || *workstation.Cron.ExpiryWindow != "20s" {
		t.Fatalf("expected generated cron expiryWindow to decode, got %#v", workstation.Cron.ExpiryWindow)
	}
}

func assertCanonicalCronJSON(t *testing.T, generated factoryapi.Factory) {
	t.Helper()

	generatedJSON, err := json.Marshal(generated)
	if err != nil {
		t.Fatalf("marshal generated factory boundary: %v", err)
	}
	serialized := string(generatedJSON)
	for _, field := range []string{`"schedule"`, `"triggerAtStart"`, `"jitter"`, `"expiryWindow"`} {
		if !strings.Contains(serialized, field) {
			t.Fatalf("expected generated boundary JSON to retain canonical cron field %s: %s", field, serialized)
		}
	}
	for _, retiredField := range []string{`"trigger_at_start"`, `"expiry_window"`, `"interval"`} {
		if strings.Contains(serialized, retiredField) {
			t.Fatalf("generated boundary JSON must not include retired cron field %s: %s", retiredField, serialized)
		}
	}
}

func assertGeneratedNestedFactoryBoundary(t *testing.T, generated factoryapi.Factory) {
	t.Helper()

	assertGeneratedNestedFactoryIdentity(t, generated)
	workstation := requireSingleGeneratedWorkstation(t, generated)
	assertGeneratedNestedWorkstationBody(t, workstation)
	assertGeneratedNestedFactoryJSON(t, generated)
	assertGeneratedNestedWorkstationResources(t, workstation)
	assertGeneratedNestedWorkstationGuard(t, workstation)
}

func assertGeneratedNestedFactoryJSON(t *testing.T, generated factoryapi.Factory) {
	t.Helper()

	generatedJSON, err := json.Marshal(generated)
	if err != nil {
		t.Fatalf("marshal generated factory boundary: %v", err)
	}
	var serialized struct {
		Workstations []map[string]any `json:"workstations"`
	}
	if err := json.Unmarshal(generatedJSON, &serialized); err != nil {
		t.Fatalf("unmarshal generated factory boundary JSON: %v", err)
	}
	if len(serialized.Workstations) != 1 {
		t.Fatalf("expected one serialized workstation, got %#v", serialized.Workstations)
	}
	if _, ok := serialized.Workstations[0]["promptTemplate"]; ok {
		t.Fatalf("expected generated workstation JSON to omit promptTemplate, got %#v", serialized.Workstations[0])
	}
	if body, ok := serialized.Workstations[0]["body"].(string); !ok || body != "Finish {{ .WorkID }}." {
		t.Fatalf("expected generated workstation JSON body to stay canonical, got %#v", serialized.Workstations[0])
	}
}

func assertGeneratedNestedFactoryIdentity(t *testing.T, generated factoryapi.Factory) {
	t.Helper()

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
}

func assertGeneratedNestedWorkstationBody(t *testing.T, workstation factoryapi.Workstation) {
	t.Helper()

	if workstation.Body == nil || *workstation.Body != "Finish {{ .WorkID }}." {
		t.Fatalf("expected generated body to survive boundary decode, got %#v", workstation.Body)
	}
}

func assertGeneratedNestedWorkstationResources(t *testing.T, workstation factoryapi.Workstation) {
	t.Helper()

	if workstation.Resources == nil || len(*workstation.Resources) != 1 || (*workstation.Resources)[0].Capacity != 2 {
		t.Fatalf("expected generated resources capacity 2, got %#v", workstation.Resources)
	}
}

func assertGeneratedNestedWorkstationGuard(t *testing.T, workstation factoryapi.Workstation) {
	t.Helper()

	if len(workstation.Inputs) != 2 || workstation.Inputs[1].Guards == nil || len(*workstation.Inputs[1].Guards) != 1 {
		t.Fatalf("expected generated nested guards to survive boundary decode, got %#v", workstation.Inputs)
	}
	guard := (*workstation.Inputs[1].Guards)[0]
	if guard.ParentInput == nil || *guard.ParentInput != "chapter" || guard.SpawnedBy == nil || *guard.SpawnedBy != "chapter-parser" {
		t.Fatalf("expected generated guard camelCase fields to survive boundary decode, got %#v", guard)
	}
}

func assertRuntimeNestedFactoryConfig(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()

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

func assertGeneratedAndRuntimeInputGuardMapping(t *testing.T, cfgJSON []byte, generatedType factoryapi.GuardType, runtimeType interfaces.GuardType) {
	t.Helper()

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	guard := requireGeneratedInputGuard(t, generated)
	if guard.Type != generatedType {
		t.Fatalf("expected generated guard type %s, got %#v", generatedType, guard.Type)
	}
	if guard.MatchInput == nil || *guard.MatchInput != "planItem" {
		t.Fatalf("expected generated guard matchInput planItem, got %#v", guard.MatchInput)
	}
	if guard.ParentInput != nil || guard.SpawnedBy != nil {
		t.Fatalf("expected generated guard to keep parent-aware fields unset, got %#v", guard)
	}

	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	runtimeGuard := cfg.Workstations[0].Inputs[1].Guard
	if runtimeGuard == nil {
		t.Fatal("expected runtime guard to survive generated mapping")
	}
	if runtimeGuard.Type != runtimeType || runtimeGuard.MatchInput != "planItem" {
		t.Fatalf("expected runtime guard fields to match generated boundary, got %#v", runtimeGuard)
	}
	if runtimeGuard.ParentInput != "" || runtimeGuard.SpawnedBy != "" {
		t.Fatalf("expected runtime guard to keep parent-aware fields empty, got %#v", runtimeGuard)
	}
}

func requireGeneratedInputGuard(t *testing.T, generated factoryapi.Factory) factoryapi.Guard {
	t.Helper()

	workstation := requireSingleGeneratedWorkstation(t, generated)
	if len(workstation.Inputs) != 2 || workstation.Inputs[1].Guards == nil || len(*workstation.Inputs[1].Guards) != 1 {
		t.Fatalf("expected generated input guard to survive boundary decode, got %#v", workstation.Inputs)
	}
	return (*workstation.Inputs[1].Guards)[0]
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
