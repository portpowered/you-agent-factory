package smoke

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

func TestPackagedGoalRun_RealCLIWritesSummaryPrimaryResult(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI packaged goal invocation smoke")
	}

	dir := scaffoldPackagedGoalInvocationFactoryForSmoke(t)
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	submittedGoal := fmt.Sprintf("functional-smoke-packaged-goal-%d", time.Now().UnixNano())
	wantSummary := "mock worker accepted"

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		submittedGoal,
	)
	cmd.Dir = dir

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --factory packaged goal: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := stdout.String(); got != wantSummary {
		t.Fatalf("stdout = %q, want summary %q", got, wantSummary)
	}
	if strings.Contains(stdout.String(), submittedGoal) {
		t.Fatalf("stdout echoed submitted goal text %q", submittedGoal)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr on successful invocation", stderr.String())
	}
}

func scaffoldPackagedGoalInvocationFactoryForSmoke(t *testing.T) string {
	t.Helper()

	cfg := factoryPromptRunSmokeConfig()
	cfg["name"] = "@you/goal"
	cfg["invocationReturn"] = map[string]any{
		"policy":        "EXPLICIT",
		"workTypeName":  "goal",
		"terminalState": "complete",
	}
	workTypes := cfg["workTypes"].([]map[string]any)
	workTypes[0]["name"] = "goal"
	workstations := cfg["workstations"].([]map[string]any)
	workstations[0]["name"] = "execute-goal"
	workstations[0]["worker"] = "goal-executor"
	for _, ioKey := range []string{"inputs", "outputs", "onFailure"} {
		ios := workstations[0][ioKey].([]map[string]string)
		for i := range ios {
			ios[i]["workType"] = "goal"
		}
	}
	cfg["workers"] = []map[string]string{{"name": "goal-executor"}}

	dir := support.ScaffoldFactory(t, cfg)
	support.WriteAgentConfig(
		t,
		dir,
		"goal-executor",
		support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"),
	)
	return dir
}

func TestFactoryPromptRun_RealCLIWritesPrimaryResultFromPositionalText(t *testing.T) {
	dir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfig())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	prompt := fmt.Sprintf("functional-smoke-factory-prompt-%d", time.Now().UnixNano())

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		prompt,
	)
	cmd.Dir = dir

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --factory: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := stdout.String(); got != prompt {
		t.Fatalf("stdout = %q, want only primary result %q", got, prompt)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr on successful invocation", stderr.String())
	}
	functionalevidence.Covers(t, "cli/you.run")
}

func TestFactoryPromptRun_RealCLIWritesPrimaryResultFromStdin(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI factory prompt run smoke")
	}

	dir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfig())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	prompt := fmt.Sprintf("functional-smoke-stdin-factory-prompt-%d", time.Now().UnixNano())

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)
	cmd := exec.Command(
		binaryPath,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
	)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("you run --factory via stdin: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if got := stdout.String(); got != prompt {
		t.Fatalf("stdout = %q, want stdin primary result %q", got, prompt)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr on successful stdin invocation", stderr.String())
	}
}

func TestFactoryPromptRun_RealCLIRejectsConflictingPositionalAndStdinInput(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI factory prompt run smoke")
	}

	dir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfig())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)
	cmd := exec.Command(
		binaryPath,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		"from positional",
	)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("from stdin")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err == nil {
		t.Fatal("expected conflicting positional and stdin invocation inputs to fail")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on conflict failure", stdout.String())
	}
	if !strings.Contains(stderr.String(), "INVOCATION_INPUT_SOURCE_CONFLICT") {
		t.Fatalf("stderr = %q, want stable conflict code", stderr.String())
	}
}

func TestFactoryPromptRun_RealCLIFailureWritesNoSuccessPayloadToStdout(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI factory prompt run smoke")
	}

	dir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfigWithUnresolvedInvocationReturn())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)
	cmd := exec.Command(
		binaryPath,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		"trigger unresolved result",
	)
	cmd.Dir = dir

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err == nil {
		t.Fatal("expected unresolved invocation primary result to fail")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invocation failure", stdout.String())
	}
	if !strings.Contains(stderr.String(), "INVOCATION_PRIMARY_RESULT_UNRESOLVED") {
		t.Fatalf("stderr = %q, want stable unresolved-result code", stderr.String())
	}
}

