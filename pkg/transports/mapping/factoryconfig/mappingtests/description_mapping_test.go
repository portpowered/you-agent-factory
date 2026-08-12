package mappingtests

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	. "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"gopkg.in/yaml.v3"
)

func TestFactoryEntityDescriptionsRoundTripWithoutAffectingTopology(t *testing.T) {
	cfg := factoryWithLocalizedDescriptions()
	wantTopology := topologyIdentity(cfg)

	public, err := FactoryConfigToOpenAPI(cfg)
	if err != nil {
		t.Fatalf("FactoryConfigToOpenAPI: %v", err)
	}
	roundTrip, err := FactoryConfigFromOpenAPI(public)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	assertDescriptionsEqual(t, &roundTrip, cfg)
	if got := topologyIdentity(&roundTrip); !reflect.DeepEqual(got, wantTopology) {
		t.Fatalf("topology changed after mapping: got %#v, want %#v", got, wantTopology)
	}

	mapper := NewFactoryConfigMapper()
	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("Flatten: %v", err)
	}
	expanded, err := mapper.Expand(flattened)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	assertDescriptionsEqual(t, expanded, cfg)
}

func TestFactoryConfigMapper_ExpandAndFlattenPreservesWebhookDeclarations(t *testing.T) {
	mapper := NewFactoryConfigMapper()
	raw := []byte(`{
		"name":"webhook-roundtrip",
		"webhooks":[{
			"name":"monitor",
			"enabled":true,
			"url":"https://hooks.example.test/factory",
			"signingSecretRef":"secrets/factory-monitor",
			"filter":{"eventTypes":["WORK_STATE_CHANGE","DISPATCH_RECONCILED"],"dispatchStatuses":["FAILED"]},
			"deliveryPolicy":{"requestTimeout":"15s","maxAttempts":3,"initialBackoff":"2s","backoffMultiplier":1.5,"maxBackoff":"10s"}
		}]
	}`)

	cfg, err := mapper.Expand(raw)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}
	assertExpandedWebhookDeclaration(t, cfg)

	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}
	assertFlattenedWebhookDeclaration(t, flattened)

	roundTrip, err := mapper.Expand(flattened)
	if err != nil {
		t.Fatalf("mapper.Expand(round trip): %v", err)
	}
	if len(roundTrip.Webhooks) != 1 || roundTrip.Webhooks[0].Name != "monitor" {
		t.Fatalf("round-tripped webhooks = %#v", roundTrip.Webhooks)
	}
}

func assertExpandedWebhookDeclaration(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	if len(cfg.Webhooks) != 1 {
		t.Fatalf("expanded webhooks = %#v, want one declaration", cfg.Webhooks)
	}
	webhook := cfg.Webhooks[0]
	if webhook.Name != "monitor" || !webhook.Enabled || webhook.URL != "https://hooks.example.test/factory" {
		t.Fatalf("expanded webhook identity = %#v", webhook)
	}
	if webhook.SigningSecretRef != "secrets/factory-monitor" {
		t.Fatalf("expanded secret reference = %q", webhook.SigningSecretRef)
	}
	if len(webhook.Filter.EventTypes) != 2 || webhook.Filter.EventTypes[1] != interfaces.FactoryWebhookEventTypeDispatchReconciled {
		t.Fatalf("expanded event filter = %#v", webhook.Filter)
	}
	if len(webhook.Filter.DispatchStatuses) != 1 || webhook.Filter.DispatchStatuses[0] != interfaces.FactoryWebhookDispatchStatusFailed {
		t.Fatalf("expanded dispatch filter = %#v", webhook.Filter)
	}
	if webhook.DeliveryPolicy == nil || webhook.DeliveryPolicy.MaxAttempts == nil || *webhook.DeliveryPolicy.MaxAttempts != 3 {
		t.Fatalf("expanded delivery policy = %#v", webhook.DeliveryPolicy)
	}
}

func assertFlattenedWebhookDeclaration(t *testing.T, flattened []byte) {
	t.Helper()
	payload := mustDecodeFactoryPayload(t, flattened)
	webhooks, ok := payload["webhooks"].([]any)
	if !ok || len(webhooks) != 1 {
		t.Fatalf("flattened webhooks = %#v", payload["webhooks"])
	}
	webhookPayload := webhooks[0].(map[string]any)
	if webhookPayload["signingSecretRef"] != "secrets/factory-monitor" {
		t.Fatalf("flattened secret reference = %#v", webhookPayload["signingSecretRef"])
	}
	if strings.Contains(string(flattened), "resolved-secret-value") {
		t.Fatalf("flattened factory leaked resolved secret material: %s", flattened)
	}
}

