package packaged_factory_portability

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	packagedGoalName              = "@you/goal"
	wantGoalInvocationPrimaryText = "mock worker accepted"
)

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
		materializedDir,
		filepath.Join(materializedDir, "factory.yaml"),
		outsideWorkingDir,
		env,
		"materialized-init portability goal",
	)
	bootstrapDir := support.InstallPackagedFactory(t, homeDir, packagedGoalName)
	bootstrapResponse := invokePackagedGoalFromWorkingDirectory(
		t,
		bootstrapDir,
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
		materializedDir,
		filepath.Join(materializedDir, "factory.yaml"),
		outsideWorkingDir,
		env,
		"   ",
	)
	if string(emptyInputErr.Code) != "INVOCATION_INPUT_EMPTY" {
		t.Fatalf("empty invocation error code = %q, want INVOCATION_INPUT_EMPTY", emptyInputErr.Code)
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
	factoryDir string,
	factoryConfigPath string,
	workingDirectory string,
	env []string,
	goalText string,
) factoryapi.InvocationResponse {
	t.Helper()

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		FactoryConfigPath:         factoryConfigPath,
		WorkingDirectory:          workingDirectory,
		WaitForServiceModeRuntime: true,
		MockWorkersConfig:         packagedGoalMockWorkersConfig(),
		Env:                       env,
	})
	defer server.Stop(t)

	return postPackagedGoalInvocation(t, server.URL(), goalText)
}

func postPackagedGoalInvocation(
	t *testing.T,
	serverURL string,
	goalText string,
) factoryapi.InvocationResponse {
	t.Helper()

	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: goalText,
	}); err != nil {
		t.Fatalf("build invocation text content: %v", err)
	}
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := factoryapi.WorkContent{part}
	body, err := json.Marshal(factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    &content,
	})
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	response, err := http.Post(
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/~default/invocations: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"POST /factory-sessions/~default/invocations status = %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(payload)),
		)
	}
	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

func postPackagedGoalInvocationExpectError(
	t *testing.T,
	factoryDir string,
	factoryConfigPath string,
	workingDirectory string,
	env []string,
	goalText string,
) factoryapi.ErrorResponse {
	t.Helper()

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		FactoryConfigPath:         factoryConfigPath,
		WorkingDirectory:          workingDirectory,
		WaitForServiceModeRuntime: true,
		MockWorkersConfig:         packagedGoalMockWorkersConfig(),
		Env:                       env,
	})
	defer server.Stop(t)

	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: goalText,
	}); err != nil {
		t.Fatalf("build invocation text content: %v", err)
	}
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := factoryapi.WorkContent{part}
	body, err := json.Marshal(factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    &content,
	})
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	response, err := http.Post(
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/~default/invocations: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"POST /factory-sessions/~default/invocations status = %d, want 400: %s",
			response.StatusCode,
			strings.TrimSpace(string(payload)),
		)
	}
	var decoded factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation error response: %v", err)
	}
	return decoded
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