func TestFactoryPromptRun_RealCLIRejectsFactoryWithoutDefaultHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI factory prompt run smoke")
	}

	dir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfigWithoutDefault())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)

	binaryPath := buildYouCLIBinary(t)
	cmd := exec.Command(
		binaryPath,
		"run",
		"--factory", factoryPath,
		"--no-record",
		"--quiet",
		"missing-default-handling",
	)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure without DEFAULT handling work type, output:\n%s", output)
	}
	combined := string(output)
	if !strings.Contains(combined, "handlingBehavior DEFAULT") {
		t.Fatalf("error output = %q, want handlingBehavior DEFAULT guidance", combined)
	}
}

func TestFactoryPromptRun_RealCLICleanInvocationStdoutRemainsPipeableAcrossRuns(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI factory prompt run smoke")
	}

	dir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfig())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)

	for _, prompt := range []string{
		"functional-clean-stdout-first",
		"functional-clean-stdout-second",
	} {
		stdout, stderr, err := runFactoryPromptCLI(t, dir, binaryPath, mockWorkersPath, nil, factoryPath, prompt)
		if err != nil {
			t.Fatalf("run clean invocation for prompt %q: %v\nstdout:\n%s\nstderr:\n%s", prompt, err, stdout, stderr)
		}
		assertFactoryPromptCleanInvocationStdout(t, stdout, prompt)
	}
}

func TestFactoryPromptRun_RealCLIStdinOnlyCleanInvocationStdoutRemainsPipeable(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI factory prompt run smoke")
	}

	dir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfig())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)

	stdout, stderr, err := runFactoryPromptCLI(
		t,
		dir,
		binaryPath,
		mockWorkersPath,
		strings.NewReader("functional-clean-stdin-only\n"),
		factoryPath,
	)
	if err != nil {
		t.Fatalf("run stdin-only clean invocation: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	assertFactoryPromptCleanInvocationStdout(t, stdout, "functional-clean-stdin-only")
}

func TestFactoryPromptRun_RealCLIAmbiguousPromptAndStdinFailsBeforeRuntimeStartup(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI factory prompt run smoke")
	}

	dir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfig())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)

	stdout, stderr, err := runFactoryPromptCLI(
		t,
		dir,
		binaryPath,
		mockWorkersPath,
		strings.NewReader("functional-clean-stdin-conflict\n"),
		factoryPath,
		"functional-clean-positional-conflict",
	)
	if err == nil {
		t.Fatalf("expected ambiguous stdin and prompt invocation to fail\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty on ambiguous input failure", stdout)
	}
	for _, want := range []string{
		"INVOCATION_INPUT_SOURCE_CONFLICT",
		"positional_text",
		"stdin_text",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr = %q, want %q", stderr, want)
		}
	}
}

func TestNamedFactoryRun_RealCLIResolvesGlobalFactoryFromUnrelatedWorkingDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named-factory run smoke")
	}

	homeDir := t.TempDir()

	sourceDir := support.ScaffoldFactory(t, factoryPromptRunSmokeConfig())
	loaded, err := factoryconfig.LoadRuntimeConfig(sourceDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(source): %v", err)
	}
	canonical, err := factoryconfig.MarshalCanonicalFactoryConfig(loaded.FactoryConfig())
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	globalRoot := defaultpaths.NamedFactoriesRoot(homeDir)
	namedFactoryDir, err := factoryconfig.PersistNamedFactory(globalRoot, "alpha", canonical)
	if err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}

	prompt := fmt.Sprintf("functional-smoke-named-factory-%d", time.Now().UnixNano())
	testutil.WriteSeedRequest(t, namedFactoryDir, work.SubmitRequest{
		WorkID:     "named-factory-smoke-work",
		WorkTypeID: defaultPromptRunWorkTypeName,
		TraceID:    "named-factory-smoke-trace",
		Payload:    []byte(prompt),
	})

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	binaryPath := buildYouCLIBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	unrelatedWorkingDir := t.TempDir()
	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"run",
		"--named", "alpha",
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--continuously",
		"--quiet",
		mockWorkersPath,
	)
	cmd.Dir = unrelatedWorkingDir
	cmd.Env = namedFactorySmokeEnvironment(homeDir)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start you run --named: %v", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	item, err := waitForFactoryPromptWorkComplete(ctx, baseURL, defaultPromptRunWorkTypeName, prompt, 20*time.Second)
	if err != nil {
		if waitErr := <-waitCh; waitErr != nil {
			t.Fatalf("you run --named: %v\nstdout:\n%s\nstderr:\n%s", waitErr, stdout.String(), stderr.String())
		}
		t.Fatalf("wait for completed named-factory work: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if stringPointerValue(item.WorkTypeName) != defaultPromptRunWorkTypeName {
		t.Fatalf("work type = %q, want %q", stringPointerValue(item.WorkTypeName), defaultPromptRunWorkTypeName)
	}
	if !factoryPromptRunWorkContentIncludes(item, prompt) {
		t.Fatalf("work content = %#v, want prompt text %q", item.Content, prompt)
	}

	cancel()
	_ = <-waitCh
}

