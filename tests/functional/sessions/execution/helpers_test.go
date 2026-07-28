package execution_test

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
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	terminalSuccessPrimaryResult         = "primary result COMPLETE"
	inlineJavaScriptWorkflowFileName     = "results-dispatches.workflow.js"
	dispatchCorrelationWorkflowFileName  = "results-dispatches-correlation.workflow.js"
	partialResultWorkflowName              = "resumable-two-step-fake-children"
	dispatchCorrelationChildLabel        = "dispatch-correlation-child"
	dispatchCorrelationChildPrompt       = "prove-dispatch-correlation"
	partialResultCheckpointLabel         = "after-step-one"
	partialResultFirstDispatchID         = "dispatch-1"
	partialResultSecondDispatchID        = "dispatch-2"
)

type partialResultBlockingProvider struct {
	mu              sync.Mutex
	calls           int
	blockedOnce     bool
	contextCanceled int
	workflowName    string
}

func newPartialResultBlockingProvider(workflowName string) *partialResultBlockingProvider {
	return &partialResultBlockingProvider{workflowName: workflowName}
}

func (p *partialResultBlockingProvider) waitForCanceledInfer(t *testing.T, timeout time.Duration) {
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

func (p *partialResultBlockingProvider) Infer(
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
				ID:       "partial-result-provider-session-1",
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
			ID:       "partial-result-provider-session-2",
		},
	}, nil
}

var dispatchCorrelationWorkflow = `return (async function () {
  const child = await agent.run({
    prompt: "` + dispatchCorrelationChildPrompt + `",
    label: "` + dispatchCorrelationChildLabel + `",
  });
  return { child };
})();`

type blockingInvocationRunner struct {
	started chan struct{}
}

func newBlockingInvocationRunner() *blockingInvocationRunner {
	return &blockingInvocationRunner{started: make(chan struct{}, 1)}
}

func (r *blockingInvocationRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return platformprocess.CommandResult{}, ctx.Err()
}

var _ platformprocess.CommandRunner = (*blockingInvocationRunner)(nil)

func simplePipelineConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func scaffoldInvocationFactory(t *testing.T, overrides map[string]any) string {
	t.Helper()

	cfg := simplePipelineConfig()
	for key, value := range overrides {
		cfg[key] = value
	}
	workTypes := cfg["workTypes"].([]map[string]any)
	for i := range workTypes {
		if name, _ := workTypes[i]["name"].(string); name == "task" {
			workTypes[i]["handlingBehavior"] = []string{"DEFAULT"}
		}
	}
	dir := support.ScaffoldFactory(t, cfg)
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir
}

func scaffoldDispatchCorrelationFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "results-dispatches-correlation"})
	if err := os.WriteFile(
		filepath.Join(dir, dispatchCorrelationWorkflowFileName),
		[]byte(dispatchCorrelationWorkflow),
		0o600,
	); err != nil {
		t.Fatalf("write dispatch correlation workflow: %v", err)
	}
	return dir
}

func scaffoldPartialResultFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{"name": "results-dispatches-partial"})
	workflowDir := filepath.Join(dir, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	fixturePath := support.AgentFactoryPath(
		t,
		filepath.Join("tests", "fixtures", "javascript_runtime", partialResultWorkflowName+".workflow.js"),
	)
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read workflow fixture %s: %v", fixturePath, err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, partialResultWorkflowName+".js"), raw, 0o600); err != nil {
		t.Fatalf("write partial result workflow: %v", err)
	}
	return dir
}

func scaffoldInlineJavaScriptFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"inlineSource": map[string]any{
					"encoding": "utf-8",
					"inline":   `workflow.final("` + terminalSuccessPrimaryResult + `");`,
				},
			},
		},
	})
	if err := os.WriteFile(
		filepath.Join(dir, inlineJavaScriptWorkflowFileName),
		[]byte(`workflow.final("`+terminalSuccessPrimaryResult+`");`),
		0o600,
	); err != nil {
		t.Fatalf("write inline JavaScript workflow: %v", err)
	}
	return dir
}

func startInvocationServer(
	t *testing.T,
	factoryDir string,
	providerRunner, scriptRunner platformprocess.CommandRunner,
) *support.FunctionalAPIServer {
	t.Helper()

	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, providerRunner, scriptRunner)
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
}

func textInvocationRequest(t *testing.T, text string, timeoutMillis *int64) factoryapi.InvocationRequest {
	t.Helper()

	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		t.Fatalf("build invocation text content: %v", err)
	}
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := factoryapi.WorkContent{part}
	return factoryapi.InvocationRequest{
		SourceKind:    &sourceKind,
		Content:       &content,
		TimeoutMillis: timeoutMillis,
	}
}

