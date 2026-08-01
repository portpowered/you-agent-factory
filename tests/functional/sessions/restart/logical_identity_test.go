package restart_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFactorySessionRestartRemapsLiveIDToLogicalIdentity proves that after a
// simulated backend restart invalidates a previously live factorySessionID, the
// public sync-preflight surface remaps the stale selector through logical
// identity (backendScopeID + logicalSessionKeyID) to the current live session
// and stream generation for the same logical target.
func TestFactorySessionRestartRemapsLiveIDToLogicalIdentity(t *testing.T) {
	factoryDir := support.ScaffoldSingleStepFactory(t, "logical-identity-restart")
	home := t.TempDir()
	env := functionalHomeEnvironment(home)

	beforeRestart := startLogicalIdentityRestartServer(t, factoryDir, env)
	staleSession := support.GetDefaultSession(t, beforeRestart.URL())
	staleIdentity := requireLiveStreamIdentity(t, staleSession)
	if staleIdentity.FactorySessionID == "" {
		t.Fatal("pre-restart factorySessionID unexpectedly empty")
	}

	beforeRestart.Stop(t)

	afterRestart := startLogicalIdentityRestartServer(t, factoryDir, env)
	currentSession := support.GetDefaultSession(t, afterRestart.URL())
	currentIdentity := requireLiveStreamIdentity(t, currentSession)
	if currentIdentity.FactorySessionID == staleIdentity.FactorySessionID {
		t.Fatalf(
			"post-restart factorySessionID = %q, want distinct live id from pre-restart %q",
			currentIdentity.FactorySessionID,
			staleIdentity.FactorySessionID,
		)
	}
	if currentIdentity.LogicalSessionKeyID != staleIdentity.LogicalSessionKeyID {
		t.Fatalf(
			"post-restart logicalSessionKeyID = %q, want stable logical key %q",
			currentIdentity.LogicalSessionKeyID,
			staleIdentity.LogicalSessionKeyID,
		)
	}

	preflight := getFactorySessionSyncPreflight(
		t,
		afterRestart.URL(),
		staleIdentity.FactorySessionID,
		staleIdentity.BackendScopeID,
		staleIdentity.LogicalSessionKeyID,
	)
	if preflight.ReasonCode != factoryapi.LogicalSessionRemap {
		t.Fatalf("sync-preflight reasonCode = %q, want %q", preflight.ReasonCode, factoryapi.LogicalSessionRemap)
	}
	if preflight.CheckpointReusable {
		t.Fatal("sync-preflight checkpointReusable = true, want false for logical remap")
	}
	if preflight.FactorySessionId == nil || *preflight.FactorySessionId != currentIdentity.FactorySessionID {
		t.Fatalf(
			"sync-preflight factorySessionId = %#v, want current live id %q",
			preflight.FactorySessionId,
			currentIdentity.FactorySessionID,
		)
	}
	if preflight.StreamGenerationId == nil || *preflight.StreamGenerationId != currentIdentity.StreamGenerationID {
		t.Fatalf(
			"sync-preflight streamGenerationId = %#v, want current stream generation %q",
			preflight.StreamGenerationId,
			currentIdentity.StreamGenerationID,
		)
	}
	if preflight.BackendScopeId == nil || *preflight.BackendScopeId != currentIdentity.BackendScopeID {
		t.Fatalf(
			"sync-preflight backendScopeId = %#v, want current backend scope %q",
			preflight.BackendScopeId,
			currentIdentity.BackendScopeID,
		)
	}
	if preflight.LogicalSessionKeyId == nil ||
		*preflight.LogicalSessionKeyId != staleIdentity.LogicalSessionKeyID {
		t.Fatalf(
			"sync-preflight logicalSessionKeyId = %#v, want stable logical key %q",
			preflight.LogicalSessionKeyId,
			staleIdentity.LogicalSessionKeyID,
		)
	}
}