const defaultPromptRunWorkTypeName = "prompt-task"

func factoryPromptRunSmokeConfig() map[string]any {
	return map[string]any{
		"name": "factory-prompt-run-smoke",
		"workTypes": []map[string]any{
			{
				"name":             defaultPromptRunWorkTypeName,
				"handlingBehavior": []string{"DEFAULT"},
				"states":           promptRunWorkTypeStates(),
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-prompt",
				"worker":    "mock-worker",
				"inputs":    []map[string]string{{"workType": defaultPromptRunWorkTypeName, "state": "init"}},
				"outputs":   []map[string]string{{"workType": defaultPromptRunWorkTypeName, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": defaultPromptRunWorkTypeName, "state": "failed"}},
			},
		},
	}
}

func factoryPromptRunSmokeConfigWithoutDefault() map[string]any {
	cfg := factoryPromptRunSmokeConfig()
	workTypes := cfg["workTypes"].([]map[string]any)
	withoutDefault := make([]map[string]any, len(workTypes))
	for i, workType := range workTypes {
		cloned := make(map[string]any, len(workType))
		for key, value := range workType {
			if key == "handlingBehavior" {
				continue
			}
			cloned[key] = value
		}
		withoutDefault[i] = cloned
	}
	cfg["workTypes"] = withoutDefault
	return cfg
}

func factoryPromptRunSmokeConfigWithUnresolvedInvocationReturn() map[string]any {
	cfg := factoryPromptRunSmokeConfig()
	cfg["invocationReturn"] = map[string]any{
		"policy":        "EXPLICIT",
		"workTypeName":  "summary",
		"terminalState": "complete",
	}
	cfg["workTypes"] = append(cfg["workTypes"].([]map[string]any), map[string]any{
		"name": "summary",
		"states": []map[string]string{
			{"name": "init", "type": "INITIAL"},
			{"name": "complete", "type": "TERMINAL"},
			{"name": "failed", "type": "FAILED"},
		},
	})
	return cfg
}

func promptRunWorkTypeStates() []map[string]string {
	return []map[string]string{
		{"name": "init", "type": "INITIAL"},
		{"name": "complete", "type": "TERMINAL"},
		{"name": "failed", "type": "FAILED"},
	}
}

func writeDefaultMockWorkersConfig(t *testing.T) string {
	t.Helper()

	data, err := json.MarshalIndent(factoryconfig.NewEmptyMockWorkersConfig(), "", "  ")
	if err != nil {
		t.Fatalf("marshal default mock-workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write mock-workers config: %v", err)
	}
	return path
}

func writePackagedGoalBuiltinMockWorkersConfig(t *testing.T) string {
	t.Helper()

	cfg := factoryconfig.MockWorkersConfig{
		MockWorkers: []factoryconfig.MockWorkerConfig{
			{
				WorkerName:      "goal-planner",
				WorkstationName: "plan-goal",
				RunType:         factoryconfig.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-executor",
				WorkstationName: "execute-goal",
				RunType:         factoryconfig.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-checker",
				WorkstationName: "check-goal",
				RunType:         factoryconfig.MockWorkerRunTypeScript,
				ScriptConfig: &factoryconfig.MockWorkerScriptConfig{
					Command: "/bin/echo",
					Args:    []string{"plain"},
				},
			},
			{
				WorkerName:      "goal-reviewer",
				WorkstationName: "review-goal",
				RunType:         factoryconfig.MockWorkerRunTypeScript,
				ScriptConfig: &factoryconfig.MockWorkerScriptConfig{
					Command: "/bin/echo",
					Args:    []string{"accepted"},
				},
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal packaged goal mock-workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers-packaged-goal.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write packaged goal mock-workers config: %v", err)
	}
	return path
}

func runFactoryPromptCLI(
	t *testing.T,
	dir string,
	binaryPath string,
	mockWorkersPath string,
	stdin *strings.Reader,
	factoryPath string,
	promptArgs ...string,
) (stdout string, stderr string, runErr error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	args := []string{
		"run",
		"--factory", factoryPath,
		"--with-mock-workers",
		"--no-record",
		"--quiet",
		mockWorkersPath,
	}
	args = append(args, promptArgs...)

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	cmd.Dir = dir
	if stdin != nil {
		cmd.Stdin = stdin
	}

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr = cmd.Run()
	return outBuf.String(), errBuf.String(), runErr
}

func assertFactoryPromptCleanInvocationStdout(t *testing.T, got string, want string) {
	t.Helper()

	got = strings.TrimSuffix(got, "\n")
	if got != want {
		t.Fatalf("stdout = %q, want exact primary clean invocation output", got)
	}
	for _, forbidden := range []string{
		"Factory initiated",
		"Dashboard URL",
		"Runtime log",
		"Opening dashboard",
		"Recording saved to",
		"Factory:",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("stdout = %q, should not contain operator chatter %q", got, forbidden)
		}
	}
}

func buildYouCLIBinary(t *testing.T) string {
	t.Helper()

	binaryName := "you"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/factory")
	build.Dir = testutil.MustRepoRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build you CLI: %v\n%s", err, string(output))
	}
	return binaryPath
}

func reserveLocalTCPPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", listener.Addr())
	}
	return addr.Port, nil
}

