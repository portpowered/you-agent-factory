package resume_from_recording_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	resumeFromRecordingWorkflowName = "resume-from-recording-two-step"
	resumeFromRecordingTimeout      = 15 * time.Second
)

// TestKilledFactorySessionResumesOriginalDispatchesAfterRestart proves that a
// durable multi-dispatch session can be interrupted at a deterministic
// provider boundary, observed after a fresh public process lifetime, and
// resumed without re-admitting the session or repeating its completed child.
// The command runner is installed through edges.Edges so the scenario still
// crosses the real Workers, Factory Runtime, and Factory Sessions paths.
func TestKilledFactorySessionResumesOriginalDispatchesAfterRestart(t *testing.T) {
	t.Parallel()

	scenario := newResumeFromRecordingScenario(t)
	t.Cleanup(scenario.runner.ReleaseRemainingDispatch)
	first := scenario.startProcess(t, "first")
	started := startResumeFromRecordingSession(t, first.URL())
	if started.SessionId == "" {
		t.Fatal("durable session start returned an empty session id")
	}
	scenario.sessionID = started.SessionId
	scenario.awaitKillBoundary(t)
	boundary := scenario.interruptAndCapture(t, first.URL())

	// Stop joins the first root-built process before the replacement is
	// constructed. The interrupted durable session is the public recovery
	// handle; this test never edits or finalizes its persisted state.
	first.Stop(t)
	select {
	case <-first.Done():
	default:
		t.Fatal("first process stop returned before its command joined")
	}

	second := scenario.startProcess(t, "successor")
	scenario.assertRestored(t, second.URL(), boundary)
	scenario.resumeAndFinish(t, second.URL(), boundary)
}

type resumeFromRecordingScenario struct {
	projectRoot string
	home        string
	env         []string
	sessionID   string
	runner      *resumeFromRecordingCommandRunner
}

type resumeFromRecordingBoundary struct {
	completed   factoryapi.FactorySessionDispatchSummary
	interrupted factoryapi.FactorySessionDispatchSummary
}

func newResumeFromRecordingScenario(t *testing.T) *resumeFromRecordingScenario {
	t.Helper()
	home := t.TempDir()
	return &resumeFromRecordingScenario{
		projectRoot: setupResumeFromRecordingFixture(t),
		home:        home,
		env:         []string{"HOME=" + home, "USERPROFILE=" + home},
		runner:      newResumeFromRecordingCommandRunner(),
	}
}

func (scenario *resumeFromRecordingScenario) startProcess(
	t *testing.T,
	name string,
) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                scenario.projectRoot,
		WaitForServiceModeRuntime: true,
		Env:                       scenario.env,
		Args:                      []string{"--record", filepath.Join(scenario.home, name+".recording.json")},
		Edges:                     serviceedges.Edges{ProviderCommandRunner: scenario.runner},
	})
}

func (scenario *resumeFromRecordingScenario) awaitKillBoundary(t *testing.T) {
	t.Helper()
	scenario.runner.wait(t, scenario.runner.firstReturned, "first provider dispatch")
	scenario.runner.wait(t, scenario.runner.secondEntered, "second provider dispatch")
	if got := scenario.runner.CallCount(); got != 2 {
		t.Fatalf("provider command calls at kill boundary = %d, want 2", got)
	}
}

func (scenario *resumeFromRecordingScenario) interruptAndCapture(
	t *testing.T,
	baseURL string,
) resumeFromRecordingBoundary {
	t.Helper()
	interruptResumeFromRecordingDispatch(t, baseURL, scenario.sessionID)
	scenario.runner.wait(t, scenario.runner.secondCanceled, "canceled second provider dispatch")
	beforeRestart := waitForResumeFromRecordingSession(t, baseURL, scenario.sessionID, func(session factoryapi.FactorySessionDurableReadModel) bool {
		return session.Status == factoryapi.FactorySessionDurableLifecycleStatusInterrupted
	})
	if beforeRestart.Progress == nil || valueOrZero(beforeRestart.Progress.CompletedDispatches) != 1 {
		t.Fatalf("pre-restart progress = %#v, want one completed dispatch", beforeRestart.Progress)
	}
	listed := listResumeFromRecordingDispatches(t, baseURL, scenario.sessionID)
	return resumeFromRecordingBoundary{
		completed: requireResumeFromRecordingDispatch(t, listed, "dispatch-1", factoryapi.FactoryDispatchStatusCOMPLETED),
		interrupted: requireResumeFromRecordingDispatch(
			t, listed, "dispatch-2", factoryapi.FactoryDispatchStatusINTERRUPTED, factoryapi.FactoryDispatchStatusRUNNING,
		),
	}
}

