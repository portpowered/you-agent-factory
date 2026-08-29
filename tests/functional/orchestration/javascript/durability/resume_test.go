package durability_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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

const javascriptDurabilityResumeRequestID = "req-js-durability-resume-interrupt-001"

// TestJavaScriptInterruptedSessionResumesWithoutRepeatingCompletedChildren proves
// that a durable JavaScript Factory Session interrupted after the first child
// dispatch completes resumes through the public lifecycle boundary without
// replaying completed children: completed Dispatch identities stay COMPLETED,
// only the remaining child dispatch continues, and the session reaches a
// successful terminal outcome.
func TestJavaScriptInterruptedSessionResumesWithoutRepeatingCompletedChildren(t *testing.T) {
	// C01 isolation reason: this row holds an injected provider command open,
	// interrupts that live dispatch, and resumes the same durable session. Keep
	// its cancellation gate and recovery lifecycle on a fresh process so it
	// cannot share interrupted runtime state with another behavior row.
	const workflowName = "resumable-two-step-fake-children"
	projectRoot := setupJavaScriptDurabilityResumeWorkflowFixture(t, workflowName)
	provider := newJavaScriptDurabilityResumeBlockingCommandRunner(workflowName)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: projectRoot,
		Edges:      serviceedges.Edges{ProviderCommandRunner: provider},
	})
	t.Cleanup(func() {
		server.Stop(t)
		t.Logf("durability isolated lifecycle: process_starts=1 process_stops=1 isolation_reason=%q", "blocked dispatch cancellation and same-session resume")
	})
	baseURL := strings.TrimSuffix(server.URL(), "/")

	sessionID := startInterruptedJavaScriptDurabilitySession(t, baseURL, provider, workflowName)

	before := readDurableJavaScriptSession(t, baseURL, sessionID)
	assertDurableProgressCounts(t, before.Progress, 1, 2, 0)
	if before.Status != factoryapi.FactorySessionDurableLifecycleStatusInterrupted {
		t.Fatalf("pre-resume status = %q, want INTERRUPTED", before.Status)
	}

	beforeDispatches := listJavaScriptSessionDispatches(t, baseURL, sessionID)
	dispatchOneBefore := requireDispatchSummary(
		t,
		beforeDispatches,
		"dispatch-1",
		factoryapi.FactoryDispatchStatusCOMPLETED,
	)
	dispatchTwoBefore := requireDispatchSummary(
		t,
		beforeDispatches,
		"dispatch-2",
		factoryapi.FactoryDispatchStatusINTERRUPTED,
		factoryapi.FactoryDispatchStatusRUNNING,
	)
	if len(beforeDispatches.Dispatches) != 2 {
		t.Fatalf("pre-resume dispatch count = %d, want 2", len(beforeDispatches.Dispatches))
	}

	assertAcceptedJavaScriptResume(t, resumeJavaScriptSession(t, baseURL, sessionID), sessionID)

	after := waitForDurableJavaScriptSessionStatus(
		t,
		baseURL,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		8*time.Second,
	)
	assertDurableProgressCounts(t, after.Progress, 2, 2, 0)
	if after.Lifecycle == nil || after.Lifecycle.InterruptedAt == nil || after.Lifecycle.ResumedAt == nil {
		t.Fatalf("post-resume lifecycle = %#v, want interruptedAt and resumedAt continuity", after.Lifecycle)
	}
	if before.Lifecycle == nil || before.Lifecycle.InterruptedAt == nil {
		t.Fatal("pre-resume lifecycle missing interruptedAt")
	}
	if !after.Lifecycle.InterruptedAt.Equal(*before.Lifecycle.InterruptedAt) {
		t.Fatalf(
			"interruptedAt changed across resume: before=%s after=%s",
			before.Lifecycle.InterruptedAt,
			after.Lifecycle.InterruptedAt,
		)
	}
	if after.ResultSummary == nil || after.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("post-resume resultSummary = %#v, want FINAL", after.ResultSummary)
	}

	afterDispatches := listJavaScriptSessionDispatches(t, baseURL, sessionID)
	if len(afterDispatches.Dispatches) != 2 {
		t.Fatalf("post-resume dispatch count = %d, want 2 (no replayed child dispatches)", len(afterDispatches.Dispatches))
	}
	dispatchOneAfter := requireDispatchSummary(
		t,
		afterDispatches,
		"dispatch-1",
		factoryapi.FactoryDispatchStatusCOMPLETED,
	)
	requireDispatchSummary(t, afterDispatches, "dispatch-2", factoryapi.FactoryDispatchStatusCOMPLETED)
	assertDispatchSummaryParity(t, dispatchOneBefore, dispatchOneAfter)
	if dispatchTwoBefore.Status == factoryapi.FactoryDispatchStatusINTERRUPTED {
		dispatchTwoAfter := requireDispatchSummary(
			t,
			afterDispatches,
			"dispatch-2",
			factoryapi.FactoryDispatchStatusCOMPLETED,
		)
		if dispatchTwoAfter.Id != dispatchTwoBefore.Id {
			t.Fatalf("dispatch-2 id changed across resume: %q -> %q", dispatchTwoBefore.Id, dispatchTwoAfter.Id)
		}
	}

	if provider.callCount() != 3 {
		t.Fatalf(
			"provider infer calls = %d, want exactly 3 (step-one once, blocked step-two once, resumed step-two once)",
			provider.callCount(),
		)
	}
}

