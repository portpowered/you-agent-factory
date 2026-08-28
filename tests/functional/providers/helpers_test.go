package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	providercontract "github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	providerbase "github.com/portpowered/infinite-you/tests/functional/providers/base"
)

const commandRunnerCompletedLogEvent = "command_runner.completed"

const sharedProviderFixtureTimeout = providerbase.FixtureTimeout

const (
	sharedMockAgentAcceptWorkID   = "shared-mock-agent-accept"
	sharedMockAgentRejectWorkID   = "shared-mock-agent-reject"
	sharedMockScriptAcceptWorkID  = "shared-mock-script-accept"
	sharedMockScriptRejectWorkID  = "shared-mock-script-reject"
	sharedMockServiceModelWorkID  = "shared-mock-service-model"
	sharedMockServiceScriptWorkID = "shared-mock-service-script"
)

type sharedProviderProcessFixture = providerbase.ProcessFixture
type sharedProviderScenario struct {
	*providerbase.Scenario
}

func (scenario *sharedProviderScenario) stop(t *testing.T) {
	t.Helper()
	scenario.Stop(t)
}

func (scenario *sharedProviderScenario) waitForTerminal(t testing.TB, timeout time.Duration) {
	t.Helper()
	scenario.WaitForTerminal(t, timeout)
}

func (scenario *sharedProviderScenario) listWork(t testing.TB) factoryapi.ListWorkResponse {
	t.Helper()
	return scenario.ListWork(t)
}

func (scenario *sharedProviderScenario) factoryEvents(t testing.TB) []factoryapi.FactoryEvent {
	t.Helper()
	return scenario.FactoryEvents(t)
}

func TestMain(m *testing.M) {
	code := m.Run()
	if err := providerbase.CloseGlobalFixture(); err != nil {
		fmt.Fprintf(os.Stderr, "close shared provider fixture: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

type fakeCommandRunner struct {
	stdout   string
	stderr   string
	exitCode int
}

func (f *fakeCommandRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: []byte(f.stdout), Stderr: []byte(f.stderr), ExitCode: f.exitCode}, nil
}

type canceledCommandRunner struct{}

func (canceledCommandRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, context.Canceled
}

type captureCommandRunner struct {
	mu       sync.Mutex
	workDirs []string
	envs     [][]string
}

func (r *captureCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.workDirs = append(r.workDirs, req.WorkDir)
	copiedEnv := make([]string, len(req.Env))
	copy(copiedEnv, req.Env)
	r.envs = append(r.envs, copiedEnv)
	r.mu.Unlock()
	return platformprocess.CommandResult{Stdout: []byte("script-output-ok")}, nil
}

func (r *captureCommandRunner) LastWorkDir() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.workDirs) == 0 {
		return ""
	}
	return r.workDirs[len(r.workDirs)-1]
}

func (r *captureCommandRunner) LastEnv() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.envs) == 0 {
		return nil
	}
	copied := make([]string, len(r.envs[len(r.envs)-1]))
	copy(copied, r.envs[len(r.envs)-1])
	return copied
}

func (r *captureCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.workDirs)
}

type timeoutThenSuccessCommandRunner struct {
	mu        sync.Mutex
	callCount int
}

func newTimeoutThenSuccessCommandRunner() *timeoutThenSuccessCommandRunner {
	return &timeoutThenSuccessCommandRunner{}
}

func (r *timeoutThenSuccessCommandRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.callCount++
	call := r.callCount
	r.mu.Unlock()

	if call == 1 {
		<-ctx.Done()
		return platformprocess.CommandResult{}, ctx.Err()
	}

	return platformprocess.CommandResult{Stdout: []byte("script-output-after-retry")}, nil
}

func (r *timeoutThenSuccessCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callCount
}

type echoArgsRunner struct{}

func (e *echoArgsRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: []byte(strings.Join(req.Args, "\n"))}, nil
}

type templateCaptureCommandRunner struct {
	mu      sync.Mutex
	request platformprocess.CommandRequest
}

func (r *templateCaptureCommandRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.request = req
	r.mu.Unlock()

	return platformprocess.CommandResult{Stdout: []byte(strings.Join(req.Args, "\n"))}, nil
}

func (r *templateCaptureCommandRunner) LastRequest() platformprocess.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.request
}

func failureRunner(stderr string) platformprocess.CommandRunner {
	return &fakeCommandRunner{stderr: stderr, exitCode: 1}
}

func sharedProviderFixtureFor(t *testing.T) *sharedProviderProcessFixture {
	t.Helper()
	return providerbase.FixtureFor(t)
}

type providerResultCommandRunner struct {
	result platformprocess.CommandResult
	err    error
}

func (runner providerResultCommandRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.result, runner.err
}

func sharedProviderRefusalRunner() platformprocess.CommandRunner {
	return providerResultCommandRunner{result: platformprocess.CommandResult{
		ExitCode: 7,
	}, err: providercontract.ExecuteFailure{
		Kind:    providercontract.ExecuteFailureKindInvalidRequest,
		Message: "provider error: permanent_bad_request: provider rejected the execution request",
	}}
}

func sharedScriptFailureRunner() platformprocess.CommandRunner {
	return providerResultCommandRunner{result: platformprocess.CommandResult{
		Stdout:   []byte("script configured stdout"),
		Stderr:   []byte("script configured stderr"),
		ExitCode: 9,
	}}
}

func configureSharedCodexWorker(t *testing.T, dir string) {
	t.Helper()
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))
}

