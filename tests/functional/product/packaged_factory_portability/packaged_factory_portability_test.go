package packaged_factory_portability

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	packagedGoalName              = "@you/goal"
	wantGoalInvocationPrimaryText = "mock worker accepted"
)

// TestPackagedFactoryInitMaterialization_InvokesOutsideRepositoryWithBootstrapParity
// proves packaged Factory init materializes outside the repository and invokes with
// bootstrap parity through the customer process boundary.
func TestPackagedFactoryInitMaterialization_InvokesOutsideRepositoryWithBootstrapParity(t *testing.T) {
	if testing.Short() {
		t.Skip("slow packaged Factory init portability functional path")
	}

	homeDir := t.TempDir()
	outsideWorkingDir := t.TempDir()
	installRoot := filepath.Join(t.TempDir(), "customer-factories")
	env := customerEnvironment(homeDir)

	materializedDir := initPackagedFactoryViaCLI(
		t,
		env,
		outsideWorkingDir,
		installRoot,
		"yaml",
		false,
	)
	factoryPath := filepath.Join(materializedDir, "factory.yaml")
	if _, err := os.Stat(factoryPath); err != nil {
		t.Fatalf("materialized YAML root missing at %s: %v", factoryPath, err)
	}
	assertRestoredGoalPromptAsset(t, materializedDir)
	assertPortableMaterializedLayout(t, materializedDir)
	if _, err := support.LoadedFactory(t, materializedDir); err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(materialized): %v", err)
	}

	materializedResponse := invokePackagedGoalFromWorkingDirectory(
		t,
		filepath.Join(materializedDir, "factory.yaml"),
		outsideWorkingDir,
		env,
		"materialized-init portability goal",
	)
	bootstrapDir := support.InstallPackagedFactory(t, homeDir, packagedGoalName)
	bootstrapResponse := invokePackagedGoalFromWorkingDirectory(
		t,
		filepath.Join(bootstrapDir, "factory.json"),
		outsideWorkingDir,
		env,
		"materialized-init portability goal",
	)
	assertPackagedGoalInvocationParity(t, bootstrapResponse, materializedResponse, wantGoalInvocationPrimaryText)

	marker := filepath.Join(materializedDir, "customer-owned.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write customer marker: %v", err)
	}
	var repeatStdout bytes.Buffer
	if err := executeInit(
		t,
		env,
		outsideWorkingDir,
		&repeatStdout,
		installRoot,
		"yaml",
		false,
	); err != nil {
		t.Fatalf("repeat init error = %v", err)
	}
	if got := repeatStdout.String(); !strings.Contains(got, "already installed") {
		t.Fatalf("repeat init stdout = %q, want already-installed skip", got)
	}
	if content, err := os.ReadFile(marker); err != nil || string(content) != "keep" {
		t.Fatalf("customer marker after repeat init = %q, %v", content, err)
	}

	emptyInputErr := postPackagedGoalInvocationExpectError(
		t,
		filepath.Join(materializedDir, "factory.yaml"),
		outsideWorkingDir,
		env,
	)
	if string(emptyInputErr.Code) != "INVOCATION_ARGUMENT_MISSING_REQUIRED_INPUT" {
		t.Fatalf(
			"empty invocation error code = %q, want INVOCATION_ARGUMENT_MISSING_REQUIRED_INPUT",
			emptyInputErr.Code,
		)
	}
}

func initPackagedFactoryViaCLI(
	t *testing.T,
	env []string,
	workingDirectory string,
	installRoot string,
	format string,
	replace bool,
) string {
	t.Helper()
	var stdout bytes.Buffer
	if err := executeInit(t, env, workingDirectory, &stdout, installRoot, format, replace); err != nil {
		t.Fatalf("init packaged Factory: %v\nstdout:\n%s", err, stdout.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Installed packaged factory "+packagedGoalName) {
		t.Fatalf("init stdout = %q, want packaged install success", got)
	}
	factoryDir := filepath.Join(installRoot, filepath.FromSlash(packagedGoalName))
	if _, err := os.Stat(factoryDir); err != nil {
		t.Fatalf("materialized Factory directory missing at %s: %v", factoryDir, err)
	}
	return factoryDir
}

func executeInit(
	t *testing.T,
	env []string,
	workingDirectory string,
	stdout io.Writer,
	installRoot string,
	format string,
	replace bool,
) error {
	t.Helper()
	args := []string{
		"you", "init",
		"--package", packagedGoalName,
		"--dir", installRoot,
	}
	if format != "" {
		args = append(args, "--format", format)
	}
	if replace {
		args = append(args, "--replace")
	}
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{})
	if err != nil {
		return err
	}
	return process.Execute(root.Input{
		Args:             args,
		Env:              env,
		Stdin:            strings.NewReader(""),
		Stdout:           stdout,
		Stderr:           io.Discard,
		Context:          t.Context(),
		WorkingDirectory: workingDirectory,
	})
}