// TestJavaScriptResumeRestoresCheckpointAndFinalResult proves that a durable
// JavaScript Factory Session interrupted after writing a workflow checkpoint
// resumes through the public lifecycle boundary with restored durable progress
// (latest checkpoint and dispatch counts preserved, not a blank restart) and
// reaches the expected terminal primary result for the completed workflow.
func TestJavaScriptResumeRestoresCheckpointAndFinalResult(t *testing.T) {
	// C01 isolation reason: this row validates checkpoint identity and restored
	// primary-result state after interruption. Keep its durable snapshot and
	// replay lifecycle on a fresh process rather than sharing recovery state.
	const workflowName = "resumable-two-step-fake-children"
	projectRoot := setupJavaScriptDurabilityResumeWorkflowFixture(t, workflowName)
	provider := newJavaScriptDurabilityResumeBlockingCommandRunner(workflowName)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: projectRoot,
		Edges:      serviceedges.Edges{ProviderCommandRunner: provider},
	})
	t.Cleanup(func() {
		server.Stop(t)
		t.Logf("durability isolated lifecycle: process_starts=1 process_stops=1 isolation_reason=%q", "checkpoint identity and restored final-result recovery")
	})
	baseURL := strings.TrimSuffix(server.URL(), "/")

	sessionID := startInterruptedJavaScriptDurabilitySession(t, baseURL, provider, workflowName)

	before := readDurableJavaScriptSession(t, baseURL, sessionID)
	assertDurableProgressCounts(t, before.Progress, 1, 2, 0)
	if before.Status != factoryapi.FactorySessionDurableLifecycleStatusInterrupted {
		t.Fatalf("pre-resume status = %q, want INTERRUPTED", before.Status)
	}
	checkpointBefore := requireLatestCheckpointLabel(t, before, "after-step-one")

	resumeResponse := resumeJavaScriptSession(t, baseURL, sessionID)
	if resumeResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume outcome = %q, want ACCEPTED", resumeResponse.Outcome)
	}

	after := waitForDurableJavaScriptSessionStatus(
		t,
		baseURL,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		8*time.Second,
	)
	assertDurableProgressCounts(t, after.Progress, 2, 2, 0)
	checkpointAfter := requireLatestCheckpointLabel(t, after, "after-step-one")
	if checkpointAfter.Id != checkpointBefore.Id {
		t.Fatalf(
			"latestCheckpoint id changed across resume: before=%q after=%q",
			checkpointBefore.Id,
			checkpointAfter.Id,
		)
	}
	if after.Lifecycle == nil || after.Lifecycle.InterruptedAt == nil || after.Lifecycle.ResumedAt == nil {
		t.Fatalf("post-resume lifecycle = %#v, want interruptedAt and resumedAt continuity", after.Lifecycle)
	}
	if before.Lifecycle == nil || before.Lifecycle.InterruptedAt == nil {
		t.Fatal("pre-resume lifecycle missing interruptedAt")
	}
	if !after.Lifecycle.InterruptedAt.Equal(*before.Lifecycle.InterruptedAt) {
		t.Fatalf(
			"interruptedAt changed across resume: before=%s after=%s",
			before.Lifecycle.InterruptedAt,
			after.Lifecycle.InterruptedAt,
		)
	}
	if after.ResultSummary == nil || after.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("post-resume resultSummary = %#v, want FINAL", after.ResultSummary)
	}

	result := readJavaScriptSessionFinalResult(t, baseURL, sessionID)
	if result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("terminal resultStatus = %q, want FINAL", result.ResultStatus)
	}
	assertResumableTwoStepFinalPrimaryResult(t, result, workflowName)
}

