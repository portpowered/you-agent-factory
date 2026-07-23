package yaml_io_parity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	goalFactoryName      = "portable-goal"
	goalWorkstationName  = "execute-goal"
	goalWorkerName       = "goal-executor"
	wantInvocationResult = "mock worker accepted"
)

func TestPackagedFactoryJSONAndYAMLValidateFlattenAndRunParity(t *testing.T) {
	jsonDir := materializePackagedGoal(t, "factory.json")
	yamlDir := materializePackagedGoal(t, "factory.yaml")
	ymlDir := materializePackagedGoal(t, "factory.yml")

	jsonPath := filepath.Join(jsonDir, "factory.json")
	yamlPath := filepath.Join(yamlDir, "factory.yaml")
	ymlPath := filepath.Join(ymlDir, "factory.yml")
	ambiguousDir := materializePackagedGoal(t, "factory.json")
	copyFile(t, yamlPath, filepath.Join(ambiguousDir, "factory.yaml"))
	for _, path := range []string{jsonPath, yamlPath, ymlPath, jsonDir, yamlDir, ymlDir} {
		validateFactory(t, path)
	}

	jsonFactory := flattenFactory(t, jsonPath)
	for _, path := range []string{yamlPath, ymlPath, jsonDir, yamlDir, ymlDir} {
		if got := flattenFactory(t, path); !reflect.DeepEqual(got, jsonFactory) {
			t.Fatalf("flattened Factory from %s differs from packaged JSON", path)
		}
	}

	for _, source := range []struct {
		name string
		args []string
	}{
		{name: "explicit JSON", args: []string{"--factory", jsonPath}},
		{name: "explicit YAML", args: []string{"--factory", yamlPath}},
		{name: "explicit YML", args: []string{"--factory", ymlPath}},
		{
			name: "explicit YAML in ambiguous directory",
			args: []string{"--factory", filepath.Join(ambiguousDir, "factory.yaml")},
		},
		{name: "JSON directory", args: []string{"--factory", jsonDir}},
		{name: "YAML directory", args: []string{"--factory", yamlDir}},
		{name: "YML directory", args: []string{"--factory", ymlDir}},
	} {
		source := source
		t.Run(source.name, func(t *testing.T) {
			if got := invokeGoal(t, os.Environ(), t.TempDir(), source.args...); got != wantInvocationResult {
				t.Fatalf("invocation result = %q, want %q", got, wantInvocationResult)
			}
		})
	}
}

func TestYAMLCreateAndUpdateRemainRunnableAfterCanonicalPersistence(t *testing.T) {
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	yamlSource := filepath.Join(materializePackagedGoal(t, "factory.yaml"), "factory.yaml")
	jsonSource := filepath.Join(materializePackagedGoal(t, "factory.json"), "factory.json")

	factoryDir := support.CreateNamedFactory(
		t,
		homeDir,
		workingDirectory,
		goalFactoryName,
		yamlSource,
	)
	env := customerEnvironment(homeDir)
	if got := invokeGoal(t, env, workingDirectory, "--named", goalFactoryName); got != wantInvocationResult {
		t.Fatalf("YAML-created invocation result = %q, want %q", got, wantInvocationResult)
	}
	if _, err := os.Stat(filepath.Join(factoryDir, "factory.json")); err != nil {
		t.Fatalf("YAML-created Factory missing canonical factory.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(factoryDir, "factory.yaml")); !os.IsNotExist(err) {
		t.Fatalf("YAML-created Factory unexpectedly persisted factory.yaml: %v", err)
	}

	update := support.FakeInputs(t.Context(), []string{
		"you", "--json", "factory", "update", goalFactoryName,
		"--from", jsonSource,
		"--dir", filepath.Dir(factoryDir),
	})
	update.Input.Env = env
	update.Input.WorkingDirectory = workingDirectory
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(update.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory update) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			update.Stdout(),
			update.Stderr(),
		)
	}
	if got := invokeGoal(t, env, workingDirectory, "--named", goalFactoryName); got != wantInvocationResult {
		t.Fatalf("JSON-updated invocation result = %q, want %q", got, wantInvocationResult)
	}
}

