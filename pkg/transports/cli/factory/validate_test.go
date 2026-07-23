package factory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestValidateAcceptsDecodedExplicitJSONYAMLAndYMLSources(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"factory.json", "factory.yaml", "factory.yml"} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			var output strings.Builder
			err := ValidateWithServices(
				ValidateConfig{Context: context.Background(), Path: path, Output: &output},
				testTopologyFactoryDefinitionValidator(factorydefinitions.ValidationResult{}),
				func(path string) (factorydefinitions.AuthoredFactorySource, error) {
					return testAuthoredSource(path, []byte(`{"name":"supported"}`)), nil
				},
			)
			if err != nil {
				t.Fatalf("ValidateWithServices(%s): %v", path, err)
			}
			if !strings.Contains(output.String(), "Factory validation passed.") {
				t.Fatalf("output = %q", output.String())
			}
		})
	}
}

func TestValidateFailureReportsSourcePathAndFormat(t *testing.T) {
	t.Parallel()

	for path, format := range map[string]string{
		"customer/factory.json": "JSON",
		"customer/factory.yaml": "YAML",
		"customer/factory.yml":  "YAML",
	} {
		path := path
		format := format
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			err := ValidateWithServices(
				ValidateConfig{Context: context.Background(), Path: path, Output: &strings.Builder{}},
				testTopologyFactoryDefinitionValidator(testFactoryDefinitionValidationFailure(
					factorydefinitions.ValidationCodeWorkerWorkstationBehaviorCompatibility,
					"incompatible taxonomy",
					"worker",
				)),
				func(path string) (factorydefinitions.AuthoredFactorySource, error) {
					return testAuthoredSource(path, []byte(`{"name":"invalid"}`)), nil
				},
			)
			if err == nil {
				t.Fatal("expected blocking validation error")
			}
			for _, want := range []string{path, format, "validation"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q, want substring %q", err, want)
				}
			}
		})
	}
}

func TestValidateDirectoryFailureReportsSelectedYAMLRoot(t *testing.T) {
	t.Parallel()

	const directory = "customer/factory"
	selectedPath := directory + "/factory.yaml"
	err := ValidateWithServices(
		ValidateConfig{
			Context: context.Background(),
			Path:    directory,
			Output:  &strings.Builder{},
		},
		testTopologyFactoryDefinitionValidator(testFactoryDefinitionValidationFailure(
			factorydefinitions.ValidationCodeWorkerWorkstationBehaviorCompatibility,
			"incompatible taxonomy",
			"worker",
		)),
		func(path string) (factorydefinitions.AuthoredFactorySource, error) {
			if path != directory {
				t.Fatalf("source path = %q, want %q", path, directory)
			}
			return testAuthoredSource(
				selectedPath,
				[]byte(`{"name":"invalid"}`),
			), nil
		},
	)
	if err == nil {
		t.Fatal("expected blocking validation error")
	}
	for _, want := range []string{selectedPath, "YAML", "validation"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err, want)
		}
	}
	if strings.Contains(err.Error(), directory+" (") {
		t.Fatalf("error used directory instead of selected source: %q", err)
	}
}

func TestValidateUsesInjectedAuthoredSourceLoader(t *testing.T) {
	wantErr := errors.New("injected source failure")
	calls := 0
	err := ValidateWithServices(
		ValidateConfig{Context: context.Background(), Path: "customer/factory", Output: &strings.Builder{}},
		testTopologyFactoryDefinitionValidator(factorydefinitions.ValidationResult{}),
		func(path string) (factorydefinitions.AuthoredFactorySource, error) {
			calls++
			if path != "customer/factory" {
				t.Fatalf("source path = %q, want customer/factory", path)
			}
			return factorydefinitions.AuthoredFactorySource{}, wantErr
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ValidateWithServices() error = %v, want %v", err, wantErr)
	}
	if calls != 1 {
		t.Fatalf("source loader calls = %d, want 1", calls)
	}
}

func TestValidateRequiresInjectedAuthoredSourceLoader(t *testing.T) {
	err := ValidateWithServices(
		ValidateConfig{Context: context.Background(), Path: "customer/factory", Output: &strings.Builder{}},
		testTopologyFactoryDefinitionValidator(factorydefinitions.ValidationResult{}),
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "source loader is required") {
		t.Fatalf("ValidateWithServices() error = %v", err)
	}
}

func TestValidate_HumanOutputShowsNewTaxonomyAndCompatibilityFinding(t *testing.T) {
	path, loadSource := validateFixture(newTaxonomyFactoryJSON())

	var out strings.Builder
	err := ValidateWithServices(
		ValidateConfig{Context: context.Background(), Path: path, Output: &out},
		testTopologyFactoryDefinitionValidator(testFactoryDefinitionValidationFailure(
			factorydefinitions.ValidationCodeWorkerWorkstationBehaviorCompatibility,
			`workstation "agent-with-infer" uses agent-run behavior with incompatible worker type INFERENCE_WORKER`,
			"agent-with-infer",
		)),
		loadSource,
	)
	if err == nil {
		t.Fatal("expected incompatible taxonomy validation to fail")
	}

	text := out.String()
	for _, want := range []string{
		"Factory validation failed.",
		"Runtime taxonomy:",
		"worker infer: INFERENCE_WORKER",
		"workstation agent-with-infer: AGENT_RUN (worker=infer)",
		"Blocking targets:",
		"workstation-worker-behavior-compatibility",
		"agent-run",
		"INFERENCE_WORKER",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output = %q, want substring %q", text, want)
		}
	}
}