func setupJavaScriptDurabilityResumeWorkflowFixture(t *testing.T, workflowName string) string {
	t.Helper()

	projectRoot := support.ScaffoldSingleStepFactory(t, "js-durability-resume")
	workflowDir := filepath.Join(projectRoot, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "fixtures", "javascript_runtime", workflowName+".workflow.js"))
	if err != nil {
		t.Fatalf("read workflow fixture %s: %v", workflowName, err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, workflowName+".js"), raw, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func startInterruptedJavaScriptDurabilitySession(
	t *testing.T,
	baseURL string,
	provider *javascriptDurabilityResumeBlockingCommandRunner,
	workflowName string,
) string {
	t.Helper()

	started := postJavaScriptDurabilityJSON[factoryapi.FactorySessionExecutionResponse](
		t,
		baseURL+"/factory-sessions/async",
		factoryapi.FactorySessionExecutionRequest{
			RequestId: javascriptDurabilityResumeRequestID,
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
				WorkflowName: strPtr(workflowName),
			},
			Args: &map[string]any{
				"subject": "workflows",
			},
		},
	)
	sessionID := started.SessionId
	if sessionID == "" {
		t.Fatal("session id unexpectedly empty")
	}

	waitForJavaScriptDispatchStatus(
		t,
		baseURL,
		sessionID,
		"dispatch-1",
		factoryapi.FactoryDispatchStatusCOMPLETED,
		5*time.Second,
	)
	waitForJavaScriptDispatchStatus(
		t,
		baseURL,
		sessionID,
		"dispatch-2",
		factoryapi.FactoryDispatchStatusRUNNING,
		5*time.Second,
	)
	// RUNNING is published before the worker necessarily reaches the provider
	// edge. Wait for the injected provider's entry signal before interrupting;
	// otherwise the cancellation can win before Infer observes the workflow
	// context, which is the scheduling race behind the CI failure.
	provider.waitForBlockedInfer(t, 5*time.Second)

	reason := "javascript durability resume interrupt"
	postJavaScriptDurabilityJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/interrupt-dispatch",
		factoryapi.FactorySessionInterruptDispatchRequest{
			DispatchId: "dispatch-2",
			Reason:     &reason,
		},
	)
	provider.waitForCanceledInfer(t, 5*time.Second)
	waitForDurableJavaScriptSessionStatus(
		t,
		baseURL,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
		5*time.Second,
	)
	return sessionID
}

func readDurableJavaScriptSession(
	t *testing.T,
	baseURL string,
	sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID,
	)
	session, err := response.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode durable session read model: %v", err)
	}
	return session
}

func resumeJavaScriptSession(
	t *testing.T,
	baseURL string,
	sessionID string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	return postJavaScriptDurabilityJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/resume",
		factoryapi.FactorySessionLifecycleControlRequest{},
	)
}

func assertAcceptedJavaScriptResume(
	t *testing.T,
	response factoryapi.FactorySessionLifecycleControlResponse,
	sessionID string,
) {
	t.Helper()
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("resume operation = %q, want RESUME", response.Operation)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume outcome = %q, want ACCEPTED", response.Outcome)
	}
	if response.SessionId != sessionID {
		t.Fatalf("resume sessionId = %q, want %q", response.SessionId, sessionID)
	}
}

func javaScriptDurableSessionPersistencePath(projectRoot, sessionID string) string {
	return filepath.Join(projectRoot, ".you-agent-factory", "durable-sessions", sessionID+".json")
}

func assertJavaScriptDurableSessionPersistence(t *testing.T, projectRoot, sessionID string) {
	t.Helper()

	persistencePath := javaScriptDurableSessionPersistencePath(projectRoot, sessionID)
	info, err := os.Stat(persistencePath)
	if err != nil {
		t.Fatalf("project-local durable session snapshot stat error = %v, want exist", err)
	}
	if info.IsDir() {
		t.Fatalf("project-local durable session snapshot %s is a directory, want a file", persistencePath)
	}
	if info.Size() == 0 {
		t.Fatalf("project-local durable session snapshot %s is empty, want persisted session state", persistencePath)
	}
}

func listJavaScriptSessionDispatches(
	t *testing.T,
	baseURL string,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()

	listed := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/dispatches",
	)
	if listed.SessionId != sessionID {
		t.Fatalf("dispatch sessionId = %q, want %q", listed.SessionId, sessionID)
	}
	if listed.Dispatches == nil {
		t.Fatal("dispatch list unexpectedly missing")
	}
	return listed
}

func waitForDurableJavaScriptSessionStatus(
	t *testing.T,
	baseURL string,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	timeout time.Duration,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := readDurableJavaScriptSession(t, baseURL, sessionID)
		if session.Status == want {
			return session
		}
		time.Sleep(15 * time.Millisecond)
	}
	session := readDurableJavaScriptSession(t, baseURL, sessionID)
	t.Fatalf("session %s status = %q, want %q within %s", sessionID, session.Status, want, timeout)
	return session
}