func (scenario *resumeFromRecordingScenario) assertRestored(
	t *testing.T,
	baseURL string,
	boundary resumeFromRecordingBoundary,
) {
	t.Helper()
	read := waitForResumeFromRecordingSession(t, baseURL, scenario.sessionID, func(session factoryapi.FactorySessionDurableReadModel) bool {
		return session.Status == factoryapi.FactorySessionDurableLifecycleStatusInterrupted
	})
	if read.SessionId != scenario.sessionID {
		t.Fatalf("post-restart session id = %q, want %q", read.SessionId, scenario.sessionID)
	}
	if read.Progress == nil || valueOrZero(read.Progress.CompletedDispatches) != 1 {
		t.Fatalf("post-restart progress = %#v, want restored one completed dispatch", read.Progress)
	}
	listed := listResumeFromRecordingDispatches(t, baseURL, scenario.sessionID)
	completed := requireResumeFromRecordingDispatch(t, listed, "dispatch-1", factoryapi.FactoryDispatchStatusCOMPLETED)
	interrupted := requireResumeFromRecordingDispatch(t, listed, "dispatch-2", factoryapi.FactoryDispatchStatusINTERRUPTED, factoryapi.FactoryDispatchStatusRUNNING)
	if completed.Id != boundary.completed.Id || interrupted.Id != boundary.interrupted.Id {
		t.Fatalf("dispatch identities changed across restart: completed %q/%q, interrupted %q/%q", boundary.completed.Id, completed.Id, boundary.interrupted.Id, interrupted.Id)
	}
}

func (scenario *resumeFromRecordingScenario) resumeAndFinish(
	t *testing.T,
	baseURL string,
	boundary resumeFromRecordingBoundary,
) {
	t.Helper()
	resume := postResumeFromRecordingJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+"/factory-sessions/"+url.PathEscape(scenario.sessionID)+"/resume",
		factoryapi.FactorySessionLifecycleControlRequest{},
	)
	if resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume outcome = %q, want ACCEPTED", resume.Outcome)
	}
	scenario.runner.wait(t, scenario.runner.thirdEntered, "resumed second provider dispatch")
	scenario.runner.ReleaseRemainingDispatch()
	finished := waitForResumeFromRecordingSession(t, baseURL, scenario.sessionID, func(session factoryapi.FactorySessionDurableReadModel) bool {
		return session.Status == factoryapi.FactorySessionDurableLifecycleStatusSucceeded
	})
	if finished.Progress == nil || valueOrZero(finished.Progress.CompletedDispatches) != 2 {
		t.Fatalf("post-resume progress = %#v, want two completed dispatches", finished.Progress)
	}
	if got := scenario.runner.CallCount(); got != 3 {
		t.Fatalf("provider command calls after resume = %d, want exactly 3", got)
	}
	final := listResumeFromRecordingDispatches(t, baseURL, scenario.sessionID)
	completed := requireResumeFromRecordingDispatch(t, final, "dispatch-1", factoryapi.FactoryDispatchStatusCOMPLETED)
	if completed.Id != boundary.completed.Id {
		t.Fatalf("completed dispatch was replaced after resume: %q -> %q", boundary.completed.Id, completed.Id)
	}
	requireResumeFromRecordingDispatch(t, final, "dispatch-2", factoryapi.FactoryDispatchStatusCOMPLETED)
}

func setupResumeFromRecordingFixture(t *testing.T) string {
	t.Helper()

	projectRoot := support.ScaffoldSingleStepFactory(t, "resume-from-recording")
	workflowDir := filepath.Join(projectRoot, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create workflow directory: %v", err)
	}
	fixturePath := support.AgentFactoryPath(
		t,
		"tests/fixtures/javascript_runtime/resumable-two-step-fake-children.workflow.js",
	)
	source, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read workflow fixture: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(workflowDir, resumeFromRecordingWorkflowName+".js"),
		source,
		0o600,
	); err != nil {
		t.Fatalf("write workflow fixture: %v", err)
	}
	return projectRoot
}

func startResumeFromRecordingSession(
	t *testing.T,
	baseURL string,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()
	workflowName := resumeFromRecordingWorkflowName
	return postResumeFromRecordingJSON[factoryapi.FactorySessionExecutionResponse](
		t,
		baseURL+"/factory-sessions/async",
		factoryapi.FactorySessionExecutionRequest{
			RequestId: "resume-from-recording-start-001",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
				WorkflowName: &workflowName,
			},
			Args: &map[string]interface{}{"subject": "resume-from-recording"},
		},
	)
}

func interruptResumeFromRecordingDispatch(t *testing.T, baseURL, sessionID string) {
	t.Helper()
	reason := "kill-and-resume functional boundary"
	response := postResumeFromRecordingJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+"/factory-sessions/"+url.PathEscape(sessionID)+"/interrupt-dispatch",
		factoryapi.FactorySessionInterruptDispatchRequest{
			DispatchId: "dispatch-2",
			Reason:     &reason,
		},
	)
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("interrupt outcome = %q, want ACCEPTED", response.Outcome)
	}
}

