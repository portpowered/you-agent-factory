package lifecycle_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const fscp01ObservationTimeout = 5 * time.Second

// TestFSCP01StartOpenIdentityStatusModeMatrix records the current public
// identity/status/mode split between live folder opens and durable starts. The
// source and policy tuple is held constant for the async/sync durable pair;
// request IDs differ because they are idempotency identities, not execution
// semantics.
func TestFSCP01StartOpenIdentityStatusModeMatrix(t *testing.T) {
	t.Parallel()
	fixture := requireSharedPlacementFixture(t)
	selected, autoOpened := openFSCP01LivePair(t, fixture)
	durable := startFSCP01DurablePair(t, fixture)

	t.Logf("FSCP01-HERMETIC-002 live selected id=%q status=%q mode=live folder=%q", selected.id, selected.read.Runtime.Status, selected.folder)
	t.Logf("FSCP01-HERMETIC-002 live folder id=%q status=%q mode=live folder=%q", autoOpened.id, autoOpened.read.Runtime.Status, autoOpened.folder)
	t.Logf("FSCP01-HERMETIC-002 durable async id=%q status=%q mode=durable/async", durable.asyncStarted.SessionId, durable.asyncStarted.Status)
	t.Logf("FSCP01-HERMETIC-002 durable sync id=%q status=%q mode=durable/sync outcome=%q result=%q", durable.syncStarted.SessionId, durable.syncStarted.Status, durable.syncStarted.SyncOutcome, durable.syncStarted.Result.ResultStatus)
}

// TestFSCP01InvokeLifecycleAndResultOutcomeMatrix joins the existing public
// invocation path to the current live and durable control/result outcomes. It
// deliberately observes only public responses and session read models; the
// canonical reconnect/artifact and field-provenance edges remain story 003.
func TestFSCP01InvokeLifecycleAndResultOutcomeMatrix(t *testing.T) {
	if fixture := requireSharedPlacementFixture(t); fixture.client != nil {
		fixture.client.initializeCustomerHome(t, fixture.clientWorkingDir)
	}
	t.Parallel()
	fixture := requireSharedPlacementFixture(t)
	invokedStatus, invokedResult := assertFSCP01Invocation(t, fixture)
	liveID, pauseOutcome, resumeOutcome := exerciseFSCP01LiveControls(t, fixture)
	finalStarted, finalResult := startFSCP01FinalResult(t, fixture)
	missingCode := assertFSCP01MissingSource(t, fixture)
	noCancel, canceled := exerciseFSCP01Timeouts(t, fixture)

	if finalStarted.SessionId == noCancel.started.SessionId || finalStarted.SessionId == canceled.started.SessionId || noCancel.started.SessionId == canceled.started.SessionId {
		t.Fatalf("durable session identities are not isolated: final=%q noCancel=%q cancel=%q", finalStarted.SessionId, noCancel.started.SessionId, canceled.started.SessionId)
	}
	t.Logf("FSCP01-HERMETIC-002 invoke status=%q result=%q; live id=%q pause=%q resume=%q; final id=%q status=%q result=%q; missing source code=%q; timeout(no-cancel) id=%q status=%q; timeout(cancel) id=%q terminal=%q", invokedStatus, invokedResult, liveID, pauseOutcome, resumeOutcome, finalStarted.SessionId, finalStarted.Status, finalResult.ResultStatus, missingCode, noCancel.started.SessionId, noCancel.read.Status, canceled.started.SessionId, canceled.read.Status)
}

type fscp01LiveObservation struct {
	id     string
	folder string
	read   factoryapi.FactorySession
}

type fscp01DurablePair struct {
	asyncStarted factoryapi.FactorySessionExecutionResponse
	syncStarted  factoryapi.FactorySessionSyncExecutionResponse
}

type fscp01TimeoutObservation struct {
	started factoryapi.FactorySessionSyncExecutionResponse
	read    factoryapi.FactorySessionDurableReadModel
}

