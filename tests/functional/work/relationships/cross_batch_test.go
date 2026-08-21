package relationships

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	crossBatchPrerequisiteName = "prerequisite-a"
	crossBatchPrerequisiteID   = "work-prerequisite-a"
	crossBatchDependentName    = "dependent-b"
	crossBatchDependentID      = "work-dependent-b"
	crossBatchMarkerName       = "cross-batch-merged.marker"
)

// TestCrossBatchDependsOnActivePrerequisiteReleasesAfterCompletion proves the
// public two-batch flow: a dependency admitted while its target is active is
// visible at init without a dispatch, then releases once after the target's
// completion event and starts from a fresh Git checkout containing the target's
// committed marker.
func TestCrossBatchDependsOnActivePrerequisiteReleasesAfterCompletion(t *testing.T) {
	requireCrossBatchGit(t)
	run := newCrossBatchFunctionalRun(t)

	executeCrossBatchSubmit(t, run.submitProcess, run.baseURL, crossBatchPrerequisiteBatchJSON())
	run.runner.WaitForFinishDispatch(t, 15*time.Second)
	assertCrossBatchWorkState(t, run.baseURL, crossBatchPrerequisiteID, "processing", "active prerequisite")
	if got := run.runner.CallCount(); got != 2 {
		t.Fatalf("provider calls before dependent admission = %d, want prerequisite start and gated finish", got)
	}

	executeCrossBatchSubmit(t, run.submitProcess, run.baseURL, crossBatchDependentBatchJSON())
	assertCrossBatchWorkState(t, run.baseURL, crossBatchDependentID, "init", "gated dependent")
	if got := run.runner.CallCount(); got != 2 {
		t.Fatalf("provider calls while prerequisite is active = %d, want no dependent dispatch", got)
	}
	assertCrossBatchNoDependentStartDispatch(t, support.GetFactoryEventsAt(t, run.baseURL))
	if run.runner.MarkerCommitted() {
		t.Fatal("merged marker committed before the prerequisite completion gate was released")
	}

	run.runner.Release()
	support.WaitForSessionTerminalStatus(t, run.baseURL, run.session.Id, 15*time.Second)
	assertCrossBatchCompletion(t, run)

	events := support.GetFactoryEventsAt(t, run.baseURL)
	prerequisiteCompleteSequence, dependentStartSequence := crossBatchDispatchOrdering(t, events)
	if dependentStartSequence <= prerequisiteCompleteSequence {
		t.Fatalf(
			"dependent start dispatch sequence = %d, want after prerequisite completion sequence %d",
			dependentStartSequence,
			prerequisiteCompleteSequence,
		)
	}
}

type crossBatchFunctionalRun struct {
	baseURL       string
	session       factoryapi.FactorySession
	submitProcess support.Process
	runner        *crossBatchDependencyCommandRunner
}

func newCrossBatchFunctionalRun(t *testing.T) crossBatchFunctionalRun {
	t.Helper()
	factoryDir := scaffoldCrossBatchGitFactory(t)
	runner := newCrossBatchDependencyCommandRunner(factoryDir)
	t.Cleanup(runner.Release)
	api := support.NewProcessAPIServer()
	daemonProcess := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: runner,
	})
	support.CleanupProcess(t, daemonProcess)

	runInputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", factoryDir, "--continuously", "--with-server",
		"--server", "http://127.0.0.1:1", "--quiet", "--no-record",
	})
	homeDir := t.TempDir()
	runInputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	runInputs.Input.WorkingDirectory = factoryDir
	support.StartProcessCommand(t, daemonProcess, runInputs.Input)

	baseURL := api.WaitForURL(t)
	submitProcess := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, submitProcess)
	return crossBatchFunctionalRun{
		baseURL:       baseURL,
		session:       support.GetDefaultSession(t, baseURL),
		submitProcess: submitProcess,
		runner:        runner,
	}
}

func scaffoldCrossBatchGitFactory(t *testing.T) string {
	t.Helper()
	factoryDir := support.ScaffoldFactory(t, crossBatchDependencyFactoryConfig())
	support.WriteAgentConfig(t, factoryDir, "worker", support.BuildModelWorkerConfig("codex", "test-model"))
	support.WriteWorkstationConfig(t, factoryDir, "start", "---\ntype: MODEL_WORKSTATION\n---\nAdvance cross-batch Work.\n")
	support.WriteWorkstationConfig(t, factoryDir, "finish", "---\ntype: MODEL_WORKSTATION\n---\nComplete cross-batch Work and merge its marker.\n")
	initCrossBatchGitRepository(t, factoryDir)
	return factoryDir
}

func requireCrossBatchGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for cross-batch checkout propagation")
	}
}

func assertCrossBatchWorkState(t *testing.T, baseURL, workID, state, description string) {
	t.Helper()
	wantPlace := support.WorkCustomerLocation("task", state)
	listed, err := support.WaitForObservation(
		15*time.Second,
		func() (factoryapi.ListWorkResponse, error) {
			return support.ListDefaultSessionWork(t, baseURL), nil
		},
		func(listed factoryapi.ListWorkResponse) bool {
			return support.HasWorkAtCustomerState(listed, workID, wantPlace)
		},
	)
	if err != nil {
		t.Fatalf("observe %s: %v; listed=%#v", description, err, listed)
	}
}

func assertCrossBatchCompletion(t *testing.T, run crossBatchFunctionalRun) {
	t.Helper()
	listed := support.ListDefaultSessionWork(t, run.baseURL)
	for _, workID := range []string{crossBatchPrerequisiteID, crossBatchDependentID} {
		if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", "complete")) {
			t.Fatalf("Work %q did not reach complete: %#v", workID, listed)
		}
	}
	if got := run.runner.CallCount(); got != 4 {
		t.Fatalf("provider calls after release = %d, want exactly four dispatch attempts", got)
	}
	if !run.runner.MarkerObserved() {
		t.Fatal("dependent provider did not observe the prerequisite's merged marker in its checkout")
	}
	if workDir := run.runner.DependentWorkDir(); filepath.Base(workDir) != crossBatchDependentName {
		t.Fatalf("dependent provider work directory = %q, want checkout named %q", workDir, crossBatchDependentName)
	}
}

func crossBatchDependencyFactoryConfig() map[string]any {
	return map[string]any{
		"name": "cross-batch-dependency-functional",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "processing", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker"}},
		"workstations": []map[string]any{
			{
				"name":      "start",
				"worker":    "worker",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "processing"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
				"worktree":  "{{ (index .Inputs 0).Name }}",
			},
			{
				"name":      "finish",
				"worker":    "worker",
				"inputs":    []map[string]string{{"workType": "task", "state": "processing"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func crossBatchPrerequisiteBatchJSON() string {
	return fmt.Sprintf(`{
		"requestId": "cross-batch-prerequisite",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [{
			"name": %q,
			"workId": %q,
			"workTypeName": "task",
			"payload": {"title": "Cross-batch prerequisite"}
		}]
	}`, crossBatchPrerequisiteName, crossBatchPrerequisiteID)
}

func crossBatchDependentBatchJSON() string {
	return fmt.Sprintf(`{
		"requestId": "cross-batch-dependent",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [{
			"name": %q,
			"workId": %q,
			"workTypeName": "task",
			"payload": {"title": "Cross-batch dependent"}
		}],
		"relations": [{
			"type": "DEPENDS_ON",
			"sourceWorkName": %q,
			"targetWorkName": %q
		}]
	}`, crossBatchDependentName, crossBatchDependentID, crossBatchDependentName, crossBatchPrerequisiteName)
}

func executeCrossBatchSubmit(t *testing.T, process support.Process, baseURL, batchJSON string) {
	t.Helper()
	homeDir := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", baseURL, "--json", "submit", "batch", batchJSON,
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = homeDir
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("")
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(submit batch) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
}

func assertCrossBatchNoDependentStartDispatch(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode dependent-gate dispatch event: %v", err)
		}
		if payload.TransitionId == "start" && dispatchRequestIncludesWork(payload, crossBatchDependentID) {
			t.Fatalf("dependent Work received start dispatch before prerequisite completion at sequence %d", event.Context.Sequence)
		}
	}
}

func crossBatchDispatchOrdering(t *testing.T, events []factoryapi.FactoryEvent) (int, int) {
	t.Helper()
	prerequisiteCompleteSequence := -1
	dependentStartSequence := -1
	dependentStartCount := 0
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode prerequisite completion event: %v", err)
			}
			if payload.TransitionId == "finish" &&
				payload.Outcome == factoryapi.WorkOutcomeAccepted &&
				dispatchEventIncludesWork(event.Context.WorkIds, crossBatchPrerequisiteID) {
				prerequisiteCompleteSequence = event.Context.Sequence
			}
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				t.Fatalf("decode dependent start event: %v", err)
			}
			if payload.TransitionId == "start" && dispatchRequestIncludesWork(payload, crossBatchDependentID) {
				dependentStartCount++
				if dependentStartSequence < 0 {
					dependentStartSequence = event.Context.Sequence
				}
			}
		}
	}
	if prerequisiteCompleteSequence < 0 {
		t.Fatalf("prerequisite %q has no accepted finish dispatch", crossBatchPrerequisiteID)
	}
	if dependentStartSequence < 0 {
		t.Fatalf("dependent %q has no start dispatch", crossBatchDependentID)
	}
	if dependentStartCount != 1 {
		t.Fatalf("dependent %q start dispatch count = %d, want exactly one", crossBatchDependentID, dependentStartCount)
	}
	return prerequisiteCompleteSequence, dependentStartSequence
}

