package definitions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"gopkg.in/yaml.v3"
)

const legacyMetadataFactoryJSON = `{
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
}`

// TestFactoryMetadataContractLoadsLegacyExamplesAndRendersCanonicalHelp proves
// legacy invocationSignature examples map into canonical Factory metadata and
// render customer-visible run help through the public CLI without dispatching
// external provider command execution.
func TestFactoryMetadataContractLoadsLegacyExamplesAndRendersCanonicalHelp(t *testing.T) {
	t.Parallel()

	runner := support.NewRecordingCommandRunner("runtime must not execute")
	edges := serviceedges.Edges{ProviderCommandRunner: runner}

	cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON([]byte(legacyMetadataFactoryJSON))
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

	factoryDir := t.TempDir()
	factoryPath := filepath.Join(factoryDir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(legacyMetadataFactoryJSON), 0o600); err != nil {
		t.Fatalf("write legacy metadata factory: %v", err)
	}

	homeDir := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", factoryPath, "--help",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = factoryDir

	if err := support.BuildProcess(t, edges).Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(run --factory --help) error = %v; stdout=%q stderr=%q",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	for _, want := range []string{
		"# Legacy explanation",
		"printf '%s",
		"'body text'",
		"hello --tag alpha --tag beta",
	} {
		if !strings.Contains(inputs.Stdout(), want) {
			t.Fatalf("run --factory --help missing %q:\n%s", want, inputs.Stdout())
		}
	}
	if !strings.Contains(inputs.Stdout(), "you run --factory "+factoryPath) {
		t.Fatalf("run --factory --help missing selected factory example path:\n%s", inputs.Stdout())
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 for read-only invocation help", runner.CallCount())
	}
}

// TestFactoryMetadataSnapshotRejectsInvalidProgrammaticExampleArguments proves
// Factory snapshot mapping rejects programmatic example arguments that cannot
// be represented in the public Factory contract.
func TestFactoryMetadataSnapshotRejectsInvalidProgrammaticExampleArguments(t *testing.T) {
	t.Parallel()

	cfg := &interfaces.FactoryConfig{
		Name: "invalid-example",
		Examples: []interfaces.InvocationExampleConfig{{
			Name:        "invalid",
			Description: interfaces.NameValueConfig{Type: interfaces.NameValueTypeLocalizableAsset, Value: "Invalid"},
			Args:        interfaces.InvocationExampleArguments{"tag": []interface{}{"alpha", 3}},
		}},
	}
	got, err := factorysnapshot.ObjectFromFactoryConfig(cfg)
	if err == nil || got != nil || !strings.Contains(err.Error(), "factory.examples[0].args.tag must be a string or array of strings") {
		t.Fatalf("ObjectFromFactoryConfig() = (%#v, %v), want field-specific rejection", got, err)
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
