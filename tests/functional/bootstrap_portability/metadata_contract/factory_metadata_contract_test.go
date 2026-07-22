package metadata_contract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	clirun "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"gopkg.in/yaml.v3"
)

func TestFactoryMetadataContractLoadsLegacyExamplesAndRendersCanonicalHelp(t *testing.T) {
	t.Parallel()

	payload := []byte(`{
  "name": "described-example",
  "description": {"type":"LOCALIZABLE_ASSET","value":"Factory base","locales":["en-US"],"values":{"fr-FR":"Usine"},"id":"factory-description"},
  "invocationSignature": {
    "parameters": [
      {"name":"input","required":true,"bindings":[{"kind":"POSITIONAL","position":1}]},
      {"name":"tag","externalName":"tag","valueMode":"REPEATED","bindings":[{"kind":"NAMED"}]},
      {"name":"body","bindings":[{"kind":"STDIN"}]}
    ],
    "examples": [{"name":"legacy","description":"Legacy explanation","argv":["hello","--tag","alpha","--tag=beta"],"stdin":"body text"}]
  },
  "workTypes": [{"name":"task","description":{"type":"LOCALIZABLE_ASSET","value":"Task base"},"states":[]}],
  "workers": [{"name":"worker","description":{"type":"LOCALIZABLE_ASSET","value":"Worker base","locales":["en-US"],"values":{"fr-FR":"Ouvrier"}}}],
  "workstations": [{"name":"station","worker":"worker","description":{"type":"LOCALIZABLE_ASSET","value":"Station base"},"inputs":[]}]
}`)

	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(payload)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if got := interfaces.ResolveNameValue(*cfg.Description, "fr-FR"); got != "Usine" {
		t.Fatalf("localized factory description = %q, want Usine", got)
	}
	if len(cfg.Examples) != 1 || cfg.Examples[0].Description.Value != "Legacy explanation" {
		t.Fatalf("canonical examples = %#v", cfg.Examples)
	}
	if got := cfg.Examples[0].Args["input"]; got != "hello" {
		t.Fatalf("example input = %#v, want hello", got)
	}
	if got := cfg.Examples[0].Args["tag"]; !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Fatalf("example tags = %#v", got)
	}
	if got := cfg.Examples[0].Args["body"]; got != "body text" {
		t.Fatalf("example body = %#v, want body text", got)
	}

	canonical, err := factorymapping.MarshalCanonicalFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	if strings.Contains(string(canonical), `"argv"`) || strings.Contains(string(canonical), `"invocationSignature":{"examples"`) {
		t.Fatalf("canonical output retained legacy examples: %s", canonical)
	}

	jsonData, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var jsonRoundTrip interfaces.FactoryConfig
	if err := json.Unmarshal(jsonData, &jsonRoundTrip); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	assertFactoryMetadataContract(t, &jsonRoundTrip, cfg)

	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var yamlRoundTrip interfaces.FactoryConfig
	if err := yaml.Unmarshal(yamlData, &yamlRoundTrip); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	assertFactoryMetadataContract(t, &yamlRoundTrip, cfg)

	clonedWorker := interfaces.CloneWorkerConfig(cfg.Workers[0])
	clonedWorker.Description.Locales[0] = "de-DE"
	clonedWorker.Description.Values["fr-FR"] = "Changed"
	if cfg.Workers[0].Description.Locales[0] != "en-US" || cfg.Workers[0].Description.Values["fr-FR"] != "Ouvrier" {
		t.Fatalf("worker clone mutated source description: %#v", cfg.Workers[0].Description)
	}

	var help strings.Builder
	written, err := clirun.WriteFactoryInvocationHelp(&help, "you", clirun.RunConfig{
		FactoryConfigPath: "factory.json",
		LoadFactoryConfigFile: func(string) (*interfaces.FactoryConfig, error) {
			return cfg, nil
		},
	})
	if err != nil || !written {
		t.Fatalf("WriteFactoryInvocationHelp = (%t, %v)", written, err)
	}
	for _, want := range []string{
		"# Legacy explanation",
		"printf '%s\\n' 'body text' | you run --factory factory.json hello --tag alpha --tag beta",
	} {
		if !strings.Contains(help.String(), want) {
			t.Fatalf("invocation help missing %q:\n%s", want, help.String())
		}
	}
}

func assertFactoryMetadataContract(t *testing.T, got, want *interfaces.FactoryConfig) {
	t.Helper()
	gotDescriptions := []any{got.Description, got.WorkTypes[0].Description, got.Workers[0].Description, got.Workstations[0].Description}
	wantDescriptions := []any{want.Description, want.WorkTypes[0].Description, want.Workers[0].Description, want.Workstations[0].Description}
	if !reflect.DeepEqual(gotDescriptions, wantDescriptions) {
		t.Fatalf("descriptions = %#v, want %#v", gotDescriptions, wantDescriptions)
	}
	if !reflect.DeepEqual(got.Examples, want.Examples) {
		t.Fatalf("examples = %#v, want %#v", got.Examples, want.Examples)
	}
}