// TestFactorySessionResumeDoesNotRepeatCompletedDispatch proves that a Factory
// Session interrupted after at least one child Dispatch completes resumes through
// the public resume boundary without replaying those completed children:
// completed Dispatch identities stay COMPLETED, only remaining work continues,
// and progress on public session surfaces reflects durable continuity.
func TestFactorySessionResumeDoesNotRepeatCompletedDispatch(t *testing.T) {
	const workflowName = "resumable-two-step-fake-children"
	factoryDir := setupResumableTwoStepWorkflowFixture(t, workflowName)
	provider := newLogicalIdentityResumeBlockingProvider(workflowName)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir,
		Edges:      serviceedges.Edges{ProviderOverride: provider},
	})
	baseURL := strings.TrimSuffix(server.URL(), "/")

	sessionID := startInterruptedResumableSession(t, baseURL, provider, workflowName)

	beforeShow := readDurableFactorySession(t, baseURL, sessionID)
	assertDurableProgressCounts(t, beforeShow.Progress, 1, 2, 0)
	if beforeShow.Status != factoryapi.FactorySessionDurableLifecycleStatusInterrupted {
		t.Fatalf("pre-resume status = %q, want INTERRUPTED", beforeShow.Status)
	}

	beforeDispatches := listFactorySessionDispatches(t, baseURL, sessionID)
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

	resumeResponse := resumeFactorySession(t, baseURL, sessionID)
	if resumeResponse.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("resume operation = %q, want RESUME", resumeResponse.Operation)
	}
	if resumeResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume outcome = %q, want ACCEPTED", resumeResponse.Outcome)
	}
	if resumeResponse.SessionId != sessionID {
		t.Fatalf("resume sessionId = %q, want %q", resumeResponse.SessionId, sessionID)
	}

	afterShow := waitForDurableFactorySessionStatus(
		t,
		baseURL,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		8*time.Second,
	)
	assertDurableProgressCounts(t, afterShow.Progress, 2, 2, 0)
	if afterShow.Lifecycle == nil || afterShow.Lifecycle.InterruptedAt == nil || afterShow.Lifecycle.ResumedAt == nil {
		t.Fatalf("post-resume lifecycle = %#v, want interruptedAt and resumedAt continuity", afterShow.Lifecycle)
	}
	if beforeShow.Lifecycle == nil || beforeShow.Lifecycle.InterruptedAt == nil {
		t.Fatal("pre-resume lifecycle missing interruptedAt")
	}
	if !afterShow.Lifecycle.InterruptedAt.Equal(*beforeShow.Lifecycle.InterruptedAt) {
		t.Fatalf(
			"interruptedAt changed across resume: before=%s after=%s",
			beforeShow.Lifecycle.InterruptedAt,
			afterShow.Lifecycle.InterruptedAt,
		)
	}

	afterDispatches := listFactorySessionDispatches(t, baseURL, sessionID)
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

// TestFactorySessionHistoryIsNotPersistedByDefault proves an interrupted
// Factory Session remains inspectable for its process lifetime but is absent
// after restart while durable snapshot recording is disabled by default.
func TestFactorySessionHistoryIsNotPersistedByDefault(t *testing.T) {
	const workflowName = "resumable-two-step-fake-children"
	factoryDir := setupResumableTwoStepWorkflowFixture(t, workflowName)
	home := t.TempDir()
	env := functionalHomeEnvironment(home)
	provider := newLogicalIdentityResumeBlockingProvider(workflowName)

	beforeRestart := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir,
		Edges:      serviceedges.Edges{ProviderOverride: provider},
		Env:        env,
	})
	beforeURL := strings.TrimSuffix(beforeRestart.URL(), "/")

	sessionID := startInterruptedResumableSession(t, beforeURL, provider, workflowName)

	beforeShow := readDurableFactorySession(t, beforeURL, sessionID)
	assertDurableProgressCounts(t, beforeShow.Progress, 1, 2, 0)
	if beforeShow.Status != factoryapi.FactorySessionDurableLifecycleStatusInterrupted {
		t.Fatalf("pre-restart status = %q, want INTERRUPTED", beforeShow.Status)
	}
	if beforeShow.Lifecycle == nil || beforeShow.Lifecycle.InterruptedAt == nil {
		t.Fatalf("pre-restart lifecycle = %#v, want interruptedAt", beforeShow.Lifecycle)
	}

	beforeDispatches := listFactorySessionDispatches(t, beforeURL, sessionID)
	requireDispatchSummary(
		t,
		beforeDispatches,
		"dispatch-1",
		factoryapi.FactoryDispatchStatusCOMPLETED,
	)
	requireDispatchSummary(
		t,
		beforeDispatches,
		"dispatch-2",
		factoryapi.FactoryDispatchStatusINTERRUPTED,
		factoryapi.FactoryDispatchStatusRUNNING,
	)

	beforeEvents := listFactorySessionEvents(t, beforeURL, sessionID)
	if len(beforeEvents) == 0 {
		t.Fatal("pre-restart factory events unexpectedly empty")
	}

	defaultBefore := support.GetDefaultSession(t, beforeURL)
	staleIdentity := requireLiveStreamIdentity(t, defaultBefore)

	beforeRestart.Stop(t)

	afterRestart := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir,
		Edges:      serviceedges.Edges{ProviderOverride: provider},
		Env:        env,
	})
	afterURL := strings.TrimSuffix(afterRestart.URL(), "/")

	preflight := getFactorySessionSyncPreflight(
		t,
		afterURL,
		staleIdentity.FactorySessionID,
		staleIdentity.BackendScopeID,
		staleIdentity.LogicalSessionKeyID,
	)
	if preflight.ReasonCode != factoryapi.LogicalSessionRemap {
		t.Fatalf("sync-preflight reasonCode = %q, want %q", preflight.ReasonCode, factoryapi.LogicalSessionRemap)
	}

	endpoint := afterURL + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"GET %s status = %d body = %s, want 404 while persistence is disabled",
			endpoint,
			response.StatusCode,
			body,
		)
	}
}

