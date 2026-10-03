package yaml_parity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	goalFactoryName      = "portable-goal"
	goalWorkerName       = "goal-executor"
	goalWorkstationName  = "execute-goal"
	wantInvocationResult = "controlled Codex accepted"
)

// TestCLIFactoryJSONAndYAMLValidateFlattenAndRunParity proves validate, flatten,
// and run behave equivalently for representative packaged Factory sources
// authored as JSON and YAML, and each yields the same public primary result.
func TestCLIFactoryJSONAndYAMLValidateFlattenAndRunParity(t *testing.T) {
	baseURL := startAuthoredSourceHost(t)
	jsonDir := materializePackagedGoal(t, "factory.json")
	yamlDir := materializePackagedGoal(t, "factory.yaml")

	jsonPath := filepath.Join(jsonDir, "factory.json")
	yamlPath := filepath.Join(yamlDir, "factory.yaml")
	for _, path := range []string{jsonPath, yamlPath} {
		validateFactory(t, path)
	}

	jsonFactory := flattenFactory(t, jsonPath)
	if got := flattenFactory(t, yamlPath); !reflect.DeepEqual(got, jsonFactory) {
		t.Fatalf("flattened Factory from %s differs from packaged JSON", yamlPath)
	}

	for _, source := range []struct {
		name string
		args []string
	}{
		{name: "explicit JSON", args: []string{"--factory", jsonPath}},
		{name: "explicit YAML", args: []string{"--factory", yamlPath}},
	} {
		source := source
		t.Run(source.name, func(t *testing.T) {
			t.Parallel()
			if got := invokeGoal(t, baseURL, customerEnvironment(t.TempDir()), t.TempDir(), source.args...); got != wantInvocationResult {
				t.Fatalf("invocation result = %q, want %q", got, wantInvocationResult)
			}
		})
	}
	t.Run("YAML create and JSON update", func(t *testing.T) {
		t.Parallel()
		runYAMLCreateAndUpdate(t, baseURL)
	})
	t.Run("TestCLIFactoryRejectedAuthoredSourcesFailBeforeRuntimeExecution", func(t *testing.T) {
		t.Parallel()
		runRejectedAuthoredSources(t, baseURL)
	})
}

// TestCLIFactoryYAMLCreateAndUpdateRemainRunnableAfterCanonicalPersistence proves
// a named Factory created from YAML and later updated from JSON remains runnable
// via the public named-factory run path after the CLI persists the canonical
// factory.json durable form.
func runYAMLCreateAndUpdate(t *testing.T, baseURL string) {
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	yamlSource := filepath.Join(materializePackagedGoal(t, "factory.yaml"), "factory.yaml")
	jsonSource := filepath.Join(materializePackagedGoal(t, "factory.json"), "factory.json")

	factoryDir := support.CreateNamedFactoryWithProcess(
		t,
		yamlParityCLIProcess,
		homeDir,
		workingDirectory,
		goalFactoryName,
		yamlSource,
	)
	env := customerEnvironment(homeDir)
	if got := invokeGoal(t, baseURL, env, workingDirectory, "--named", goalFactoryName); got != wantInvocationResult {
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
	if err := yamlParityCLIProcess.Execute(update.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory update) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			update.Stdout(),
			update.Stderr(),
		)
	}
	if got := invokeGoal(t, baseURL, env, workingDirectory, "--named", goalFactoryName); got != wantInvocationResult {
		t.Fatalf("JSON-updated invocation result = %q, want %q", got, wantInvocationResult)
	}
}

// TestCLIFactoryRejectedAuthoredSourcesFailBeforeRuntimeExecution proves malformed,
// mismatched, unsupported, missing, and ambiguous authored Factory sources are
// rejected by the public CLI before provider/runtime execution and retain
// actionable source context in public diagnostics.
func runRejectedAuthoredSources(t *testing.T, baseURL string) {
	t.Helper()
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
			t.Parallel()
			sourceArgs := test.prepare(t)
			before := captureAuthoredSource(t, sourceArgs[1])
			args := append([]string{"you", "run"}, sourceArgs...)
			args = append(args, "runtime must not start")
			inputs := support.FakeInputs(t.Context(), args)
			inputs.Input.WorkingDirectory = t.TempDir()
			inputs.Input.Env = customerEnvironment(t.TempDir())
			err := yamlParityCLIProcess.Execute(inputs.Input)
			if err == nil {
				t.Fatal("Process.Execute() error = nil")
			}
			diagnostic := err.Error() + "\n" + inputs.Stderr()
			for _, want := range test.wants {
				if !strings.Contains(diagnostic, want) {
					t.Fatalf("diagnostic %q does not contain %q", diagnostic, want)
				}
			}
			if got := yamlParityCommands.callCount(inputs.Input.WorkingDirectory); got != 0 {
				t.Fatalf("rejected source dispatched %d provider commands, want zero", got)
			}
			assertRemoteSourceRejectedWithoutWork(t, baseURL, sourceArgs, test.wants)
			after := captureAuthoredSource(t, sourceArgs[1])
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("rejected authored source changed: before=%q after=%q", before, after)
			}
		})
	}
}

