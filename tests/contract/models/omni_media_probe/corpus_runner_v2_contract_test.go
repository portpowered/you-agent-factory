package omni_media_probe

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	contractCorpusCommit    = "443ee4715e6e0f8ec02489e4fda9554d15434e08"
	contractCorpusIndexHash = "ac4eeb02e6d1e1065176dcf5c18f74367b241c698ec6344ca48829b7ab770c28"
	contractCorpusPairs     = 370
	contractCorpusSamples   = 9
)

func TestCorpusRunnerV2SchemasPinAdmissionAndEvidence(t *testing.T) {
	t.Parallel()
	root := contractRepositoryRoot(t)
	base := filepath.Join(root, "tests", "integration", "models", "omni_media_probe")
	input := readContractSchema(t, filepath.Join(base, "probe-input.schema.json"))
	report := readContractSchema(t, filepath.Join(base, "report.schema.json"))
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
	if modes, ok := contractObjectValue(t, reportProperties, "mode", "enum").([]any); !ok || len(modes) != 2 || modes[0] != "PREFLIGHT" || modes[1] != "EXECUTE" {
		t.Fatalf("runner report modes = %#v, want PREFLIGHT and EXECUTE", contractObjectValue(t, reportProperties, "mode", "enum"))
	}
	reportPolicy := contractNestedProperties(t, reportProperties, "policy")
	if got := contractObjectValue(t, reportPolicy, "platform", "pattern"); got != "^[a-z0-9_]+/[a-z0-9_]+$" {
		t.Fatalf("runner report platform pattern = %#v", got)
	}
	if got := contractObjectValue(t, reportPolicy, "perCallTimeoutSeconds", "const"); got != float64(180) {
		t.Fatalf("runner report per-call timeout = %#v, want 180 seconds", got)
	}
	if got := contractObjectValue(t, reportPolicy, "maxCalls", "const"); got != float64(10) {
		t.Fatalf("runner report call ceiling = %#v, want ten", got)
	}
	callsSchema := reportProperties["calls"].(map[string]any)
	if got := callsSchema["maxItems"]; got != float64(10) {
		t.Fatalf("runner report call array ceiling = %#v, want ten", got)
	}
	definitions := report["$defs"].(map[string]any)
	failureSchema := definitions["failure"].(map[string]any)
	if failureSchema["additionalProperties"] != false {
		t.Fatal("runner report failure object is not closed to unreviewed fields")
	}
	reportCorpus := contractNestedProperties(t, reportProperties, "corpus")
	if contractObjectValue(t, reportCorpus, "commit", "const") != contractCorpusCommit || contractObjectValue(t, reportCorpus, "indexSha256", "const") != contractCorpusIndexHash {
		t.Fatal("runner report corpus authority differs from the shared corpus authority")
	}
	for field, expected := range map[string]any{
		"uniqueClips": float64(contractCorpusPairs), "uniquePrompts": float64(contractCorpusPairs),
		"copiedBytes": float64(0), "uploadedBytes": float64(0), "readOnly": true,
	} {
		if got := contractObjectValue(t, reportCorpus, field, "const"); got != expected {
			t.Errorf("corpus.%s const = %#v, want %#v", field, got, expected)
		}
	}
	selectedSamples := contractObjectValue(t, reportCorpus, "selectedSamples", "minItems")
	if selectedSamples != float64(contractCorpusSamples) || contractObjectValue(t, reportCorpus, "selectedSamples", "maxItems") != selectedSamples {
		t.Fatalf("selected sample contract = %v..%v, want exactly %d", selectedSamples, contractObjectValue(t, reportCorpus, "selectedSamples", "maxItems"), contractCorpusSamples)
	}
}

func TestCandidateManifestSchemaPinsExactWindowsArtifactProvenance(t *testing.T) {
	t.Parallel()
	root := contractRepositoryRoot(t)
	manifestSchema := readContractSchema(t, filepath.Join(root, "tests", "integration", "models", "omni_media_probe", "candidate-manifest.schema.json"))
	schemaID := "urn:you-agent-factory:tests:omni-video-candidate-manifest:v1"
	if manifestSchema["$id"] != schemaID {
		t.Fatalf("candidate manifest schema id = %#v, want %q", manifestSchema["$id"], schemaID)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(schemaID, manifestSchema); err != nil {
		t.Fatalf("register candidate manifest schema: %v", err)
	}
	compiled, err := compiler.Compile(schemaID)
	if err != nil {
		t.Fatalf("compile candidate manifest schema: %v", err)
	}
	manifest := map[string]any{
		"schemaVersion": "you.localai.omni-video-candidate-manifest.v1",
		"path":          "C:/Users/andre/work/portos/infinite-you/docs/temp/projects/localai/validation/artifacts/cycle-151-v20-video-candidate/you.exe",
		"sourceCommit":  strings.Repeat("a", 40),
		"sourceTree":    strings.Repeat("b", 40),
		"version":       "you@candidate",
		"toolchain":     "go version go1.25.1 windows/amd64",
		"goos":          "windows",
		"goarch":        "amd64",
		"bytes":         int64(1),
		"sha256":        strings.Repeat("0", 64),
	}
	if err := compiled.Validate(manifest); err != nil {
		t.Fatalf("valid Windows candidate manifest rejected: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "unknown property", mutate: func(value map[string]any) { value["reviewerNote"] = "unreviewed" }},
		{name: "candidate path", mutate: func(value map[string]any) { value["path"] = "C:/temp/you.exe" }},
		{name: "source commit", mutate: func(value map[string]any) { value["sourceCommit"] = strings.Repeat("a", 39) }},
		{name: "source tree", mutate: func(value map[string]any) { value["sourceTree"] = strings.Repeat("B", 40) }},
		{name: "missing version", mutate: func(value map[string]any) { delete(value, "version") }},
		{name: "platform", mutate: func(value map[string]any) { value["goos"] = "linux" }},
		{name: "architecture", mutate: func(value map[string]any) { value["goarch"] = "arm64" }},
		{name: "empty artifact", mutate: func(value map[string]any) { value["bytes"] = int64(0) }},
		{name: "sha256", mutate: func(value map[string]any) { value["sha256"] = strings.Repeat("A", 64) }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			document := make(map[string]any, len(manifest))
			for key, value := range manifest {
				document[key] = value
			}
			testCase.mutate(document)
			if err := compiled.Validate(document); err == nil {
				t.Fatal("invalid candidate manifest unexpectedly passed schema validation")
			}
		})
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
