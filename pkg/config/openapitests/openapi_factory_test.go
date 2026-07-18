package openapitests

import (
	"encoding/json"
	"strings"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const generatedFactoryBoundaryErrorPrefix = "decode factory generated-schema boundary"

func assertFindingMatch(t *testing.T, findings []Finding, rule string, pathSubstring string, messageSubstring string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Rule != rule || finding.Severity != SeverityError {
			continue
		}
		if !strings.Contains(finding.Path, pathSubstring) {
			t.Fatalf("finding path = %q, want substring %q", finding.Path, pathSubstring)
		}
		if !strings.Contains(finding.Message, messageSubstring) {
			t.Fatalf("finding message = %q, want substring %q", finding.Message, messageSubstring)
		}
		return
	}
	t.Fatalf("expected error finding with rule %q, got %v", rule, findings)
}

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

func TestFactoryConfigFromOpenAPIJSON_MapsOptionalGraphableEntityIDs(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"stable-id-factory",
		"workTypes": [
			{"id":"work-type-story","name":"story","states":[
				{"id":"state-ready","name":"ready","type":"INITIAL"},
				{"id":"state-done","name":"done","type":"TERMINAL"}
			]}
		],
		"resources": [{"id":"resource-agent-slot","name":"agent-slot","capacity":2}],
		"workers": [{"id":"worker-executor","name":"executor","type":"MODEL_WORKER"}],
		"workstations": [{
			"id":"workstation-execute-story",
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"ready"}],
			"outputs":[{"workType":"story","state":"done"}]
		}]
	}`)
	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	assertInternalGraphableEntityIDs(t, cfg)
	assertPublicGraphableEntityIDs(t, cfg)
	assertCanonicalGraphableEntityIDs(t, cfg)
}

// pkgmaintcheck:ignore-cyclomatic-complexity this contract test keeps internal, public, and canonical layout mapping assertions together on the same seam.
func TestFactoryConfigFromOpenAPIJSON_MapsPortableLayoutContract(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"layout-factory",
		"layout":{
			"schemaVersion":1,
			"nodes":[{"id":"workstation:review","position":{"x":420,"y":180},"size":{"width":156,"height":196},"locked":false,"emptyState":{"text":"No review activity yet."}}],
			"edges":[{"id":"workstation-output:workstation:review->work-state:task:done","waypoints":[{"x":540,"y":220}],"labelPosition":{"x":590,"y":204}}],
			"groups":[{"id":"review-lane","label":"Review","bounds":{"x":360,"y":120,"width":520,"height":360},"nodeIds":["workstation:review"],"parentGroupId":null,"color":"blue","locked":false}],
			"annotations":[
				{"id":"review-note","kind":"NOTE","position":{"x":240,"y":96},"note":{"title":"Review guidance","body":"Inspect the result before approving.\nKeep this note literal.","tone":"INFO"}},
				{"id":"review-image","kind":"IMAGE","position":{"x":820,"y":96},"size":{"width":180,"height":120},"image":{"source":{"kind":"EMBEDDED","mediaType":"image/png","data":"AQID"},"alternativeText":"Review workflow illustration"}}
			],
			"viewport":{"x":0,"y":0,"zoom":1},
			"preferences":{"direction":"RIGHT"}
		},
		"workTypes": [{"id":"task","name":"task","states":[{"id":"ready","name":"ready","type":"INITIAL"},{"id":"done","name":"done","type":"TERMINAL"}]}],
		"workers": [{"id":"writer","name":"writer","type":"MODEL_WORKER"}],
		"workstations": [{
			"id":"review",
			"name":"review",
			"worker":"writer",
			"inputs":[{"workType":"task","state":"ready"}],
			"outputs":[{"workType":"task","state":"done"}]
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Layout == nil {
		t.Fatal("expected layout to map into internal config")
	}
	if cfg.Layout.SchemaVersion != 1 || cfg.Layout.Nodes[0].ID != "workstation:review" {
		t.Fatalf("unexpected internal layout: %#v", cfg.Layout)
	}
	if cfg.Layout.Nodes[0].Locked == nil || *cfg.Layout.Nodes[0].Locked {
		t.Fatalf("expected node locked=false to roundtrip, got %#v", cfg.Layout.Nodes[0].Locked)
	}
	if cfg.Layout.Nodes[0].EmptyState == nil || cfg.Layout.Nodes[0].EmptyState.Text != "No review activity yet." {
		t.Fatalf("expected literal node empty state to roundtrip, got %#v", cfg.Layout.Nodes[0].EmptyState)
	}
	if cfg.Layout.Groups[0].ParentGroupID != nil {
		t.Fatalf("expected parentGroupId null to remain nil, got %#v", cfg.Layout.Groups[0].ParentGroupID)
	}
	if len(cfg.Layout.Annotations) != 2 || cfg.Layout.Annotations[0].Note == nil || cfg.Layout.Annotations[1].Image == nil {
		t.Fatalf("layout annotations = %#v, want one note and one image", cfg.Layout.Annotations)
	}
	if cfg.Layout.Annotations[0].Note.Body != "Inspect the result before approving.\nKeep this note literal." {
		t.Fatalf("note body = %q, want literal line break preserved", cfg.Layout.Annotations[0].Note.Body)
	}
	if cfg.Layout.Annotations[1].Image.Source.MediaType != "image/png" || cfg.Layout.Annotations[1].Image.AlternativeText != "Review workflow illustration" {
		t.Fatalf("image annotation = %#v", cfg.Layout.Annotations[1].Image)
	}
	public := FactoryConfigToOpenAPI(cfg)
	if public.Layout == nil || public.Layout.Preferences == nil || public.Layout.Preferences.Direction == nil || *public.Layout.Preferences.Direction != factoryapi.RIGHT {
		t.Fatalf("expected public layout preferences direction RIGHT, got %#v", public.Layout)
	}
	if public.Layout.Annotations == nil || len(*public.Layout.Annotations) != 2 || (*public.Layout.Annotations)[1].Image == nil {
		t.Fatalf("expected annotation public roundtrip, got %#v", public.Layout.Annotations)
	}
	if (*public.Layout.Nodes)[0].EmptyState == nil || (*public.Layout.Nodes)[0].EmptyState.Text == nil || *(*public.Layout.Nodes)[0].EmptyState.Text != "No review activity yet." {
		t.Fatalf("expected node empty state public roundtrip, got %#v", (*public.Layout.Nodes)[0].EmptyState)
	}
	canonical, err := MarshalCanonicalFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(canonical, &decoded); err != nil {
		t.Fatalf("unmarshal canonical config: %v", err)
	}
	layout := decoded["layout"].(map[string]any)
	if layout["schemaVersion"] != float64(1) {
		t.Fatalf("canonical schemaVersion = %#v", layout["schemaVersion"])
	}
	annotations := layout["annotations"].([]any)
	if annotations[0].(map[string]any)["kind"] != "NOTE" || annotations[1].(map[string]any)["kind"] != "IMAGE" {
		t.Fatalf("canonical annotations = %#v", annotations)
	}
	nodes := layout["nodes"].([]any)
	if nodes[0].(map[string]any)["emptyState"].(map[string]any)["text"] != "No review activity yet." {
		t.Fatalf("canonical node emptyState = %#v", nodes[0])
	}
	viewport := layout["viewport"].(map[string]any)
	if viewport["zoom"] != float64(1) {
		t.Fatalf("canonical viewport.zoom = %#v", viewport["zoom"])
	}
}
func TestFactoryConfigFromOpenAPIJSON_AllowsPortableLayoutNodesWithoutSize(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"layout-factory",
		"layout":{
			"schemaVersion":1,
			"nodes":[
				{"id":"workstation:review","position":{"x":420,"y":180}},
				{"id":"workstation:approve","position":{"x":620,"y":220}}
			],
			"edges":[{"id":"workstation-output:workstation:review->work-state:task:done","waypoints":[{"x":540,"y":220},{"x":580,"y":240}]}],
			"viewport":{"x":0,"y":0,"zoom":1}
		},
		"workTypes": [{"id":"task","name":"task","states":[{"id":"ready","name":"ready","type":"INITIAL"},{"id":"done","name":"done","type":"TERMINAL"}]}],
		"workers": [{"id":"writer","name":"writer","type":"MODEL_WORKER"}],
		"workstations": [{
			"id":"review",
			"name":"review",
			"worker":"writer",
			"inputs":[{"workType":"task","state":"ready"}],
			"outputs":[{"workType":"task","state":"done"}]
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Layout == nil || len(cfg.Layout.Nodes) != 2 {
		t.Fatalf("layout nodes = %#v, want 2 nodes", cfg.Layout)
	}
	if cfg.Layout.Nodes[0].Size != nil || cfg.Layout.Nodes[1].Size != nil {
		t.Fatalf("layout node sizes = %#v, want sizeless nodes", cfg.Layout.Nodes)
	}
	if len(cfg.Layout.Edges) != 1 || len(cfg.Layout.Edges[0].Waypoints) != 2 {
		t.Fatalf("layout edges = %#v, want one edge with two waypoints", cfg.Layout.Edges)
	}

	public := FactoryConfigToOpenAPI(cfg)
	if public.Layout == nil || public.Layout.Nodes == nil || len(*public.Layout.Nodes) != 2 {
		t.Fatalf("public layout nodes = %#v, want 2", public.Layout)
	}
	if (*public.Layout.Nodes)[0].Size != nil || (*public.Layout.Nodes)[1].Size != nil {
		t.Fatalf("public layout node sizes = %#v, want omitted sizes", *public.Layout.Nodes)
	}
}

func TestFactoryConfigFromOpenAPIJSON_RejectsMalformedPortableLayoutContract(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"layout-factory",
		"layout":{
			"nodes":[{"id":"workstation:review","position":{"x":420,"y":180}}]
		},
		"workTypes": [{"name":"task","states":[{"name":"ready","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],
		"workers": [{"name":"writer","type":"MODEL_WORKER"}],
		"workstations": [{
			"name":"review",
			"worker":"writer",
			"inputs":[{"workType":"task","state":"ready"}],
			"outputs":[{"workType":"task","state":"done"}]
		}]
	}`)

	_, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err == nil {
		t.Fatal("expected malformed layout payload to be rejected")
	}
	if !strings.Contains(err.Error(), "layout.schemaVersion is required") {
		t.Fatalf("expected missing schemaVersion error, got %v", err)
	}
}

