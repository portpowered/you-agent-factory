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

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type fakeCommandRunner struct {
	stdout   string
	stderr   string
	exitCode int
}

func (f *fakeCommandRunner) Run(_ context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{Stdout: []byte(f.stdout), Stderr: []byte(f.stderr), ExitCode: f.exitCode}, nil
}

type captureCommandRunner struct {
	mu       sync.Mutex
	workDirs []string
	envs     [][]string
}

func (r *captureCommandRunner) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.workDirs = append(r.workDirs, req.WorkDir)
	copiedEnv := make([]string, len(req.Env))
	copy(copiedEnv, req.Env)
	r.envs = append(r.envs, copiedEnv)
	r.mu.Unlock()
	return workers.CommandResult{Stdout: []byte("script-output-ok")}, nil
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

type timeoutThenSuccessCommandRunner struct {
	mu        sync.Mutex
	callCount int
}

func newTimeoutThenSuccessCommandRunner() *timeoutThenSuccessCommandRunner {
	return &timeoutThenSuccessCommandRunner{}
}

func (r *timeoutThenSuccessCommandRunner) Run(ctx context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.callCount++
	call := r.callCount
	r.mu.Unlock()

	if call == 1 {
		<-ctx.Done()
		return workers.CommandResult{}, ctx.Err()
	}

	return workers.CommandResult{Stdout: []byte("script-output-after-retry")}, nil
}

func (r *timeoutThenSuccessCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callCount
}

type echoArgsRunner struct{}

func (e *echoArgsRunner) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{Stdout: []byte(strings.Join(req.Args, "\n"))}, nil
}

type templateCaptureCommandRunner struct {
	mu      sync.Mutex
	request workers.CommandRequest
}

func (r *templateCaptureCommandRunner) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.request = req
	r.mu.Unlock()

	return workers.CommandResult{Stdout: []byte(strings.Join(req.Args, "\n"))}, nil
}

func (r *templateCaptureCommandRunner) LastRequest() workers.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.request
}

func successRunner(stdout string) workers.CommandRunner {
	return &fakeCommandRunner{stdout: stdout, exitCode: 0}
}

func failureRunner(stderr string) workers.CommandRunner {
	return &fakeCommandRunner{stderr: stderr, exitCode: 1}
}

func buildModelWorkerConfig(provider workers.ModelProvider, model string) string {
	return fmt.Sprintf(`---
type: MODEL_WORKER
model: %s
modelProvider: %s
stopToken: COMPLETE
---
Process the input task.
`, model, provider)
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

func writeNamedWorkerAgents(t *testing.T, dir, workerName, content string) {
	t.Helper()

	support.WriteAgentConfig(t, dir, workerName, content)
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

func writeExecutionTemplateWorkstationAgents(t *testing.T, dir, workstationName string) {
	t.Helper()

	agentsMD := strings.Join([]string{
		"---",
		"type: MODEL_WORKSTATION",
		`workingDirectory: '/workspace/{{ (index .Inputs 0).Name }}/{{ index (index .Inputs 0).Tags "branch" }}'`,
		`worktree: 'worktrees/{{ index (index .Inputs 0).Tags "branch" }}/{{ (index .Inputs 0).WorkID }}'`,
		"env:",
		`  TEMPLATE_BRANCH: '{{ index (index .Inputs 0).Tags "branch" }}'`,
		`  TEMPLATE_NAME: '{{ (index .Inputs 0).Name }}'`,
		`  TEMPLATE_PAYLOAD: '{{ (index .Inputs 0).Payload }}'`,
		`  TEMPLATE_WORKID: '{{ (index .Inputs 0).WorkID }}'`,
		"---",
		executionTemplatePrompt(),
	}, "\n") + "\n"
	writeFixtureFile(t, dir, []string{"workstations", workstationName, "AGENTS.md"}, agentsMD)
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

func configureResourceGatedTemplateWorkstation(t *testing.T, dir string) {
	t.Helper()

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["resources"] = []any{
			map[string]any{"name": "aaa-slot", "capacity": 1},
			map[string]any{"name": "zzz-slot", "capacity": 1},
		}

		workstations := cfg["workstations"].([]any)
		workstation := workstations[0].(map[string]any)
		workstation["resources"] = []any{
			map[string]any{"name": "aaa-slot", "capacity": 1},
			map[string]any{"name": "zzz-slot", "capacity": 1},
		}
	})
}

func configureExecutionTemplateWorkstation(t *testing.T, dir string) {
	t.Helper()

	workstationName := ""
	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["resources"] = []any{
			map[string]any{"name": "template-slot", "capacity": 1},
		}

		workstations := cfg["workstations"].([]any)
		workstation := workstations[0].(map[string]any)
		workstationName = workstation["name"].(string)
		workstation["resources"] = []any{
			map[string]any{"name": "template-slot", "capacity": 1},
		}
	})
	writeExecutionTemplateWorkstationAgents(t, dir, workstationName)
}

func configureTwoInputResourceGatedTemplateWorkstation(t *testing.T, dir, workstationName, workerName string) {
	t.Helper()

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["workTypes"] = []any{
			map[string]any{
				"name": "zeta-resource",
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "done", "type": "TERMINAL"},
					map[string]any{"name": "failed", "type": "FAILED"},
				},
			},
			map[string]any{
				"name": "alpha-resource",
				"states": []any{
					map[string]any{"name": "init", "type": "INITIAL"},
					map[string]any{"name": "done", "type": "TERMINAL"},
					map[string]any{"name": "failed", "type": "FAILED"},
				},
			},
		}
		cfg["resources"] = []any{
			map[string]any{"name": "repo-slot", "capacity": 1},
			map[string]any{"name": "gpu-slot", "capacity": 1},
		}
		cfg["workers"] = []any{map[string]any{"name": workerName}}
		cfg["workstations"] = []any{map[string]any{
			"name":   workstationName,
			"worker": workerName,
			"inputs": []any{
				map[string]any{"workType": "zeta-resource", "state": "init"},
				map[string]any{"workType": "alpha-resource", "state": "init"},
			},
			"outputs": []any{
				map[string]any{"workType": "zeta-resource", "state": "done"},
				map[string]any{"workType": "alpha-resource", "state": "done"},
			},
			"onFailure": map[string]any{"workType": "zeta-resource", "state": "failed"},
			"resources": []any{map[string]any{"name": "repo-slot", "capacity": 1}, map[string]any{"name": "gpu-slot", "capacity": 1}},
		}}
	})
}