func postInvocationExpectStatus(
	t *testing.T,
	serverURL string,
	request factoryapi.InvocationRequest,
	wantStatus int,
) factoryapi.ErrorResponse {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	response, err := http.Post(
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/~default/invocations: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"POST /factory-sessions/~default/invocations status = %d, want %d: %s",
			response.StatusCode,
			wantStatus,
			strings.TrimSpace(string(payload)),
		)
	}

	var decoded factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation error response: %v", err)
	}
	return decoded
}

func postInvocation(t *testing.T, serverURL string, request factoryapi.InvocationRequest) factoryapi.InvocationResponse {
	t.Helper()

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	response, err := http.Post(
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/~default/invocations: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf(
			"POST /factory-sessions/~default/invocations status = %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(payload)),
		)
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

func startDispatchCorrelationSync(
	t *testing.T,
	serverURL, factoryDir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(factoryDir, dispatchCorrelationWorkflowFileName)
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "results-dispatches-correlation-sync",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal dispatch correlation sync request: %v", err)
	}

	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build dispatch correlation sync request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, strings.TrimSpace(string(payload)))
	}

	var result factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode dispatch correlation sync response: %v", err)
	}
	return result
}

func listFactorySessionDispatches(
	t *testing.T,
	serverURL, sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()

	return support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/dispatches",
	)
}

func getFactorySessionDispatch(
	t *testing.T,
	serverURL, sessionID, dispatchID string,
) factoryapi.FactoryDispatch {
	t.Helper()

	return support.GetJSON[factoryapi.FactoryDispatch](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/dispatches/"+dispatchID,
	)
}

func startInlineJavaScriptSync(
	t *testing.T,
	serverURL, factoryDir string,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	workflowPath := filepath.Join(factoryDir, inlineJavaScriptWorkflowFileName)
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "results-dispatches-inline-js-sync",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
			WorkflowFile: &workflowPath,
		},
	})
	if err != nil {
		t.Fatalf("marshal sync execution request: %v", err)
	}

	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/sync"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build sync execution request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, strings.TrimSpace(string(payload)))
	}

	var result factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode sync execution response: %v", err)
	}
	return result
}

func readDurableSessionResult(
	t *testing.T,
	serverURL, sessionID string,
) factoryapi.FactorySessionResult {
	t.Helper()

	return readDurableSessionResultWithMode(t, serverURL, sessionID, "final")
}

func readDurableSessionResultWithMode(
	t *testing.T,
	serverURL, sessionID, mode string,
) factoryapi.FactorySessionResult {
	t.Helper()

	return support.GetJSON[factoryapi.FactorySessionResult](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID+"/results?mode="+mode,
	)
}

func readDurableSession(
	t *testing.T,
	serverURL, sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID,
	)
	session, err := response.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode durable session %s: %v", sessionID, err)
	}
	if session.SessionId != sessionID {
		t.Fatalf("durable session id = %q, want %q", session.SessionId, sessionID)
	}
	return session
}

func startPartialResultAsync(
	t *testing.T,
	serverURL string,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()

	workflowName := partialResultWorkflowName
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "results-dispatches-partial-async",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: &workflowName,
		},
		Args: &map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("marshal partial result async request: %v", err)
	}

	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/async"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build partial result async request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, strings.TrimSpace(string(payload)))
	}

	var started factoryapi.FactorySessionExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode partial result async response: %v", err)
	}
	return started
}