func openFSCP01LivePair(t *testing.T, fixture *sharedLifecycleFixture) (fscp01LiveObservation, fscp01LiveObservation) {
	t.Helper()
	selectedFolder := filepath.Join(t.TempDir(), "selected-live-factory")
	if err := os.MkdirAll(selectedFolder, 0o755); err != nil {
		t.Fatalf("create selected live Factory folder: %v", err)
	}
	if err := writeLifecycleFactory(selectedFolder); err != nil {
		t.Fatalf("write selected live Factory: %v", err)
	}
	defaultTarget := factoryapi.FactorySessionTargetRef{Kind: factoryapi.FactorySessionTargetRefKindDefault}
	selectedResponse := postFSCP01Open(t, fixture.baseURL, factoryapi.OpenFactorySessionRequest{
		FolderPath: selectedFolder,
		Target:     &defaultTarget,
	})
	selected := assertFSCP01OpenedLive(t, fixture, selectedResponse, selectedFolder)

	autoFolder := filepath.Join(t.TempDir(), "auto-live-factory")
	if err := os.MkdirAll(autoFolder, 0o755); err != nil {
		t.Fatalf("create auto live Factory folder: %v", err)
	}
	initNewFactory := true
	autoResponse := postFSCP01Open(t, fixture.baseURL, factoryapi.OpenFactorySessionRequest{
		FolderPath:     autoFolder,
		InitNewFactory: &initNewFactory,
	})
	auto := assertFSCP01OpenedLive(t, fixture, autoResponse, autoFolder)
	if selected.id == auto.id {
		t.Fatalf("live open identities = %q/%q, want distinct sessions", selected.id, auto.id)
	}
	return selected, auto
}

func assertFSCP01OpenedLive(t *testing.T, fixture *sharedLifecycleFixture, opened factoryapi.OpenFactorySessionResponse, folder string) fscp01LiveObservation {
	t.Helper()
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("live open response = %#v, want a session identity", opened)
	}
	sessionID := opened.Session.Id
	registerLifecycleSessionCleanupAt(t, fixture.baseURL, sessionID, folder)
	read := readFSCP01LiveSession(t, fixture.baseURL, sessionID)
	assertFSCP01LiveSession(t, read, sessionID, folder)
	return fscp01LiveObservation{id: sessionID, folder: folder, read: read}
}

func startFSCP01DurablePair(t *testing.T, fixture *sharedLifecycleFixture) fscp01DurablePair {
	t.Helper()
	asyncStarted := postRemoteFunctionalExecution(t, fixture.baseURL, fscp01WorkflowRequest("fscp01-async-"+uuid.NewString(), placementSuccessFactoryName))
	assertFSCP01DurableStart(t, asyncStarted.SessionId, asyncStarted.Status, "async")
	registerLifecycleSessionCleanup(t, fixture.baseURL, asyncStarted.SessionId)

	syncStarted := postFSCP01Sync(t, fixture.baseURL, fscp01WorkflowRequest("fscp01-sync-"+uuid.NewString(), placementSuccessFactoryName))
	if strings.TrimSpace(syncStarted.SessionId) == "" {
		t.Fatalf("durable sync start = %#v, want a session identity", syncStarted)
	}
	registerLifecycleSessionCleanup(t, fixture.baseURL, syncStarted.SessionId)
	if syncStarted.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded || syncStarted.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted {
		t.Fatalf("durable sync start = %#v, want SUCCEEDED/COMPLETED", syncStarted)
	}
	if syncStarted.Result == nil || syncStarted.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("durable sync result = %#v, want FINAL result", syncStarted.Result)
	}
	if asyncStarted.SessionId == syncStarted.SessionId {
		t.Fatalf("async/sync session identities = %q/%q, want independent identities", asyncStarted.SessionId, syncStarted.SessionId)
	}
	asyncRead := readFSCP01DurableSession(t, fixture.baseURL, asyncStarted.SessionId)
	syncRead := readFSCP01DurableSession(t, fixture.baseURL, syncStarted.SessionId)
	if asyncRead.SessionId != asyncStarted.SessionId || syncRead.SessionId != syncStarted.SessionId {
		t.Fatalf("durable read identities = %q/%q, want start identities %q/%q", asyncRead.SessionId, syncRead.SessionId, asyncStarted.SessionId, syncStarted.SessionId)
	}
	if asyncRead.OrchestratorKind != factoryapi.JAVASCRIPT || syncRead.OrchestratorKind != factoryapi.JAVASCRIPT {
		t.Fatalf("durable modes/orchestrators = %q/%q, want JAVASCRIPT", asyncRead.OrchestratorKind, syncRead.OrchestratorKind)
	}
	return fscp01DurablePair{asyncStarted: asyncStarted, syncStarted: syncStarted}
}