type crossBatchDependencyCommandRunner struct {
	repoDir string

	finishEntered chan struct{}
	release       chan struct{}
	releaseOnce   sync.Once

	mu               sync.Mutex
	calls            int
	markerCommitted  bool
	markerObserved   bool
	dependentWorkDir string
}

func newCrossBatchDependencyCommandRunner(repoDir string) *crossBatchDependencyCommandRunner {
	return &crossBatchDependencyCommandRunner{
		repoDir:       repoDir,
		finishEntered: make(chan struct{}),
		release:       make(chan struct{}),
	}
}

func (r *crossBatchDependencyCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()

	if call == 2 {
		close(r.finishEntered)
		select {
		case <-r.release:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
		if err := r.commitMergedMarker(ctx); err != nil {
			return platformprocess.CommandResult{}, err
		}
	}
	if call == 3 {
		markerPath := filepath.Join(request.WorkDir, crossBatchMarkerName)
		contents, err := os.ReadFile(markerPath)
		if err != nil {
			return platformprocess.CommandResult{}, fmt.Errorf("read dependent checkout marker %s: %w", markerPath, err)
		}
		if strings.TrimSpace(string(contents)) != "merged prerequisite marker" {
			return platformprocess.CommandResult{}, fmt.Errorf("dependent checkout marker %s has unexpected content %q", markerPath, contents)
		}
		r.mu.Lock()
		r.markerObserved = true
		r.dependentWorkDir = request.WorkDir
		r.mu.Unlock()
	}

	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("COMPLETE")}, nil
}

func (r *crossBatchDependencyCommandRunner) commitMergedMarker(ctx context.Context) error {
	markerPath := filepath.Join(r.repoDir, crossBatchMarkerName)
	if err := os.WriteFile(markerPath, []byte("merged prerequisite marker\n"), 0o644); err != nil {
		return fmt.Errorf("write merged marker: %w", err)
	}
	if err := runCrossBatchGitCommand(ctx, r.repoDir, "add", "--", crossBatchMarkerName); err != nil {
		return fmt.Errorf("stage merged marker: %w", err)
	}
	if err := runCrossBatchGitCommand(ctx, r.repoDir, "commit", "-m", "merge prerequisite marker"); err != nil {
		return fmt.Errorf("commit merged marker: %w", err)
	}
	r.mu.Lock()
	r.markerCommitted = true
	r.mu.Unlock()
	return nil
}

func (r *crossBatchDependencyCommandRunner) WaitForFinishDispatch(t *testing.T, timeout time.Duration) {
	t.Helper()
	// The channel is the deterministic provider-edge observation; the timeout
	// only converts a missing active prerequisite dispatch into a bounded failure.
	select {
	case <-r.finishEntered:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for prerequisite finish dispatch after %s", timeout)
	}
}

func (r *crossBatchDependencyCommandRunner) Release() {
	if r == nil {
		return
	}
	r.releaseOnce.Do(func() { close(r.release) })
}

func (r *crossBatchDependencyCommandRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *crossBatchDependencyCommandRunner) MarkerCommitted() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.markerCommitted
}

func (r *crossBatchDependencyCommandRunner) MarkerObserved() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.markerObserved
}

func (r *crossBatchDependencyCommandRunner) DependentWorkDir() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dependentWorkDir
}

func initCrossBatchGitRepository(t *testing.T, repoDir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "cross-batch-functional@example.com"},
		{"config", "user.name", "cross-batch functional"},
		{"add", "--all"},
		{"commit", "--allow-empty", "-m", "initial factory"},
	} {
		if err := runCrossBatchGitCommand(context.Background(), repoDir, args...); err != nil {
			t.Fatalf("git %s: %v", strings.Join(args, " "), err)
		}
	}
}

func runCrossBatchGitCommand(ctx context.Context, repoDir string, args ...string) error {
	commandArgs := append([]string{"-C", repoDir}, args...)
	command := exec.CommandContext(ctx, "git", commandArgs...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

var _ platformprocess.CommandRunner = (*crossBatchDependencyCommandRunner)(nil)