func waitForDurableSessionStatus(
	t *testing.T,
	serverURL, sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := readDurableSession(t, serverURL, sessionID)
		if session.Status == want {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	session := readDurableSession(t, serverURL, sessionID)
	t.Fatalf("durable session %s status = %q, want %q within %s", sessionID, session.Status, want, timeout)
}

func waitForFactoryDispatchStatus(
	t *testing.T,
	serverURL, sessionID, dispatchID string,
	want factoryapi.FactoryDispatchStatus,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed := listFactorySessionDispatches(t, serverURL, sessionID)
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

func waitForDurablePartialResult(
	t *testing.T,
	serverURL, sessionID string,
	timeout time.Duration,
) factoryapi.FactorySessionResult {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		partial := readDurableSessionResultWithMode(t, serverURL, sessionID, "partial")
		if partial.ResultStatus == factoryapi.FactorySessionResultStatusPartial &&
			partial.PrimaryResult != nil && len(*partial.PrimaryResult) > 0 {
			return partial
		}
		time.Sleep(25 * time.Millisecond)
	}
	partial := readDurableSessionResultWithMode(t, serverURL, sessionID, "partial")
	t.Fatalf(
		"partial result = %#v, want PARTIAL status with primaryResult before %s",
		partial,
		timeout,
	)
	return partial
}

func interruptFactoryDispatch(
	t *testing.T,
	serverURL, sessionID, dispatchID, reason string,
) {
	t.Helper()

	payload, err := json.Marshal(factoryapi.FactorySessionInterruptDispatchRequest{
		DispatchId: dispatchID,
		Reason:     &reason,
	})
	if err != nil {
		t.Fatalf("marshal interrupt dispatch request: %v", err)
	}

	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + sessionID + "/interrupt-dispatch"
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build interrupt dispatch request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
}

func releaseBlockedPartialResultSession(
	t *testing.T,
	serverURL string,
	provider *partialResultBlockingProvider,
	sessionID string,
) {
	t.Helper()

	reason := "results dispatches partial result cleanup"
	interruptFactoryDispatch(t, serverURL, sessionID, partialResultSecondDispatchID, reason)
	provider.waitForCanceledInfer(t, 5*time.Second)
	waitForDurableSessionStatus(
		t,
		serverURL,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
		5*time.Second,
	)
}

func assertInvocationPrimaryResultText(
	t *testing.T,
	response factoryapi.InvocationResponse,
	wantText string,
) {
	t.Helper()

	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED", response.Status)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("invocation primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	if part.Text != wantText {
		t.Fatalf("primaryResult text = %q, want %q", part.Text, wantText)
	}
}

func assertAPIInvocationMatchesCLICompatibleFacts(
	t *testing.T,
	apiResponse factoryapi.InvocationResponse,
	cliResponse factoryapi.InvocationResponse,
	wantPrimaryResult string,
) {
	t.Helper()

	assertInvocationPrimaryResultText(t, apiResponse, wantPrimaryResult)
	assertInvocationPrimaryResultText(t, cliResponse, wantPrimaryResult)

	if strings.TrimSpace(apiResponse.RequestId) == "" || strings.TrimSpace(apiResponse.TraceId) == "" {
		t.Fatalf(
			"API invocation identity = request %q trace %q, want non-empty run correlation",
			apiResponse.RequestId,
			apiResponse.TraceId,
		)
	}
	if strings.TrimSpace(cliResponse.RequestId) == "" || strings.TrimSpace(cliResponse.TraceId) == "" {
		t.Fatalf(
			"CLI invocation identity = request %q trace %q, want non-empty run correlation",
			cliResponse.RequestId,
			cliResponse.TraceId,
		)
	}

	apiText := invocationPrimaryResultText(t, apiResponse)
	cliText := invocationPrimaryResultText(t, cliResponse)
	if apiText != cliText {
		t.Fatalf("primaryResult mismatch: API = %q, CLI-compatible = %q", apiText, cliText)
	}
	if apiResponse.Status != cliResponse.Status {
		t.Fatalf("invocation status mismatch: API = %q, CLI-compatible = %q", apiResponse.Status, cliResponse.Status)
	}
}

func invocationPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()

	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	return part.Text
}

func assertFactorySessionResultPrimaryText(
	t *testing.T,
	result factoryapi.FactorySessionResult,
	wantText string,
) {
	t.Helper()

	if result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", result.ResultStatus)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as json part: %v", err)
	}
	got, ok := part.Json.(string)
	if !ok || got != wantText {
		t.Fatalf("primaryResult json = %#v, want string %q", part.Json, wantText)
	}
}

func assertTerminalWorkPrimaryText(
	t *testing.T,
	serverURL, wantText string,
) {
	t.Helper()

	ok, diagnostic := tryReadTerminalWorkPrimaryText(serverURL, wantText)
	if !ok {
		t.Fatal(diagnostic)
	}
}

// waitForTerminalWorkPrimaryText polls the public /work surface until one
// terminal work item exposes the expected primary text, or times out. Hosted
// CLI invocations complete asynchronously relative to the first successful
// API read, so visibility proofs must wait for terminal projection instead of
// asserting on the first PROCESSING snapshot.
func waitForTerminalWorkPrimaryText(
	t *testing.T,
	serverURL, wantText string,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var lastDiagnostic string
	for {
		ok, diagnostic := tryReadTerminalWorkPrimaryText(serverURL, wantText)
		if ok {
			return
		}
		lastDiagnostic = diagnostic
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf(
				"timed out waiting for terminal work with text %q at %s: %s",
				wantText,
				serverURL,
				lastDiagnostic,
			)
		}
	}
}