func TestFactoryEntityDescriptionsPreserveJSONAndYAMLRepresentations(t *testing.T) {
	want := factoryWithLocalizedDescriptions()

	jsonData, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var jsonRoundTrip interfaces.FactoryConfig
	if err := json.Unmarshal(jsonData, &jsonRoundTrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	assertDescriptionsEqual(t, &jsonRoundTrip, want)

	yamlData, err := yaml.Marshal(want)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var yamlRoundTrip interfaces.FactoryConfig
	if err := yaml.Unmarshal(yamlData, &yamlRoundTrip); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	assertDescriptionsEqual(t, &yamlRoundTrip, want)
}

func TestFactoryEntityDescriptionValidationReportsEntityPath(t *testing.T) {
	invalid := localizedDescription("invalid", "Base")
	invalid.Locales = []string{"en-us"}
	_, err := FactoryConfigFromOpenAPI(factoryapi.Factory{
		Name: factoryapi.FactoryName("described"),
		WorkTypes: &[]factoryapi.WorkType{{
			Name:        "task",
			Description: NameValueAPIFromInternal(invalid),
			States:      []factoryapi.WorkState{},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "factory.workTypes[0].description.locales[0]") {
		t.Fatalf("error = %v, want field-specific description diagnostic", err)
	}
}

func TestCanonicalWritersRejectInvalidFactoryMetadata(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*interfaces.FactoryConfig)
		wantPath string
	}{
		{
			name: "factory unsupported discriminator",
			mutate: func(cfg *interfaces.FactoryConfig) {
				cfg.Description = localizedDescription("factory", "Factory")
				cfg.Description.Type = "TEXT"
			},
			wantPath: "factory.description.type",
		},
		{
			name: "work type blank fallback",
			mutate: func(cfg *interfaces.FactoryConfig) {
				cfg.WorkTypes = []interfaces.WorkTypeConfig{{
					Name: "task", Description: localizedDescription("work-type", " "),
				}}
			},
			wantPath: "factory.workTypes[0].description.value",
		},
		{
			name: "worker non-canonical locale",
			mutate: func(cfg *interfaces.FactoryConfig) {
				description := localizedDescription("worker", "Worker")
				description.Locales = []string{"en-us"}
				cfg.Workers = []interfaces.FactoryWorkerConfig{{Name: "worker", Description: description}}
			},
			wantPath: "factory.workers[0].description.locales[0]",
		},
		{
			name: "workstation unsupported discriminator",
			mutate: func(cfg *interfaces.FactoryConfig) {
				description := localizedDescription("workstation", "Workstation")
				description.Type = "TEXT"
				cfg.Workstations = []interfaces.FactoryWorkstationConfig{{Name: "station", Description: description}}
			},
			wantPath: "factory.workstations[0].description.type",
		},
		{
			name: "example non-canonical locale",
			mutate: func(cfg *interfaces.FactoryConfig) {
				description := *localizedDescription("example", "Example")
				description.Values = map[string]string{"fr-fr": "Exemple"}
				cfg.Examples = []interfaces.InvocationExampleConfig{{Name: "sample", Description: description, Args: map[string]interface{}{}}}
			},
			wantPath: `factory.examples[0].description.values["fr-fr"]`,
		},
		{
			name: "example blank name",
			mutate: func(cfg *interfaces.FactoryConfig) {
				cfg.Examples = []interfaces.InvocationExampleConfig{{
					Name: " ", Description: *localizedDescription("example", "Example"), Args: map[string]interface{}{},
				}}
			},
			wantPath: "factory.examples[0].name",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := &interfaces.FactoryConfig{Name: "metadata-validation"}
			test.mutate(cfg)

			_, apiErr := FactoryConfigToOpenAPI(cfg)
			if apiErr == nil || !strings.Contains(apiErr.Error(), test.wantPath) {
				t.Fatalf("FactoryConfigToOpenAPI() error = %v, want path %q", apiErr, test.wantPath)
			}

			_, flattenErr := NewFactoryConfigMapper().Flatten(cfg)
			if flattenErr == nil || !strings.Contains(flattenErr.Error(), test.wantPath) {
				t.Fatalf("Flatten() error = %v, want path %q", flattenErr, test.wantPath)
			}
		})
	}
}

func TestFactoryEntityDescriptionsKeepStrictUnknownFieldRejection(t *testing.T) {
	mapper := NewFactoryConfigMapper()
	payloads := []string{
		`{"name":"strict","description":{"type":"LOCALIZABLE_ASSET","value":"Factory","unexpected":true},"workTypes":[],"workers":[],"workstations":[]}`,
		`{"name":"strict","workTypes":[{"name":"task","description":{"type":"LOCALIZABLE_ASSET","value":"Work type","unexpected":true},"states":[]}],"workers":[],"workstations":[]}`,
		`{"name":"strict","workTypes":[],"workers":[{"name":"worker","description":{"type":"LOCALIZABLE_ASSET","value":"Worker","unexpected":true}}],"workstations":[]}`,
		`{"name":"strict","workTypes":[],"workers":[],"workstations":[{"name":"station","worker":"worker","description":{"type":"LOCALIZABLE_ASSET","value":"Station","unexpected":true},"inputs":[]}]}`,
	}
	for _, payload := range payloads {
		_, err := mapper.Expand([]byte(payload))
		if err == nil || !strings.Contains(err.Error(), `json: unknown field "unexpected"`) {
			t.Fatalf("Expand(%s) error = %v, want unknown-field rejection", payload, err)
		}
	}
}

func factoryWithLocalizedDescriptions() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		Name:        "described",
		Description: localizedDescription("factory-description", "Factory base"),
		WorkTypes: []interfaces.WorkTypeConfig{{
			ID:          "task-id",
			Name:        "task",
			Description: localizedDescription("work-type-description", "Work type base"),
			States: []interfaces.StateConfig{
				{Name: "ready", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.FactoryWorkerConfig{{
			ID:          "worker-id",
			Name:        "worker",
			Description: localizedDescription("worker-description", "Worker base"),
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			ID:             "station-id",
			Name:           "station",
			Description:    localizedDescription("station-description", "Station base"),
			WorkerTypeName: "worker",
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: "ready"}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
		}},
	}
}

func localizedDescription(id, base string) *interfaces.NameValueConfig {
	return &interfaces.NameValueConfig{
		Type:    interfaces.NameValueTypeLocalizableAsset,
		Value:   base,
		Locales: []string{"en-US"},
		Values:  map[string]string{"fr-FR": base + " FR"},
		ID:      id,
	}
}

func assertDescriptionsEqual(t *testing.T, got, want *interfaces.FactoryConfig) {
	t.Helper()
	gotDescriptions := []any{got.Description, got.WorkTypes[0].Description, got.Workers[0].Description, got.Workstations[0].Description}
	wantDescriptions := []any{want.Description, want.WorkTypes[0].Description, want.Workers[0].Description, want.Workstations[0].Description}
	if !reflect.DeepEqual(gotDescriptions, wantDescriptions) {
		t.Fatalf("descriptions = %#v, want %#v", gotDescriptions, wantDescriptions)
	}
}

func topologyIdentity(cfg *interfaces.FactoryConfig) []string {
	return []string{
		cfg.Name,
		cfg.WorkTypes[0].ID,
		cfg.WorkTypes[0].Name,
		cfg.WorkTypes[0].States[0].Name,
		cfg.Workers[0].ID,
		cfg.Workers[0].Name,
		cfg.Workstations[0].ID,
		cfg.Workstations[0].Name,
		cfg.Workstations[0].WorkerTypeName,
		cfg.Workstations[0].Inputs[0].WorkTypeName,
		cfg.Workstations[0].Outputs[0].StateName,
	}
}

func TestFactoryConfigMapper_ExpectedArtifactsRoundTripThroughPublicJSON(t *testing.T) {
	mapper := NewFactoryConfigMapper()
	payload := []byte(`{
		"name": "artifact-contracts",
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}],"expectedArtifacts":[{"name":"report","pattern":"reports/{{ (index .Inputs 0).Name }}.json","nonEmpty":true}]}],
		"workers": [{"name":"processor"}],
		"workstations": [{"name":"process-task","worker":"processor","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"expectedArtifacts":[{"name":"manifest","pattern":"reports/manifest.json"}]}]
	}`)

	cfg, err := mapper.Expand(payload)
	if err != nil {
		t.Fatalf("mapper.Expand: %v", err)
	}
	if len(cfg.WorkTypes[0].ExpectedArtifacts) != 1 || cfg.WorkTypes[0].ExpectedArtifacts[0].Pattern != "reports/{{ (index .Inputs 0).Name }}.json" || !cfg.WorkTypes[0].ExpectedArtifacts[0].NonEmpty {
		t.Fatalf("expanded work type expected artifacts = %#v", cfg.WorkTypes[0].ExpectedArtifacts)
	}
	if len(cfg.Workstations[0].ExpectedArtifacts) != 1 || cfg.Workstations[0].ExpectedArtifacts[0].Name != "manifest" {
		t.Fatalf("expanded workstation expected artifacts = %#v", cfg.Workstations[0].ExpectedArtifacts)
	}

	flattened, err := mapper.Flatten(cfg)
	if err != nil {
		t.Fatalf("mapper.Flatten: %v", err)
	}
	var object map[string]any
	if err := json.Unmarshal(flattened, &object); err != nil {
		t.Fatalf("json.Unmarshal flattened payload: %v", err)
	}
	workType := object["workTypes"].([]any)[0].(map[string]any)
	workTypeArtifact := workType["expectedArtifacts"].([]any)[0].(map[string]any)
	if workTypeArtifact["pattern"] != "reports/{{ (index .Inputs 0).Name }}.json" {
		t.Fatalf("flattened work type expected artifact = %#v", workTypeArtifact)
	}
	workstation := object["workstations"].([]any)[0].(map[string]any)
	workstationArtifact := workstation["expectedArtifacts"].([]any)[0].(map[string]any)
	if workstationArtifact["name"] != "manifest" {
		t.Fatalf("flattened workstation expected artifact = %#v", workstationArtifact)
	}
}