func TestFactoryConfigFromOpenAPIJSON_RejectsDuplicateLayoutAnnotationIDs(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"layout-factory",
		"layout":{"schemaVersion":1,"annotations":[
			{"id":"duplicate","kind":"NOTE","position":{"x":10,"y":20},"note":{"body":"First note","tone":"INFO"}},
			{"id":"duplicate","kind":"NOTE","position":{"x":30,"y":40},"note":{"body":"Second note","tone":"WARNING"}}
		]},
		"workTypes":[{"name":"task","states":[{"name":"ready","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],
		"workers":[{"name":"writer","type":"MODEL_WORKER"}],
		"workstations":[{"name":"review","worker":"writer","inputs":[{"workType":"task","state":"ready"}],"outputs":[{"workType":"task","state":"done"}]}]
	}`)

	_, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err == nil {
		t.Fatal("expected duplicate annotation id to be rejected")
	}
	if !strings.Contains(err.Error(), `layout.annotations[1].id "duplicate" duplicates`) {
		t.Fatalf("expected duplicate annotation path, got %v", err)
	}
}

func assertInternalGraphableEntityIDs(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()

	if cfg.WorkTypes[0].ID != "work-type-story" {
		t.Fatalf("work type id = %q", cfg.WorkTypes[0].ID)
	}
	if cfg.WorkTypes[0].States[0].ID != "state-ready" {
		t.Fatalf("work state id = %q", cfg.WorkTypes[0].States[0].ID)
	}
	if cfg.Resources[0].ID != "resource-agent-slot" {
		t.Fatalf("resource id = %q", cfg.Resources[0].ID)
	}
	if cfg.Workers[0].ID != "worker-executor" {
		t.Fatalf("worker id = %q", cfg.Workers[0].ID)
	}
}

func assertPublicGraphableEntityIDs(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()

	public := FactoryConfigToOpenAPI(cfg)
	if public.WorkTypes == nil || (*public.WorkTypes)[0].Id == nil || *(*public.WorkTypes)[0].Id != "work-type-story" {
		t.Fatalf("public work type id = %#v", public.WorkTypes)
	}
	if (*public.WorkTypes)[0].States[0].Id == nil || *(*public.WorkTypes)[0].States[0].Id != "state-ready" {
		t.Fatalf("public work state id = %#v", (*public.WorkTypes)[0].States[0].Id)
	}
	if public.Resources == nil || (*public.Resources)[0].Id == nil || *(*public.Resources)[0].Id != "resource-agent-slot" {
		t.Fatalf("public resource id = %#v", public.Resources)
	}
	if public.Workers == nil || (*public.Workers)[0].Id == nil || *(*public.Workers)[0].Id != "worker-executor" {
		t.Fatalf("public worker id = %#v", public.Workers)
	}
}

func assertCanonicalGraphableEntityIDs(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()

	canonical, err := MarshalCanonicalFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(canonical, &decoded); err != nil {
		t.Fatalf("unmarshal canonical config: %v", err)
	}
	workType := decoded["workTypes"].([]any)[0].(map[string]any)
	if workType["id"] != "work-type-story" {
		t.Fatalf("canonical work type id = %#v", workType["id"])
	}
	state := workType["states"].([]any)[0].(map[string]any)
	if state["id"] != "state-ready" {
		t.Fatalf("canonical work state id = %#v", state["id"])
	}
	resource := decoded["resources"].([]any)[0].(map[string]any)
	if resource["id"] != "resource-agent-slot" {
		t.Fatalf("canonical resource id = %#v", resource["id"])
	}
	worker := decoded["workers"].([]any)[0].(map[string]any)
	if worker["id"] != "worker-executor" {
		t.Fatalf("canonical worker id = %#v", worker["id"])
	}
}

func TestFactoryConfigFromOpenAPIJSON_AllowsLegacyNameKeyedGraphableEntities(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"legacy-name-keyed-factory",
		"workTypes": [{"name":"story","states":[{"name":"ready","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],
		"resources": [{"name":"agent-slot","capacity":2}],
		"workers": [{"name":"executor","type":"MODEL_WORKER"}],
		"workstations": [{
			"id":"workstation-execute-story",
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"ready"}],
			"outputs":[{"workType":"story","state":"done"}]
		}]
	}`)

	cfg, err := FactoryConfigFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	if cfg.WorkTypes[0].ID != "" || cfg.WorkTypes[0].States[0].ID != "" || cfg.Resources[0].ID != "" || cfg.Workers[0].ID != "" {
		t.Fatalf("legacy ids should remain empty, got workType=%q state=%q resource=%q worker=%q", cfg.WorkTypes[0].ID, cfg.WorkTypes[0].States[0].ID, cfg.Resources[0].ID, cfg.Workers[0].ID)
	}
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
	}`), factoryapi.InputGuardTypeSAMENAME, interfaces.GuardTypeSameName)
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
	}`), factoryapi.InputGuardTypeSAMETRACEID, interfaces.GuardTypeSameTraceID)
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
	if guard.Type != factoryapi.WorkstationGuardTypeMATCHESFIELDS {
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
	if guard.Type != factoryapi.FactoryGuardTypeInferenceThrottle {
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

func assertGeneratedAndRuntimeInputGuardMapping(t *testing.T, cfgJSON []byte, generatedType factoryapi.InputGuardType, runtimeType interfaces.GuardType) {
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

func requireGeneratedInputGuard(t *testing.T, generated factoryapi.Factory) factoryapi.InputGuard {
	t.Helper()

	workstation := requireSingleGeneratedWorkstation(t, generated)
	if len(workstation.Inputs) != 2 || workstation.Inputs[1].Guards == nil || len(*workstation.Inputs[1].Guards) != 1 {
		t.Fatalf("expected generated input guard to survive boundary decode, got %#v", workstation.Inputs)
	}
	return (*workstation.Inputs[1].Guards)[0]
}
