package runtime_api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSessionInvocationAPI_ReturnsPrimaryResult(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	recorder := &capturingInvocationMetricsRecorder{}
	server := startFunctionalServerWithArgs(t, dir, false, nil, withWorkerCommands(support.NewStaticSuccessCommandRunner("primary result COMPLETE"), nil),
		withInvocationMetricsRecorder(recorder))

	response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke this", nil))
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
	if part.Text != "primary result COMPLETE" {
		t.Fatalf("primaryResult text = %q, want %q", part.Text, "primary result COMPLETE")
	}

	recorder.assertContainsMetric(t, "invocation.attempts", map[string]string{"input_source": "COMPATIBILITY_CONTENT"})
	recorder.assertContainsMetric(t, "invocation.fallback_policy_used", map[string]string{"input_source": "COMPATIBILITY_CONTENT"})
	recorder.assertContainsMetric(t, "invocation.success", map[string]string{"input_source": "COMPATIBILITY_CONTENT"})
	recorder.assertContainsMetric(t, "invocation.result_type", map[string]string{"input_source": "COMPATIBILITY_CONTENT", "result_type": "text"})
}

func TestSessionInvocationAPI_AcceptsStructuredArgsWithActiveSignature(t *testing.T) {
	dir := scaffoldStructuredArgsInvocationFactory(t)
	server := startFunctionalServerWithArgs(t, dir, false, nil, withWorkerCommands(support.NewStaticSuccessCommandRunner("structured primary COMPLETE"), nil))

	response := postInvocation(t, server.URL(), factoryapi.InvocationRequest{
		Args: &map[string]any{"input": "structured invoke"},
	})
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
	if part.Text != "structured primary COMPLETE" {
		t.Fatalf("primaryResult text = %q, want %q", part.Text, "structured primary COMPLETE")
	}
}

func TestSessionInvocationAPI_RejectsWhitespaceOnlyText(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	server := startFunctionalServerWithArgs(t, dir, false, nil, withWorkerCommands(support.NewStaticSuccessCommandRunner("primary result COMPLETE"), nil))

	response := postInvocationExpectStatus(
		t,
		server.URL(),
		textInvocationRequest(t, "   ", nil),
		http.StatusBadRequest,
	)
	if string(response.Code) != "INVOCATION_INPUT_EMPTY" {
		t.Fatalf("invocation error code = %q, want INVOCATION_INPUT_EMPTY", response.Code)
	}
}

func TestSessionInvocationAPI_RejectsArgsWithoutActiveSignature(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	server := startFunctionalServerWithArgs(t, dir, false, nil, withWorkerCommands(support.NewStaticSuccessCommandRunner("primary result COMPLETE"), nil))

	response := postInvocationExpectStatus(
		t,
		server.URL(),
		factoryapi.InvocationRequest{
			Args: &map[string]any{"input": "hello"},
		},
		http.StatusBadRequest,
	)
	if string(response.Code) != "INVOCATION_ARGUMENT_INVALID_ACTIVE_SIGNATURE" {
		t.Fatalf("invocation error code = %q, want INVOCATION_ARGUMENT_INVALID_ACTIVE_SIGNATURE", response.Code)
	}
}

func TestSessionInvocationAPI_RejectsInvalidStructuredArgValueShape(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	server := startFunctionalServerWithArgs(t, dir, false, nil, withWorkerCommands(support.NewStaticSuccessCommandRunner("primary result COMPLETE"), nil))

	response := postInvocationExpectStatus(
		t,
		server.URL(),
		factoryapi.InvocationRequest{
			Args: &map[string]any{"input": 7},
		},
		http.StatusBadRequest,
	)
	if string(response.Code) != "BAD_REQUEST" {
		t.Fatalf("invocation error code = %q, want BAD_REQUEST", response.Code)
	}
}

