package validate_persist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const invalidFactoryWithDanglingWorker = `{
	"name":"invalid-validate-persist",
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

// TestCLIFactoryValidateRejectsInvalidDefinitionActionably proves the public
// Factory CLI validate command rejects an invalid authored definition with
// actionable diagnostics before runtime execution or persistence side effects.
func TestCLIFactoryValidateRejectsInvalidDefinitionActionably(t *testing.T) {
	for _, test := range []struct {
		name    string
		body    string
		wants   []string
	}{
		{
			name: "semantic validation failure",
			body: invalidFactoryWithDanglingWorker,
			wants: []string{
				"factory validation found blocking issues",
				"factory.worker.danglingReference",
				"missing-worker",
			},
		},
		{
			name: "parse failure",
			body: `{"name":["invalid"]}`,
			wants: []string{
				"parse factory config",
				"name",
				"cannot unmarshal",
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			sourcePath := writeFactoryFile(t, test.body)
			assertValidateRejectedActionably(t, sourcePath, test.wants...)
		})
	}
}

func assertValidateRejectedActionably(t *testing.T, factorySource string, wants ...string) {
	t.Helper()

	runner := support.NewRecordingCommandRunner("runtime must not execute")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "factory", "config", "validate", factorySource,
	})
	inputs.Input.WorkingDirectory = filepath.Dir(factorySource)
	err := support.BuildProcess(
		t,
		serviceedges.Edges{ProviderCommandRunner: runner},
	).Execute(inputs.Input)
	if err == nil {
		t.Fatal("Process.Execute(factory config validate) error = nil, want rejection")
	}

	diagnostic := err.Error() + "\n" + inputs.Stdout() + "\n" + inputs.Stderr()
	for _, want := range wants {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf("validate diagnostic missing %q:\n%s", want, diagnostic)
		}
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 before validate completes", runner.CallCount())
	}
}

func writeFactoryFile(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
	return path
}
