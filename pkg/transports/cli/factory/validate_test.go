package factory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate_EnforcesAuthoredPortableLayoutBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		data          string
		width         int
		alt           string
		positionExtra string
		sourceExtra   string
		wantPath      string
	}{
		{name: "valid annotated factory", data: "AQID", width: 120, alt: "diagram"},
		{
			name:     "non-strict base64",
			data:     "AQI=\n",
			width:    120,
			alt:      "diagram",
			wantPath: "layout.annotations[0].image.source.data",
		},
		{
			name:     "empty alternative text",
			data:     "AQID",
			width:    120,
			wantPath: "layout.annotations[0].image.alternativeText",
		},
		{
			name:     "zero image width",
			data:     "AQID",
			width:    0,
			alt:      "diagram",
			wantPath: "layout.annotations[0].size.width",
		},
		{
			name:        "remote URL source field",
			data:        "AQID",
			width:       120,
			alt:         "diagram",
			sourceExtra: `,"url":"https://example.invalid/image.png"`,
			wantPath:    "layout.annotations[0].image.source.url",
		},
		{
			name:          "nested connection reference",
			data:          "AQID",
			width:         120,
			alt:           "diagram",
			positionExtra: `,"connection":{"nodeId":"workstation:review"}`,
			wantPath:      "layout.annotations[0].position.connection",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := writeValidateFixture(t, portableLayoutFactoryJSON(
				test.data,
				test.width,
				test.alt,
				test.positionExtra,
				test.sourceExtra,
			))

			var out strings.Builder
			err := Validate(ValidateConfig{Path: path, Output: &out})
			if test.wantPath == "" {
				if err != nil {
					t.Fatalf("Validate valid annotated factory: %v", err)
				}
				if !strings.Contains(out.String(), "Factory validation passed.") {
					t.Fatalf("output = %q, want validation success", out.String())
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate invalid annotated factory error = nil, want path %q", test.wantPath)
			}
			if !strings.Contains(err.Error(), test.wantPath) {
				t.Fatalf("Validate error = %q, want field path %q", err, test.wantPath)
			}
		})
	}
}

func portableLayoutFactoryJSON(data string, width int, alternativeText, positionExtra, sourceExtra string) string {
	return fmt.Sprintf(`{
		"name":"layout-cli",
		"layout":{"schemaVersion":1,"annotations":[{
			"id":"image-1","kind":"IMAGE","position":{"x":0,"y":0%s},"size":{"width":%d,"height":80},
			"image":{"alternativeText":%q,"source":{"kind":"EMBEDDED","mediaType":"image/png","data":%q%s}}
		}],"viewport":{"x":0,"y":0,"zoom":1},"preferences":{"direction":"RIGHT"}}
	}`, positionExtra, width, alternativeText, data, sourceExtra)
}

func TestValidate_HumanOutputShowsNewTaxonomyAndCompatibilityFinding(t *testing.T) {
	path := writeValidateFixture(t, newTaxonomyFactoryJSON())

	var out strings.Builder
	err := Validate(ValidateConfig{Path: path, Output: &out})
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
	path := writeValidateFixture(t, legacyTaxonomyFactoryJSON())

	var out strings.Builder
	if err := Validate(ValidateConfig{Path: path, Output: &out}); err != nil {
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
	path := writeValidateFixture(t, legacyPollerTaxonomyFactoryJSON())

	var out strings.Builder
	if err := Validate(ValidateConfig{Path: path, Output: &out}); err != nil {
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
	path := writeValidateFixture(t, newTaxonomyFactoryJSON())

	var out bytes.Buffer
	err := Validate(ValidateConfig{Path: path, JSON: true, Output: &out})
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

func writeValidateFixture(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
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
