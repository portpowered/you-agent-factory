package yaml_io_parity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestPublicMappingFailuresRetainJSONAndYAMLSourceContext(t *testing.T) {
	for _, test := range []struct {
		rootName string
		body     string
		format   string
	}{
		{
			rootName: "factory.json",
			body:     `{"name":["invalid"]}`,
			format:   "(JSON)",
		},
		{
			rootName: "factory.yaml",
			body:     "name:\n  - invalid\n",
			format:   "(YAML)",
		},
	} {
		test := test
		t.Run(test.rootName, func(t *testing.T) {
			dir := t.TempDir()
			sourcePath := writeFile(t, filepath.Join(dir, test.rootName), test.body)
			assertPublicValidationFailureBeforeProvider(
				t,
				sourcePath,
				test.format,
				"parse factory config",
				"name",
				"cannot unmarshal",
			)
		})
	}
}

func TestPublicBlockingValidationRetainsJSONAndYAMLSourceContext(t *testing.T) {
	for _, test := range []struct {
		rootName string
		body     string
		format   string
	}{
		{rootName: "factory.json", body: invalidRuntimeTopologyJSON, format: "(JSON)"},
		{rootName: "factory.yaml", body: invalidRuntimeTopologyYAML, format: "(YAML)"},
	} {
		test := test
		t.Run(test.rootName, func(t *testing.T) {
			dir := t.TempDir()
			sourcePath := writeFile(t, filepath.Join(dir, test.rootName), test.body)
			assertRuntimeFailureBeforeProvider(
				t,
				"--factory",
				sourcePath,
				"runtime must not start",
				test.format,
				"validate factory config",
				"factory.worker.danglingReference",
				"missing-worker",
			)
		})
	}
}

func TestRuntimeMappingFailureRetainsSelectedSourceContext(t *testing.T) {
	dir := t.TempDir()
	sourcePath := writeFile(
		t,
		filepath.Join(dir, "factory.json"),
		`{"name":["invalid"]}`,
	)
	assertRuntimeFailureBeforeProvider(
		t,
		"--dir",
		dir,
		"",
		sourcePath,
		"(JSON)",
		"parse factory config",
		"name",
		"cannot unmarshal",
	)
}

func assertRuntimeFailureBeforeProvider(
	t *testing.T,
	sourceFlag string,
	factorySource string,
	invocationInput string,
	wants ...string,
) {
	t.Helper()
	runner := support.NewRecordingCommandRunner("runtime must not execute")
	args := []string{"you", "run", sourceFlag, factorySource, "--no-record", "--quiet"}
	if invocationInput != "" {
		args = append(args, invocationInput)
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = t.TempDir()
	err := support.BuildProcess(
		t,
		serviceedges.Edges{ProviderCommandRunner: runner},
	).Execute(inputs.Input)
	if err == nil {
		t.Fatal("Process.Execute() error = nil")
	}
	diagnostic := err.Error() + "\n" + inputs.Stdout() + "\n" + inputs.Stderr()
	for _, want := range wants {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf("diagnostic %q does not contain %q", diagnostic, want)
		}
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0", runner.CallCount())
	}
}

func assertPublicValidationFailureBeforeProvider(
	t *testing.T,
	factorySource string,
	wants ...string,
) {
	t.Helper()
	runner := support.NewRecordingCommandRunner("runtime must not execute")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "factory", "config", "validate", factorySource,
	})
	inputs.Input.WorkingDirectory = t.TempDir()
	err := support.BuildProcess(
		t,
		serviceedges.Edges{ProviderCommandRunner: runner},
	).Execute(inputs.Input)
	if err == nil {
		t.Fatal("Process.Execute() error = nil")
	}
	diagnostic := err.Error() + "\n" + inputs.Stdout() + "\n" + inputs.Stderr()
	for _, want := range wants {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf("diagnostic %q does not contain %q", diagnostic, want)
		}
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0", runner.CallCount())
	}
}

const invalidRuntimeTopologyJSON = `{
	"name":"invalid",
	"workTypes":[{
		"name":"task",
		"states":[
			{"name":"init","type":"INITIAL"},
			{"name":"complete","type":"TERMINAL"},
			{"name":"failed","type":"FAILED"}
		]
	}],
	"workstations":[{
		"name":"execute",
		"worker":"missing-worker",
		"inputs":[{"workType":"task","state":"init"}],
		"outputs":[{"workType":"task","state":"complete"}],
		"onFailure":[{"workType":"task","state":"failed"}],
		"type":"MODEL_WORKSTATION"
	}]
}`

const invalidRuntimeTopologyYAML = `name: invalid
workTypes:
  - name: task
    states:
      - {name: init, type: INITIAL}
      - {name: complete, type: TERMINAL}
      - {name: failed, type: FAILED}
workstations:
  - name: execute
    worker: missing-worker
    inputs:
      - {workType: task, state: init}
    outputs:
      - {workType: task, state: complete}
    onFailure:
      - {workType: task, state: failed}
    type: MODEL_WORKSTATION
`

func writeFile(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}
