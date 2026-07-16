package runtime_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSessionInvocationAPI_ReturnsPrimaryResult(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	core, observedLogs := observer.New(zap.InfoLevel)
	recorder := &capturingInvocationMetricsRecorder{}
	server := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		support.ConfigureWorkerCommands(t, cfg, support.NewStaticSuccessCommandRunner("primary result COMPLETE"), nil)
		cfg.Logger = zap.New(core)
		cfg.InvocationMetricsRecorder = recorder
	})

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

	submitted := observedLogs.FilterMessage("factory session invocation submitted").All()
	if len(submitted) != 1 {
		t.Fatalf("submitted invocation log count = %d, want 1", len(submitted))
	}
	submittedFields := submitted[0].ContextMap()
	if got := submittedFields["input_source"]; got != "COMPATIBILITY_CONTENT" {
		t.Fatalf("submitted input_source = %#v, want COMPATIBILITY_CONTENT", got)
	}
	if got := submittedFields["invocation_return_policy_mode"]; got != "fallback" {
		t.Fatalf("submitted invocation_return_policy_mode = %#v, want fallback", got)
	}
	if got := submittedFields["policy_resolution_path"]; got != "submitted_work_terminal" {
		t.Fatalf("submitted policy_resolution_path = %#v, want submitted_work_terminal", got)
	}

	completed := observedLogs.FilterMessage("factory session invocation completed").All()
	if len(completed) != 1 {
		t.Fatalf("completed invocation log count = %d, want 1", len(completed))
	}
	completedFields := completed[0].ContextMap()
	if got := completedFields["status"]; got != "COMPLETED" {
		t.Fatalf("completed status = %#v, want COMPLETED", got)
	}
	if got := completedFields["result_type"]; got != "text" {
		t.Fatalf("completed result_type = %#v, want text", got)
	}
	if _, ok := completedFields["resolved_work_id"]; !ok {
		t.Fatal("expected resolved_work_id field in completed invocation log")
	}

	recorder.assertContainsMetric(t, "invocation.attempts", map[string]string{"input_source": "COMPATIBILITY_CONTENT"})
	recorder.assertContainsMetric(t, "invocation.fallback_policy_used", map[string]string{"input_source": "COMPATIBILITY_CONTENT"})
	recorder.assertContainsMetric(t, "invocation.success", map[string]string{"input_source": "COMPATIBILITY_CONTENT"})
	recorder.assertContainsMetric(t, "invocation.result_type", map[string]string{"input_source": "COMPATIBILITY_CONTENT", "result_type": "text"})
	functionalevidence.Covers(t, "rest/invokeFactorySessionBySessionId")
}

func TestSessionInvocationAPI_RejectsWhitespaceOnlyText(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	server := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		support.ConfigureWorkerCommands(t, cfg, support.NewStaticSuccessCommandRunner("primary result COMPLETE"), nil)
	})

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
	server := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		support.ConfigureWorkerCommands(t, cfg, support.NewStaticSuccessCommandRunner("primary result COMPLETE"), nil)
	})

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
	server := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		support.ConfigureWorkerCommands(t, cfg, support.NewStaticSuccessCommandRunner("primary result COMPLETE"), nil)
	})

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
	server := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		support.ConfigureWorkerCommands(t, cfg, support.NewStaticSuccessCommandRunner("task output COMPLETE"), nil)
	})

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
	server := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		support.ConfigureWorkerCommands(t, cfg, blocking, nil)
	})

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
	var svc *service.FactoryService
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		CaptureService: func(captured *service.FactoryService) {
			svc = captured
		},
		Configure: func(cfg *service.FactoryServiceConfig) {
			cfg.RuntimeMode = interfaces.RuntimeModeService
			support.ConfigureWorkerCommands(t, cfg, support.NewStaticSuccessCommandRunner("primary result COMPLETE"), nil)
			cfg.Logger = zap.NewNop()
		},
	})
	if _, err := svc.PauseLiveFactorySession(context.Background(), factorysessions.DefaultSessionID, factoryapi.FactorySessionLifecycleControlRequest{}); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}

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

func TestSessionInvocationService_CanceledContextReturnsCanceledStatus(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	blocking := newBlockingInvocationRunner()
	var svc *service.FactoryService
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		CaptureService: func(captured *service.FactoryService) {
			svc = captured
		},
		Configure: func(cfg *service.FactoryServiceConfig) {
			cfg.RuntimeMode = interfaces.RuntimeModeService
			support.ConfigureWorkerCommands(t, cfg, blocking, nil)
			cfg.Logger = zap.NewNop()
		},
	})
	_ = server
	if svc == nil {
		t.Fatal("expected functional server to capture factory service")
	}

	ctx, cancel := context.WithCancel(context.Background())
	resultCh := make(chan factoryapi.InvocationResponse, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := svc.InvokeFactorySession(ctx, factorysessions.DefaultSessionID, textInvocationRequest(t, "invoke this", nil))
		if err != nil {
			errCh <- err
			return
		}
		response := factoryapi.InvocationResponse{
			RequestId: result.RequestID,
			TraceId:   result.TraceID,
			Status:    factoryapi.InvocationTerminalStatus(result.Status),
		}
		if result.ErrorCode != "" {
			code := factoryapi.InvocationResponseErrorCode(result.ErrorCode)
			response.ErrorCode = &code
		}
		resultCh <- response
	}()

	<-blocking.started
	cancel()

	select {
	case err := <-errCh:
		t.Fatalf("InvokeFactorySession returned error: %v", err)
	case response := <-resultCh:
		if response.Status != factoryapi.InvocationTerminalStatusCanceled {
			t.Fatalf("invocation status = %q, want CANCELED", response.Status)
		}
		if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONCANCELED {
			t.Fatalf("invocation errorCode = %#v, want INVOCATION_CANCELED", response.ErrorCode)
		}
	}
}

type blockingInvocationRunner struct {
	started chan struct{}
}

func newBlockingInvocationRunner() *blockingInvocationRunner {
	return &blockingInvocationRunner{started: make(chan struct{}, 1)}
}

func (r *blockingInvocationRunner) Run(ctx context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return workers.CommandResult{}, ctx.Err()
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
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.Codex, "gpt-5-codex"))
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
		t.Fatalf("POST /factory-sessions/~default/invocations status = %d", response.StatusCode)
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

var _ workers.CommandRunner = (*blockingInvocationRunner)(nil)

type capturingInvocationMetricsRecorder struct {
	mu      sync.Mutex
	metrics []service.InvocationMetric
}

func (r *capturingInvocationMetricsRecorder) RecordInvocationMetric(metric service.InvocationMetric) {
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
