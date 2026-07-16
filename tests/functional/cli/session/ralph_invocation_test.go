package session

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config/configinit"
	"github.com/portpowered/infinite-you/pkg/factory/packages/ralph"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestPackagedRalphInvocationThroughLiveCLIServiceSession(t *testing.T) {
	if testing.Short() {
		t.Skip("live CLI packaged Ralph invocation coverage")
	}
	cleanPackagedRalphCLIRuntimeState(t)

	t.Run("default named workflow plans then iterates to completion", func(t *testing.T) {
		runner := &packagedRalphRecordingRunner{}
		stderr, exitCode := runPackagedRalphCLI(t, runner, nil, "finish the default request")
		assertPackagedRalphCLICompletion(t, runner, stderr, exitCode)
		requests := runner.Requests()
		assertPackagedRalphWorkstationOrder(t, requests, []string{
			ralph.PackagedPlanWorkstationName,
			ralph.PackagedExecuteWorkstationName,
			ralph.PackagedExecuteWorkstationName,
		})
		if !strings.Contains(requestText(requests[0]), "finish the default request") {
			t.Fatalf("planner request = %q, want submitted request", requestText(requests[0]))
		}
		if !strings.Contains(requestText(requests[1]), packagedRalphPlanOutput) {
			t.Fatalf("first executor request = %q, want planner output %q", requestText(requests[1]), packagedRalphPlanOutput)
		}
		if !strings.Contains(requestText(requests[2]), packagedRalphContinueOutput) {
			t.Fatalf("second executor request = %q, want continued execution output %q", requestText(requests[2]), packagedRalphContinueOutput)
		}
	})

	t.Run("configured parameters reach their rendered worker requests", func(t *testing.T) {
		runner := &packagedRalphRecordingRunner{}
		stderr, exitCode := runPackagedRalphCLI(t, runner,
			[]string{"--planning-detail", "brief", "--execution-style", "direct"},
			"finish the configured request")
		assertPackagedRalphCLICompletion(t, runner, stderr, exitCode)
		requests := runner.Requests()
		assertPackagedRalphWorkstationOrder(t, requests, []string{
			ralph.PackagedPlanWorkstationName,
			ralph.PackagedExecuteWorkstationName,
			ralph.PackagedExecuteWorkstationName,
		})
		if !strings.Contains(requestText(requests[0]), "Planning detail: brief") {
			t.Fatalf("planner request = %q, want configured planning detail", requestText(requests[0]))
		}
		if !strings.Contains(requestText(requests[1]), "Execution style: direct") {
			t.Fatalf("executor request = %q, want configured execution style", requestText(requests[1]))
		}
	})

	t.Run("invalid parameter rejects before worker dispatch", func(t *testing.T) {
		runner := &packagedRalphRecordingRunner{}
		err := executePackagedRalphCLI(t, runner,
			[]string{"--planning-detail", "verbose"}, "reject this request")
		if err == nil {
			t.Fatal("invalid planning detail error = nil, want failure")
		}
		if !strings.Contains(err.Error(), "planningDetail") || !strings.Contains(err.Error(), "declared choices") {
			t.Fatalf("invalid planning detail error = %v, want actionable planningDetail choice diagnostic", err)
		}
		if got := len(runner.Requests()); got != 0 {
			t.Fatalf("worker dispatches = %d, want no dispatch before invalid parameter failure", got)
		}
	})

	t.Run("model and provider flags reach every worker with configured parameters", func(t *testing.T) {
		runner := &packagedRalphRecordingRunner{}
		stderr, exitCode := runPackagedRalphCLI(t, runner,
			[]string{
				"--default-worker-model-provider", "CODEX",
				"--default-worker-model", "gpt-5-codex",
				"--planning-detail", "brief",
			},
			"finish the flagged request")
		assertPackagedRalphCLICompletion(t, runner, stderr, exitCode)
		requests := runner.Requests()
		assertPackagedRalphWorkstationOrder(t, requests, []string{
			ralph.PackagedPlanWorkstationName,
			ralph.PackagedExecuteWorkstationName,
			ralph.PackagedExecuteWorkstationName,
		})
		for index, request := range requests {
			if request.Command != string(interfaces.ModelProviderCodex) {
				t.Fatalf("worker request %d command = %q, want %q", index, request.Command, interfaces.ModelProviderCodex)
			}
			if !containsArgument(request.Args, "gpt-5-codex") {
				t.Fatalf("worker request %d args = %q, want model override", index, request.Args)
			}
		}
		if !strings.Contains(requestText(requests[0]), "Planning detail: brief") {
			t.Fatalf("planner request = %q, want configured planning detail alongside flags", requestText(requests[0]))
		}
	})
}