func tryReadTerminalWorkPrimaryText(serverURL, wantText string) (bool, string) {
	endpoint := support.DefaultSessionWorkURL(serverURL, "/work")
	response, err := http.Get(endpoint)
	if err != nil {
		return false, fmt.Sprintf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		return false, fmt.Sprintf(
			"GET %s status = %d: %s",
			endpoint,
			response.StatusCode,
			strings.TrimSpace(string(payload)),
		)
	}
	var listed factoryapi.ListWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		return false, fmt.Sprintf("decode GET %s: %v", endpoint, err)
	}
	if len(listed.Results) != 1 {
		return false, fmt.Sprintf(
			"listed work count = %d, want 1; listed=%#v",
			len(listed.Results),
			listed.Results,
		)
	}
	item := listed.Results[0]
	if item.State == nil || generatedWorkStateType(item.State) != factoryapi.WorkStateTypeTERMINAL {
		return false, fmt.Sprintf("work state = %#v, want TERMINAL", item.State)
	}
	if item.Content == nil || len(*item.Content) != 1 {
		return false, fmt.Sprintf("work content = %#v, want one text part", item.Content)
	}
	part, err := (*item.Content)[0].AsWorkTextContentPart()
	if err != nil {
		return false, fmt.Sprintf("work content[0] as text part: %v", err)
	}
	if part.Text != wantText {
		return false, fmt.Sprintf("work content text = %q, want %q", part.Text, wantText)
	}
	return true, ""
}

func generatedWorkStateType(state *factoryapi.WorkState) factoryapi.WorkStateType {
	if state == nil {
		return ""
	}
	return state.Type
}

func assertDispatchListDetailPublicCorrelation(
	t *testing.T,
	sessionID string,
	summary factoryapi.FactorySessionDispatchSummary,
	detail factoryapi.FactoryDispatch,
) {
	t.Helper()

	if strings.TrimSpace(summary.Id) == "" {
		t.Fatal("dispatch summary id is empty, want public dispatch identifier")
	}
	if detail.Id != summary.Id {
		t.Fatalf("dispatch detail id = %q, want list summary id %q", detail.Id, summary.Id)
	}
	if detail.SessionId != sessionID {
		t.Fatalf("dispatch detail sessionId = %q, want %q", detail.SessionId, sessionID)
	}
	if detail.Status != summary.Status {
		t.Fatalf("dispatch detail status = %q, want list summary status %q", detail.Status, summary.Status)
	}
	if detail.DispatchKind != summary.DispatchKind {
		t.Fatalf("dispatch detail dispatchKind = %q, want list summary dispatchKind %q", detail.DispatchKind, summary.DispatchKind)
	}
	if detail.OrchestratorKind != factoryapi.JAVASCRIPT {
		t.Fatalf("dispatch detail orchestratorKind = %q, want JAVASCRIPT", detail.OrchestratorKind)
	}
	if summary.Label == nil || detail.Label == nil || *detail.Label != *summary.Label {
		t.Fatalf("dispatch detail label = %#v, want list summary label %#v", detail.Label, summary.Label)
	}
	if summary.ProviderSessionRefs != nil && len(*summary.ProviderSessionRefs) > 0 {
		if detail.ProviderSessionRefs == nil {
			t.Fatalf("dispatch detail providerSessionRefs = nil, want list summary refs %#v", summary.ProviderSessionRefs)
		}
		if len(*detail.ProviderSessionRefs) != len(*summary.ProviderSessionRefs) {
			t.Fatalf(
				"dispatch detail providerSessionRefs count = %d, want %d from list summary",
				len(*detail.ProviderSessionRefs),
				len(*summary.ProviderSessionRefs),
			)
		}
		for i := range *summary.ProviderSessionRefs {
			summaryRef := (*summary.ProviderSessionRefs)[i]
			detailRef := (*detail.ProviderSessionRefs)[i]
			if detailRef.Id != summaryRef.Id {
				t.Fatalf(
					"providerSessionRefs[%d].id = %q, want list summary ref %q",
					i,
					detailRef.Id,
					summaryRef.Id,
				)
			}
			if detailRef.Provider != summaryRef.Provider {
				t.Fatalf(
					"providerSessionRefs[%d].provider = %q, want list summary ref %q",
					i,
					detailRef.Provider,
					summaryRef.Provider,
				)
			}
		}
	}
}

func assertPartialResultObservableBeforeTerminal(
	t *testing.T,
	partial factoryapi.FactorySessionResult,
) {
	t.Helper()

	if partial.ResultStatus != factoryapi.FactorySessionResultStatusPartial {
		t.Fatalf("resultStatus = %q, want PARTIAL", partial.ResultStatus)
	}
	if partial.PrimaryResult == nil || len(*partial.PrimaryResult) == 0 {
		t.Fatalf("primaryResult = %#v, want observable partial content", partial.PrimaryResult)
	}
	part, err := (*partial.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as json part: %v", err)
	}
	payload, ok := part.Json.(map[string]any)
	if !ok {
		t.Fatalf("primaryResult json = %#v, want object", part.Json)
	}
	step, ok := payload["step"].(float64)
	if !ok || int(step) != 1 {
		t.Fatalf("primaryResult step = %#v, want checkpoint step 1", payload["step"])
	}
}

func waitForBlockingInvocationStart(t *testing.T, blocking *blockingInvocationRunner) {
	t.Helper()

	select {
	case <-blocking.started:
	case <-time.After(time.Second):
		t.Fatal("blocking invocation did not start")
	}
}