func assertFSCP01Invocation(t *testing.T, fixture *sharedLifecycleFixture) (factoryapi.InvocationTerminalStatus, string) {
	t.Helper()
	invocation := executePlacementRun(t, fixture.client, fixture.clientWorkingDir, placementSuccessFactoryName, "", false)
	if invocation.err != nil {
		t.Fatalf("public invocation error = %v\nstdout:\n%s\nstderr:\n%s", invocation.err, invocation.stdout, invocation.stderr)
	}
	invoked := support.DecodeInvocationResponseJSON(t, invocation.stdout)
	if invoked.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("public invocation status = %q, want COMPLETED", invoked.Status)
	}
	if invoked.PrimaryResult == nil || len(*invoked.PrimaryResult) != 1 {
		t.Fatalf("public invocation primaryResult = %#v, want one result part", invoked.PrimaryResult)
	}
	invokedPart, err := (*invoked.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("public invocation primaryResult text part: %v", err)
	}
	if invokedPart.Text != "placement parity complete" {
		t.Fatalf("public invocation primaryResult = %q, want placement parity result", invokedPart.Text)
	}
	return invoked.Status, invokedPart.Text
}

func exerciseFSCP01LiveControls(t *testing.T, fixture *sharedLifecycleFixture) (string, factoryapi.FactorySessionLifecycleControlOutcome, factoryapi.FactorySessionLifecycleControlOutcome) {
	t.Helper()
	liveFolder := filepath.Join(t.TempDir(), "control-live-factory")
	if err := os.MkdirAll(liveFolder, 0o755); err != nil {
		t.Fatalf("create lifecycle live Factory folder: %v", err)
	}
	initNewFactory := true
	opened := postFSCP01Open(t, fixture.baseURL, factoryapi.OpenFactorySessionRequest{FolderPath: liveFolder, InitNewFactory: &initNewFactory})
	if opened.Session == nil || strings.TrimSpace(opened.Session.Id) == "" {
		t.Fatalf("lifecycle live open = %#v, want a session identity", opened)
	}
	liveID := opened.Session.Id
	registerLifecycleSessionCleanupAt(t, fixture.baseURL, liveID, liveFolder)
	paused := postFSCP01LiveControl(t, fixture.baseURL, liveID, "pause")
	if paused.Operation != factoryapi.FactorySessionLifecycleControlKindPause || paused.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("live pause = %#v, want accepted PAUSE", paused)
	}
	pausedRead := readFSCP01LiveSession(t, fixture.baseURL, liveID)
	if pausedRead.Runtime.LifecycleControlStatus == nil || *pausedRead.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("live paused lifecycle status = %#v, want PAUSED", pausedRead.Runtime.LifecycleControlStatus)
	}
	resumed := postFSCP01LiveControl(t, fixture.baseURL, liveID, "resume")
	if resumed.Operation != factoryapi.FactorySessionLifecycleControlKindResume || resumed.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("live resume = %#v, want accepted RESUME", resumed)
	}
	resumedRead := readFSCP01LiveSession(t, fixture.baseURL, liveID)
	if resumedRead.Runtime.LifecycleControlStatus == nil || *resumedRead.Runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("live resumed lifecycle status = %#v, want RUNNING", resumedRead.Runtime.LifecycleControlStatus)
	}
	return liveID, paused.Outcome, resumed.Outcome
}

func startFSCP01FinalResult(t *testing.T, fixture *sharedLifecycleFixture) (factoryapi.FactorySessionSyncExecutionResponse, factoryapi.FactorySessionResult) {
	t.Helper()
	started := postFSCP01Sync(t, fixture.baseURL, fscp01WorkflowRequest("fscp01-final-"+uuid.NewString(), placementSuccessFactoryName))
	if strings.TrimSpace(started.SessionId) == "" {
		t.Fatalf("final durable start = %#v, want a session identity", started)
	}
	registerLifecycleSessionCleanup(t, fixture.baseURL, started.SessionId)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded || started.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted {
		t.Fatalf("final durable start = %#v, want SUCCEEDED/COMPLETED", started)
	}
	result := readFSCP01DurableResult(t, fixture.baseURL, started.SessionId)
	if result.ResultStatus != factoryapi.FactorySessionResultStatusFinal || result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("final durable result = %#v, want FINAL with one primary result part", result)
	}
	return started, result
}