func TestRejectedAuthoredSourcesFailBeforeRuntimeExecution(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(*testing.T) []string
		wants   []string
	}{
		{
			name: "malformed YAML",
			prepare: func(t *testing.T) []string {
				path := writeFile(t, filepath.Join(t.TempDir(), "factory.yaml"), "name: [\n")
				return []string{"--factory", path}
			},
			wants: []string{"factory.yaml", "YAML"},
		},
		{
			name: "JSON representation mismatch",
			prepare: func(t *testing.T) []string {
				path := writeFile(t, filepath.Join(t.TempDir(), "factory.json"), `{"name":["invalid"]}`)
				return []string{"--factory", path}
			},
			wants: []string{"factory.json", "(JSON)", "parse factory config"},
		},
		{
			name: "YAML representation mismatch",
			prepare: func(t *testing.T) []string {
				path := writeFile(t, filepath.Join(t.TempDir(), "factory.yaml"), "name:\n  - invalid\n")
				return []string{"--factory", path}
			},
			wants: []string{"factory.yaml", "(YAML)", "parse factory config"},
		},
		{
			name: "unsupported extension",
			prepare: func(t *testing.T) []string {
				path := writeFile(t, filepath.Join(t.TempDir(), "factory.toml"), "name = 'factory'\n")
				return []string{"--factory", path}
			},
			wants: []string{".json", ".yaml", ".yml"},
		},
		{
			name: "missing directory root",
			prepare: func(t *testing.T) []string {
				return []string{"--factory", t.TempDir()}
			},
			wants: []string{"factory.json", "factory.yaml", "factory.yml"},
		},
		{
			name: "ambiguous directory roots",
			prepare: func(t *testing.T) []string {
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, "factory.json"), "{}")
				writeFile(t, filepath.Join(dir, "factory.yaml"), "{}")
				return []string{"--factory", dir}
			},
			wants: []string{"factory.json", "factory.yaml", "ambiguous"},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			runner := support.NewRecordingCommandRunner("runtime must not execute")
			args := append([]string{"you", "run"}, test.prepare(t)...)
			args = append(args, "runtime must not start")
			inputs := support.FakeInputs(t.Context(), args)
			inputs.Input.WorkingDirectory = t.TempDir()
			err := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner}).Execute(inputs.Input)
			if err == nil {
				t.Fatal("Process.Execute() error = nil")
			}
			diagnostic := err.Error() + "\n" + inputs.Stderr()
			for _, want := range test.wants {
				if !strings.Contains(diagnostic, want) {
					t.Fatalf("diagnostic %q does not contain %q", diagnostic, want)
				}
			}
			if runner.CallCount() != 0 {
				t.Fatalf("provider command runner call count = %d, want 0", runner.CallCount())
			}
		})
	}
}

func materializePackagedGoal(t *testing.T, rootName string) string {
	t.Helper()
	dir := t.TempDir()
	extension := filepath.Ext(rootName)
	sourceName := "factory" + extension
	if extension == ".yml" {
		sourceName = "factory.yaml"
	}
	sourcePath := support.AgentFactoryPath(
		t,
		filepath.Join("packages", "packaged-factories", "generated", "factories", "goal", sourceName),
	)
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read packaged Goal %s: %v", sourcePath, err)
	}
	writeFile(t, filepath.Join(dir, rootName), string(data))

	promptPath := support.AgentFactoryPath(
		t,
		filepath.Join("packages", "packaged-factories", "factories", "goal", "prompts", "executor.md"),
	)
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read packaged Goal prompt %s: %v", promptPath, err)
	}
	writeFile(t, filepath.Join(dir, "prompts", "executor.md"), string(prompt))
	return dir
}

func validateFactory(t *testing.T, path string) {
	t.Helper()
	inputs := support.FakeInputs(
		t.Context(),
		[]string{"you", "factory", "config", "validate", path},
	)
	inputs.Input.WorkingDirectory = filepath.Dir(path)
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory config validate %s) error = %v\nstdout:\n%s\nstderr:\n%s",
			path,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
}

func flattenFactory(t *testing.T, path string) map[string]any {
	t.Helper()
	payload, err := support.FlattenFactoryConfig(t, path)
	if err != nil {
		t.Fatalf("flatten Factory %s: %v", path, err)
	}
	var factory map[string]any
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("decode flattened Factory %s: %v\npayload:\n%s", path, err, payload)
	}
	return factory
}

func invokeGoal(
	t *testing.T,
	env []string,
	workingDirectory string,
	sourceArgs ...string,
) string {
	t.Helper()
	mockWorkersPath := support.WriteMockWorkersConfig(t, &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      goalWorkerName,
			WorkstationName: goalWorkstationName,
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	})
	args := []string{"you", "run"}
	args = append(args, sourceArgs...)
	args = append(
		args,
		"--with-mock-workers", mockWorkersPath,
		"--no-record",
		"--quiet",
		"prove packaged YAML parity",
	)
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = workingDirectory
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s",
			args,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	if inputs.Stderr() != "" {
		t.Fatalf("Process.Execute(%v) stderr = %q, want empty", args, inputs.Stderr())
	}
	return inputs.Stdout()
}

func customerEnvironment(homeDir string) []string {
	return append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
}

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

func copyFile(t *testing.T, sourcePath, destinationPath string) {
	t.Helper()
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}
	writeFile(t, destinationPath, string(data))
}