func runSharedProviderFactory(
	t *testing.T,
	dir, workDir string,
	runner platformprocess.CommandRunner,
	timeout time.Duration,
) (*sharedProviderScenario, factoryapi.ListWorkResponse) {
	t.Helper()
	scenario, listed := providerbase.RunFactory(t, dir, workDir, runner, timeout)
	return &sharedProviderScenario{Scenario: scenario}, listed
}

func runSharedMockFactory(
	t *testing.T,
	dir string,
	runner platformprocess.CommandRunner,
	timeout time.Duration,
) (*sharedProviderScenario, factoryapi.ListWorkResponse) {
	t.Helper()
	return runSharedProviderFactory(t, dir, dir, runner, timeout)
}

func findSharedRuntimeLogRecord(
	t *testing.T,
	fixture *sharedProviderProcessFixture,
	workDir string,
	exitCode int,
) map[string]any {
	t.Helper()
	return providerbase.FindRuntimeLogRecord(t, fixture, workDir, exitCode)
}

func updateScriptFixtureFactory(t *testing.T, dir string, mutate func(map[string]any)) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}

	mutate(cfg)

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func writeWorkstationPromptTemplate(t *testing.T, dir, templateBody string) {
	t.Helper()

	writeNamedWorkstationPromptTemplate(t, dir, "run-script", templateBody)
}

func writeNamedWorkstationPromptTemplate(t *testing.T, dir, workstationName, templateBody string) {
	t.Helper()

	path := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	content := "---\ntype: MODEL_WORKSTATION\n---\n" + templateBody + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func writeFixtureFile(t *testing.T, dir string, pathParts []string, content string) {
	t.Helper()

	path := filepath.Join(append([]string{dir}, pathParts...)...)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeScriptWorkerArgs(t *testing.T, dir string, args []string) {
	t.Helper()

	lines := []string{"---", "type: SCRIPT_WORKER", "command: echo", "args:"}
	for _, arg := range args {
		lines = append(lines, "  - "+quoteYAMLString(arg))
	}
	lines = append(lines, "---", "Execute the script.")
	writeFixtureFile(t, dir, []string{"workers", "script-worker", "AGENTS.md"}, strings.Join(lines, "\n")+"\n")
}

func writeRuntimeMergeWorkstationConfig(t *testing.T, dir string) {
	t.Helper()

	body := strings.Join([]string{
		`runtime prompt name={{ (index .Inputs 0).Name }}`,
		`runtime prompt work={{ (index .Inputs 0).WorkID }}`,
		`runtime prompt workdir={{ .Context.WorkDir }}`,
		`runtime prompt env={{ index .Context.Env "RUNTIME_BRANCH" }}`,
	}, "\n")
	agentsMD := strings.Join([]string{
		"---",
		"type: MODEL_WORKSTATION",
		"worker: script-worker",
		"outputs:",
		"  - workType: task",
		"    state: runtime-done",
		`workingDirectory: '/runtime/{{ (index .Inputs 0).Name }}/{{ index (index .Inputs 0).Tags "branch" }}'`,
		`worktree: 'worktrees/{{ index (index .Inputs 0).Tags "branch" }}/{{ (index .Inputs 0).WorkID }}'`,
		"env:",
		`  RUNTIME_BRANCH: '{{ index (index .Inputs 0).Tags "branch" }}'`,
		`  RUNTIME_NAME: '{{ (index .Inputs 0).Name }}'`,
		"---",
		body,
	}, "\n") + "\n"

	writeFixtureFile(t, dir, []string{"workstations", "run-script", "AGENTS.md"}, agentsMD)
}

func quoteYAMLString(value string) string {
	return strconv.Quote(value)
}

func assertCommandArgs(t *testing.T, req platformprocess.CommandRequest, want []string) {
	t.Helper()

	if !reflect.DeepEqual(req.Args, want) {
		t.Fatalf("command args = %#v, want %#v", req.Args, want)
	}
}

func assertRuntimeMergeCommandRequest(t *testing.T, dir string, req platformprocess.CommandRequest) {
	t.Helper()

	if req.Command != "echo" {
		t.Fatalf("command = %q, want %q", req.Command, "echo")
	}
	if req.WorkDir != support.ResolvedRuntimePath(dir, "/runtime/runtime-template-name/feature-runtime-config") {
		t.Fatalf("work dir = %q, want resolved runtime working_directory", req.WorkDir)
	}
	for _, want := range []string{
		"INLINE_ONLY=true",
		"RUNTIME_BRANCH=feature-runtime-config",
		"RUNTIME_NAME=runtime-template-name",
	} {
		if !containsEnv(req.Env, want) {
			t.Fatalf("script runner env missing %s in %v", want, req.Env)
		}
	}
}

func findRuntimeLogRecord(t *testing.T, path, eventName string) map[string]any {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open runtime log %s: %v", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("decode runtime log record: %v", err)
		}
		if record["event_name"] == eventName {
			return record
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan runtime log %s: %v", path, err)
	}
	t.Fatalf("runtime log %s did not contain event_name %q", path, eventName)
	return nil
}

func containsEnv(env []string, expected string) bool {
	for _, entry := range env {
		if entry == expected {
			return true
		}
	}
	return false
}

func assertSessionPlaces(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}

func assertDispatchOutput(t *testing.T, events []factoryapi.FactoryEvent, want string) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Output == nil || *payload.Output != want {
			t.Fatalf("dispatch output = %#v, want %q", payload.Output, want)
		}
		return
	}
	t.Fatalf("Factory Event history has no dispatch response: %#v", events)
}