func assertFSCP01MissingSource(t *testing.T, fixture *sharedLifecycleFixture) factoryapi.ErrorResponseCode {
	t.Helper()
	missingWorkflow := "fscp01-missing-" + uuid.NewString()
	status, failedResponse, body := postRemoteFunctionalExecutionRaw(t, fixture.baseURL, fscp01WorkflowRequest("fscp01-missing-"+uuid.NewString(), missingWorkflow))
	if status != http.StatusBadRequest {
		t.Fatalf("missing durable source status = %d body=%s, want 400", status, body)
	}
	if strings.TrimSpace(failedResponse.SessionId) != "" || failedResponse.Links != nil {
		t.Fatalf("missing durable source response = %#v, want no success payload", failedResponse)
	}
	var missingErr factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &missingErr); err != nil {
		t.Fatalf("decode missing durable source error: %v body=%s", err, body)
	}
	if missingErr.Code != factoryapi.ErrorResponseCodeBADREQUEST || strings.TrimSpace(missingErr.Message) == "" {
		t.Fatalf("missing durable source error = %#v, want typed BAD_REQUEST", missingErr)
	}
	return missingErr.Code
}

func exerciseFSCP01Timeouts(t *testing.T, fixture *sharedLifecycleFixture) (fscp01TimeoutObservation, fscp01TimeoutObservation) {
	t.Helper()
	return startFSCP01Timeout(t, fixture, false), startFSCP01Timeout(t, fixture, true)
}

func startFSCP01Timeout(t *testing.T, fixture *sharedLifecycleFixture, cancel bool) fscp01TimeoutObservation {
	t.Helper()
	timeoutMillis := int64(50)
	request := fscp01WorkflowRequest("fscp01-timeout-"+map[bool]string{false: "running", true: "canceled"}[cancel]+"-"+uuid.NewString(), "remote-placement")
	request.Wait = &factoryapi.FactorySessionExecutionWaitOptions{TimeoutMillis: &timeoutMillis, CancelOnTimeout: &cancel}
	started := postFSCP01Sync(t, fixture.baseURL, request)
	if strings.TrimSpace(started.SessionId) == "" || started.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeTimedOut || started.TimedOut == nil || !*started.TimedOut {
		t.Fatalf("sync timeout cancel=%t = %#v, want timed out durable execution", cancel, started)
	}
	if cancel && (started.SessionCanceledByTimeout == nil || !*started.SessionCanceledByTimeout) {
		t.Fatalf("sync timeout with cancel response = %#v, want sessionCanceledByTimeout=true", started.SessionCanceledByTimeout)
	}
	if !cancel && started.SessionCanceledByTimeout != nil && *started.SessionCanceledByTimeout {
		t.Fatalf("sync timeout without cancel response = %#v, must not claim cancellation", started.SessionCanceledByTimeout)
	}
	registerLifecycleSessionCleanup(t, fixture.baseURL, started.SessionId)
	if !cancel {
		read := readFSCP01DurableSession(t, fixture.baseURL, started.SessionId)
		if read.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning && read.Status != factoryapi.FactorySessionDurableLifecycleStatusQueued {
			t.Fatalf("sync timeout without cancel session status = %q, want RUNNING or QUEUED", read.Status)
		}
		return fscp01TimeoutObservation{started: started, read: read}
	}
	read := waitFSCP01DurableTerminal(t, fixture.baseURL, started.SessionId)
	if read.Status != factoryapi.FactorySessionDurableLifecycleStatusCanceled && read.Status != factoryapi.FactorySessionDurableLifecycleStatusInterrupted && read.Status != factoryapi.FactorySessionDurableLifecycleStatusTerminated {
		t.Fatalf("sync timeout with cancel terminal status = %q, want cancellation/interrupt/termination", read.Status)
	}
	return fscp01TimeoutObservation{started: started, read: read}
}