func writeTwoInputResourceSeeds(t *testing.T, dir string) {
	t.Helper()

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "zeta-input-name",
		WorkID:     "zeta-work",
		WorkTypeID: "zeta-resource",
		TraceID:    "trace-two-input-resources",
		Payload:    []byte("zeta-payload"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "alpha-input-name",
		WorkID:     "alpha-work",
		WorkTypeID: "alpha-resource",
		TraceID:    "trace-two-input-resources",
		Payload:    []byte("alpha-payload"),
	})
}

func writeExecutionTemplateSeed(t *testing.T, dir string) {
	t.Helper()

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "execution-template-name",
		WorkID:     "work-execution-template",
		WorkTypeID: "task",
		TraceID:    "trace-execution-template",
		Payload:    []byte("execution-template-payload"),
		Tags: map[string]string{
			"branch": "feature-token-branch",
		},
	})
}

func twoInputTemplateArgs() []string {
	return []string{
		`first_name={{ (index .Inputs 0).Name }}`,
		`first_payload={{ (index .Inputs 0).Payload }}`,
		`second_name={{ (index .Inputs 1).Name }}`,
		`second_payload={{ (index .Inputs 1).Payload }}`,
		`inputs={{ len .Inputs }}`,
	}
}

func executionTemplatePrompt() string {
	return strings.Join([]string{
		`name={{ (index .Inputs 0).Name }}`,
		`payload={{ (index .Inputs 0).Payload }}`,
		`context_workdir={{ .Context.WorkDir }}`,
		`env_branch={{ index .Context.Env "TEMPLATE_BRANCH" }}`,
		`env_workid={{ index .Context.Env "TEMPLATE_WORKID" }}`,
		`inputs={{ len .Inputs }}`,
	}, "\n")
}

func executionTemplateWantPrompt(dir string) string {
	return strings.Join([]string{
		"name=execution-template-name",
		"payload=execution-template-payload",
		"context_workdir=" + support.ResolvedRuntimePath(dir, "/workspace/execution-template-name/feature-token-branch"),
		"env_branch=feature-token-branch",
		"env_workid=work-execution-template",
		"inputs=1",
	}, "\n")
}

func quoteYAMLString(value string) string {
	return strconv.Quote(value)
}

func assertCommandArgs(t *testing.T, req workers.CommandRequest, want []string) {
	t.Helper()

	if !reflect.DeepEqual(req.Args, want) {
		t.Fatalf("command args = %#v, want %#v", req.Args, want)
	}
}

func assertProviderArgsPrompt(t *testing.T, req workers.CommandRequest, want string) {
	t.Helper()

	if len(req.Args) == 0 {
		t.Fatal("provider args were empty")
	}
	if got := req.Args[len(req.Args)-1]; got != want {
		t.Fatalf("provider prompt arg = %q, want %q", got, want)
	}
}

func assertProviderStdin(t *testing.T, req workers.CommandRequest, want string) {
	t.Helper()

	if got := string(req.Stdin); got != want {
		t.Fatalf("provider stdin = %q, want %q", got, want)
	}
}

func assertProviderExecutionFields(t *testing.T, dir string, req workers.CommandRequest) {
	t.Helper()

	if req.WorkDir != support.ResolvedRuntimePath(dir, "/workspace/execution-template-name/feature-token-branch") {
		t.Fatalf("provider work dir = %q, want resolved workstation working_directory", req.WorkDir)
	}
	for _, want := range []string{
		"TEMPLATE_BRANCH=feature-token-branch",
		"TEMPLATE_NAME=execution-template-name",
		"TEMPLATE_PAYLOAD=execution-template-payload",
		"TEMPLATE_WORKID=work-execution-template",
	} {
		if !containsEnv(req.Env, want) {
			t.Fatalf("provider env missing %s in %v", want, req.Env)
		}
	}
}

func assertRuntimeMergeCommandRequest(t *testing.T, dir string, req workers.CommandRequest) {
	t.Helper()

	if req.Command != "echo" {
		t.Fatalf("command = %q, want %q", req.Command, "echo")
	}
	if req.WorkDir != support.ResolvedRuntimePath(dir, "/runtime/runtime-template-name/feature-runtime-config") {
		t.Fatalf("work dir = %q, want resolved runtime working_directory", req.WorkDir)
	}
	if req.WorkstationName != "run-script" {
		t.Fatalf("workstation name = %q, want run-script", req.WorkstationName)
	}
	if req.WorkerType != "script-worker" {
		t.Fatalf("worker type = %q, want script-worker", req.WorkerType)
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

func assertTokenPayload(t *testing.T, snap *petri.MarkingSnapshot, placeID, want string) {
	t.Helper()

	for _, tok := range snap.Tokens {
		if tok.PlaceID == placeID {
			if got := string(tok.Color.Payload); got != want {
				t.Fatalf("expected payload %q, got %q", want, got)
			}
			return
		}
	}

	t.Fatalf("no token found in %s", placeID)
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
