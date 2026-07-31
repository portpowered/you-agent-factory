package fullflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestPackagedFullFlowRunsParallelWorktreesMergesAndReplansToCompletion(t *testing.T) {
	repository := initializeFullFlowRepository(t)
	home := t.TempDir()
	factoryDir := support.InstallPackagedFactory(t, home, factorydefinitions.PackagedFullFlowFactoryName)
	runner := &fullFlowRunner{repository: repository}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir, WorkingDirectory: repository, WaitForServiceModeRuntime: true,
		Args:  []string{"--provider", "CODEX", "--model", "gpt-5"},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner},
	})
	response := invokeFullFlow(t, server, map[string]any{
		"request": "Deliver two independent changes", "baseBranch": "main",
		"maxCycles": "3", "maxTasksPerCycle": "2",
	})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response = %#v, message = %q, unexpected prompt = %q", response, optionalString(response.Message), runner.UnexpectedPrompt())
	}
	for _, task := range []string{"task-a", "task-b"} {
		content, err := os.ReadFile(filepath.Join(repository, task+".txt"))
		if err != nil || string(content) != task+"\n" {
			t.Fatalf("merged %s = %q, %v", task, content, err)
		}
	}
	longPaths, err := fullFlowGit(repository, "config", "--get", "core.longpaths")
	if err != nil || longPaths != "true" {
		t.Fatalf("repository core.longpaths = %q, %v, want persisted true for isolated-HOME task agents", longPaths, err)
	}
	planners, maximum, merges, _ := runner.Observations()
	if planners != 2 {
		t.Fatalf("planner calls = %d, want initial wave plus completion replan", planners)
	}
	if maximum < 2 {
		t.Fatalf("maximum concurrent implementations = %d, want at least 2", maximum)
	}
	if strings.Join(merges, ",") != "task-a,task-b" && strings.Join(merges, ",") != "task-b,task-a" {
		t.Fatalf("merged branches = %v", merges)
	}
	assertFullFlowReplay(t, server)
}

func invokeFullFlow(t *testing.T, server *support.FunctionalAPIServer, args map[string]any) factoryapi.InvocationResponse {
	t.Helper()
	requestID := fmt.Sprintf("full-flow-%d", time.Now().UnixNano())
	payload, err := json.Marshal(factoryapi.InvocationRequest{RequestId: &requestID, Args: &args})
	if err != nil {
		t.Fatalf("marshal invocation: %v", err)
	}
	response, err := http.Post(server.URL()+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST invocation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST invocation status = %d", response.StatusCode)
	}
	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation: %v", err)
	}
	return decoded
}

func assertFullFlowReplay(t *testing.T, server *support.FunctionalAPIServer) {
	t.Helper()
	events := server.GetFactoryEvents(t)
	waves := 0
	var observed []string
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil || payload.Works == nil {
			continue
		}
		names := make([]string, 0, len(*payload.Works))
		for _, item := range *payload.Works {
			names = append(names, item.Name)
		}
		source := ""
		if payload.Source != nil {
			source = *payload.Source
		}
		observed = append(observed, fmt.Sprintf("%v source=%s relations=%d", names, source, func() int {
			if payload.Relations == nil {
				return 0
			}
			return len(*payload.Relations)
		}()))
		if strings.Join(names, ",") == "task-a,task-b,work-1" {
			if payload.Relations == nil || len(*payload.Relations) < 5 {
				t.Fatalf("first replayed wave relations = %#v", payload.Relations)
			}
			waves++
		}
		if strings.Join(names, ",") == "work-1" && payload.Relations != nil {
			waves++
		}
	}
	if waves != 2 {
		t.Fatalf("replayed full-flow waves = %d, want implementation and completion waves; requests = %v", waves, observed)
	}
	dispatches := support.ObserveDispatchEvents(t, events)
	mergeCount := 0
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId == "merge-task" {
			mergeCount++
		}
	}
	if mergeCount != 2 {
		t.Fatalf("replayed merge dispatches = %d, want 2", mergeCount)
	}
	sequence := support.ReconnectSequenceForFactoryEvent(events[0])
	replayed := server.GetFactoryEventsAfter(t, support.FactoryEventReadCursor{AfterEventID: events[0].Id, AfterSequence: &sequence})
	if len(replayed) != len(events)-1 {
		t.Fatalf("retained replay events = %d, want %d", len(replayed), len(events)-1)
	}
}