func waitForJavaScriptDispatchStatus(
	t *testing.T,
	baseURL string,
	sessionID string,
	dispatchID string,
	want factoryapi.FactoryDispatchStatus,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := listJavaScriptSessionDispatches(t, baseURL, sessionID)
		for _, dispatch := range listed.Dispatches {
			if dispatch.Id != dispatchID {
				continue
			}
			if dispatch.Status == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("dispatch %s did not reach %s within %s", dispatchID, want, timeout)
}

func postJavaScriptDurabilityJSON[T any](t *testing.T, endpoint string, request any) T {
	t.Helper()

	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request for %s: %v", endpoint, err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read POST %s response: %v", endpoint, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		t.Fatalf("POST %s status = %d\n%s", endpoint, response.StatusCode, body)
	}
	var decoded T
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode POST %s response: %v\n%s", endpoint, err, body)
	}
	return decoded
}

func requireLatestCheckpointLabel(
	t *testing.T,
	session factoryapi.FactorySessionDurableReadModel,
	wantLabel string,
) factoryapi.FactorySessionCheckpointRef {
	t.Helper()
	if session.LatestCheckpoint == nil {
		t.Fatalf("latestCheckpoint = nil, want label %q", wantLabel)
	}
	if session.LatestCheckpoint.Id == "" {
		t.Fatal("latestCheckpoint.id unexpectedly empty")
	}
	if session.LatestCheckpoint.Label == nil || *session.LatestCheckpoint.Label != wantLabel {
		t.Fatalf("latestCheckpoint.label = %#v, want %q", session.LatestCheckpoint.Label, wantLabel)
	}
	return *session.LatestCheckpoint
}

func readJavaScriptSessionFinalResult(
	t *testing.T,
	baseURL string,
	sessionID string,
) factoryapi.FactorySessionResult {
	t.Helper()

	result := support.GetJSON[factoryapi.FactorySessionResult](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/results",
	)
	if result.SessionId != sessionID {
		t.Fatalf("result sessionId = %q, want %q", result.SessionId, sessionID)
	}
	return result
}

func assertResumableTwoStepFinalPrimaryResult(
	t *testing.T,
	result factoryapi.FactorySessionResult,
	workflowName string,
) {
	t.Helper()

	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want exactly one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	payload, ok := part.Json.(map[string]any)
	if !ok {
		t.Fatalf("primary json payload = %#v, want object", part.Json)
	}
	if payload["label"] != workflowName {
		t.Fatalf("result label = %#v, want %q", payload["label"], workflowName)
	}
	if payload["subject"] != "workflows" {
		t.Fatalf("result subject = %#v, want workflows", payload["subject"])
	}
	first, ok := payload["first"].(map[string]any)
	if !ok {
		t.Fatalf("result first = %#v, want object with step-one label", payload["first"])
	}
	if first["label"] != "step-one" {
		t.Fatalf("result first.label = %#v, want step-one", first["label"])
	}
	second, ok := payload["second"].(map[string]any)
	if !ok {
		t.Fatalf("result second = %#v, want object with step-two label", payload["second"])
	}
	if second["label"] != "step-two" {
		t.Fatalf("result second.label = %#v, want step-two", second["label"])
	}
}

func assertDurableProgressCounts(
	t *testing.T,
	progress *factoryapi.FactorySessionDurableProgressCounts,
	wantCompleted, wantTotal, wantInFlight int,
) {
	t.Helper()
	if progress == nil {
		t.Fatalf("progress = nil, want completed=%d total=%d inFlight=%d", wantCompleted, wantTotal, wantInFlight)
	}
	if intValueOrZero(progress.CompletedDispatches) != wantCompleted {
		t.Fatalf("completedDispatches = %#v, want %d", progress.CompletedDispatches, wantCompleted)
	}
	if intValueOrZero(progress.TotalDispatches) != wantTotal {
		t.Fatalf("totalDispatches = %#v, want %d", progress.TotalDispatches, wantTotal)
	}
	if intValueOrZero(progress.InFlightDispatches) != wantInFlight {
		t.Fatalf("inFlightDispatches = %#v, want %d", progress.InFlightDispatches, wantInFlight)
	}
}

func intValueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func requireDispatchSummary(
	t *testing.T,
	listed factoryapi.ListFactorySessionDispatchesResponse,
	dispatchID string,
	allowedStatuses ...factoryapi.FactoryDispatchStatus,
) factoryapi.FactorySessionDispatchSummary {
	t.Helper()

	for _, dispatch := range listed.Dispatches {
		if dispatch.Id != dispatchID {
			continue
		}
		for _, want := range allowedStatuses {
			if dispatch.Status == want {
				return dispatch
			}
		}
		t.Fatalf("dispatch %s status = %q, want one of %#v", dispatchID, dispatch.Status, allowedStatuses)
	}
	t.Fatalf("dispatch %s missing from %#v", dispatchID, listed.Dispatches)
	return factoryapi.FactorySessionDispatchSummary{}
}

func assertDispatchSummaryParity(
	t *testing.T,
	before, after factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()

	if before.Id != after.Id {
		t.Fatalf("dispatch id changed: %q -> %q", before.Id, after.Id)
	}
	if before.Status != after.Status && after.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("dispatch %s status drifted unexpectedly: before=%q after=%q", before.Id, before.Status, after.Status)
	}
	if before.DispatchKind != after.DispatchKind {
		t.Fatalf("dispatch %s kind changed: %q -> %q", before.Id, before.DispatchKind, after.DispatchKind)
	}
	if before.OutputArtifactIds != nil && after.OutputArtifactIds != nil {
		if len(*before.OutputArtifactIds) != len(*after.OutputArtifactIds) {
			t.Fatalf(
				"dispatch %s outputArtifactIds length changed: before=%#v after=%#v",
				before.Id,
				*before.OutputArtifactIds,
				*after.OutputArtifactIds,
			)
		}
		for i := range *before.OutputArtifactIds {
			if (*before.OutputArtifactIds)[i] != (*after.OutputArtifactIds)[i] {
				t.Fatalf(
					"dispatch %s outputArtifactIds changed: before=%#v after=%#v",
					before.Id,
					*before.OutputArtifactIds,
					*after.OutputArtifactIds,
				)
			}
		}
	}
}

func strPtr(value string) *string {
	return &value
}

// javascriptDurabilityResumeBlockingCommandRunner replaces only the injected
// ProviderCommandRunner edge. Its first provider-shaped result completes the
// first child, its second call is held until dispatch cancellation, and later
// calls complete the resumed child. The test therefore observes the real
// provider-command path without constructing a custom in-process Provider.
type javascriptDurabilityResumeBlockingCommandRunner struct {
	mu                  sync.Mutex
	calls               int
	blockedOnce         bool
	workflowName        string
	inferStarted        chan struct{}
	inferStartedOnce    sync.Once
	contextCanceled     chan struct{}
	contextCanceledOnce sync.Once
}

func newJavaScriptDurabilityResumeBlockingCommandRunner(workflowName string) *javascriptDurabilityResumeBlockingCommandRunner {
	return &javascriptDurabilityResumeBlockingCommandRunner{
		workflowName:    workflowName,
		inferStarted:    make(chan struct{}),
		contextCanceled: make(chan struct{}),
	}
}

func (p *javascriptDurabilityResumeBlockingCommandRunner) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *javascriptDurabilityResumeBlockingCommandRunner) Run(
	ctx context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	alreadyBlocked := p.blockedOnce
	if call == 2 && !alreadyBlocked {
		p.blockedOnce = true
		p.inferStartedOnce.Do(func() { close(p.inferStarted) })
	}
	p.mu.Unlock()

	if call == 1 {
		return javascriptDurabilityProviderCommandResult(
			p.workflowName,
			"step-one",
			"step-one",
		), nil
	}

	if !alreadyBlocked {
		<-ctx.Done()
		p.contextCanceledOnce.Do(func() { close(p.contextCanceled) })
		return platformprocess.CommandResult{}, ctx.Err()
	}

	return javascriptDurabilityProviderCommandResult(
		p.workflowName,
		"step-two",
		"step-two",
	), nil
}

func javascriptDurabilityProviderCommandResult(
	workflowName string,
	step string,
	label string,
) platformprocess.CommandResult {
	return platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout(
			fmt.Sprintf(`{"text":"live:%s:%s:%s:workflows","label":"%s"}`, workflowName, step, step, label),
		),
	}
}

func (p *javascriptDurabilityResumeBlockingCommandRunner) waitForBlockedInfer(t *testing.T, timeout time.Duration) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-p.inferStarted:
		return
	case <-timer.C:
		t.Fatal("provider Infer did not start the blocking call")
	}
}

func (p *javascriptDurabilityResumeBlockingCommandRunner) waitForCanceledInfer(t *testing.T, timeout time.Duration) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-p.contextCanceled:
		return
	case <-timer.C:
		t.Fatal("provider Infer did not observe canceled workflow context")
	}
}