func TestValidate_HumanOutputPreservesLegacyTaxonomyValues(t *testing.T) {
	path, loadSource := validateFixture(legacyTaxonomyFactoryJSON())

	var out strings.Builder
	if err := ValidateWithServices(
		ValidateConfig{Context: context.Background(), Path: path, Output: &out},
		testTopologyFactoryDefinitionValidator(factorydefinitions.ValidationResult{}),
		loadSource,
	); err != nil {
		t.Fatalf("Validate legacy factory: %v", err)
	}

	text := out.String()
	for _, want := range []string{
		"Factory validation passed.",
		"worker legacy: MODEL_WORKER",
		"workstation legacy-run: MODEL_INVOKE (worker=legacy)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output = %q, want substring %q", text, want)
		}
	}
}

func TestValidate_HumanOutputLabelsLegacyPollerBehaviorWithoutType(t *testing.T) {
	path, loadSource := validateFixture(legacyPollerTaxonomyFactoryJSON())

	var out strings.Builder
	if err := ValidateWithServices(
		ValidateConfig{Context: context.Background(), Path: path, Output: &out},
		testTopologyFactoryDefinitionValidator(factorydefinitions.ValidationResult{}),
		loadSource,
	); err != nil {
		t.Fatalf("Validate legacy poller factory: %v", err)
	}

	text := out.String()
	for _, want := range []string{
		"Factory validation passed.",
		"workstation poll-tasks: legacy poller kind (worker=script-poller)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output = %q, want substring %q", text, want)
		}
	}
}

func TestValidate_JSONIncludesTaxonomySummary(t *testing.T) {
	path, loadSource := validateFixture(newTaxonomyFactoryJSON())

	var out bytes.Buffer
	err := ValidateWithServices(
		ValidateConfig{Context: context.Background(), Path: path, JSON: true, Output: &out},
		testTopologyFactoryDefinitionValidator(testFactoryDefinitionValidationFailure(
			factorydefinitions.ValidationCodeWorkerWorkstationBehaviorCompatibility,
			`workstation "agent-with-infer" uses agent-run behavior with incompatible worker type INFERENCE_WORKER`,
			"agent-with-infer",
		)),
		loadSource,
	)
	if err == nil {
		t.Fatal("expected incompatible taxonomy validation to fail")
	}

	var payload struct {
		Valid    bool `json:"valid"`
		Taxonomy []struct {
			Kind   string `json:"kind"`
			Name   string `json:"name"`
			Type   string `json:"type"`
			Worker string `json:"worker,omitempty"`
		} `json:"taxonomy"`
		Targets []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"targets"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if payload.Valid {
		t.Fatal("expected valid=false")
	}
	if len(payload.Taxonomy) < 2 || payload.Taxonomy[0].Type != "INFERENCE_WORKER" {
		t.Fatalf("taxonomy = %#v, want INFERENCE_WORKER summary", payload.Taxonomy)
	}
	if len(payload.Targets) == 0 || payload.Targets[0].Code != "workstation-worker-behavior-compatibility" {
		t.Fatalf("targets = %#v, want taxonomy compatibility target", payload.Targets)
	}
}

func validateFixture(
	body string,
) (string, factorydefinitions.AuthoredFactorySourceLoader) {
	const path = "injected/factory.json"
	return path, func(gotPath string) (factorydefinitions.AuthoredFactorySource, error) {
		if gotPath != path {
			return factorydefinitions.AuthoredFactorySource{}, fmt.Errorf(
				"source path = %q, want %q",
				gotPath,
				path,
			)
		}
		return testAuthoredSource(path, []byte(body)), nil
	}
}

func testAuthoredSource(
	path string,
	data []byte,
) factorydefinitions.AuthoredFactorySource {
	format := factorydefinitions.AuthoredFactoryFormatJSON
	if filepath.Ext(path) != ".json" {
		format = factorydefinitions.AuthoredFactoryFormatYAML
	}
	return factorydefinitions.AuthoredFactorySource{
		Path:   path,
		Format: format,
		Data:   data,
	}
}

func newTaxonomyFactoryJSON() string {
	return taxonomyMismatchFactoryJSON
}

func legacyPollerTaxonomyFactoryJSON() string {
	return `{
  "name": "legacy-poller-taxonomy",
  "workTypes": [{
    "name": "story",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "queued", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{
    "name": "script-poller",
    "type": "SCRIPT_WORKER",
    "command": "factory/scripts/poll.sh"
  }],
  "workstations": [{
    "name": "poll-tasks",
    "behavior": "POLLER",
    "worker": "script-poller",
    "inputs": [{"workType": "story", "state": "init"}],
    "outputs": [{"workType": "story", "state": "queued"}],
    "onFailure": [{"workType": "story", "state": "failed"}]
  }]
}`
}

func legacyTaxonomyFactoryJSON() string {
	return `{
  "name": "legacy-taxonomy",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "done", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{
    "name": "legacy",
    "type": "MODEL_WORKER",
    "operations": [{
      "name": "TTS",
      "inputs": [{"name": "text", "contentTypes": ["TEXT"]}],
      "outputs": [{"name": "audio", "contentTypes": ["AUDIO"]}]
    }]
  }],
  "workstations": [{
    "name": "legacy-run",
    "type": "MODEL_INVOKE",
    "operation": "TTS",
    "worker": "legacy",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "done"}]
  }]
}`
}

const taxonomyMismatchFactoryJSON = `{
  "name": "taxonomy-cli-api",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "done", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{
    "name": "infer",
    "type": "INFERENCE_WORKER",
    "operations": [{
      "name": "TTS",
      "inputs": [{"name": "text", "contentTypes": ["TEXT"]}],
      "outputs": [{"name": "audio", "contentTypes": ["AUDIO"]}]
    }]
  }],
  "workstations": [{
    "name": "agent-with-infer",
    "type": "AGENT_RUN",
    "worker": "infer",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "done"}]
  }]
}`