func fscp01WorkflowRequest(requestID, workflowName string) factoryapi.FactorySessionExecutionRequest {
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr(workflowName),
		},
	}
}

func postFSCP01Open(
	t *testing.T,
	baseURL string,
	request factoryapi.OpenFactorySessionRequest,
) factoryapi.OpenFactorySessionResponse {
	t.Helper()
	return postSessionLifecycleJSON[factoryapi.OpenFactorySessionResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions",
		request,
		"open FSCP-01 live Factory Session",
	)
}

func readFSCP01LiveSession(t *testing.T, baseURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	live, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode live Factory Session %q: %v", sessionID, err)
	}
	return live
}

func readFSCP01DurableSession(t *testing.T, baseURL, sessionID string) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	durable, err := response.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode durable Factory Session %q: %v", sessionID, err)
	}
	return durable
}

func assertFSCP01LiveSession(t *testing.T, session factoryapi.FactorySession, sessionID, folderPath string) {
	t.Helper()
	if session.Id != sessionID || strings.TrimSpace(session.Id) == "" {
		t.Fatalf("live session identity = %q, want %q", session.Id, sessionID)
	}
	if session.FolderPath != folderPath {
		t.Fatalf("live session folder = %q, want %q", session.FolderPath, folderPath)
	}
	if session.Runtime.Status != factoryapi.FactorySessionStatusACTIVE && session.Runtime.Status != factoryapi.FactorySessionStatusIDLE {
		t.Fatalf("live session initial status = %q, want ACTIVE or IDLE", session.Runtime.Status)
	}
}

func assertFSCP01DurableStart(t *testing.T, sessionID string, status factoryapi.FactorySessionDurableLifecycleStatus, mode string) {
	t.Helper()
	if strings.TrimSpace(sessionID) == "" {
		t.Fatalf("durable %s session identity is empty", mode)
	}
	switch status {
	case factoryapi.FactorySessionDurableLifecycleStatusQueued,
		factoryapi.FactorySessionDurableLifecycleStatusRunning,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded:
	default:
		t.Fatalf("durable %s initial status = %q, want QUEUED, RUNNING, or SUCCEEDED", mode, status)
	}
}

func postFSCP01Sync(t *testing.T, baseURL string, request factoryapi.FactorySessionExecutionRequest) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal FSCP-01 sync request: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/sync"
	httpRequest, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build FSCP-01 sync request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read POST %s response: %v", endpoint, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var decoded factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		t.Fatalf("decode POST %s response: %v body=%s", endpoint, err, responseBody)
	}
	return decoded
}

func postFSCP01LiveControl(t *testing.T, baseURL, sessionID, operation string) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/" + operation
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("build live %s request: %v", operation, err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read POST %s response: %v", endpoint, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		t.Fatalf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var decoded factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		t.Fatalf("decode POST %s response: %v body=%s", endpoint, err, responseBody)
	}
	return decoded
}

func readFSCP01DurableResult(t *testing.T, baseURL, sessionID string) factoryapi.FactorySessionResult {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/results?mode=final"
	return support.GetJSON[factoryapi.FactorySessionResult](t, endpoint)
}

func waitFSCP01DurableTerminal(t *testing.T, baseURL, sessionID string) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	last, err := support.WaitForObservation(
		fscp01ObservationTimeout,
		func() (factoryapi.FactorySessionDurableReadModel, error) {
			return readFSCP01DurableSession(t, baseURL, sessionID), nil
		},
		func(session factoryapi.FactorySessionDurableReadModel) bool {
			switch session.Status {
			case factoryapi.FactorySessionDurableLifecycleStatusCanceled,
				factoryapi.FactorySessionDurableLifecycleStatusFailed,
				factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
				factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
				factoryapi.FactorySessionDurableLifecycleStatusTerminated,
				factoryapi.FactorySessionDurableLifecycleStatusTimedOut:
				return true
			default:
				return false
			}
		},
	)
	if err != nil {
		t.Fatalf("durable Factory Session %q did not reach terminal status: %v", sessionID, err)
	}
	return last
}