func assertRemoteSourceRejectedWithoutWork(t *testing.T, baseURL string, sourceArgs, wants []string) {
	t.Helper()
	validDir := materializePackagedGoal(t, "factory.json")
	opened := support.OpenFactorySessionAt(t, baseURL, validDir)
	sessionID := opened.Session.Id
	runner := testutil.NewProviderCommandRunner()
	yamlParityCommands.registerSession(t, sessionID, runner)
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, sessionID) })
	workURL := baseURL + "/factory-sessions/" + sessionID + "/work"
	if work := support.GetJSON[factoryapi.ListWorkResponse](t, workURL); len(work.Results) != 0 {
		t.Fatalf("new session has Work before rejected input: %#v", work.Results)
	}
	args := []string{"you", "--remote", "--server", baseURL, "run", "--session", sessionID}
	args = append(args, sourceArgs...)
	args = append(args, "runtime must not start")
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = customerEnvironment(t.TempDir())
	inputs.Input.WorkingDirectory = t.TempDir()
	err := yamlParityCLIProcess.Execute(inputs.Input)
	if err == nil {
		t.Fatal("remote source rejection returned no error")
	}
	diagnostic := err.Error() + "\n" + inputs.Stderr()
	for _, want := range wants {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf("remote diagnostic %q does not contain %q", diagnostic, want)
		}
	}
	if work := support.GetJSON[factoryapi.ListWorkResponse](t, workURL); len(work.Results) != 0 {
		t.Fatalf("rejected input admitted Work: %#v", work.Results)
	}
	if calls := runner.CallCount(); calls != 0 {
		t.Fatalf("rejected input dispatched %d session provider commands", calls)
	}
}

func captureAuthoredSource(t *testing.T, path string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(path, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		files[name] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("read authored source %s: %v", path, err)
	}
	return files
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
	inputs.Input.Env = customerEnvironment(t.TempDir())
	if err := yamlParityCLIProcess.Execute(inputs.Input); err != nil {
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
	payload, err := support.FlattenFactoryConfigWithProcessAndEnv(t, yamlParityCLIProcess, customerEnvironment(t.TempDir()), path)
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
	baseURL string,
	env []string,
	workingDirectory string,
	sourceArgs ...string,
) string {
	t.Helper()
	factoryDir := filepath.Dir(sourceArgs[1])
	if sourceArgs[0] == "--factory" {
		// Session opening consumes canonical layouts. Persist the selected authored
		// source through the public create command before opening; run still uses
		// the original JSON/YAML selection and its invocation signature.
		factoryDir = support.CreateNamedFactoryWithProcess(t, yamlParityCLIProcess,
			t.TempDir(), workingDirectory, goalFactoryName, sourceArgs[1])
	}
	if sourceArgs[0] == "--named" {
		for _, entry := range env {
			if strings.HasPrefix(entry, "HOME=") {
				factoryDir = filepath.Join(strings.TrimPrefix(entry, "HOME="), ".you-agent-factory", "factories", sourceArgs[1])
			}
		}
	}
	opened := support.OpenFactorySessionAt(t, baseURL, factoryDir)
	sessionID := opened.Session.Id
	runner := testutil.NewProviderCommandRunner(support.CodexDecisionCommandResult(wantInvocationResult))
	yamlParityCommands.registerSession(t, sessionID, runner)
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, sessionID) })
	args := []string{"you", "--remote", "--server", baseURL, "run", "--session", sessionID}
	args = append(args, sourceArgs...)
	args = append(args, "--no-record", "--quiet", "--executor-provider", "codex", "prove packaged YAML parity")
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = workingDirectory
	if err := yamlParityCLIProcess.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("Process.Execute(%v) stderr = %q, want empty", args, inputs.Stderr())
	}
	calls := runner.Requests()
	if len(calls) != 1 || calls[0].Command != "codex" || calls[0].ExecutionScopeID != sessionID {
		t.Fatalf("provider requests = %#v, want one Codex request scoped to %s", calls, sessionID)
	}
	if !strings.Contains(string(calls[0].Stdin), "prove packaged YAML parity") {
		t.Fatalf("provider prompt = %q, want submitted Work input", calls[0].Stdin)
	}
	work := support.GetJSON[factoryapi.ListWorkResponse](t,
		baseURL+"/factory-sessions/"+sessionID+"/work")
	if len(work.Results) != 1 || work.Results[0].State == nil ||
		work.Results[0].State.Name != "complete" || work.Results[0].State.Type != "TERMINAL" {
		t.Fatalf("session Work = %#v, want one completed terminal Work item", work.Results)
	}
	return inputs.Stdout()
}

func customerEnvironment(homeDir string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") || strings.EqualFold(name, "HOMEDRIVE") || strings.EqualFold(name, "HOMEPATH") {
			continue
		}
		env = append(env, entry)
	}
	return append(env, "HOME="+homeDir, "USERPROFILE="+homeDir)
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