func TestPackagedFullFlowBoundsImplementationContinueLoopAndFailsProject(t *testing.T) {
	repository := initializeFullFlowRepository(t)
	home := t.TempDir()
	support.InstallPackagedFactory(t, home, factorydefinitions.PackagedFullFlowFactoryName)
	runner := &fullFlowRunner{repository: repository, stallImplementation: true}
	args := []string{
		"you", "--json", "run", "--named", factorydefinitions.PackagedFullFlowFactoryName,
		"--provider", "CODEX", "--model", "gpt-5", "--base-branch", "main",
		"--max-cycles", "3", "--max-tasks-per-cycle", "2", "--no-record",
		"--to", "Exercise the implementation bound",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = repository
	if err := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner}).Execute(inputs.Input); err == nil {
		t.Fatalf("full-flow invocation unexpectedly succeeded\nstdout:\n%s", inputs.Stdout())
	}
	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	if response.Status != factoryapi.InvocationTerminalStatusFailed || response.WorkState == nil || !strings.HasSuffix(*response.WorkState, ":failed") {
		t.Fatalf("response = %#v, want bounded project failure", response)
	}
	_, _, merges, implementationCalls := runner.Observations()
	if implementationCalls < 2 || implementationCalls > 42 {
		t.Fatalf("implementation calls = %d, want finite guarded attempts", implementationCalls)
	}
	if len(merges) != 0 {
		t.Fatalf("merged branches = %v, want none after implementation exhaustion", merges)
	}
}

func TestPackagedFullFlowEnforcesCallerSelectedTaskBound(t *testing.T) {
	repository := initializeFullFlowRepository(t)
	home := t.TempDir()
	factoryDir := support.InstallPackagedFactory(t, home, factorydefinitions.PackagedFullFlowFactoryName)
	runner := &fullFlowRunner{repository: repository}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir, WorkingDirectory: repository, WaitForServiceModeRuntime: true,
		Args:  []string{"--provider", "CODEX", "--model", "gpt-5"},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner},
	})

	response := invokeFullFlow(t, server, map[string]any{
		"request": "Reject a planner wave above the caller bound", "baseBranch": "main",
		"maxCycles": "3", "maxTasksPerCycle": "1",
	})
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("response = %#v, want over-budget planner wave failure", response)
	}
	planners, _, merges, implementationCalls := runner.Observations()
	if planners != 1 || implementationCalls != 0 || len(merges) != 0 {
		t.Fatalf("observations = planners %d implementations %d merges %v, want atomic rejection before task execution", planners, implementationCalls, merges)
	}
}

func TestPackagedFullFlowEnforcesCallerSelectedCycleBound(t *testing.T) {
	repository := initializeFullFlowRepository(t)
	home := t.TempDir()
	factoryDir := support.InstallPackagedFactory(t, home, factorydefinitions.PackagedFullFlowFactoryName)
	runner := &fullFlowRunner{repository: repository}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir, WorkingDirectory: repository, WaitForServiceModeRuntime: true,
		Args:  []string{"--provider", "CODEX", "--model", "gpt-5"},
		Edges: serviceedges.Edges{ProviderCommandRunner: runner},
	})

	response := invokeFullFlow(t, server, map[string]any{
		"request": "Stop after one incomplete delivery cycle", "baseBranch": "main",
		"maxCycles": "1", "maxTasksPerCycle": "2",
	})
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("response = %#v, want caller-selected cycle exhaustion", response)
	}
	planners, _, merges, _ := runner.Observations()
	if planners != 1 || len(merges) != 2 {
		t.Fatalf("observations = planners %d merges %v, want one completed wave and no second plan", planners, merges)
	}
}

type fullFlowRunner struct {
	mu                  sync.Mutex
	repository          string
	plannerCalls        int
	active              int
	maxActive           int
	merges              []string
	unexpected          string
	stallImplementation bool
	implementationCalls int
}