func invokePackagedGoalFromWorkingDirectory(
	t *testing.T,
	factoryConfigPath string,
	workingDirectory string,
	env []string,
	goalText string,
) factoryapi.InvocationResponse {
	t.Helper()

	mockWorkersPath := writePackagedGoalMockWorkersConfig(t)
	args := []string{
		"you", "--json", "run",
		"--factory", factoryConfigPath,
		"--with-mock-workers=" + mockWorkersPath,
		"--no-record", goalText,
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append([]string(nil), env...)
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
	return support.DecodeInvocationResponseJSON(t, inputs.Stdout())
}

func postPackagedGoalInvocationExpectError(
	t *testing.T,
	factoryConfigPath string,
	workingDirectory string,
	env []string,
) factoryapi.ErrorResponse {
	t.Helper()

	args := []string{
		"you", "--json", "run",
		"--factory", factoryConfigPath,
		"--no-record",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = workingDirectory
	err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input)
	if err == nil {
		t.Fatalf("Process.Execute(%v) succeeded for empty required input", args)
	}
	if strings.TrimSpace(inputs.Stdout()) != "" {
		t.Fatalf("empty-input stdout = %q, want empty", inputs.Stdout())
	}
	var decoded factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stderr())), &decoded); err != nil {
		t.Fatalf("decode invocation error response: %v\nstderr:\n%s", err, inputs.Stderr())
	}
	return decoded
}

func writePackagedGoalMockWorkersConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mock-workers.json")
	payload, err := json.Marshal(packagedGoalMockWorkersConfig())
	if err != nil {
		t.Fatalf("marshal packaged goal mock-workers config: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write packaged goal mock-workers config: %v", err)
	}
	return path
}

func packagedGoalMockWorkersConfig() *workers.MockWorkersConfig {
	checkerCommand, checkerArgs := mockWorkerEchoCommand("plain")
	reviewerCommand, reviewerArgs := mockWorkerEchoCommand("accepted")
	return &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      "goal-planner",
				WorkstationName: "plan-goal",
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-executor",
				WorkstationName: "execute-goal",
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-checker",
				WorkstationName: "check-goal",
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: checkerCommand,
					Args:    checkerArgs,
				},
			},
			{
				WorkerName:      "goal-reviewer",
				WorkstationName: "review-goal",
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: reviewerCommand,
					Args:    reviewerArgs,
				},
			},
		},
	}
}

func mockWorkerEchoCommand(output string) (string, []string) {
	if runtime.GOOS == "windows" {
		literal := strings.ReplaceAll(output, "'", "''")
		return "powershell.exe", []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			"[Console]::Out.Write('" + literal + "')",
		}
	}
	return "/bin/echo", []string{output}
}

func assertPackagedGoalInvocationParity(
	t *testing.T,
	bootstrapResponse factoryapi.InvocationResponse,
	materializedResponse factoryapi.InvocationResponse,
	wantPrimaryResult string,
) {
	t.Helper()
	for label, response := range map[string]factoryapi.InvocationResponse{
		"bootstrap":    bootstrapResponse,
		"materialized": materializedResponse,
	} {
		if response.Status != factoryapi.InvocationTerminalStatusCompleted {
			t.Fatalf("%s status = %q, want COMPLETED", label, response.Status)
		}
		if got := invocationPrimaryResultText(t, response); got != wantPrimaryResult {
			t.Fatalf("%s primaryResult = %q, want %q", label, got, wantPrimaryResult)
		}
	}
	if invocationPrimaryResultText(t, bootstrapResponse) != invocationPrimaryResultText(t, materializedResponse) {
		t.Fatal("bootstrap and materialized primary results differ")
	}
}

func invocationPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	return part.Text
}

func assertRestoredGoalPromptAsset(t *testing.T, factoryDir string) {
	t.Helper()
	publishedPath := support.AgentFactoryPath(
		t,
		filepath.Join("packages", "packaged-factories", "factories", "goal", "prompts", "executor.md"),
	)
	published, err := os.ReadFile(publishedPath)
	if err != nil {
		t.Fatalf("read published goal prompt: %v", err)
	}
	materializedPath := filepath.Join(
		factoryDir,
		"workstations",
		"execute-goal",
		"prompts",
		"executor.md",
	)
	materialized, err := os.ReadFile(materializedPath)
	if err != nil {
		t.Fatalf("read materialized goal prompt: %v", err)
	}
	if string(materialized) != string(published) {
		t.Fatalf("materialized prompt differs from published content")
	}
	workerAgentsPath := filepath.Join(factoryDir, "workers", "goal-executor", "AGENTS.md")
	if _, err := os.Stat(workerAgentsPath); err != nil {
		t.Fatalf("materialized worker AGENTS.md missing at %s: %v", workerAgentsPath, err)
	}
}

func assertPortableMaterializedLayout(t *testing.T, factoryDir string) {
	t.Helper()
	repoRoot := testutil.MustRepoRoot(t)
	if same, err := pathsShareRoot(factoryDir, repoRoot); err != nil {
		t.Fatalf("compare factory dir to repository root: %v", err)
	} else if same {
		t.Fatalf("factory dir %q must live outside the repository", factoryDir)
	}
	err := filepath.WalkDir(factoryDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, forbidden := range []string{
			"packages/packaged-factories",
			"generated/factories",
			"node_modules",
		} {
			if strings.Contains(string(content), forbidden) {
				t.Fatalf("%s contains non-portable reference %q", path, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk materialized Factory: %v", err)
	}
}

func pathsShareRoot(left, right string) (bool, error) {
	leftAbs, err := filepath.Abs(left)
	if err != nil {
		return false, err
	}
	rightAbs, err := filepath.Abs(right)
	if err != nil {
		return false, err
	}
	leftRel, err := filepath.Rel(rightAbs, leftAbs)
	if err != nil {
		return false, err
	}
	return leftRel != ".." && !strings.HasPrefix(leftRel, ".."+string(os.PathSeparator)), nil
}

func customerEnvironment(homeDir string) []string {
	return append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
}