func startLogicalIdentityRestartServer(
	t *testing.T,
	factoryDir string,
	env []string,
) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Env:                       env,
	})
}

func requireLiveStreamIdentity(
	t *testing.T,
	session factoryapi.FactorySession,
) factoryapi.FactorySessionStreamIdentity {
	t.Helper()
	if session.Runtime.StreamIdentity == nil {
		t.Fatalf("session %q streamIdentity = nil, want public logical identity facts", session.Id)
	}
	identity := *session.Runtime.StreamIdentity
	if strings.TrimSpace(identity.BackendScopeID) == "" {
		t.Fatalf("streamIdentity.backendScopeID = %#v, want non-empty backend scope", identity)
	}
	if strings.TrimSpace(identity.LogicalSessionKeyID) == "" {
		t.Fatalf("streamIdentity.logicalSessionKeyID = %#v, want non-empty logical key", identity)
	}
	if strings.TrimSpace(identity.StreamGenerationID) == "" {
		t.Fatalf("streamIdentity.streamGenerationID = %#v, want non-empty stream generation", identity)
	}
	if strings.TrimSpace(identity.FactorySessionID) == "" {
		t.Fatalf("streamIdentity.factorySessionID = %#v, want non-empty live session id", identity)
	}
	return identity
}

func getFactorySessionSyncPreflight(
	t *testing.T,
	baseURL string,
	staleSessionID string,
	backendScopeID string,
	logicalSessionKeyID string,
) factoryapi.FactorySessionSyncPreflightResponse {
	t.Helper()

	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(staleSessionID) + "/sync-preflight"
	query := url.Values{}
	query.Set("backend_scope_id", backendScopeID)
	query.Set("logical_session_key_id", logicalSessionKeyID)
	return support.GetJSON[factoryapi.FactorySessionSyncPreflightResponse](
		t,
		endpoint+"?"+query.Encode(),
	)
}

func functionalHomeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{"USERPROFILE=" + home}
	}
	if runtime.GOOS == "plan9" {
		return []string{"home=" + home}
	}
	return []string{fmt.Sprintf("HOME=%s", home)}
}

func setupResumableTwoStepWorkflowFixture(t *testing.T, workflowName string) string {
	t.Helper()

	projectRoot := support.ScaffoldSingleStepFactory(t, "logical-identity-resume")
	workflowDir := filepath.Join(projectRoot, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	raw, err := os.ReadFile(
		filepath.Join("..", "..", "..", "fixtures", "javascript_runtime", workflowName+".workflow.js"),
	)
	if err != nil {
		t.Fatalf("read workflow fixture %s: %v", workflowName, err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, workflowName+".js"), raw, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
}

func startInterruptedResumableSession(
	t *testing.T,
	baseURL string,
	provider *logicalIdentityResumeBlockingProvider,
	workflowName string,
) string {
	t.Helper()

	started := postLogicalIdentityJSON[factoryapi.FactorySessionExecutionResponse](
		t,
		baseURL+"/factory-sessions/async",
		factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-logical-identity-resume-interrupt-001",
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

	waitForFactorySessionDispatchStatus(
		t,
		baseURL,
		sessionID,
		"dispatch-1",
		factoryapi.FactoryDispatchStatusCOMPLETED,
		5*time.Second,
	)
	waitForFactorySessionDispatchStatus(
		t,
		baseURL,
		sessionID,
		"dispatch-2",
		factoryapi.FactoryDispatchStatusRUNNING,
		5*time.Second,
	)

	reason := "logical identity resume interrupt"
	postLogicalIdentityJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/interrupt-dispatch",
		factoryapi.FactorySessionInterruptDispatchRequest{
			DispatchId: "dispatch-2",
			Reason:     &reason,
		},
	)
	provider.waitForCanceledInfer(t, 5*time.Second)
	waitForDurableFactorySessionStatus(
		t,
		baseURL,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
		5*time.Second,
	)
	return sessionID
}

func readDurableFactorySession(
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

func resumeFactorySession(
	t *testing.T,
	baseURL string,
	sessionID string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	return postLogicalIdentityJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/resume",
		factoryapi.FactorySessionLifecycleControlRequest{},
	)
}

func listFactorySessionEvents(
	t *testing.T,
	baseURL string,
	sessionID string,
) []factoryapi.FactoryEvent {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(sessionID) + "/events"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build factory session events request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET factory session events: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET factory session events status = %d: %s", response.StatusCode, body)
	}

	var collected []factoryapi.FactoryEvent
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			t.Fatalf("decode factory session event: %v", err)
		}
		collected = append(collected, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read factory session events: %v", err)
	}
	return collected
}

