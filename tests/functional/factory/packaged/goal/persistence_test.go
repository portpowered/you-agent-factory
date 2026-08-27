package goal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var persistentGoalWorkID = regexp.MustCompile(`persistent goal work ([A-Za-z0-9._-]+) (?:at|in)`)

// TestPackagedGoalPersistsProgressAndClassifiesLoopThroughRootProcess proves the
// customer CLI boundary, a Codex-shaped provider, the workspace progress file,
// and classifier routing together. The first pass persists active progress and
// chooses needs_changes; the same executor then loads that state, persists
// completion, and chooses accepted.
// Isolation is intentional: this fixture owns a unique workspace, customer
// home, Work-derived state path, and provider selector because durable progress
// and atomic replacement are the properties under test. Dependency fidelity is
// local-real root.BuildProcess/Process.Execute, packaged Factory, and real
// filesystem I/O, with only ProviderCommandRunner controlled at the external
// provider-command edge.
func TestPackagedGoalPersistsProgressAndClassifiesLoopThroughRootProcess(t *testing.T) {
	rootDir := t.TempDir()
	workspace := filepath.Join(rootDir, "workspace")
	home := filepath.Join(rootDir, "home")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("create persistent Goal workspace: %v", err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("create persistent Goal home: %v", err)
	}
	model := nextPackagedGoalSelector("persistence")
	runner := &persistentGoalProviderRunner{workspace: workspace, model: model}
	environment := packagedGoalEnvironment(home)
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	support.InstallPackagedFactoryWithProcess(
		t, process, environment, workspace, factorydefinitions.PackagedGoalFactoryName,
	)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--named", factorydefinitions.PackagedGoalFactoryName,
		"--provider", "CODEX", "--model", model, "--no-record",
		"--to", "persist progress and finish the classifier-controlled goal",
	})
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = workspace
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("packaged goal invocation error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}

	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response status = %q, want COMPLETED: %#v", response.Status, response)
	}
	if got := primaryResultText(t, response); got != packagedGoalContinueThenCompleteSummary {
		t.Fatalf("primary result = %q, want %q", got, packagedGoalContinueThenCompleteSummary)
	}
	if got := runner.CallCount(); got != 2 {
		t.Fatalf("provider invocation count = %d, want 2", got)
	}

	statePath := runner.StatePath()
	state := readPersistentGoalState(t, statePath)
	if state.Status != "completed" || state.Iteration != 2 || state.LastResult != packagedGoalContinueThenCompleteSummary {
		t.Fatalf("persisted goal state = %#v, want completed iteration 2", state)
	}
	if _, err := os.Stat(statePath + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary state file remains after atomic replacement: %v", err)
	}
}

type persistentGoalState struct {
	Version    int    `json:"version"`
	GoalID     string `json:"goalId"`
	Objective  string `json:"objective"`
	Status     string `json:"status"`
	Iteration  int    `json:"iteration"`
	LastResult string `json:"lastResult"`
	UpdatedAt  string `json:"updatedAt"`
}

type persistentGoalProviderRunner struct {
	mu        sync.Mutex
	workspace string
	model     string
	calls     int
	statePath string
}

func (runner *persistentGoalProviderRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()

	if got := packagedGoalModelSelector(request.Args); got != runner.model {
		return platformprocess.CommandResult{}, fmt.Errorf("goal invocation used model %q, want unique selector %q", got, runner.model)
	}
	prompt := string(request.Stdin)
	if !strings.Contains(prompt, "Persistent goal state file, relative to the current worker workspace:") ||
		!strings.Contains(prompt, `"decision":"needs_changes"`) ||
		!strings.Contains(prompt, "termination is an external control") {
		return platformprocess.CommandResult{}, fmt.Errorf("goal prompt omitted persistence, classifier, or termination contract: %s", prompt)
	}
	matches := persistentGoalWorkID.FindStringSubmatch(prompt)
	if len(matches) != 2 {
		return platformprocess.CommandResult{}, fmt.Errorf("goal prompt omitted stable Work ID: %s", prompt)
	}
	workID := matches[1]
	statePath := filepath.Join(runner.workspace, ".you-goals", "~default", workID+".json")
	if runner.statePath == "" {
		runner.statePath = statePath
	} else if runner.statePath != statePath {
		return platformprocess.CommandResult{}, fmt.Errorf("goal loop changed state path from %s to %s", runner.statePath, statePath)
	}

	runner.calls++
	switch runner.calls {
	case 1:
		if err := writePersistentGoalState(statePath, persistentGoalState{
			Version: 1, GoalID: workID,
			Objective: "persist progress and finish the classifier-controlled goal",
			Status:    "active", Iteration: 1, LastResult: "ordinary partial progress",
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		}); err != nil {
			return platformprocess.CommandResult{}, err
		}
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(
			goalDecisionEnvelope("needs_changes", "continue with verification", "ordinary partial progress"),
		)}, nil
	case 2:
		previous, err := loadPersistentGoalState(statePath)
		if err != nil {
			return platformprocess.CommandResult{}, err
		}
		if previous.GoalID != workID ||
			previous.Objective != "persist progress and finish the classifier-controlled goal" ||
			previous.Status != "active" || previous.Iteration != 1 ||
			!strings.Contains(prompt, "ordinary partial progress") {
			return platformprocess.CommandResult{}, fmt.Errorf("second pass omitted persisted or prior progress: state=%#v prompt=%s", previous, prompt)
		}
		previous.Status = "completed"
		previous.Iteration = 2
		previous.LastResult = packagedGoalContinueThenCompleteSummary
		previous.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := writePersistentGoalState(statePath, previous); err != nil {
			return platformprocess.CommandResult{}, err
		}
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(
			goalDecisionEnvelope("accepted", "", packagedGoalContinueThenCompleteSummary),
		)}, nil
	default:
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected goal pass %d", runner.calls)
	}
}

func (runner *persistentGoalProviderRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

func (runner *persistentGoalProviderRunner) StatePath() string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.statePath
}

func writePersistentGoalState(path string, state persistentGoalState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create goal state directory: %w", err)
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal goal state: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, payload, 0o600); err != nil {
		return fmt.Errorf("write temporary goal state: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("commit goal state: %w", err)
	}
	return nil
}

func loadPersistentGoalState(path string) (persistentGoalState, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return persistentGoalState{}, fmt.Errorf("read goal state: %w", err)
	}
	var state persistentGoalState
	if err := json.Unmarshal(payload, &state); err != nil {
		return persistentGoalState{}, fmt.Errorf("decode goal state: %w", err)
	}
	return state, nil
}

func readPersistentGoalState(t *testing.T, path string) persistentGoalState {
	t.Helper()
	state, err := loadPersistentGoalState(path)
	if err != nil {
		t.Fatal(err)
	}
	return state
}
