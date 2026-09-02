package definitions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestMinimalWorkflowExampleValidation validates every minimal workflow example against the public definition contract.
func TestMinimalWorkflowExampleValidation(t *testing.T) {
	t.Parallel()
	minimalWorkflow := support.AgentFactoryPath(t, filepath.Join("examples", "minimal-workflow"))
	formerObjectShape := copyWithFormerObjectOnFailure(t, minimalWorkflow)

	for _, test := range []struct {
		name        string
		path        string
		wantErr     bool
		diagnostics []string
	}{
		{
			name: "corrected array shape",
			path: minimalWorkflow,
			diagnostics: []string{
				"Factory validation passed.",
			},
		},
		{
			name:    "former object shape",
			path:    formerObjectShape,
			wantErr: true,
			diagnostics: []string{
				"onFailure",
				"cannot unmarshal object",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			inputs := support.FakeInputs(t.Context(), []string{
				"you", "factory", "config", "validate", test.path,
			})
			inputs.Input.Env = isolatedHomeEnvironment(t)
			inputs.Input.WorkingDirectory = filepath.Dir(test.path)

			err := buildDefinitionsProcess(t).Execute(inputs.Input)
			if test.wantErr && err == nil {
				t.Fatal("Process.Execute(factory config validate) error = nil, want rejection")
			}
			if !test.wantErr && err != nil {
				t.Fatalf(
					"Process.Execute(factory config validate %s) error = %v\nstdout:\n%s\nstderr:\n%s",
					test.path,
					err,
					inputs.Stdout(),
					inputs.Stderr(),
				)
			}

			diagnostic := inputs.Stdout() + "\n" + inputs.Stderr()
			if err != nil {
				diagnostic += "\n" + err.Error()
			}
			for _, want := range test.diagnostics {
				if !strings.Contains(diagnostic, want) {
					t.Fatalf("validation diagnostic missing %q:\n%s", want, diagnostic)
				}
			}
		})
	}
}

func copyWithFormerObjectOnFailure(t *testing.T, sourceDir string) string {
	t.Helper()

	sourcePath := filepath.Join(sourceDir, "factory.json")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read minimal workflow example: %v", err)
	}

	var factory map[string]json.RawMessage
	if err := json.Unmarshal(source, &factory); err != nil {
		t.Fatalf("decode minimal workflow example: %v", err)
	}
	var workstations []map[string]json.RawMessage
	if err := json.Unmarshal(factory["workstations"], &workstations); err != nil {
		t.Fatalf("decode minimal workflow workstations: %v", err)
	}
	if len(workstations) != 1 {
		t.Fatalf("minimal workflow workstations = %d, want 1", len(workstations))
	}
	var failureRoutes []json.RawMessage
	if err := json.Unmarshal(workstations[0]["onFailure"], &failureRoutes); err != nil {
		t.Fatalf("decode minimal workflow onFailure: %v", err)
	}
	if len(failureRoutes) != 1 {
		t.Fatalf("minimal workflow onFailure routes = %d, want 1", len(failureRoutes))
	}
	workstations[0]["onFailure"] = failureRoutes[0]
	workstationsJSON, err := json.Marshal(workstations)
	if err != nil {
		t.Fatalf("encode former object-shaped workstations: %v", err)
	}
	factory["workstations"] = workstationsJSON
	mutated, err := json.MarshalIndent(factory, "", "  ")
	if err != nil {
		t.Fatalf("encode former object-shaped minimal workflow: %v", err)
	}

	path := filepath.Join(t.TempDir(), "factory.json")
	if err := os.WriteFile(path, mutated, 0o600); err != nil {
		t.Fatalf("write former object-shaped factory: %v", err)
	}
	return path
}

func isolatedHomeEnvironment(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	return append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
}