func cleanPackagedRalphCLIRuntimeState(t *testing.T) {
	t.Helper()
	const runtimeStateDir = ".you-agent-factory"
	if _, err := os.Stat(runtimeStateDir); err == nil {
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect CLI runtime state directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(runtimeStateDir); err != nil {
			t.Errorf("remove test-owned CLI runtime state: %v", err)
		}
	})
}

func runPackagedRalphCLI(t *testing.T, runner *packagedRalphRecordingRunner, options []string, request string) (string, int) {
	t.Helper()
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	input := BasicCliInputWithArgs(t, packagedRalphCLIArgs(options, request))
	setPackagedRalphCLIHome(t, &input)
	input.Stdout = &stdout
	input.Stderr = &stderr

	exitCode := root.Run(input, root.Dependencies{GraphBuilder: packagedRalphCLIGraphBuilder{runner: runner}})
	return stderr.String(), exitCode
}

func executePackagedRalphCLI(t *testing.T, runner *packagedRalphRecordingRunner, options []string, request string) error {
	t.Helper()
	input := BasicCliInputWithArgs(t, packagedRalphCLIArgs(options, request))
	setPackagedRalphCLIHome(t, &input)
	return root.ExecuteWithDependencies(input, root.Dependencies{GraphBuilder: packagedRalphCLIGraphBuilder{runner: runner}})
}

func setPackagedRalphCLIHome(t *testing.T, input *root.Input) {
	t.Helper()
	homeDir := t.TempDir()
	if _, err := configinit.Init(homeDir); err != nil {
		t.Fatalf("initialize isolated CLI home: %v", err)
	}
	input.Env = append(input.Env, "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func packagedRalphCLIArgs(options []string, request string) []string {
	args := []string{"you", "--json", "run", "--named", ralph.PackagedFactoryName, "--no-record", "--quiet"}
	args = append(args, options...)
	return append(args, request)
}

func assertPackagedRalphCLICompletion(t *testing.T, runner *packagedRalphRecordingRunner, stderr string, exitCode int) {
	t.Helper()
	if exitCode != 0 {
		t.Fatalf("@you/ralph CLI exit code = %d, want 0; stderr: %s", exitCode, stderr)
	}
	if got := runner.ExecutorInvokes(); got != 2 {
		t.Fatalf("executor invocations = %d, want continuation followed by completion", got)
	}
}

type packagedRalphCLIGraphBuilder struct {
	runner *packagedRalphRecordingRunner
}

func (builder packagedRalphCLIGraphBuilder) Build(ctx context.Context, request root.GraphRequest) (*root.ApplicationGraph, error) {
	return wire.BuildProcessGraphWithFunctionalEdges(ctx, request.Startup, request.Policy, wire.FunctionalEdges{
		ProviderCommandRunner: builder.runner,
	})
}

const packagedRalphPlanOutput = "ordered plan from the planner"
const packagedRalphContinueOutput = "continue the plan\n<CONTINUE>"

type packagedRalphRecordingRunner struct {
	mu              sync.Mutex
	requests        []workers.CommandRequest
	executorInvokes int
}

func (runner *packagedRalphRecordingRunner) Run(_ context.Context, request workers.CommandRequest) (workers.CommandResult, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.requests = append(runner.requests, request)
	switch request.WorkstationName {
	case ralph.PackagedPlanWorkstationName:
		return workers.CommandResult{Stdout: []byte(packagedRalphPlanOutput)}, nil
	case ralph.PackagedExecuteWorkstationName:
		runner.executorInvokes++
		if runner.executorInvokes == 1 {
			return workers.CommandResult{Stdout: []byte(packagedRalphContinueOutput)}, nil
		}
		return workers.CommandResult{Stdout: []byte("completed the plan\n<COMPLETE>")}, nil
	default:
		return workers.CommandResult{}, errors.New("unexpected Ralph workstation " + request.WorkstationName)
	}
}

func (runner *packagedRalphRecordingRunner) Requests() []workers.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]workers.CommandRequest(nil), runner.requests...)
}

func (runner *packagedRalphRecordingRunner) ExecutorInvokes() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.executorInvokes
}

func assertPackagedRalphWorkstationOrder(t *testing.T, requests []workers.CommandRequest, want []string) {
	t.Helper()
	if len(requests) != len(want) {
		t.Fatalf("worker requests = %d, want %d: %#v", len(requests), len(want), requests)
	}
	for index, workstation := range want {
		if got := requests[index].WorkstationName; got != workstation {
			t.Fatalf("worker request %d workstation = %q, want %q", index, got, workstation)
		}
	}
}

func requestText(request workers.CommandRequest) string {
	return strings.Join(append(append([]string{}, request.Args...), string(request.Stdin)), "\n")
}

func containsArgument(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