func readResumeFromRecordingSession(
	t *testing.T,
	baseURL, sessionID string,
) (factoryapi.FactorySessionDurableReadModel, error) {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		baseURL+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	read, err := response.AsFactorySessionDurableReadModel()
	if err != nil {
		return factoryapi.FactorySessionDurableReadModel{}, fmt.Errorf("decode durable session: %w", err)
	}
	return read, nil
}

func waitForResumeFromRecordingSession(
	t *testing.T,
	baseURL, sessionID string,
	accept func(factoryapi.FactorySessionDurableReadModel) bool,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	read, err := support.WaitForObservation(
		resumeFromRecordingTimeout,
		func() (factoryapi.FactorySessionDurableReadModel, error) {
			return readResumeFromRecordingSession(t, baseURL, sessionID)
		},
		accept,
	)
	if err != nil {
		t.Fatalf("wait for durable session %q: %v", sessionID, err)
	}
	return read
}

func listResumeFromRecordingDispatches(
	t *testing.T,
	baseURL, sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	return support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		baseURL+"/factory-sessions/"+url.PathEscape(sessionID)+"/dispatches",
	)
}

func requireResumeFromRecordingDispatch(
	t *testing.T,
	listed factoryapi.ListFactorySessionDispatchesResponse,
	id string,
	allowed ...factoryapi.FactoryDispatchStatus,
) factoryapi.FactorySessionDispatchSummary {
	t.Helper()
	for _, dispatch := range listed.Dispatches {
		if dispatch.Id != id {
			continue
		}
		for _, status := range allowed {
			if dispatch.Status == status {
				return dispatch
			}
		}
		t.Fatalf("dispatch %q status = %q, want one of %#v", id, dispatch.Status, allowed)
	}
	t.Fatalf("dispatch %q missing from %#v", id, listed.Dispatches)
	return factoryapi.FactorySessionDispatchSummary{}
}

func valueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func postResumeFromRecordingJSON[T any](t *testing.T, endpoint string, request any) T {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal %s request: %v", endpoint, err)
	}
	httpRequest, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		endpoint,
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("build POST %s: %v", endpoint, err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read POST %s: %v", endpoint, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		t.Fatalf("POST %s status = %d: %s", endpoint, response.StatusCode, body)
	}
	var decoded T
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode POST %s: %v\n%s", endpoint, err, body)
	}
	return decoded
}

type resumeFromRecordingCommandRunner struct {
	delegate *support.ShapedProviderCommandRunner

	mu                  sync.Mutex
	calls               int
	firstReturned       chan struct{}
	secondEntered       chan struct{}
	secondCanceled      chan struct{}
	thirdEntered        chan struct{}
	releaseRemaining    chan struct{}
	releaseRemainingOne sync.Once
}

func newResumeFromRecordingCommandRunner() *resumeFromRecordingCommandRunner {
	return &resumeFromRecordingCommandRunner{
		delegate: support.NewShapedProviderCommandRunner(
			platformprocess.CommandResult{Stdout: []byte("step-one COMPLETE")},
			platformprocess.CommandResult{Stdout: []byte("step-two interrupted COMPLETE")},
			platformprocess.CommandResult{Stdout: []byte("step-two resumed COMPLETE")},
		),
		firstReturned:    make(chan struct{}),
		secondEntered:    make(chan struct{}),
		secondCanceled:   make(chan struct{}),
		thirdEntered:     make(chan struct{}),
		releaseRemaining: make(chan struct{}),
	}
}

func (runner *resumeFromRecordingCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.calls++
	call := runner.calls
	runner.mu.Unlock()

	result, err := runner.delegate.Run(ctx, request)
	if err != nil {
		return result, err
	}
	switch call {
	case 1:
		close(runner.firstReturned)
		return result, nil
	case 2:
		close(runner.secondEntered)
		select {
		case <-ctx.Done():
			close(runner.secondCanceled)
			return platformprocess.CommandResult{}, ctx.Err()
		case <-runner.releaseRemaining:
			return result, nil
		}
	case 3:
		close(runner.thirdEntered)
		select {
		case <-runner.releaseRemaining:
			return result, nil
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	default:
		return result, nil
	}
}

func (runner *resumeFromRecordingCommandRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

func (runner *resumeFromRecordingCommandRunner) ReleaseRemainingDispatch() {
	runner.releaseRemainingOne.Do(func() { close(runner.releaseRemaining) })
}

func (runner *resumeFromRecordingCommandRunner) wait(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	timer := time.NewTimer(resumeFromRecordingTimeout)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("%s did not reach its deterministic signal within %s", name, resumeFromRecordingTimeout)
	}
}

var _ platformprocess.CommandRunner = (*resumeFromRecordingCommandRunner)(nil)
