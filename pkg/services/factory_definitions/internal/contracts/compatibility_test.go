package factorycontracts

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	workertaxonomy "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers/taxonomy"
	"gopkg.in/yaml.v3"
)

func TestNameValueValidationAndResolution(t *testing.T) {
	t.Parallel()
	valid := NameValueConfig{
		Type:    NameValueTypeLocalizableAsset,
		Value:   "Base",
		Locales: []string{"en-US"},
		Values:  map[string]string{"fr-FR": "Français"},
		ID:      "factory-summary",
	}
	if err := ValidateNameValue(valid); err != nil {
		t.Fatalf("ValidateNameValue(valid) = %v", err)
	}
	if got := ResolveNameValue(valid, "fr-FR"); got != "Français" {
		t.Fatalf("exact override = %q, want Français", got)
	}
	for _, requested := range []string{"fr", "fr-fr", "fr-CA", ""} {
		if got := ResolveNameValue(valid, requested); got != "Base" {
			t.Fatalf("ResolveNameValue(%q) = %q, want base fallback", requested, got)
		}
	}

	tests := []struct {
		name      string
		value     NameValueConfig
		wantField string
		wantText  string
	}{
		{name: "unsupported discriminator", value: NameValueConfig{Type: "TEXT", Value: "Base"}, wantField: "type", wantText: "unsupported"},
		{name: "empty fallback", value: NameValueConfig{Type: NameValueTypeLocalizableAsset, Value: "  "}, wantField: "value", wantText: "must not be empty"},
		{name: "malformed locale", value: NameValueConfig{Type: NameValueTypeLocalizableAsset, Value: "Base", Locales: []string{"en_US"}}, wantField: "locales[0]", wantText: "not a valid BCP 47"},
		{name: "noncanonical locale", value: NameValueConfig{Type: NameValueTypeLocalizableAsset, Value: "Base", Locales: []string{"en-us"}}, wantField: "locales[0]", wantText: "use \"en-US\""},
		{name: "normalized locale duplicate", value: NameValueConfig{Type: NameValueTypeLocalizableAsset, Value: "Base", Locales: []string{"en-US", "en-us"}}, wantField: "locales[1]", wantText: "collides"},
		{name: "normalized values duplicate", value: NameValueConfig{Type: NameValueTypeLocalizableAsset, Value: "Base", Values: map[string]string{"en-US": "One", "en-us": "Two"}}, wantField: "values[\"en-us\"]", wantText: "collides"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateNameValue(test.value)
			if err == nil || !strings.Contains(err.Error(), test.wantField) || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("ValidateNameValue() = %v, want field %q containing %q", err, test.wantField, test.wantText)
			}
		})
	}
}

func TestNameValueRepresentationRoundTripsPreserveMetadata(t *testing.T) {
	t.Parallel()
	want := NameValueConfig{
		Type:    NameValueTypeLocalizableAsset,
		Value:   "General fallback",
		Locales: []string{"en-US", "de-DE"},
		Values:  map[string]string{"fr-FR": "Valeur", "ja-JP": "値"},
		ID:      "stable-copy-id",
	}

	jsonPayload, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var jsonRoundTrip NameValueConfig
	if err := json.Unmarshal(jsonPayload, &jsonRoundTrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(jsonRoundTrip, want) {
		t.Fatalf("JSON round trip = %#v, want %#v", jsonRoundTrip, want)
	}

	yamlPayload, err := yaml.Marshal(want)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var yamlRoundTrip NameValueConfig
	if err := yaml.Unmarshal(yamlPayload, &yamlRoundTrip); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(yamlRoundTrip, want) {
		t.Fatalf("YAML round trip = %#v, want %#v", yamlRoundTrip, want)
	}
}

func TestInvocationExampleArgumentsRejectInvalidJSONAndYAMLValues(t *testing.T) {
	t.Parallel()

	var repeated InvocationExampleArguments
	if err := json.Unmarshal([]byte(`{"tag":["alpha","beta"]}`), &repeated); err != nil {
		t.Fatalf("json.Unmarshal(repeated args): %v", err)
	}
	if got := repeated["tag"]; !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Fatalf("repeated args = %#v", got)
	}

	for _, payload := range []string{`{`, `{"count":3}`} {
		var decoded InvocationExampleArguments
		if err := json.Unmarshal([]byte(payload), &decoded); err == nil {
			t.Fatalf("json.Unmarshal(%s) unexpectedly succeeded", payload)
		}
	}

	for _, payload := range []string{"tag: [alpha, 3]\n", "count: 3\n"} {
		var decoded InvocationExampleArguments
		if err := yaml.Unmarshal([]byte(payload), &decoded); err == nil {
			t.Fatalf("yaml.Unmarshal(%q) unexpectedly succeeded", payload)
		}
	}
}

func TestWorkerWorkstationCompatibilityPreservesLegacyAndStrictPairings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		worker      string
		workstation Workstation
		want        bool
	}{
		{workertaxonomy.WorkerTypeAgent, Workstation{Type: workertaxonomy.WorkstationTypeAgent}, true},
		{workertaxonomy.WorkerTypeInference, Workstation{Type: workertaxonomy.WorkstationTypeAgent}, false},
		{workertaxonomy.WorkerTypeModel, Workstation{Type: workertaxonomy.WorkstationTypeModel}, true},
		{workertaxonomy.WorkerTypeHosted, Workstation{Kind: workertaxonomy.WorkstationKindPoller}, true},
	}
	for _, tc := range cases {
		if got := WorkerMatchesWorkstationBehavior(tc.worker, tc.workstation); got != tc.want {
			t.Fatalf("WorkerMatchesWorkstationBehavior(%q, %#v) = %v, want %v", tc.worker, tc.workstation, got, tc.want)
		}
	}
}

func TestPublicWorkerTypeForFactoryUsagePreservesMixedLegacyAlias(t *testing.T) {
	t.Parallel()
	worker := workerconfig.Config{Name: "executor", Type: workertaxonomy.WorkerTypeModel}
	workstations := []Workstation{{Type: workertaxonomy.WorkstationTypeModel, WorkerTypeName: "executor"}, {Type: workertaxonomy.WorkstationTypeInvoke, WorkerTypeName: "executor"}}
	if got := PublicWorkerTypeForFactoryUsage(worker, workstations); got != workertaxonomy.WorkerTypeModel {
		t.Fatalf("mixed usage = %q", got)
	}
}