func assertFactoryEventHistoryContinuity(
	t *testing.T,
	before, after []factoryapi.FactoryEvent,
) {
	t.Helper()

	for index, prior := range before {
		if index >= len(after) {
			t.Fatalf("post-restart events shorter than pre-restart prefix at index %d", index)
		}
		current := after[index]
		if prior.Id != current.Id {
			t.Fatalf(
				"event[%d] id changed across restart: before=%q after=%q",
				index,
				prior.Id,
				current.Id,
			)
		}
		if prior.Type != current.Type {
			t.Fatalf(
				"event[%d] type changed across restart: before=%q after=%q",
				index,
				prior.Type,
				current.Type,
			)
		}
	}
}

func listFactorySessionDispatches(
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

func waitForDurableFactorySessionStatus(
	t *testing.T,
	baseURL string,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	timeout time.Duration,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := readDurableFactorySession(t, baseURL, sessionID)
		if session.Status == want {
			return session
		}
		time.Sleep(15 * time.Millisecond)
	}
	session := readDurableFactorySession(t, baseURL, sessionID)
	t.Fatalf("session %s status = %q, want %q within %s", sessionID, session.Status, want, timeout)
	return session
}

func waitForFactorySessionDispatchStatus(
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
		listed := listFactorySessionDispatches(t, baseURL, sessionID)
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

func postLogicalIdentityJSON[T any](t *testing.T, endpoint string, request any) T {
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
}

func strPtr(value string) *string {
	return &value
}

type logicalIdentityResumeBlockingProvider struct {
	mu              sync.Mutex
	calls           int
	blockedOnce     bool
	contextCanceled int
	workflowName    string
}

func newLogicalIdentityResumeBlockingProvider(workflowName string) *logicalIdentityResumeBlockingProvider {
	return &logicalIdentityResumeBlockingProvider{workflowName: workflowName}
}

func (p *logicalIdentityResumeBlockingProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *logicalIdentityResumeBlockingProvider) Execute(
	ctx context.Context,
	_ workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	alreadyBlocked := p.blockedOnce
	p.mu.Unlock()

	if call == 1 {
		return workerexecution.InferenceResponse{
			Content: fmt.Sprintf(`{"text":"live:%s:step-one:step-one:workflows","label":"step-one"}`, p.workflowName),
			ProviderSession: &workerexecution.ProviderSessionMetadata{
				Provider: "mock",
				Kind:     "session_id",
				ID:       "live-provider-session-1",
			},
		}, nil
	}

	if !alreadyBlocked {
		p.mu.Lock()
		p.blockedOnce = true
		p.mu.Unlock()

		<-ctx.Done()
		p.mu.Lock()
		p.contextCanceled++
		p.mu.Unlock()
		return workerexecution.InferenceResponse{}, ctx.Err()
	}

	return workerexecution.InferenceResponse{
		Content: fmt.Sprintf(`{"text":"live:%s:step-two:step-two:workflows","label":"step-two"}`, p.workflowName),
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-session-2",
		},
	}, nil
}

func (p *logicalIdentityResumeBlockingProvider) waitForCanceledInfer(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		canceled := p.contextCanceled
		p.mu.Unlock()
		if canceled > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("provider Infer did not observe canceled workflow context")
}