func waitForFactoryPromptWorkComplete(
	ctx context.Context,
	baseURL string,
	workTypeName string,
	wantPrompt string,
	timeout time.Duration,
) (factoryapi.Work, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return factoryapi.Work{}, ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, support.DefaultSessionWorkURL(baseURL, "/work"), nil)
		if err != nil {
			return factoryapi.Work{}, err
		}
		resp, err := client.Do(req)
		if err != nil {
			select {
			case <-ctx.Done():
				return factoryapi.Work{}, ctx.Err()
			case <-time.After(10 * time.Millisecond):
				continue
			}
		}

		var work factoryapi.ListWorkResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&work)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return factoryapi.Work{}, decodeErr
		}
		if resp.StatusCode != http.StatusOK {
			return factoryapi.Work{}, fmt.Errorf("GET /work status = %d", resp.StatusCode)
		}

		for _, item := range work.Results {
			if stringPointerValue(item.WorkTypeName) != workTypeName {
				continue
			}
			if factoryPromptRunWorkStateName(item.State) != "complete" {
				continue
			}
			if factoryPromptRunWorkStateType(item.State) != factoryapi.WorkStateTypeTERMINAL {
				continue
			}
			if !factoryPromptRunWorkContentIncludes(item, wantPrompt) {
				continue
			}
			return item, nil
		}

		select {
		case <-ctx.Done():
			return factoryapi.Work{}, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}

	return factoryapi.Work{}, fmt.Errorf("timed out waiting for completed %q work with prompt %q", workTypeName, wantPrompt)
}

func factoryPromptRunWorkContentIncludes(item factoryapi.Work, wantPrompt string) bool {
	if item.Content == nil {
		return false
	}
	for _, part := range *item.Content {
		textPart, err := part.AsWorkTextContentPart()
		if err != nil {
			continue
		}
		if textPart.Text == wantPrompt {
			return true
		}
	}
	return false
}

func factoryPromptRunWorkStateName(state *factoryapi.WorkState) string {
	if state == nil {
		return ""
	}
	return state.Name
}

func factoryPromptRunWorkStateType(state *factoryapi.WorkState) factoryapi.WorkStateType {
	if state == nil {
		return ""
	}
	return state.Type
}