func TestSessionInvocationAPI_UnresolvedPrimaryResultReturnsFailedStatus(t *testing.T) {
	dir := scaffoldInvocationFactory(t, map[string]any{
		"invocationReturn": map[string]any{
			"policy":        "EXPLICIT",
			"workTypeName":  "summary",
			"terminalState": "complete",
		},
		"workTypes": append(simplePipelineConfig()["workTypes"].([]map[string]any), map[string]any{
			"name": "summary",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}),
	})
	server := startFunctionalServerWithArgs(t, dir, false, nil, withWorkerCommands(support.NewStaticSuccessCommandRunner("task output COMPLETE"), nil))

	response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke this", nil))
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_PRIMARY_RESULT_UNRESOLVED", response.ErrorCode)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("invocation primaryResult = %#v, want nil on unresolved output", response.PrimaryResult)
	}
}

func TestSessionInvocationAPI_TimeoutReturnsTimedOutStatus(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	blocking := newBlockingInvocationRunner()
	server := startFunctionalServerWithArgs(t, dir, false, nil, withWorkerCommands(blocking, nil))

	timeoutMillis := int64(10)
	response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke this", &timeoutMillis))
	if response.Status != factoryapi.InvocationTerminalStatusTimedOut {
		t.Fatalf("invocation status = %q, want TIMED_OUT", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONTIMEDOUT {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_TIMED_OUT", response.ErrorCode)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("invocation primaryResult = %#v, want nil on timeout", response.PrimaryResult)
	}
}

func TestSessionInvocationAPI_PausedSessionReturnsPausedStatus(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, support.NewStaticSuccessCommandRunner("primary result COMPLETE"), nil)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	postJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		server.URL()+"/factory-sessions/"+factorysessions.DefaultSessionID+"/pause",
		factoryapi.FactorySessionLifecycleControlRequest{},
		"pause live factory session",
	)

	response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke this", nil))
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.InvocationResponseErrorCode("INVOCATION_PAUSED") {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_PAUSED", response.ErrorCode)
	}
	if response.Message == nil || !strings.Contains(*response.Message, `session "`+factorysessions.DefaultSessionID+`" is paused`) {
		gotMessage := "<nil>"
		if response.Message != nil {
			gotMessage = *response.Message
		}
		t.Fatalf("invocation message = %q, want paused session detail", gotMessage)
	}
	if response.SessionId == nil || *response.SessionId != factorysessions.DefaultSessionID {
		t.Fatalf("invocation sessionId = %#v, want %q", response.SessionId, factorysessions.DefaultSessionID)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("invocation primaryResult = %#v, want nil on paused output", response.PrimaryResult)
	}
}

func TestSessionInvocationAPI_CanceledRequestContextStopsRequest(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	blocking := newBlockingInvocationRunner()
	edges := serviceedges.Edges{}
	support.ConfigureWorkerCommands(t, &edges, blocking, nil)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	body, err := json.Marshal(textInvocationRequest(t, "invoke this", nil))
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		server.URL()+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		bytes.NewReader(body),
	)
	if err != nil {
		cancel()
		t.Fatalf("build invocation request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	errCh := make(chan error, 1)
	go func() {
		response, requestErr := http.DefaultClient.Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
		errCh <- requestErr
	}()

	<-blocking.started
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled invocation request error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled invocation request did not return")
	}
}

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

func scaffoldStructuredArgsInvocationFactory(t *testing.T) string {
	t.Helper()

	cfg := simplePipelineConfig()
	cfg["invocationSignature"] = map[string]any{
		"parameters": []any{
			map[string]any{
				"name":     "input",
				"required": true,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			},
		},
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
		t.Fatalf("POST /factory-sessions/~default/invocations status = %d, want %d", response.StatusCode, wantStatus)
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

var _ platformprocess.CommandRunner = (*blockingInvocationRunner)(nil)

type capturingInvocationMetricsRecorder struct {
	mu      sync.Mutex
	metrics []factorysessions.InvocationMetric
}

func (r *capturingInvocationMetricsRecorder) RecordInvocationMetric(metric factorysessions.InvocationMetric) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metrics = append(r.metrics, metric)
}

func (r *capturingInvocationMetricsRecorder) assertContainsMetric(t *testing.T, name string, labels map[string]string) {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, metric := range r.metrics {
		if metric.Name != name {
			continue
		}
		if metricLabelsContain(metric.Labels, labels) {
			return
		}
	}
	t.Fatalf("metric %q with labels %#v not found in %#v", name, labels, r.metrics)
}

func metricLabelsContain(got, want map[string]string) bool {
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}
