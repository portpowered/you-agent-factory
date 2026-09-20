package omni_media_probe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/portpowered/infinite-you/tests/internal/localai/corpusv2"
)

func TestCorpusRunnerV2SchemasPinAdmissionAndEvidence(t *testing.T) {
	t.Parallel()
	root := contractRepositoryRoot(t)
	base := filepath.Join(root, "tests", "integration", "models", "omni_media_probe")
	input := readContractSchema(t, filepath.Join(base, "probe-input.schema.json"))
	report := readContractSchema(t, filepath.Join(base, "report.schema.json"))
	authority := corpusv2.DefaultCorpusV2Authority()

	if input["$id"] != "urn:you-agent-factory:tests:omni-video-corpus-runner-input:v2" {
		t.Fatalf("runner input schema id = %#v", input["$id"])
	}
	inputProperties := contractProperties(t, input)
	if contractObjectValue(t, inputProperties, "schemaVersion", "const") != "you.localai.omni-video-corpus-runner-input.v2" {
		t.Fatal("runner input schema version is not pinned to v2")
	}
	if contractObjectValue(t, inputProperties, "corpusInput", "$ref") != "#/$defs/fileIdentity" {
		t.Fatal("runner input does not bind an identity-checked corpus input file")
	}
	inputLimits := contractNestedProperties(t, inputProperties, "limits")
	for field, expected := range map[string]any{
		"perCallTimeoutSeconds": float64(180), "maxHeavyProcesses": float64(1),
		"maxCompilerTestProcesses": float64(4), "maxDownloadBytes": float64(0),
		"maxPaidUsd": float64(0), "maxCalls": float64(10), "maxRetries": float64(0),
		"forbiddenPort": float64(7437), "networkPolicy": "declared-local-assets-and-loopback-only",
	} {
		if got := contractObjectValue(t, inputLimits, field, "const"); got != expected {
			t.Errorf("limits.%s const = %#v, want %#v", field, got, expected)
		}
	}
	if got := contractObjectValue(t, inputLimits, "maxDiskBytes", "maximum"); got != float64(3<<30) {
		t.Errorf("maxDiskBytes maximum = %#v, want %d", got, 3<<30)
	}

	if report["$id"] != "urn:you-agent-factory:tests:omni-video-corpus-runner-report:v2" {
		t.Fatalf("runner report schema id = %#v", report["$id"])
	}
	reportProperties := contractProperties(t, report)
	if contractObjectValue(t, reportProperties, "schemaVersion", "const") != "you.localai.omni-video-corpus-runner-report.v2" {
		t.Fatal("runner report schema version is not pinned to v2")
	}
	reportCorpus := contractNestedProperties(t, reportProperties, "corpus")
	if contractObjectValue(t, reportCorpus, "commit", "const") != authority.Commit || contractObjectValue(t, reportCorpus, "indexSha256", "const") != authority.IndexSHA256 {
		t.Fatal("runner report corpus authority differs from the shared corpus authority")
	}
	for field, expected := range map[string]any{
		"uniqueClips": float64(authority.PairCount), "uniquePrompts": float64(authority.PairCount),
		"copiedBytes": float64(0), "uploadedBytes": float64(0), "readOnly": true,
	} {
		if got := contractObjectValue(t, reportCorpus, field, "const"); got != expected {
			t.Errorf("corpus.%s const = %#v, want %#v", field, got, expected)
		}
	}
	selectedSamples := contractObjectValue(t, reportCorpus, "selectedSamples", "minItems")
	if selectedSamples != float64(len(authority.ExpectedSamples)) || contractObjectValue(t, reportCorpus, "selectedSamples", "maxItems") != selectedSamples {
		t.Fatalf("selected sample contract = %v..%v, want exactly %d", selectedSamples, contractObjectValue(t, reportCorpus, "selectedSamples", "maxItems"), len(authority.ExpectedSamples))
	}
}

func readContractSchema(t *testing.T, path string) map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read authored contract schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(body, &schema); err != nil {
		t.Fatalf("decode authored contract schema: %v", err)
	}
	return schema
}

func contractProperties(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties = %#v, want object", schema["properties"])
	}
	return properties
}

func contractNestedProperties(t *testing.T, properties map[string]any, name string) map[string]any {
	t.Helper()
	value, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("schema property %q = %#v, want object", name, properties[name])
	}
	return contractProperties(t, value)
}

func contractObjectValue(t *testing.T, properties map[string]any, name, field string) any {
	t.Helper()
	value, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("schema property %q = %#v, want object", name, properties[name])
	}
	return value[field]
}

func contractRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate repository root from contract test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "..", ".."))
}