func (runner *fullFlowRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	prompt := string(request.Stdin)
	switch {
	case strings.Contains(prompt, "You are the planning stage for one bounded delivery cycle"):
		for _, required := range []string{
			"you docs agents",
			"Never run bare `you`",
			`{"request":{"type":"FACTORY_REQUEST_BATCH"`,
			`"type":"DEPENDS_ON"`,
			`"workTypeName":"cycle-control"`,
		} {
			if !strings.Contains(prompt, required) {
				return platformprocess.CommandResult{}, fmt.Errorf("full-flow planner prompt missing required contract %q", required)
			}
		}
		runner.mu.Lock()
		runner.plannerCalls++
		call := runner.plannerCalls
		runner.mu.Unlock()
		if call == 1 {
			return fullFlowCodexResult(`{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"task-a","workTypeName":"delivery-task","payload":"implement task a"},{"name":"task-b","workTypeName":"delivery-task","payload":"implement task b"},{"name":"work-1","workTypeName":"cycle-control","payload":"continue"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"work-1","targetWorkName":"task-a","requiredState":"merged"},{"type":"DEPENDS_ON","sourceWorkName":"work-1","targetWorkName":"task-b","requiredState":"merged"}]}}`), nil
		}
		return fullFlowCodexResult(`{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"work-1","workTypeName":"cycle-control","payload":"complete"}]}}`), nil
	case strings.Contains(prompt, "implementation agent for one assigned delivery task"):
		task := filepath.Base(request.WorkDir)
		runner.mu.Lock()
		runner.implementationCalls++
		runner.active++
		if runner.active > runner.maxActive {
			runner.maxActive = runner.active
		}
		runner.mu.Unlock()
		time.Sleep(75 * time.Millisecond)
		if runner.stallImplementation {
			runner.mu.Lock()
			runner.active--
			runner.mu.Unlock()
			return fullFlowCodexResult("<CONTINUE>"), nil
		}
		if err := os.WriteFile(filepath.Join(request.WorkDir, task+".txt"), []byte(task+"\n"), 0o600); err != nil {
			return platformprocess.CommandResult{}, err
		}
		if _, err := fullFlowGit(request.WorkDir, "add", task+".txt"); err != nil {
			return platformprocess.CommandResult{}, err
		}
		if _, err := fullFlowGit(request.WorkDir, "commit", "-m", "implement "+task); err != nil {
			return platformprocess.CommandResult{}, err
		}
		runner.mu.Lock()
		runner.active--
		runner.mu.Unlock()
		return fullFlowCodexResult("<COMPLETE>"), nil
	case strings.Contains(prompt, "independent reviewer with no shared conversation"):
		return fullFlowCodexResult("<COMPLETE>"), nil
	case strings.Contains(prompt, "verification stage for an independently reviewed task"):
		if _, err := fullFlowGit(request.WorkDir, "diff", "--check", "HEAD^"); err != nil {
			return platformprocess.CommandResult{}, err
		}
		return fullFlowCodexResult("<COMPLETE>"), nil
	case strings.Contains(prompt, "merge stage for one verified task"):
		task := filepath.Base(request.WorkDir)
		if _, err := fullFlowGit(runner.repository, "merge", "--no-ff", task, "-m", "merge "+task); err != nil {
			return platformprocess.CommandResult{}, err
		}
		runner.mu.Lock()
		runner.merges = append(runner.merges, task)
		runner.mu.Unlock()
		return fullFlowCodexResult("<COMPLETE>"), nil
	case strings.TrimSpace(prompt) == "continue":
		return fullFlowCodexResult("continue"), nil
	case strings.TrimSpace(prompt) == "complete":
		return fullFlowCodexResult("complete"), nil
	default:
		runner.mu.Lock()
		runner.unexpected = fmt.Sprintf("command=%q args=%q prompt=%q", request.Command, request.Args, prompt)
		runner.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected full-flow prompt: %s", prompt)
	}
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (runner *fullFlowRunner) UnexpectedPrompt() string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.unexpected
}

func (runner *fullFlowRunner) Observations() (int, int, []string, int) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.plannerCalls, runner.maxActive, append([]string(nil), runner.merges...), runner.implementationCalls
}

func initializeFullFlowRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.email", "factory@example.test"}, {"config", "user.name", "Factory Test"}} {
		if _, err := fullFlowGit(repository, args...); err != nil {
			t.Fatalf("git %s: %v", strings.Join(args, " "), err)
		}
	}
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fullFlowGit(repository, "add", "README.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := fullFlowGit(repository, "commit", "-m", "fixture"); err != nil {
		t.Fatal(err)
	}
	return repository
}

func fullFlowGit(directory string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output)), nil
}

func fullFlowCodexResult(text string) platformprocess.CommandResult {
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(text)}
}
